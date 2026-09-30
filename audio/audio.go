// Package audio holds what the instrument packages (sax, piano) share: the
// note and render-option types, reverb, WAV output and playback.
package audio

import "math"

// SampleRate of all rendered audio, in Hz.
const SampleRate = 44100

// Note is one melody event.
type Note struct {
	Midi     float64 // MIDI note number, 0 = rest
	Duration float64 // seconds
	Velocity float64 // 0..1
}

// Options are the render settings shared by all notes.
type Options struct {
	Transpose float64 // semitones added to every note
	Breath    float64 // breath noise level (1 = default, 0 = none); sax only
	Reverb    float64 // reverb wet mix, 0..1 (Render only)
	Gap       float64 // seconds added between consecutive notes (Render only)
	Seed      int64   // random seed (Render only)
}

// DefaultOptions returns the settings the synth was tuned with.
func DefaultOptions() Options {
	return Options{Breath: 1.0, Reverb: 0.18, Gap: 0.02, Seed: 1}
}

// MidiToHz converts a MIDI note number to frequency.
func MidiToHz(m float64) float64 { return 440 * math.Pow(2, (m-69)/12) }
