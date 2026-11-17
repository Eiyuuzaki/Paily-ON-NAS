package scoring

import "fmt"

// Grade is a human-facing quality band, derived from measured experience rather
// than from the 0..1 score.
//
// Why not grade the score: the score is a normalised blend whose anchors had to
// be retuned twice in one day (cold-start -> warm probing, then the speed bug).
// A band cut from it drifts, and "0.78 is a B" means nothing to a person picking
// a node.
//
// Effective latency folds the two things a user actually feels into one number
// in milliseconds:
//
//	eff = latency + jitterWeight*jitter + failShare*failCostMs
//
//   - latency+2*jitter is roughly the 97.5th percentile of a normal sample, i.e.
//     the delay felt "on a bad moment", not the lucky average.
//   - A failed probe is a stall for the user, costed at half the probe timeout.
//
// Bands then sit on the same millisecond values every client displays.
const (
	defaultJitterWeight = 2.0
	defaultFailCostMs   = 3000.0

	// Band edges in milliseconds of effective latency, placed on the measured
	// distribution of 831 live CF nodes (2026-11-17): the pool splits into a fast
	// cluster below 600ms and a slow one at 800-1200ms, with little in between.
	// An A edge of 350 fell into that gap and left the band empty.
	defaultGradeSMs = 200
	defaultGradeAMs = 400
	defaultGradeBMs = 600
	defaultGradeCMs = 1000
)

// effLatency computes the effective latency of one measurement.
func effLatency(latencyMs, jitterMs int, success, total int, jitterWeight, failCostMs float64) float64 {
	if latencyMs <= 0 {
		return 0
	}
	if jitterWeight <= 0 {
		jitterWeight = defaultJitterWeight
	}
	if failCostMs <= 0 {
		failCostMs = defaultFailCostMs
	}
	eff := float64(latencyMs) + jitterWeight*float64(jitterMs)
	if total > 0 && success < total {
		eff += float64(total-success) / float64(total) * failCostMs
	}
	return eff
}

// gradeForEff maps an effective latency to a letter using millisecond edges.
func gradeForEff(eff float64, s, a, b, c int) string {
	switch {
	case eff <= float64(s):
		return "S"
	case eff <= float64(a):
		return "A"
	case eff <= float64(b):
		return "B"
	case eff <= float64(c):
		return "C"
	default:
		return "D"
	}
}

// describe is used in logs to show why a node landed in its band.
func describe(eff float64, grade string) string {
	return fmt.Sprintf("%.0fms/%s", eff, grade)
}
