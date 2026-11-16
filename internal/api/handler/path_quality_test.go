package handler

import (
	"testing"
	"time"
)

func TestAggregatePathQualityUsesTriStateCapabilities(t *testing.T) {
	rows := []pathQualityObservation{
		{PathKey: "p", AvgLatencyMs: 100, JitterMs: 10, CheckedAt: time.Unix(1, 0), StreamingResult: `{"access-chatgpt":true,"layer-egress":false}`, Protocol: "vless", Region: "HK", RegionSource: "native_egress", Alive: true},
		{PathKey: "p", AvgLatencyMs: 300, JitterMs: 30, CheckedAt: time.Unix(2, 0), FailReasonsJSON: `{"timeout":2}`, StreamingResult: `{"access-chatgpt":false}`, Protocol: "vless", Region: "US", RegionSource: "unknown", Alive: false},
	}
	got := aggregatePathQuality(rows)
	if len(got) != 1 {
		t.Fatalf("groups=%d, want 1", len(got))
	}
	p := got[0]
	if p.Nodes != 2 || p.Alive != 1 {
		t.Fatalf("nodes/alive=%d/%d", p.Nodes, p.Alive)
	}
	if c := p.Capabilities["access-chatgpt"]; c.Passed != 1 || c.Failed != 1 || c.Unknown != 0 {
		t.Fatalf("chatgpt=%+v", c)
	}
	if p.FailReasons["timeout"] != 2 {
		t.Fatalf("fail reasons=%v", p.FailReasons)
	}
	if c := p.Capabilities["access-netflix"]; c.Unknown != 2 || c.Passed != 0 || c.Failed != 0 {
		t.Fatalf("netflix=%+v", c)
	}
	if p.ShadowClass != "standby_candidate" {
		t.Fatalf("shadow class=%s", p.ShadowClass)
	}
	if p.AvgLatencyMs == nil || *p.AvgLatencyMs != 200 || p.P50LatencyMs == nil || *p.P50LatencyMs != 100 {
		t.Fatalf("latencies avg=%v p50=%v", p.AvgLatencyMs, p.P50LatencyMs)
	}
}
