package sax

import (
	"math/rand"

	. "synth/audio"
)

// RenderNote synthesizes a single note including its release tail
// (Duration + 0.12 s of audio). Midi 0 is a rest (silence of the same length).
// rng supplies the vibrato rate and breath noise; pass a fixed seed for
// repeatable output.
func (v Voice) RenderNote(n Note, opt Options, rng *rand.Rand) []float64 {
	const release = 0.12
	out := make([]float64, int((n.Duration+release)*SampleRate))
	if n.Midi == 0 {
		return out
	}
	p := NewPlayer(v, opt, rng)
	p.NoteOn(n.Midi, n.Velocity)
	on := int(n.Duration * SampleRate)
	for i := range out {
		if i == on {
			p.NoteOff()
		}
		out[i] = p.Next()
	}
	return out
}

// Render plays notes one after another with voice v and returns mono samples
// in -1..1, with reverb applied and the peak normalized to about -3 dB.
// Consecutive notes with no rest between them are slurred like one breath;
// a rest, or a repeated pitch, starts a new attack.
func Render(notes []Note, v Voice, opt Options) []float64 {
	p := NewPlayer(v, opt, rand.New(rand.NewSource(opt.Seed)))
	var mix []float64
	emit := func(n int) {
		for i := 0; i < n; i++ {
			mix = append(mix, p.Next())
		}
	}
	for i, n := range notes {
		total := int((n.Duration + opt.Gap) * SampleRate)
		if n.Midi == 0 {
			p.NoteOff()
			emit(total)
			continue
		}
		p.NoteOn(n.Midi, n.Velocity)
		// keep blowing into the next note if there is one
		if i+1 == len(notes) || notes[i+1].Midi == 0 {
			hold := int(n.Duration * SampleRate)
			emit(hold)
			p.NoteOff()
			emit(total - hold)
		} else {
			emit(total)
		}
	}
	p.NoteOff()
	emit(int(0.3 * SampleRate)) // release tail
	return Finish(mix, opt.Reverb)
}
