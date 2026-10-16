package sax

import (
	"math"
	"math/rand"

	. "synth/audio"
)

const (
	maxHarm = 40
	refHz   = 220 // pitch whose loudness every note is matched to

	attackTime   = 0.035 // s, smoothstep rise of an articulated note
	decayTime    = 0.3   // s, settle from peak to sustain
	sustainLevel = 0.85  // fraction of peak
	legatoWindow = 0.03  // s: a note starting this soon after a release is still the same breath
	releaseTau   = 0.02  // s, exponential fade after note-off
	glideTau     = 0.005 // s, pitch glide between legato notes
	gainTau      = 0.012 // s, spectrum cross-fade between legato notes
	dipDur       = 0.05  // s, finger-movement dip in level on a legato change
)

// Player is a monophonic, streaming saxophone: one breath that keeps going
// while the pitch changes. A note that starts while the previous one is still
// sounding (or was released within a few milliseconds) is slurred: the pitch
// glides, the spectrum cross-fades, there is no new attack. Only a note after
// a real gap (or a repeated pitch, which is "tongued") gets a fresh attack.
//
// A Player is not safe for concurrent use.
type Player struct {
	v   Voice
	opt Options
	rng *rand.Rand

	phases       [maxHarm + 1]float64
	gain, target [maxHarm + 1]float64
	onset        [maxHarm + 1]float64 // extra gain of partial h while burst == 1

	f, fTarget float64 // current and target fundamental, Hz
	midi       float64
	gate       bool    // key down
	amp        float64 // smoothed envelope level
	lvl, lvlT  float64 // smoothed per-note level (velocity, jitter)

	tNote, tAttack float64 // s since this note began / since the last attack
	sinceRelease   float64
	burst          float64 // decaying onset brightness/noise, 0..1
	scoopDepth     float64
	dipDepth       float64 // level dip on a legato change
	dipT           float64 // s into a legato dip; >= dipDur when idle

	vibPhase, vibRate float64
	vibAmt            float64 // smoothed vibrato depth 0..1; survives slurs
	level, tilt       float64 // per-note jitter (linear gain, dB/octave)
	noiseLP           float64
	kGlide, kGain     float64
}

// NewPlayer returns an idle Player. rng supplies per-note variation, vibrato
// rate and breath noise.
func NewPlayer(v Voice, opt Options, rng *rand.Rand) *Player {
	return &Player{
		v: v, opt: opt, rng: rng,
		vibRate: 5.2 + rng.Float64()*0.4,
		dipT:    dipDur,
		kGlide:  1 - math.Exp(-1/(glideTau*SampleRate)),
		kGain:   1 - math.Exp(-1/(gainTau*SampleRate)),
	}
}

// Active reports whether the Player still produces sound (a key is down or
// the release has not died away).
func (p *Player) Active() bool { return p.gate || p.amp > 1e-4 }

// Gate reports whether a note is currently held.
func (p *Player) Gate() bool { return p.gate }

// NoteOn starts a note (midi 1..127, velocity 0..1), slurring from the
// current one if it is still sounding.
func (p *Player) NoteOn(midi, velocity float64) {
	v := p.v
	base := MidiToHz(midi + p.opt.Transpose + v.Shift)

	slur := p.Active() && (p.gate || p.sinceRelease < legatoWindow)

	// per-note variation, as in a real player/instrument; within a slurred
	// phrase it only drifts, so level and tone don't jump at every note
	p.scoopDepth = 0.004 + 0.012*p.rng.Float64()
	jl, jt := p.rng.Float64()*2-1, p.rng.Float64()*2-1
	if slur {
		p.level *= math.Pow(10, jl*0.25/20)
		p.tilt = 0.8*p.tilt + 0.2*jt*1.5
	} else {
		p.level = math.Pow(10, jl/20)
		p.tilt = jt * 1.5 // dB per octave above 1 kHz
	}
	level, tiltDB := p.level, p.tilt

	// velocity darkens the upper partials (piano ~15 dB below forte above 2 kHz)
	velDB := (math.Min(velocity, 0.95)-0.95)*30 - 3 // -3: curve is a pooled median, brighter than mf

	// Each note is scaled to the same total partial power as a note at
	// refHz, so loudness doesn't follow the spectral curve up the scale (it
	// is an absolute-frequency curve: high fundamentals would be much weaker).
	var raw [maxHarm + 1]float64
	power := v.partials(base, &raw)
	var refRaw [maxHarm + 1]float64
	norm := math.Min(math.Sqrt(v.partials(refHz, &refRaw)/power), 4)

	for h := 1; h <= maxHarm; h++ {
		fh := base * float64(h)
		if fh > SampleRate/2-1000 {
			p.target[h], p.onset[h] = 0, 0
			continue
		}
		hf := math.Max(0, math.Min(1, math.Log2(fh/1000))) // 0 below 1 kHz, 1 from 2 kHz
		db := velDB*hf + tiltDB*math.Max(0, math.Log2(fh/1000))
		p.target[h] = raw[h] * norm * math.Pow(10, db/20)
		p.onset[h] = (math.Pow(10, 12.0/20) - 1) * hf // bright burst at the onset
	}
	p.lvlT = level * (0.3 + 0.7*velocity)

	switch {
	case !slur: // fresh attack
		p.f = base
		p.gain = p.target
		p.lvl = p.lvlT
		p.amp = 0
		p.tAttack = 0
		p.burst = 1
		p.vibAmt = 0
	case midi == p.midi: // repeated pitch: tongue it
		p.tAttack = 0.6 * attackTime
		p.burst = 0.5
	default: // slur
		// soft articulation: the breath dips and re-swells a little, like
		// a lightly tongued/fingered change, rather than a seamless synth glide
		// higher notes are articulated more: a seamless high slur sounds synthetic
		hi := math.Max(0, math.Min(1, (base-400)/600))
		p.tAttack = (0.5 - 0.3*hi) * attackTime
		p.burst = math.Max(p.burst, 0.3+0.3*hi)
		p.dipDepth = 0.3 + 0.25*hi
		p.dipT = 0
	}
	p.fTarget = base
	p.midi = midi
	p.tNote = 0
	p.gate = true
}

