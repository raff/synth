package piano

import (
	"math"
	"testing"

	"synth/audio"
)

func TestRender(t *testing.T) {
	pcm := Render([]audio.Note{{Midi: 60, Duration: 0.5, Velocity: 0.8}, {Midi: 0, Duration: 0.2}, {Midi: 72, Duration: 0.5, Velocity: 0.8}}, audio.DefaultOptions())
	peak := 0.0
	for _, s := range pcm {
		if math.IsNaN(s) {
			t.Fatal("NaN sample")
		}
		peak = math.Max(peak, math.Abs(s))
	}
	if math.Abs(peak-0.7) > 1e-6 {
		t.Fatalf("peak = %v, want 0.7", peak)
	}
}
