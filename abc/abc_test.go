package abc

import (
	"math"
	"strings"
	"testing"
)

func pitches(t *Tune) []float64 {
	var p []float64
	for _, e := range t.Events {
		p = append(p, e.Midi)
	}
	return p
}

func mustParse(t *testing.T, src string) *Tune {
	t.Helper()
	tune, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return tune
}

func eq(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-9 {
			return false
		}
	}
	return true
}

func TestPitchesAndOctaves(t *testing.T) {
	tune := mustParse(t, "X:1\nL:1/4\nK:C\nC D E c c' C, G,\n")
	want := []float64{60, 62, 64, 72, 84, 48, 55}
	if !eq(pitches(tune), want) {
		t.Fatalf("got %v want %v", pitches(tune), want)
	}
}

func TestAccidentalsLastUntilBarLine(t *testing.T) {
	tune := mustParse(t, "L:1/4\nK:C\n^F F =F F | F _B B __B\n")
	want := []float64{66, 66, 65, 65, 65, 70, 70, 69}
	if !eq(pitches(tune), want) {
		t.Fatalf("got %v want %v", pitches(tune), want)
	}
}

func TestKeySignatures(t *testing.T) {
	cases := map[string][]float64{
		"K:G\nF":          {66},
		"K:F\nB":          {70},
		"K:Bb\nB E":       {70, 63},
		"K:Am\nF G":       {65, 67},
		"K:Dm\nB":         {70},
		"K:D dorian\nC F": {60, 65}, // D dorian = C major
		"K:E mix\nF D":    {66, 62},
	}
	for k, want := range cases {
		tune := mustParse(t, "L:1/4\n"+k+"\n")
		if !eq(pitches(tune), want) {
			t.Errorf("%q: got %v want %v", k, pitches(tune), want)
		}
	}
}

func TestLengths(t *testing.T) {
	tune := mustParse(t, "L:1/8\nK:C\nC C2 C/ C// C3/2 z2\n")
	want := []float64{1.0 / 8, 2.0 / 8, 1.0 / 16, 1.0 / 32, 1.5 / 8, 2.0 / 8}
	for i, e := range tune.Events {
		if math.Abs(e.Duration-want[i]) > 1e-9 {
			t.Errorf("event %d: got %v want %v", i, e.Duration, want[i])
		}
	}
	if tune.Events[5].Midi != 0 {
		t.Error("last event should be a rest")
	}
}

func TestDefaultUnitFromMeter(t *testing.T) {
	tune := mustParse(t, "M:4/4\nK:C\nC\n")
	if tune.Events[0].Duration != 1.0/8 {
		t.Errorf("got %v", tune.Events[0].Duration)
	}
	tune = mustParse(t, "M:2/4\nK:C\nC\n")
	if tune.Events[0].Duration != 1.0/16 {
		t.Errorf("got %v", tune.Events[0].Duration)
	}
}

func TestTies(t *testing.T) {
	tune := mustParse(t, "L:1/4\nK:C\nC2- | C2 D-D\n")
	if len(tune.Events) != 2 || tune.Events[0].Duration != 1.0 || tune.Events[1].Duration != 0.5 {
		t.Fatalf("got %+v", tune.Events)
	}
}

func TestBrokenRhythm(t *testing.T) {
	tune := mustParse(t, "L:1/4\nK:C\nC>D E<F\n")
	want := []float64{0.375, 0.125, 0.125, 0.375}
	for i, e := range tune.Events {
		if math.Abs(e.Duration-want[i]) > 1e-9 {
			t.Errorf("event %d: got %v want %v", i, e.Duration, want[i])
		}
	}
}

func TestTriplet(t *testing.T) {
	tune := mustParse(t, "L:1/8\nK:C\n(3CDE F\n")
	sum := 0.0
	for _, e := range tune.Events[:3] {
		sum += e.Duration
	}
	if math.Abs(sum-2.0/8) > 1e-9 || tune.Events[3].Duration != 1.0/8 {
		t.Fatalf("got %+v", tune.Events)
	}
}

func TestRepeats(t *testing.T) {
	tune := mustParse(t, "L:1/4\nK:C\n|: C D :| E\n")
	if !eq(pitches(tune), []float64{60, 62, 60, 62, 64}) {
		t.Fatalf("got %v", pitches(tune))
	}
	tune = mustParse(t, "L:1/4\nK:C\nC |: D E |1 F :|2 G |]\n")
	if !eq(pitches(tune), []float64{60, 62, 64, 65, 62, 64, 67}) {
		t.Fatalf("got %v", pitches(tune))
	}
	tune = mustParse(t, "L:1/4\nK:C\nC D :| E\n") // no |: means repeat from start
	if !eq(pitches(tune), []float64{60, 62, 60, 62, 64}) {
		t.Fatalf("got %v", pitches(tune))
	}
}

func TestDynamicsAndIgnoredMarkup(t *testing.T) {
	tune := mustParse(t, "L:1/4\nK:C\n% comment\n!p! \"Am\" C {D}(D E) !ff! ~F .G\n")
	if !eq(pitches(tune), []float64{60, 62, 64, 65, 67}) {
		t.Fatalf("got %v", pitches(tune))
	}
	if tune.Events[0].Velocity != 0.55 || tune.Events[3].Velocity != 0.95 {
		t.Errorf("velocities %v %v", tune.Events[0].Velocity, tune.Events[3].Velocity)
	}
}

func TestTempo(t *testing.T) {
	tune := mustParse(t, "Q:1/4=120\nL:1/4\nK:C\nC\n")
	n := tune.Notes(0)
	if math.Abs(n[0].Duration-0.5) > 1e-9 {
		t.Errorf("got %v", n[0].Duration)
	}
	if n = tune.Notes(60); math.Abs(n[0].Duration-1) > 1e-9 {
		t.Errorf("override: got %v", n[0].Duration)
	}
	tune = mustParse(t, "Q:1/2=60\nL:1/4\nK:C\nC\n")
	if n = tune.Notes(0); math.Abs(n[0].Duration-0.5) > 1e-9 {
		t.Errorf("half-note tempo: got %v", n[0].Duration)
	}
}

func TestInlineAndContinuation(t *testing.T) {
	tune := mustParse(t, "L:1/4\nK:C\nC D \\\n[K:G] F\n")
	if !eq(pitches(tune), []float64{60, 62, 66}) {
		t.Fatalf("got %v", pitches(tune))
	}
}

func TestErrors(t *testing.T) {
	for _, src := range []string{"K:C\n[CEG]\n", "K:C\nC $\n", "K:C\n^\n", "K:C\n>C\n"} {
		if _, err := Parse(src); err == nil {
			t.Errorf("%q: expected error", src)
		}
	}
	if _, err := Parse("K:C\nC $\n"); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error should mention line: %v", err)
	}
}

func TestStopsAtBlankLine(t *testing.T) {
	tune := mustParse(t, "T:One\nL:1/4\nK:C\nC D\n\nX:2\nT:Two\nK:C\nE F\n")
	if tune.Title != "One" || len(tune.Events) != 2 {
		t.Fatalf("got %q %d", tune.Title, len(tune.Events))
	}
}