// NoteOff releases the current note.
func (p *Player) NoteOff() {
	if p.gate {
		p.gate = false
		p.sinceRelease = 0
	}
}

// Next returns the next output sample (about -1..1; not normalized).
func (p *Player) Next() float64 {
	if !p.Active() {
		return 0
	}
	const dt = 1.0 / SampleRate
	p.tNote += dt
	p.tAttack += dt

	// level envelope: attack, decay to sustain, then exponential release
	var envT float64
	if p.gate {
		switch t := p.tAttack; {
		case t < attackTime:
			x := t / attackTime
			envT = x * x * (3 - 2*x)
		case t < attackTime+decayTime:
			envT = 1 - (1-sustainLevel)*(t-attackTime)/decayTime
		default:
			envT = sustainLevel
		}
		p.amp += (envT - p.amp) * (1 - math.Exp(-dt/0.004))
	} else {
		p.sinceRelease += dt
		p.amp *= math.Exp(-dt / releaseTau)
	}
	dip := 1.0
	if p.dipT < dipDur {
		p.dipT += dt
		dip = 1 - p.dipDepth*math.Sin(math.Pi*p.dipT/dipDur)
	}
	p.lvl += (p.lvlT - p.lvl) * p.kGain

	// pitch: glide to the target, scoop into fresh attacks, vibrato late in long notes
	p.f += (p.fTarget - p.f) * p.kGlide
	// vibrato blooms on held notes and fades over short ones; it is not reset
	// by a slur, so a phrase doesn't restart its vibrato at every note
	if p.tNote > 0.25 {
		p.vibAmt += (1 - p.vibAmt) * (1 - math.Exp(-dt/0.25))
	} else {
		p.vibAmt *= math.Exp(-dt / 0.4)
	}
	vibDepth := p.vibAmt * 0.35 * p.opt.Vibrato
	p.vibPhase += 2 * math.Pi * p.vibRate * dt
	vib := math.Sin(p.vibPhase) * vibDepth * 0.01
	scoop := -p.scoopDepth * math.Exp(-p.tAttack/0.03)
	f0 := p.f * (1 + vib*3 + scoop)

	bright := 0.45 + 0.55*p.amp // formant voices: louder = brighter
	sample := 0.0
	nh := min(maxHarm, int((SampleRate/2-1000)/f0))
	for h := 1; h <= nh; h++ {
		g := p.gain[h] + (p.target[h]-p.gain[h])*p.kGain
		p.gain[h] = g
		p.phases[h] += 2 * math.Pi * f0 * float64(h) * dt
		a := g * (1 + p.onset[h]*p.burst)
		if p.v.Curve == nil {
			a *= math.Pow(bright, float64(h-1)*0.25)
		}
		sample += a * math.Sin(p.phases[h])
	}
	p.burst *= math.Exp(-dt / 0.12)

	// breath noise: low-passed white noise, loud at attack, quiet sustain
	w := p.rng.Float64()*2 - 1
	p.noiseLP += 0.35 * (w - p.noiseLP)
	breath := p.noiseLP * (0.03 + 0.14*p.burst) * p.opt.Breath

	return (sample*0.35 + breath) * p.lvl * p.amp * dip
}

// partials fills amp[1..] with the body-filtered amplitude of each harmonic
// of a note at fundamental f0 (zero above the usable band) and returns their
// total power.
func (v Voice) partials(f0 float64, amp *[maxHarm + 1]float64) (power float64) {
	for h := 1; h <= maxHarm; h++ {
		fh := f0 * float64(h)
		if fh > SampleRate/2-1000 {
			amp[h] = 0
			continue
		}
		var a float64
		if v.Curve != nil {
			a = curveGain(fh, v.Curve)
		} else {
			a = 1 / float64(h) * bodyGain(fh, v)
			if h%2 == 0 {
				a *= 0.8
			}
		}
		amp[h] = a
		power += a * a
	}
	return power
}
