// Package piano is a small additive piano synthesizer: inharmonic partials
// (string stiffness), two slightly detuned strings per note so it beats, a
// hammer thump, and per-partial exponential decay (highs die first).
// A note's Duration is how long the key is held; the damper then mutes it.
package piano

import (
	"math"
	"math/rand"

	"synth/audio"
)

var rate = audio.SampleRate

var (
	attack = int(0.003 * float64(rate))
	thump  = int(0.02 * float64(rate))
)

const (
	damper = 0.15 // seconds for the damper to mute a released note
	tail   = 2 * damper
)

// RenderNote synthesizes one note (Duration + release tail). Midi 0 is a rest.
func RenderNote(n audio.Note, opt audio.Options, rng *rand.Rand) []float64 {
	total := int((n.Duration + tail) * audio.SampleRate)
	out := make([]float64, total)
	if n.Midi == 0 {
		return out
	}
	m := n.Midi + opt.Transpose
	f0 := audio.MidiToHz(m)
	const B = 0.0008 // inharmonicity; ponytail: fixed, really grows toward the bass and treble
	// fundamental decay time constant, s; bass rings long, treble dies quickly
	// (a sustained high note is what sounds plucked)
	tau := 7 * math.Pow(261.6/f0, 0.4)
	if f0 > 261.6 {
		tau = 7 * math.Pow(261.6/f0, 1.1)
	}
	vel := 0.3 + 0.7*n.Velocity
	strings := [3]float64{1 - 0.0004, 1, 1 + 0.0005}

	for h := 1; h <= 30; h++ {
		fh := f0 * float64(h) * math.Sqrt(1+B*float64(h*h))
		if fh > audio.SampleRate/2-1000 {
			break
		}
		// hammer spectrum: harder hit = brighter
		// hammer strikes ~1/8 along the string, which suppresses the 8th partial
		amp := vel / math.Pow(float64(h), 1.3) * (0.3 + 0.7*math.Abs(math.Sin(math.Pi*float64(h)/8.3))) * math.Exp(-float64(h)*(1.2-n.Velocity)*0.08)
		th := tau / (1 + 1.2*float64(h-1)) // aftersound: slow decay
		for _, s := range strings {
			w := 2 * math.Pi * fh * s / audio.SampleRate
			ph := rng.Float64() * 2 * math.Pi
			for i := range out {
				t := float64(i) / audio.SampleRate
				// piano decays in two stages: fast prompt sound, then a slow aftersound
				env := 0.55*math.Exp(-t/(th/6)) + 0.45*math.Exp(-t/th)
				if t > n.Duration {
					env *= math.Exp(-(t - n.Duration) / (damper / 4))
				}
				out[i] += 0.17 * amp * env * math.Sin(w*float64(i)+ph)
			}
		}
	}
	// hammer thump: a few ms of low-passed noise
	lp := 0.0
	for i := 0; i < thump && i < total; i++ {
		lp += 0.1 * (rng.Float64()*2 - 1 - lp)
		out[i] += lp * 0.3 * vel * math.Exp(-float64(i)/(0.004*float64(audio.SampleRate)))
	}
	// 3 ms attack ramp avoids a click
	for i := 0; i < attack && i < total; i++ {
		out[i] *= float64(i) / (0.003 * float64(audio.SampleRate))
	}
	return out
}

// Render plays notes in sequence (overlapping tails ring into each other),
// applies reverb and normalizes the peak to 0.7, like audio.Render.
func Render(notes []audio.Note, opt audio.Options) []float64 {
	rng := rand.New(rand.NewSource(opt.Seed))
	var mix []float64
	pos := 0
	for _, n := range notes {
		buf := RenderNote(n, opt, rng)
		if end := pos + len(buf); end > len(mix) {
			mix = append(mix, make([]float64, end-len(mix))...)
		}
		for i, s := range buf {
			mix[pos+i] += s
		}
		pos += int((n.Duration + opt.Gap) * audio.SampleRate)
	}
	return audio.Finish(mix, opt.Reverb)
}
