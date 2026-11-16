package handler

import "testing"

func TestAggregatePathQualityExposesNativeEgressObservations(t *testing.T) {
	rows := []pathQualityObservation{{
		PathKey: "p", RegionSource: "native_egress", Alive: true, EgressObservedRuns: 3,
		EgressIP: "203.0.113.7", EgressProvider: "ip-api", EgressASN: 64500, EgressASNOrg: "Example ASN",
	}}
	got := aggregatePathQuality(rows)
	if len(got) != 1 {
		t.Fatalf("groups=%d, want 1", len(got))
	}
	p := got[0]
	if p.EgressObservedRuns != 3 || p.EgressObservationNodes != 1 {
		t.Fatalf("egress observations=%d runs / %d nodes", p.EgressObservedRuns, p.EgressObservationNodes)
	}
	if len(p.EgressIPs) != 1 || p.EgressIPs[0] != "203.0.113.7" {
		t.Fatalf("egress ips=%v", p.EgressIPs)
	}
	if len(p.EgressProviders) != 1 || p.EgressProviders[0] != "ip-api" {
		t.Fatalf("egress providers=%v", p.EgressProviders)
	}
	if len(p.EgressASNs) != 1 || p.EgressASNs[0] != 64500 || len(p.EgressASNOrgs) != 1 {
		t.Fatalf("egress ASN=%v orgs=%v", p.EgressASNs, p.EgressASNOrgs)
	}
}

func TestStableForSchedulingRequiresThreeRunsAndCoverage(t *testing.T) {
	good := pathQualityItem{ShadowClass: "production_candidate", Nodes: 2, EgressObservedRuns: 3, EgressObservationNodes: 2}
	if !stableForScheduling([]pathQualityItem{good}) {
		t.Fatal("fully observed candidate should be stable")
	}
	good.EgressObservedRuns = 2
	if stableForScheduling([]pathQualityItem{good}) {
		t.Fatal("two-run candidate must not be stable")
	}
}

func TestStableForSchedulingRejectsEmptyOrMixedGroups(t *testing.T) {
	if stableForScheduling(nil) {
		t.Fatal("empty observations must not be stable")
	}
	good := pathQualityItem{ShadowClass: "production_candidate", Nodes: 1, EgressObservedRuns: 3, EgressObservationNodes: 1}
	standby := pathQualityItem{ShadowClass: "standby_candidate", Nodes: 1, EgressObservedRuns: 3, EgressObservationNodes: 1}
	if stableForScheduling([]pathQualityItem{good, standby}) {
		t.Fatal("mixed shadow groups must not be stable")
	}
}

func TestAggregatePathQualityDoesNotExposeInboundEgressObservations(t *testing.T) {
	rows := []pathQualityObservation{{
		PathKey: "p", RegionSource: "inbound_mmdb", Alive: true, EgressObservedRuns: 3,
		EgressIP: "198.51.100.9", EgressProvider: "ip-api", EgressASN: 64501,
	}}
	p := aggregatePathQuality(rows)[0]
	if len(p.EgressIPs) != 0 || len(p.EgressProviders) != 0 || len(p.EgressASNs) != 0 {
		t.Fatalf("inbound evidence leaked: ips=%v providers=%v asns=%v", p.EgressIPs, p.EgressProviders, p.EgressASNs)
	}
}
