package scoring

import (
	"testing"

	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
)

func TestFilterDirectDropsInferred(t *testing.T) {
	in := []model.NodeDeepCheck{
		{AvgLatencyMs: 100, Source: "direct"},
		{AvgLatencyMs: 100, Source: "inferred"},
		{AvgLatencyMs: 100, Source: ""},
		{AvgLatencyMs: 100, Source: "direct"},
	}
	got := filterDirect(in)
	if len(got) != 2 {
		t.Fatalf("want 2 direct rows, got %d", len(got))
	}
	for _, r := range got {
		if r.Source != sourceDirect {
			t.Fatalf("non-direct row slipped through: %q", r.Source)
		}
	}
}

func TestFilterDirectAllInferred(t *testing.T) {
	in := []model.NodeDeepCheck{{Source: "inferred"}, {Source: "inferred"}}
	if got := filterDirect(in); len(got) != 0 {
		t.Fatalf("want empty, got %d", len(got))
	}
}
