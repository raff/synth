package sax

import (
	"math"
	"math/rand"

	. "synth/audio"
)

// adsr envelope evaluated at time t for a note lasting dur seconds.
func adsr(t, dur float64) float64 {
	const attack, decay, sustain, release = 0.05, 0.25, 0.94, 0.12
	switch {
	case t < attack:
		x := t / attack
		return x * x * (3 - 2*x) // smoothstep
	case t < attack+decay:
		return 1 - (1-sustain)*(t-attack)/decay
	case t < dur:
		return sustain
	case t < dur+release:
		return sustain * (1 - (t-dur)/release)
	}
	return 0
}

// RenderNote synthesizes a single note including its release tail
// (Duration + 0.12 s of audio). Midi 0 is a rest (silence of the same length).
// rng supplies the vibrato rate and breath noise; pass a fixed seed for
// repeatable output.
func (v Voice) RenderNote(n Note, opt Options, rng *rand.Rand) []float64 {
	const release = 0.12
	total := int((n.Duration + release) * SampleRate)
	out := make([]float64, total)
	if n.Midi == 0 {
		return out
	}

	base := MidiToHz(n.Midi + opt.Transpose + v.Shift)
	const maxHarm = 40
	phases := [maxHarm + 1]float64{}

	vibRate := 5.2 + rng.Float64()*0.4
	vibPhase := 0.0

	// one-pole band-limited noise for breath
	noiseLP := 0.0

	for i := 0; i < total; i++ {
		t := float64(i) / SampleRate
		env := adsr(t, n.Duration)

		// vibrato: starts after ~0.25s, ramps in over 0.4s, slight depth drift
		vibDepth := 0.0
		if t > 0.25 {
			vibDepth = math.Min((t-0.25)/0.4, 1) * 0.35 // semitone-ish cents/100
		}
		vibPhase += 2 * math.Pi * vibRate / SampleRate
		vib := math.Sin(vibPhase) * vibDepth * 0.01 // fraction of freq ~ +/-0.35%... scaled below

		// pitch scoop: start ~35 cents flat, settle in 70ms
		scoop := -0.02 * math.Exp(-t/0.03)

		f0 := base * (1 + vib*3 + scoop)

		// brightness follows the envelope: louder = more upper partials
		bright := 0.45 + 0.55*env

		sample := 0.0
		for h := 1; h <= maxHarm; h++ {
			fh := f0 * float64(h)
			if fh > SampleRate/2-1000 {
				break
			}
			phases[h] += 2 * math.Pi * fh / SampleRate
			var amp float64
			if v.Curve != nil {
				amp = curveGain(fh, v.Curve)
				amp *= math.Pow(bright, float64(h-1)*0.25)
			} else {
				amp = 1 / float64(h)                       // sawtooth-like reed spectrum
				amp *= math.Pow(bright, float64(h-1)*0.25) // rolloff controlled by breath pressure
				amp *= bodyGain(fh, v)                     // body resonances
				if h%2 == 0 {
					amp *= 0.8 // sax has slightly weaker even partials than a saw
				}
			}
			sample += amp * math.Sin(phases[h])
		}

		// breath noise: low-passed white noise, loud at attack, quiet sustain
		w := rng.Float64()*2 - 1
		noiseLP += 0.35 * (w - noiseLP)
		breath := noiseLP * (0.05 + 0.12*math.Exp(-t/0.12)) * opt.Breath

		out[i] = (sample*0.35 + breath) * env * (0.3 + 0.7*n.Velocity)
	}
	return out
}

// Render plays notes one after another with voice v and returns mono samples
// in -1..1, with reverb applied and the peak normalized to about -3 dB.
func Render(notes []Note, v Voice, opt Options) []float64 {
	rng := rand.New(rand.NewSource(opt.Seed))
	var mix []float64
	pos := 0
	for i, n := range notes {
		full := n.Duration
		// repeated pitch: end the release before the next onset so it re-articulates
		if i+1 < len(notes) && notes[i+1].Midi == n.Midi && n.Midi != 0 {
			n.Duration = math.Max(n.Duration-0.12, 0.05)
		}
		buf := v.RenderNote(n, opt, rng)
		if end := pos + len(buf); end > len(mix) {
			mix = append(mix, make([]float64, end-len(mix))...)
		}
		for i, s := range buf {
			mix[pos+i] += s
		}
		pos += int((full + opt.Gap) * SampleRate)
	}
	return Finish(mix, opt.Reverb)
}
