package scoring

import "testing"

// The letter band must follow the score, so a node cannot be labelled D while
// its quality number says A. Edges are the grade_*_min values.
func TestGradeFollowsScore(t *testing.T) {
	const s, a, b, c = 0.85, 0.78, 0.60, 0.35
	cases := []struct {
		score float64
		want  string
	}{
		{0.95, "S"}, {0.85, "S"},
		{0.84, "A"}, {0.78, "A"},
		{0.77, "B"}, {0.60, "B"},
		{0.59, "C"}, {0.35, "C"},
		{0.34, "D"}, {0.01, "D"},
	}
	for _, tc := range cases {
		if got := gradeForScore(tc.score, s, a, b, c); got != tc.want {
			t.Errorf("gradeForScore(%.2f) = %q, want %q", tc.score, got, tc.want)
		}
	}
}

// A zero score means "no usable evidence", not "worst node": it must stay
// unlabelled rather than being shown as the lowest band.
func TestZeroScoreIsUnlabelled(t *testing.T) {
	if got := gradeForScore(0, 0.85, 0.78, 0.60, 0.35); got != "" {
		t.Fatalf("zero score must yield an empty grade, got %q", got)
	}
}
