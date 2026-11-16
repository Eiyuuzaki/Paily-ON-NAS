package handler

import "testing"

func TestClassifyShadowConservativeBoundaries(t *testing.T) {
	production := &pathQualityItem{Nodes: 2, Alive: 2, AvgLatencyMs: intPtr(400), ObservedRuns: 3, RegionSources: []string{"native_egress"}, Capabilities: map[string]pathCapabilityCount{"access-chatgpt": {Passed: 2}}}
	if got, _ := classifyShadow(production); got != "production_candidate" {
		t.Fatalf("production got %s", got)
	}
	production.Capabilities["access-chatgpt"] = pathCapabilityCount{Unknown: 2}
	if got, _ := classifyShadow(production); got == "production_candidate" {
		t.Fatal("unknown capability must not qualify as production")
	}
	singleRun := &pathQualityItem{Nodes: 2, Alive: 2, ObservedRuns: 1, AvgLatencyMs: intPtr(400), RegionSources: []string{"native_egress"}, Capabilities: map[string]pathCapabilityCount{"access-chatgpt": {Passed: 2}}}
	if got, _ := classifyShadow(singleRun); got == "production_candidate" {
		t.Fatal("single-run path must not be production candidate")
	}
	weak := &pathQualityItem{Nodes: 3, Alive: 1, Capabilities: map[string]pathCapabilityCount{}}
	if got, _ := classifyShadow(weak); got != "observation_only" {
		t.Fatalf("weak path got %s", got)
	}
	inbound := &pathQualityItem{Nodes: 2, Alive: 2, AvgLatencyMs: intPtr(400), ObservedRuns: 3, RegionSources: []string{"inbound_mmdb"}, Capabilities: map[string]pathCapabilityCount{"access-chatgpt": {Passed: 2}}}
	if got, reasons := classifyShadow(inbound); got == "production_candidate" || !containsReason(reasons, "region_evidence_untrusted") {
		t.Fatalf("inbound evidence got %s reasons=%v", got, reasons)
	}
}

func containsReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func intPtr(v int) *int { return &v }
