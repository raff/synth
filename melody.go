package main

import "synth/sax"

// beatNote is a melody event in musical units.
type beatNote struct{ midi, beats, vel float64 }

// demoPhrase: four rising three-note runs, each ending on a held note.
// Pitches are sounding MIDI notes; durations are in beats.
var demoPhrase = []beatNote{
	{55, 1, 0.75}, {57, 1, 0.75}, {58, 1, 0.75}, {63, 4, 0.85},
	{53, 1, 0.75}, {55, 1, 0.75}, {57, 1, 0.75}, {62, 4, 0.85},
	{52, 1, 0.75}, {53, 1, 0.75}, {55, 1, 0.75}, {60, 4, 0.85},
	{50, 1, 0.75}, {52, 1, 0.75}, {53, 1, 0.75}, {58, 4, 0.85},
}

// demoMelody plays demoPhrase, a one-beat rest, then the phrase an octave up.
func demoMelody(bpm float64) []sax.Note {
	beat := 60 / bpm
	var notes []sax.Note
	for pass, octave := range []float64{0, 12} {
		for _, n := range demoPhrase {
			notes = append(notes, sax.Note{Midi: n.midi + octave, Duration: n.beats * beat, Velocity: n.vel})
		}
		if pass == 0 {
			notes = append(notes, sax.Note{Duration: beat})
		}
	}
	return notes
}
