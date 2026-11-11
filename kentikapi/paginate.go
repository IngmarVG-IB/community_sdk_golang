package kentikapi

import (
	"context"
	"errors"
	"iter"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// DefaultPageSize is the page size Paginate requests when the request does not set one.
const DefaultPageSize = 100

// PageOption configures [Paginate].
type PageOption func(*pageConfig)

type pageConfig struct {
	size uint64
}

// WithPageSize sets the number of items requested per page.
func WithPageSize(n int) PageOption {
	return func(c *pageConfig) {
		if n > 0 {
			c.size = uint64(n)
		}
	}
}

// Paginate calls a list RPC page by page and yields every item. It understands the two paging styles in
// api-schema-public (see [MethodInfo].Pagination):
//
//   - a `pagination` field of type kentik.alerting.types.*.PaginationConfig (alerting), with
//     PaginationInfo (has_more_results or total_count) in the response;
//   - top-level integer `offset` and `limit` fields (audit), where a short page ends the listing.
//
// For requests without paging fields it makes one call. req is not modified; its own offset and limit (if
// set) are the starting offset and the page size. Iteration stops at the first error, which is yielded
// with a zero item, or when the caller breaks out of the loop.
//
//	alerts := client.AlertingPublicV202505().Alert
//	for a, err := range kentikapi.Paginate(ctx, alerts.List, &alerting.ListAlertsRequest{},
//		(*alerting.ListAlertsResponse).GetAlerts) {
//		if err != nil {
//			return err
//		}
//		fmt.Println(a.GetId())
//	}
func Paginate[Req, Resp proto.Message, Item any](
	ctx context.Context,
	call func(context.Context, Req, ...grpc.CallOption) (Resp, error),
	req Req,
	items func(Resp) []Item,
	opts ...PageOption,
) iter.Seq2[Item, error] {
	return func(yield func(Item, error) bool) {
		var zero Item
		cfg := pageConfig{}
		for _, o := range opts {
			o(&cfg)
		}
		page, ok := clone(req)
		if !ok {
			yield(zero, errors.New("kentikapi: Paginate: cannot clone request"))
			return
		}
		p := pagerFor(page.ProtoReflect())
		if p.style == "" {
			resp, err := call(ctx, page)
			if err != nil {
				yield(zero, err)
				return
			}
			for _, it := range items(resp) {
				if !yield(it, nil) {
					return
				}
			}
			return
		}

		offset, limit := p.get()
		if cfg.size > 0 {
			limit = cfg.size
		}
		if limit == 0 {
			limit = DefaultPageSize
		}
		for {
			p.set(offset, limit)
			resp, err := call(ctx, page)
			if err != nil {
				yield(zero, err)
				return
			}
			batch := items(resp)
			for _, it := range batch {
				if !yield(it, nil) {
					return
				}
			}
			n := uint64(len(batch))
			offset += n
			if n == 0 || !p.more(resp.ProtoReflect(), offset, n, limit) {
				return
			}
		}
	}
}

// CollectAll drains seq into a slice, stopping at the first error.
func CollectAll[Item any](seq iter.Seq2[Item, error]) ([]Item, error) {
	var out []Item
	for it, err := range seq {
		if err != nil {
			return out, err
		}
		out = append(out, it)
	}
	return out, nil
}

// clone deep-copies req; a nil request becomes an empty one.
func clone[Req proto.Message](req Req) (Req, bool) {
	if !req.ProtoReflect().IsValid() {
		out, ok := req.ProtoReflect().Type().New().Interface().(Req)
		return out, ok
	}
	out, ok := proto.Clone(req).(Req)
	return out, ok
}

type pager struct {
	style         string // "", "pagination_config", "offset_limit"
	target        protoreflect.Message
	offset, limit protoreflect.FieldDescriptor
	cfgField      protoreflect.FieldDescriptor // request `pagination` (config style)
}

func pagerFor(req protoreflect.Message) *pager {
	fields := req.Descriptor().Fields()
	if f := fields.ByName("pagination"); f != nil && f.Message() != nil && f.Message().Name() == "PaginationConfig" {
		cfg := req.Mutable(f).Message()
		off, lim := intField(cfg, "offset"), intField(cfg, "limit")
		if off != nil && lim != nil {
			return &pager{style: "pagination_config", target: cfg, offset: off, limit: lim, cfgField: f}
		}
	}
	off, lim := intField(req, "offset"), intField(req, "limit")
	if off != nil && lim != nil {
		return &pager{style: "offset_limit", target: req, offset: off, limit: lim}
	}
	return &pager{}
}

func intField(m protoreflect.Message, name protoreflect.Name) protoreflect.FieldDescriptor {
	f := m.Descriptor().Fields().ByName(name)
	if f == nil || f.Cardinality() == protoreflect.Repeated {
		return nil
	}
	switch f.Kind() {
	case protoreflect.Int32Kind, protoreflect.Int64Kind, protoreflect.Uint32Kind, protoreflect.Uint64Kind:
		return f
	default:
		return nil
	}
}

func (p *pager) get() (offset, limit uint64) {
	return getUint(p.target, p.offset), getUint(p.target, p.limit)
}

func (p *pager) set(offset, limit uint64) {
	setUint(p.target, p.offset, offset)
	setUint(p.target, p.limit, limit)
}

// more reports whether another page should be fetched after a page of n items.
func (p *pager) more(resp protoreflect.Message, offset, n, limit uint64) bool {
	if p.style == "pagination_config" {
		if f := resp.Descriptor().Fields().ByName("pagination"); f != nil && f.Message() != nil && resp.Has(f) {
			info := resp.Get(f).Message()
			fields := info.Descriptor().Fields()
			if hm := fields.ByName("has_more_results"); hm != nil && info.Has(hm) {
				return info.Get(hm).Bool()
			}
			if tc := fields.ByName("total_count"); tc != nil && info.Has(tc) {
				return offset < info.Get(tc).Uint()
			}
		}
	}
	return n >= limit
}

func getUint(m protoreflect.Message, f protoreflect.FieldDescriptor) uint64 {
	v := m.Get(f)
	switch f.Kind() {
	case protoreflect.Int32Kind, protoreflect.Int64Kind:
		return uint64(max(v.Int(), 0))
	default:
		return v.Uint()
	}
}

func setUint(m protoreflect.Message, f protoreflect.FieldDescriptor, n uint64) {
	switch f.Kind() {
	case protoreflect.Int32Kind:
		m.Set(f, protoreflect.ValueOfInt32(int32(min(n, 1<<31-1))))
	case protoreflect.Int64Kind:
		m.Set(f, protoreflect.ValueOfInt64(int64(min(n, 1<<63-1))))
	case protoreflect.Uint32Kind:
		m.Set(f, protoreflect.ValueOfUint32(uint32(min(n, 1<<32-1))))
	default:
		m.Set(f, protoreflect.ValueOfUint64(n))
	}
}
