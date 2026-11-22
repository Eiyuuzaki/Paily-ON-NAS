package checker

import (
	"github.com/google/uuid"
	"testing"
)

func TestGroupRegionObservationsUsesCurrentRegionAndSource(t *testing.T) {
	id := uuid.New()
	rows := []regionProbeRow{{ID: id, RawServer: "worker.example"}}
	got := groupRegionObservations(rows, map[uuid.UUID]string{id: "HK"}, map[uuid.UUID]string{id: "native_egress"})
	v := got["worker.example"]
	if v.region != "HK" || v.source != "native_egress" || v.conflict {
		t.Fatalf("unexpected observation: %+v", v)
	}
}

func TestGroupRegionObservationsMarksConflictingSiblingEvidence(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	rows := []regionProbeRow{{ID: a, RawServer: "worker.example"}, {ID: b, RawServer: "worker.example"}}
	got := groupRegionObservations(rows, map[uuid.UUID]string{a: "HK", b: "US"}, map[uuid.UUID]string{a: "native_egress", b: "native_egress"})
	if !got["worker.example"].conflict {
		t.Fatalf("expected conflicting path evidence, got %+v", got["worker.example"])
	}
}

// regionSourceRank orders evidence by how directly it observes the exit.
// A weaker source must never overwrite a stronger one: on 2026-11-22 a single
// legacy_egress round (script lookup, no egress IP) replaced three consecutive
// native_egress rounds and relabelled a Hong Kong exit as US.
func TestRegionSourceRankOrdersEvidence(t *testing.T) {
	order := []string{"native_egress", "entry_colo", "legacy_egress", "inbound_mmdb"}
	for i := 1; i < len(order); i++ {
		if regionSourceRank(order[i-1]) <= regionSourceRank(order[i]) {
			t.Fatalf("%s should outrank %s", order[i-1], order[i])
		}
	}
	if regionSourceRank("unknown") != 0 || regionSourceRank("") != 0 {
		t.Fatal("unknown sources must rank lowest")
	}
}

func TestShouldReplaceRegionRejectsWeakerEvidence(t *testing.T) {
	cases := []struct {
		name          string
		current, next string
		want          bool
	}{
		{"native overrides nothing", "", "native_egress", true},
		{"native keeps native", "native_egress", "native_egress", true},
		{"legacy cannot beat native", "native_egress", "legacy_egress", false},
		{"inbound cannot beat entry colo", "entry_colo", "inbound_mmdb", false},
		{"legacy cannot beat entry colo", "entry_colo", "legacy_egress", false},
		{"native beats entry colo", "entry_colo", "native_egress", true},
		{"entry colo beats legacy", "legacy_egress", "entry_colo", true},
	}
	for _, c := range cases {
		if got := shouldReplaceRegion(c.current, c.next); got != c.want {
			t.Fatalf("%s: shouldReplaceRegion(%q,%q)=%v want %v", c.name, c.current, c.next, got, c.want)
		}
	}
}

func TestGroupRegionObservationsDefaultsMissingSourceToUnknown(t *testing.T) {
	id := uuid.New()
	rows := []regionProbeRow{{ID: id, RawServer: "worker.example"}}
	got := groupRegionObservations(rows, map[uuid.UUID]string{id: "US"}, nil)
	if got["worker.example"].source != "unknown" {
		t.Fatalf("want unknown source, got %+v", got["worker.example"])
	}
}
