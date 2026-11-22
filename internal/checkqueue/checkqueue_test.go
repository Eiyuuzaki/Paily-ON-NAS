package checkqueue

import (
	"testing"

	"github.com/google/uuid"
)

func id(n byte) uuid.UUID {
	var u uuid.UUID
	u[15] = n
	return u
}

func TestMergeDropsDuplicatesAndPreservesOrder(t *testing.T) {
	out, added := merge([]uuid.UUID{id(1), id(2)}, []uuid.UUID{id(2), id(3), id(1)}, 10)
	if added != 1 {
		t.Fatalf("added = %d, want 1", added)
	}
	if len(out) != 3 || out[0] != id(1) || out[1] != id(2) || out[2] != id(3) {
		t.Fatalf("out = %v, want [1 2 3] in order", out)
	}
}

func TestMergeKeepsExistingQueueAtCap(t *testing.T) {
	// A full queue must not be shortened by a new enqueue, and must not grow.
	existing := []uuid.UUID{id(1), id(2)}
	out, added := merge(existing, []uuid.UUID{id(3)}, 2)
	if added != 0 {
		t.Fatalf("added = %d, want 0 when at cap", added)
	}
	if len(out) != 2 || out[0] != id(1) || out[1] != id(2) {
		t.Fatalf("out = %v, want the existing queue untouched", out)
	}
}

func TestMergeCollapsesDuplicateInput(t *testing.T) {
	// Repeated clicks on the same node must not enlarge the next round.
	out, added := merge(nil, []uuid.UUID{id(7), id(7), id(7)}, 10)
	if added != 1 || len(out) != 1 {
		t.Fatalf("added=%d len=%d, want 1/1", added, len(out))
	}
}

func TestMergeStopsAtMax(t *testing.T) {
	incoming := []uuid.UUID{id(1), id(2), id(3), id(4)}
	out, added := merge(nil, incoming, 2)
	if added != 2 || len(out) != 2 {
		t.Fatalf("added=%d len=%d, want 2/2", added, len(out))
	}
}

func TestMergeEmptyExisting(t *testing.T) {
	if out, added := merge(nil, nil, 10); added != 0 || len(out) != 0 {
		t.Fatalf("empty merge returned %d/%d", len(out), added)
	}
}
