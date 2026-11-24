package workerhealth

import "testing"

type step struct {
	kind RoundKind
	want State
	bad  int
	good int
}

// GPT-reviewed state machine (2026-11-18): every transition listed in the design
// review, including the inconclusive no-op rule.
func TestTransitions(t *testing.T) {
	cases := []struct {
		name     string
		start    State
		kind     RoundKind
		consBad  int
		consGood int
		want     State
		wantBad  int
		wantGood int
	}{
		{"healthy+bad->degraded", Healthy, RoundBad, 0, 0, Degraded, 1, 0},
		{"degraded+bad->open", Degraded, RoundBad, 1, 0, Open, 0, 0},
		{"degraded+normal->healthy", Degraded, RoundNormal, 1, 0, Healthy, 0, 0},
		{"degraded+good->healthy", Degraded, RoundGood, 1, 0, Healthy, 0, 0},
		{"open+good->half_open (counter resets)", Open, RoundGood, 0, 0, HalfOpen, 0, 0},
		{"open+normal->stays", Open, RoundNormal, 0, 0, Open, 0, 0},
		{"open+bad->stays", Open, RoundBad, 0, 0, Open, 0, 0},
		{"half_open+good (first of two)", HalfOpen, RoundGood, 0, 0, HalfOpen, 0, 1},
		{"half_open+bad->open", HalfOpen, RoundBad, 0, 1, Open, 0, 0},
		{"half_open+normal->resets", HalfOpen, RoundNormal, 0, 1, HalfOpen, 0, 0},
	}
	for _, c := range cases {
		got, bad, good := Next(c.start, c.kind, c.consBad, c.consGood)
		if got != c.want || bad != c.wantBad || good != c.wantGood {
			t.Errorf("%s: got state=%s bad=%d good=%d, want %s/%d/%d",
				c.name, got, bad, good, c.want, c.wantBad, c.wantGood)
		}
	}
}

// Inconclusive rounds are transparent: no state change, no counter movement.
func TestInconclusiveIsTransparent(t *testing.T) {
	for _, start := range []State{Healthy, Degraded, Open, HalfOpen} {
		for _, kind := range []RoundKind{RoundBad, RoundNormal, RoundGood} {
			got, bad, good := Next(start, RoundInconclusive, 2, 2)
			if got != start || bad != 2 || good != 2 {
				t.Fatalf("inconclusive from %s kind %s: got %s bad=%d good=%d", start, kind, got, bad, good)
			}
		}
	}
}

// Classification thresholds calibrated on the 44-round replay.
func TestClassify(t *testing.T) {
	cases := []struct {
		cfMed float64
		cfN   int
		air   int
		airN  int
		want  RoundKind
	}{
		{164, 30, 137, 10, RoundGood},          // ratio 1.2-ish: 164/137=1.20 -> good
		{498, 30, 337, 10, RoundNormal},        // ratio 1.48 -> normal
		{1203, 30, 147, 10, RoundBad},          // ratio 8.2 -> bad
		{1500, 30, 3, 3, RoundInconclusive},    // airport under-sampled
		{1500, 10, 300, 10, RoundInconclusive}, // CF under-sampled
	}
	for _, c := range cases {
		got := Classify(float64(c.cfMed), c.cfN, c.air, c.airN)
		if got != c.want {
			t.Errorf("Classify(cf=%v,cfN=%d,air=%d,airN=%d)=%s want %s", c.cfMed, c.cfN, c.air, c.airN, got, c.want)
		}
	}
}
