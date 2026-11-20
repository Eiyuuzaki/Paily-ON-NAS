package scoring

// Grade is the human-facing quality band. It is cut from the score, so a node
// has exactly one quality number and one label.
//
// An earlier revision banded effective latency instead, arguing that a band cut
// from the score would drift because the score's anchors had been retuned twice
// in one day. That reasoning held only while the score saturated: 359 of 1115
// nodes sat at exactly 0, so no band cut from it could separate them. With the
// exponential curve in place (2026-11-20) the score spreads over 853 distinct
// values, and the latency band became the unstable one — it read only the newest
// sample and weighted jitter 2x, so a single spike moved a node from A to D
// while its score held at 0.79. That left 341 nodes labelled D, 14 of them
// scoring above 0.78, and 225 of the 392 C nodes scoring at B level.
//
// Bands therefore come from the score, using the edges the UI already
// advertises (grade_*_min). Effective latency is kept as a display figure only.
const (
	defaultJitterWeight = 2.0
	defaultFailCostMs   = 3000.0
)

// defaultGrade{S,A,B,C}Min are the score edges used when the config table has no
// override. They mirror the values config.go seeds.
const (
	defaultGradeSMin = 0.85
	defaultGradeAMin = 0.78
	defaultGradeBMin = 0.60
	defaultGradeCMin = 0.35
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

// gradeForScore maps a 0..1 score to a letter. A score of 0 means "no usable
// evidence" rather than "worst node", so it yields an empty grade and the node
// stays unlabelled instead of being presented as很差.
func gradeForScore(score, s, a, b, c float64) string {
	switch {
	case score <= 0:
		return ""
	case score >= s:
		return "S"
	case score >= a:
		return "A"
	case score >= b:
		return "B"
	case score >= c:
		return "C"
	default:
		return "D"
	}
}
