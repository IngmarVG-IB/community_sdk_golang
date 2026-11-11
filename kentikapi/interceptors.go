package kentikapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const maxLoggedPayload = 10_000

// unaryInterceptor implements overall timeout, rate limiting, retries with backoff, per-attempt timeout,
// request id capture, logging and error wrapping for unary RPCs.
func (c *Client) unaryInterceptor(
	ctx context.Context, method string, req, reply any,
	cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
) error {
	if c.cfg.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.cfg.timeout)
		defer cancel()
	}
	policy := c.cfg.retry
	retryable := policy.MaxRetries > 0 && policy.Retryable(method)
	log := c.logger.With(slog.String("rpc.method", method))
	if c.cfg.logPayloads {
		log.DebugContext(ctx, "kentikapi request", slog.String("payload", payload(req)))
	}

	start := time.Now()
	for attempt := 1; ; attempt++ {
		if err := c.wait(ctx); err != nil {
			return c.fail(ctx, log, method, err, "", attempt-1, start)
		}
		var header, trailer metadata.MD
		attemptCtx, cancelAttempt := ctx, context.CancelFunc(func() {})
		if c.cfg.attemptTimeout > 0 {
			attemptCtx, cancelAttempt = context.WithTimeout(ctx, c.cfg.attemptTimeout)
		}
		callOpts := append(opts[:len(opts):len(opts)], grpc.Header(&header), grpc.Trailer(&trailer))
		err := invoker(attemptCtx, method, req, reply, cc, callOpts...)
		cancelAttempt()
		reqID := c.requestID(header, trailer)

		if err == nil {
			log.DebugContext(ctx, "kentikapi call",
				slog.Int("attempts", attempt), slog.Duration("duration", time.Since(start)),
				slog.String("request_id", reqID))
			if c.cfg.logPayloads {
				log.DebugContext(ctx, "kentikapi response", slog.String("payload", payload(reply)))
			}
			return nil
		}

		code := status.Code(err)
		if !retryable || attempt > policy.MaxRetries || !policy.retryOn(code) || ctx.Err() != nil {
			return c.fail(ctx, log, method, err, reqID, attempt, start)
		}
		delay := policy.backoff(attempt, defaultRand)
		if hint, ok := serverRetryDelay(err, trailer); ok && hint > delay {
			delay = hint
		}
		if dl, ok := ctx.Deadline(); ok && time.Until(dl) < delay {
			// Not enough time left for another attempt.
			return c.fail(ctx, log, method, err, reqID, attempt, start)
		}
		log.InfoContext(ctx, "kentikapi retrying",
			slog.Int("attempt", attempt), slog.String("code", code.String()),
			slog.Duration("backoff", delay), slog.String("request_id", reqID))
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return c.fail(ctx, log, method, err, reqID, attempt, start)
		case <-t.C:
		}
	}
}

// streamInterceptor applies rate limiting, logging and error wrapping to streaming RPCs. Streams are not
// retried and not subject to WithTimeout; bound them with the context.
func (c *Client) streamInterceptor(
	ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string,
	streamer grpc.Streamer, opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	log := c.logger.With(slog.String("rpc.method", method))
	start := time.Now()
	if err := c.wait(ctx); err != nil {
		return nil, c.fail(ctx, log, method, err, "", 0, start)
	}
	cs, err := streamer(ctx, desc, cc, method, opts...)
	if err != nil {
		return nil, c.fail(ctx, log, method, err, "", 1, start)
	}
	log.DebugContext(ctx, "kentikapi stream opened")
	return &wrappedStream{ClientStream: cs, c: c, method: method}, nil
}

type wrappedStream struct {
	grpc.ClientStream
	c      *Client
	method string
}

func (s *wrappedStream) SendMsg(m any) error {
	return s.wrap(s.ClientStream.SendMsg(m))
}

func (s *wrappedStream) RecvMsg(m any) error {
	return s.wrap(s.ClientStream.RecvMsg(m))
}

func (s *wrappedStream) wrap(err error) error {
	if err == nil || errors.Is(err, io.EOF) {
		return err
	}
	var reqID string
	if md, hErr := s.Header(); hErr == nil {
		reqID = s.c.requestID(md, s.Trailer())
	}
	return newError(s.method, err, reqID, 1)
}

func (c *Client) wait(ctx context.Context) error {
	if c.limiter == nil {
		return nil
	}
	if err := c.limiter.Wait(ctx); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// The wait would exceed the deadline.
		return fmt.Errorf("%w: client-side rate limit: %w", context.DeadlineExceeded, err)
	}
	return nil
}

func (c *Client) fail(
	ctx context.Context, log *slog.Logger, method string, err error, reqID string, attempts int, start time.Time,
) error {
	e := newError(method, err, reqID, attempts)
	log.WarnContext(ctx, "kentikapi call failed",
		slog.String("code", e.Code.String()), slog.String("message", e.Message),
		slog.Int("attempts", attempts), slog.Duration("duration", time.Since(start)),
		slog.String("request_id", reqID))
	return e
}

func (c *Client) requestID(mds ...metadata.MD) string {
	for _, key := range c.cfg.requestIDHeaders {
		for _, md := range mds {
			if v := md.Get(key); len(v) > 0 && v[0] != "" {
				return v[0]
			}
		}
	}
	return ""
}

func payload(m any) string {
	s := fmt.Sprint(m)
	if s == "" {
		return "<empty>"
	}
	if len(s) > maxLoggedPayload {
		return fmt.Sprintf("<%d bytes>", len(s))
	}
	return s
}
