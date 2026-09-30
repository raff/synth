// Package sax is a small additive synthesizer that imitates saxophones.
//
// What makes it sax-like:
//   - harmonic-rich source (reed) shaped by a body spectrum, either fixed
//     formants or a spectral envelope measured from real recordings
//   - brightness that follows loudness
//   - delayed, gradually growing vibrato
//   - breath noise, strongest at the attack
//   - a small pitch "scoop" up into each note
//   - a touch of room reverb
//
// Basic use:
//
//	v, _ := sax.VoiceByName("tenor")
//	pcm := sax.Render([]audio.Note{{Midi: 60, Duration: 1, Velocity: 0.8}}, v, audio.DefaultOptions())
//	audio.WriteWAV("out.wav", pcm)
//
// For real-time or per-note use, call Voice.RenderNote directly.
package sax

import (
	"math"
	"sort"
)

// Formant is a resonance peak in the body spectrum.
type Formant struct{ Freq, BW, Gain float64 }

// Voice describes the timbre of one instrument.
type Voice struct {
	Name     string
	Formants []Formant
	Floor    float64 // minimum gain so nothing vanishes completely
	// Curve, if set, replaces Formants/Floor and the 1/n rolloff: a measured
	// spectral envelope as {Hz, dB} points (piecewise linear in log-frequency).
	Curve [][2]float64
	Shift float64 // semitones added to every note (e.g. to play in the instrument's own range)
}

var voices = []Voice{
	{Name: "base", Formants: []Formant{{450, 300, 1.0}, {900, 400, 0.8}, {1800, 700, 0.45}, {2800, 900, 0.25}}, Floor: 0.06},
	{Name: "bright", Formants: []Formant{{450, 300, 0.8}, {900, 400, 0.8}, {1800, 700, 1.0}, {2800, 900, 0.8}}, Floor: 0.06},
	{Name: "dark", Formants: []Formant{{450, 300, 1.0}, {900, 400, 0.5}, {1800, 700, 0.15}, {2800, 900, 0.05}}, Floor: 0.03},
	// "bd" = bright but darker: keep the 1.8 kHz edge, trim the very top / add low body.
	{Name: "bd1", Formants: []Formant{{450, 300, 1.0}, {900, 400, 0.8}, {1800, 700, 0.8}, {2800, 900, 0.4}}, Floor: 0.06},
	{Name: "bd2", Formants: []Formant{{300, 250, 1.0}, {600, 350, 0.9}, {1800, 700, 0.9}, {2800, 900, 0.3}}, Floor: 0.06},
	{Name: "bd3", Formants: []Formant{{450, 300, 1.0}, {900, 400, 0.8}, {1800, 700, 1.0}, {2800, 900, 0.1}}, Floor: 0.04},
	{Name: "nasal", Formants: []Formant{{450, 120, 1.0}, {900, 150, 0.9}, {1800, 250, 0.6}, {2800, 300, 0.35}}, Floor: 0.03},
	{Name: "honky", Formants: []Formant{{450, 300, 1.0}, {900, 400, 0.8}, {1300, 250, 0.9}, {1800, 700, 0.45}, {2800, 900, 0.25}}, Floor: 0.06},
	{Name: "buzzy", Formants: []Formant{{450, 300, 1.0}, {900, 400, 0.8}, {1800, 700, 0.45}, {2800, 900, 0.25}}, Floor: 0.2},
	// Fitted to real tenor sax recordings (158 steady notes, median level of
	// each harmonic vs absolute frequency; the top end is smoothed).
	{Name: "tenor", Curve: [][2]float64{
		{100, -5}, {200, 0}, {300, -1}, {450, -8}, {600, -9}, {800, -15}, {1000, -15},
		{1300, -20}, {1700, -20}, {2200, -20}, {2800, -23}, {3500, -33}, {4500, -38},
		{6000, -48}, {8000, -52}, {11000, -58},
	}},
	// Alto: 49 steady notes (MIDI 49-80), band medians + fit, smoothed. Energy
	// peaks around 450-600 Hz and the fundamental of low notes is weak.
	{Name: "alto", Curve: [][2]float64{
		{100, -13}, {200, -7}, {300, -4}, {450, 0}, {600, -3}, {800, -8}, {1000, -18},
		{1300, -22}, {1700, -21}, {2200, -24}, {2800, -27}, {3500, -34}, {4500, -38},
		{6000, -46}, {8000, -50}, {11000, -58},
	}},
	// Soprano: 56 steady notes (MIDI 58-83), band medians, smoothed. Brighter
	// than tenor up to ~1.3 kHz, then falls off faster; nothing measured below ~230 Hz.
	{Name: "soprano", Shift: 12, Curve: [][2]float64{
		{250, 0}, {500, -1}, {750, -2}, {1100, -4}, {1550, -12}, {2100, -16},
		{2800, -27}, {3800, -32}, {5500, -49}, {8000, -55}, {11000, -60},
	}},
}

// VoiceByName looks up a voice by name.
func VoiceByName(name string) (Voice, bool) {
	for _, v := range voices {
		if v.Name == name {
			return v, true
		}
	}
	return Voice{}, false
}

// VoiceNames returns all voice names, sorted.
func VoiceNames() []string {
	names := make([]string, len(voices))
	for i, v := range voices {
		names[i] = v.Name
	}
	sort.Strings(names)
	return names
}

// curveGain interpolates a voice's measured envelope (dB) at freq f as a linear gain.
func curveGain(f float64, c [][2]float64) float64 {
	if f <= c[0][0] {
		return math.Pow(10, c[0][1]/20)
	}
	for i := 1; i < len(c); i++ {
		if f <= c[i][0] {
			t := math.Log(f/c[i-1][0]) / math.Log(c[i][0]/c[i-1][0])
			return math.Pow(10, (c[i-1][1]+t*(c[i][1]-c[i-1][1]))/20)
		}
	}
	return math.Pow(10, c[len(c)-1][1]/20)
}

// bodyGain returns how strongly the body boosts a partial at freq f.
func bodyGain(f float64, v Voice) float64 {
	g := v.Floor
	for _, fm := range v.Formants {
		d := (f - fm.Freq) / fm.BW
		g += fm.Gain * math.Exp(-d*d)
	}
	return g
}
