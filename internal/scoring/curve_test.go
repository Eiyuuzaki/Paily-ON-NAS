package scoring

import (
	"math"
	"testing"
)

// The exp curve exists because the log curve saturates: every sample past the
// anchor scores exactly 0, so nodes with visibly different latency tie. This
// asserts the property that motivated the change rather than a magic number.
func TestExpCurveNeverSaturates(t *testing.T) {
	score := func(lat, jit int) float64 {
		return computeScore(recs(lat, jit, 5), 0.7, 0.7, 0.3, 0.55, 0.3, 70, 1500, 400, curveExp)
	}
	// All of these are past both anchors, where the log curve returns exactly 0.
	a, b, c := score(1500, 400), score(3000, 800), score(6000, 1600)
	if a <= b || b <= c {
		t.Fatalf("exp curve must stay strictly ordered past the anchor: 1500=%.4f 3000=%.4f 6000=%.4f", a, b, c)
	}
	if c <= 0 {
		t.Fatalf("exp curve returned %v for an anchor-relative 4x sample; it must stay positive", c)
	}
}

// A node at the anchor should read the same on both curves' scale endpoints:
// exp gives 1/e for the saturated case, and never worse than 0.
func TestExpCurveAnchorValue(t *testing.T) {
	// latency-only contribution at exactly the anchor, stability near zero.
	// arg order: alpha, wLatency, wStability, wSpeed, latencyCurveK, ...
	got := computeScore(recs(1500, 0, 5), 0.7, 0.7, 0.0, 0.55, 0.3, 70, 1500, 400, curveExp)
	// score = exp(-1)*1.0 (all weight on latency when wStability=0)
	want := math.Exp(-1)
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("at the anchor with latency-only weight: got %.6f want %.6f", got, want)
	}
}

// The historical curve must remain byte-identical when selected, so switching
// the config back is a real rollback rather than an approximation.
func TestLogCurveIsDefault(t *testing.T) {
	const lat, jit = 490, 40
	withLog := computeScore(recs(lat, jit, 5), 0.7, 0.7, 0.3, 0.55, 0.3, 70, 1500, 400, curveLog)
	unset := computeScore(recs(lat, jit, 5), 0.7, 0.7, 0.3, 0.55, 0.3, 70, 1500, 400, "")
	if withLog != unset {
		t.Fatalf("empty curve must behave as log: log=%.6f empty=%.6f", withLog, unset)
	}
	if withLog <= 0 || withLog >= 1 {
		t.Fatalf("log curve out of range: %.6f", withLog)
	}
}
