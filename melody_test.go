package main

import (
	"math"
	"os"
	"testing"

	"synth/abc"
)

// examples/runs.abc is the first pass of the demo melody written in ABC.
func TestRunsExampleMatchesDemo(t *testing.T) {
	src, err := os.ReadFile("examples/runs.abc")
	if err != nil {
		t.Fatal(err)
	}
	tune, err := abc.Parse(string(src))
	if err != nil {
		t.Fatal(err)
	}
	got, want := tune.Notes(100), demoMelody(100)[:len(demoPhrase)]
	if len(got) != len(want) {
		t.Fatalf("got %d notes, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Midi != want[i].Midi || math.Abs(got[i].Duration-want[i].Duration) > 1e-9 ||
			math.Abs(got[i].Velocity-want[i].Velocity) > 1e-9 {
			t.Errorf("note %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}
