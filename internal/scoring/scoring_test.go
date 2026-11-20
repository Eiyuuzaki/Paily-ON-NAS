package scoring

import (
	"math"
	"testing"

	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
)

func recs(lat, jit int, n int) []model.NodeDeepCheck {
	out := make([]model.NodeDeepCheck, n)
	for i := range out {
		out[i] = model.NodeDeepCheck{AvgLatencyMs: lat, JitterMs: jit}
	}
	return out
}

// Measured warm-connection values (2026-11-17): a good CF edge node sits around
// 330ms, an HKG entry around 490ms, and a degraded one past 1.2s.
func TestScoreSpreadsAcrossAnchors(t *testing.T) {
	score := func(lat, jit int) float64 {
		return computeScore(recs(lat, jit, 5), 0.7, 0.7, 0.3, 0.55, 0.3, 70, 1500, 400, curveLog)
	}
	fast := score(330, 2)
	mid := score(490, 20)
	slow := score(900, 120)
	bad := score(1400, 300)

	if !(fast > mid && mid > slow && slow > bad) {
		t.Fatalf("scores not ordered: fast=%.3f mid=%.3f slow=%.3f bad=%.3f", fast, mid, slow, bad)
	}
	if fast < 0.75 {
		t.Errorf("330ms/2ms should reach grade A, got %.3f", fast)
	}
	if bad > 0.5 {
		t.Errorf("1400ms/300ms should not reach grade B, got %.3f", bad)
	}
	if fast-bad < 0.25 {
		t.Errorf("spread too flat: fast=%.3f bad=%.3f", fast, bad)
	}
}

// A node that answers quickly but fails samples must not outrank a slower node
// that answers every time.
func TestTimeoutsArePenalised(t *testing.T) {
	steady := computeScore(recs(500, 20, 5), 0.7, 0.7, 0.3, 0.55, 0.3, 70, 1500, 400, curveLog)
	flaky := append(recs(300, 5, 3), recs(-1, -1, 2)...)
	got := computeScore(flaky, 0.7, 0.7, 0.3, 0.55, 0.3, 70, 1500, 400, curveLog)
	if got >= steady {
		t.Fatalf("flaky(2 timeouts)=%.3f should score below steady=%.3f", got, steady)
	}
}

func TestAnchorFallback(t *testing.T) {
	// Zero anchors must not divide by zero; defaults apply.
	got := computeScore(recs(330, 5, 5), 0.7, 0.7, 0.3, 0.55, 0.3, 70, 0, 0, curveLog)
	if math.IsNaN(got) || got <= 0 {
		t.Fatalf("zero anchors produced %v", got)
	}
}
