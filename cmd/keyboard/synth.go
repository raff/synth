// The keyboard's voices: notes rendered by synth/piano and synth/sax, played
// back through shirei's mixer. A note is rendered once per (instrument, midi)
// and cached; key-up fades the playing buffer out.
package main

import (
	"math/rand"
	"sync"
	"sync/atomic"

	"go.hasen.dev/shirei/audio"

	synth "synth/audio"
	"synth/piano"
	"synth/sax"
)

const SampleRate = synth.SampleRate

var mixer = audio.NewMixer()

func init() {
	mixer.SetVolume(0.6)
}

const (
	holdSecs = 4.0  // longest a held note sustains before its own release
	fadeSecs = 0.08 // fade-out on key-up
)

type cacheKey struct {
	voice VoiceKind
	midi  int
}

type cacheEntry struct {
	once sync.Once
	pcm  []float32
}

var cache sync.Map // cacheKey -> *cacheEntry

func render(kind VoiceKind, midi int) []float32 {
	e, _ := cache.LoadOrStore(cacheKey{kind, midi}, &cacheEntry{})
	ce := e.(*cacheEntry)
	ce.once.Do(func() {
		n := synth.Note{Midi: float64(midi), Duration: holdSecs, Velocity: 0.8}
		opt := synth.DefaultOptions()
		rng := rand.New(rand.NewSource(int64(midi)))
		var pcm []float64
		if kind == VoicePiano {
			pcm = piano.RenderNote(n, opt, rng)
		} else {
			v, _ := sax.VoiceByName(kind.String())
			pcm = v.RenderNote(n, opt, rng)
		}
		ce.pcm = make([]float32, len(pcm))
		for i, s := range pcm {
			ce.pcm[i] = float32(s) * 0.5
		}
	})
	return ce.pcm
}

// warmGen identifies the latest warm request; older workers stop early.
var warmGen atomic.Int64

// warm renders every key (at the current octave) of every instrument in the
// background so the first press of a key doesn't stall on synthesis. It uses
// a single worker, current voice first, so it can't starve the audio callback
// (or a playing tune) of CPU; a newer call supersedes a running one.
func warm() {
	gen := warmGen.Add(1)
	cur := appData.voice
	pitches := make([]int, len(allKeys))
	for i, k := range allKeys {
		pitches[i] = k.pitch()
	}
	go func() {
		for i := range VoiceSoprano + 1 {
			kind := (cur + i) % (VoiceSoprano + 1)
			for _, p := range pitches {
				if warmGen.Load() != gen {
					return
				}
				render(kind, p)
			}
		}
	}()
}

// bufVoice plays a cached buffer; Release fades it out.
type bufVoice struct {
	pcm      []float32
	pos      int
	released atomic.Bool
	fade     int // samples faded so far
}

func (v *bufVoice) Release() { v.released.Store(true) }

func (v *bufVoice) Render(out []float32) bool {
	const fadeLen = int(fadeSecs * SampleRate)
	rel := v.released.Load()
	for i := range out {
		if v.pos >= len(v.pcm) {
			return false
		}
		g := float32(1)
		if rel {
			if v.fade >= fadeLen {
				return false
			}
			g = 1 - float32(v.fade)/float32(fadeLen)
			v.fade++
		}
		out[i] += v.pcm[v.pos] * g
		v.pos++
	}
	return true
}
