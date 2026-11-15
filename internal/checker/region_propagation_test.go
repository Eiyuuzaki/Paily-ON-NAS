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

func TestGroupRegionObservationsDefaultsMissingSourceToUnknown(t *testing.T) {
	id := uuid.New()
	rows := []regionProbeRow{{ID: id, RawServer: "worker.example"}}
	got := groupRegionObservations(rows, map[uuid.UUID]string{id: "US"}, nil)
	if got["worker.example"].source != "unknown" {
		t.Fatalf("want unknown source, got %+v", got["worker.example"])
	}
}
