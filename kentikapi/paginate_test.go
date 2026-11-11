package kentikapi_test

import (
	"context"
	"fmt"
	"testing"

	alerting "github.com/kentik/api-schema-public/gen/go/kentik/alerting/public/v202505"
	alertingtypes "github.com/kentik/api-schema-public/gen/go/kentik/alerting/types/v202506"
	audit "github.com/kentik/api-schema-public/gen/go/kentik/audit/public/v202601"
	synthetics "github.com/kentik/api-schema-public/gen/go/kentik/synthetics/v202309"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kentik/community_sdk_golang/kentikapi"
	"github.com/kentik/community_sdk_golang/kentikapitest"
)

func auditEvents(n int) []*audit.AuditEvent {
	out := make([]*audit.AuditEvent, n)
	for i := range out {
		out[i] = &audit.AuditEvent{Id: uint64(i)}
	}
	return out
}

func TestPaginateOffsetLimit(t *testing.T) {
	all := auditEvents(250)
	srv := kentikapitest.NewServer(t, kentikapitest.HandleUnary(audit.AuditService_ListAuditEvents_FullMethodName,
		func(_ context.Context, req *audit.ListAuditEventsRequest) (*audit.ListAuditEventsResponse, error) {
			lo := min(int(req.GetOffset()), len(all))
			hi := min(lo+int(req.GetLimit()), len(all))
			return &audit.ListAuditEventsResponse{Events: all[lo:hi]}, nil
		}))
	svc := srv.Client(t).AuditPublic()

	req := &audit.ListAuditEventsRequest{StartTime: "2026-01-01T00:00:00Z"}
	got, err := kentikapi.CollectAll(kentikapi.Paginate(t.Context(), svc.ListAuditEvents, req,
		(*audit.ListAuditEventsResponse).GetEvents))
	require.NoError(t, err)
	assert.Len(t, got, 250)
	assert.Equal(t, uint64(249), got[249].GetId())
	assert.Zero(t, req.GetOffset(), "caller's request is not modified")

	calls := srv.CallsTo(audit.AuditService_ListAuditEvents_FullMethodName)
	require.Len(t, calls, 3, "pages of 100, 100, 50")
	last, ok := calls[2].Request.(*audit.ListAuditEventsRequest)
	require.True(t, ok)
	assert.Equal(t, uint64(200), last.GetOffset())
	assert.Equal(t, uint64(kentikapi.DefaultPageSize), last.GetLimit())
	assert.Equal(t, "2026-01-01T00:00:00Z", last.GetStartTime())
}

func TestPaginatePageSizeAndBreak(t *testing.T) {
	all := auditEvents(100)
	srv := kentikapitest.NewServer(t, kentikapitest.HandleUnary(audit.AuditService_ListAuditEvents_FullMethodName,
		func(_ context.Context, req *audit.ListAuditEventsRequest) (*audit.ListAuditEventsResponse, error) {
			lo := min(int(req.GetOffset()), len(all))
			return &audit.ListAuditEventsResponse{Events: all[lo:min(lo+int(req.GetLimit()), len(all))]}, nil
		}))
	svc := srv.Client(t).AuditPublic()

	n := 0
	for ev, err := range kentikapi.Paginate(t.Context(), svc.ListAuditEvents, &audit.ListAuditEventsRequest{},
		(*audit.ListAuditEventsResponse).GetEvents, kentikapi.WithPageSize(10)) {
		require.NoError(t, err)
		n++
		if ev.GetId() == 14 {
			break
		}
	}
	assert.Equal(t, 15, n)
	assert.Len(t, srv.Calls(), 2, "breaking stops fetching")
}

func TestPaginateConfigHasMore(t *testing.T) {
	const total = 7
	srv := kentikapitest.NewServer(t, kentikapitest.HandleUnary(alerting.AlertService_List_FullMethodName,
		func(_ context.Context, req *alerting.AlertServiceListRequest) (*alerting.AlertServiceListResponse, error) {
			p := req.GetPagination()
			resp := &alerting.AlertServiceListResponse{}
			for i := p.GetOffset(); i < min(p.GetOffset()+p.GetLimit(), total); i++ {
				resp.Alerts = append(resp.Alerts, &alerting.Alert{Id: fmt.Sprint(i)})
			}
			more := p.GetOffset()+p.GetLimit() < total
			resp.Pagination = &alertingtypes.PaginationInfo{Info: &alertingtypes.PaginationInfo_HasMoreResults{HasMoreResults: more}}
			return resp, nil
		}))
	alerts := srv.Client(t).AlertingPublic().Alert

	got, err := kentikapi.CollectAll(kentikapi.Paginate(t.Context(), alerts.List, &alerting.AlertServiceListRequest{},
		(*alerting.AlertServiceListResponse).GetAlerts, kentikapi.WithPageSize(3)))
	require.NoError(t, err)
	require.Len(t, got, total)
	assert.Equal(t, "6", got[6].GetId())
	assert.Len(t, srv.Calls(), 3)
}

func TestPaginateConfigTotalCount(t *testing.T) {
	srv := kentikapitest.NewServer(t, kentikapitest.HandleUnary(alerting.AlertService_List_FullMethodName,
		func(_ context.Context, req *alerting.AlertServiceListRequest) (*alerting.AlertServiceListResponse, error) {
			// Server returns full pages; total_count must stop the iteration.
			resp := &alerting.AlertServiceListResponse{
				Pagination: &alertingtypes.PaginationInfo{Info: &alertingtypes.PaginationInfo_TotalCount{TotalCount: 4}},
			}
			for range req.GetPagination().GetLimit() {
				resp.Alerts = append(resp.Alerts, &alerting.Alert{})
			}
			return resp, nil
		}))
	alerts := srv.Client(t).AlertingPublicV202505().Alert

	got, err := kentikapi.CollectAll(kentikapi.Paginate(t.Context(), alerts.List,
		&alerting.AlertServiceListRequest{Pagination: &alertingtypes.PaginationConfig{Limit: 2}},
		(*alerting.AlertServiceListResponse).GetAlerts))
	require.NoError(t, err)
	assert.Len(t, got, 4)
	assert.Len(t, srv.Calls(), 2)
}

func TestPaginateErrorAndUnpaged(t *testing.T) {
	srv := kentikapitest.NewServer(t,
		kentikapitest.HandleUnary(audit.AuditService_ListAuditEvents_FullMethodName,
			func(context.Context, *audit.ListAuditEventsRequest) (*audit.ListAuditEventsResponse, error) {
				return nil, status.Error(codes.PermissionDenied, "no audit access")
			}),
		kentikapitest.HandleUnary(synthetics.SyntheticsAdminService_ListTests_FullMethodName,
			func(context.Context, *synthetics.ListTestsRequest) (*synthetics.ListTestsResponse, error) {
				return &synthetics.ListTestsResponse{Tests: []*synthetics.Test{{Id: "1"}, {Id: "2"}}}, nil
			}),
	)
	c := srv.Client(t)

	got, err := kentikapi.CollectAll(kentikapi.Paginate(t.Context(), c.AuditPublic().ListAuditEvents, nil,
		(*audit.ListAuditEventsResponse).GetEvents))
	assert.Empty(t, got)
	assert.True(t, kentikapi.IsAuthError(err))

	tests, err := kentikapi.CollectAll(kentikapi.Paginate(t.Context(), c.Synthetics().Admin.ListTests,
		&synthetics.ListTestsRequest{}, (*synthetics.ListTestsResponse).GetTests))
	require.NoError(t, err)
	assert.Len(t, tests, 2, "unpaged list RPCs are called once")
}
