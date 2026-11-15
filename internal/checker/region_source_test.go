package checker

import (
	"testing"
	"time"

	api "github.com/Eiyuuzaki/paily-on-nas/internal/api/generated"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func TestNormalizeRegionSource(t *testing.T) {
	tests := []struct {
		name  string
		input *string
		want  string
	}{
		{name: "missing old checker field", want: "checker_unspecified"},
		{name: "native egress", input: stringPtr("native_egress"), want: "native_egress"},
		{name: "legacy egress", input: stringPtr("legacy_egress"), want: "legacy_egress"},
		{name: "inbound fallback", input: stringPtr("inbound_mmdb"), want: "inbound_mmdb"},
		{name: "unknown is not trusted", input: stringPtr("made_up"), want: "checker_unspecified"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeRegionSource(tt.input); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildCheckRecordsKeepsRegionSourcePair(t *testing.T) {
	h := &Handler{}
	nativeID, inboundID := uuid.New(), uuid.New()
	nativeRegion, nativeSource := "HK", "native_egress"
	inboundRegion, inboundSource := "DE", "inbound_mmdb"
	now := time.Now()
	results := []api.CheckResult{
		{NodeId: openapi_types.UUID(nativeID), Initial: api.CheckInitialResult{LatencyMs: 1}, Deep: &api.CheckDeepResult{AvgLatencyMs: 1, JitterMs: 0, Region: &nativeRegion, RegionSource: &nativeSource}},
		{NodeId: openapi_types.UUID(inboundID), Initial: api.CheckInitialResult{LatencyMs: 2}, Deep: &api.CheckDeepResult{AvgLatencyMs: 2, JitterMs: 0, Region: &inboundRegion, RegionSource: &inboundSource}},
	}
	_, _, _, regions, sources := h.buildCheckRecords(results, "test", uuid.New(), now)
	if regions[nativeID] != nativeRegion || sources[nativeID] != nativeSource {
		t.Fatalf("native result mismatch: %q/%q", regions[nativeID], sources[nativeID])
	}
	if regions[inboundID] != inboundRegion || sources[inboundID] != inboundSource {
		t.Fatalf("inbound result mismatch: %q/%q", regions[inboundID], sources[inboundID])
	}
}

func stringPtr(s string) *string { return &s }
