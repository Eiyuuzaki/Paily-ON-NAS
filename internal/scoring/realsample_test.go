package scoring

import (
	"testing"

	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
)

func TestRealSample(t *testing.T) {
	cases := []struct {
		lat, jit int
		n        int
	}{
		{164, 0, 1}, {164, 1, 1}, {321, 211, 1}, {330, 0, 1}, {490, 40, 5},
	}
	for _, c := range cases {
		r := make([]model.NodeDeepCheck, c.n)
		for i := range r {
			r[i] = model.NodeDeepCheck{AvgLatencyMs: c.lat, JitterMs: c.jit}
		}
		got := computeScore(r, 0.7, 0.7, 0.3, 0.55, 0.3, 70, 1500, 400, curveLog)
		t.Logf("lat=%d jit=%d n=%d -> %.3f", c.lat, c.jit, c.n, got)
	}
}
