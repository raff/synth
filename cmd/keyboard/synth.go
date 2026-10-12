// The keyboard's voices, played through shirei's mixer. The piano renders a
// note once per midi pitch and caches it; key-up fades the playing buffer
// out. The saxophones are one monophonic streaming sax.Player per
// instrument: overlapping keys are slurred like a real breath.
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
	midi int
}

type cacheEntry struct {
	once sync.Once
	pcm  []float32
}

var cache sync.Map // cacheKey -> *cacheEntry (piano only)

func render(midi int) []float32 {
	e, _ := cache.LoadOrStore(cacheKey{midi}, &cacheEntry{})
	ce := e.(*cacheEntry)
	ce.once.Do(func() {
		n := synth.Note{Midi: float64(midi), Duration: holdSecs, Velocity: 0.8}
		opt := synth.DefaultOptions()
		rng := rand.New(rand.NewSource(int64(midi)))
		pcm := piano.RenderNote(n, opt, rng)
		ce.pcm = make([]float32, len(pcm))
		for i, s := range pcm {
			ce.pcm[i] = float32(s) * 0.5
		}
	})
	return ce.pcm
}

// warmGen identifies the latest warm request; older workers stop early.
var warmGen atomic.Int64

// warm renders every key (at the current octave) of the piano in the
// background so the first press of a key doesn't stall on synthesis. It uses
// a single worker so it can't starve the audio callback (or a playing tune)
// of CPU; a newer call supersedes a running one.
func warm() {
	gen := warmGen.Add(1)
	pitches := make([]int, len(allKeys))
	for i, k := range allKeys {
		pitches[i] = k.pitch()
	}
	go func() {
		for _, p := range pitches {
			if warmGen.Load() != gen {
				return
			}
			render(p)
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

// saxVoice is one streaming sax.Player in the mixer. Held keys form a stack:
// the newest sounds, and releasing it returns (slurred) to the one below.
type saxVoice struct {
	kind  VoiceKind
	mu    sync.Mutex
	p     *sax.Player
	stack []int
	vel   float64
	done  bool
}

var (
	curSaxMu sync.Mutex
	curSax   *saxVoice
)

// saxNote is what a key holds for a sax press.
type saxNote struct {
	v    *saxVoice
	midi int
}

func (n *saxNote) Release() { n.v.release(n.midi) }

// Render implements the mixer's voice interface.
func (v *saxVoice) Render(out []float32) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	for i := range out {
		if !v.p.Active() {
			v.done = true
			return false
		}
		out[i] += float32(v.p.Next()) * 0.5
	}
	return true
}

func (v *saxVoice) press(midi int, vel float64) {
	v.mu.Lock()
	v.stack = append(v.stack, midi)
	v.vel = vel
	v.p.NoteOn(float64(midi), vel)
	v.mu.Unlock()
}

func (v *saxVoice) release(midi int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	for i := len(v.stack) - 1; i >= 0; i-- {
		if v.stack[i] != midi {
			continue
		}
		top := i == len(v.stack)-1
		v.stack = append(v.stack[:i], v.stack[i+1:]...)
		switch {
		case len(v.stack) == 0:
			v.p.NoteOff()
		case top:
			v.p.NoteOn(float64(v.stack[len(v.stack)-1]), v.vel)
		}
		return
	}
}

// saxPress starts a note on the current instrument's sax, creating the
// player (and adding it to the mixer) if there is none sounding.
func saxPress(kind VoiceKind, midi int, vel float64) Note {
	curSaxMu.Lock()
	defer curSaxMu.Unlock()
	if v := curSax; v != nil && v.kind == kind {
		v.mu.Lock()
		done := v.done
		v.mu.Unlock()
		if !done {
			v.press(midi, vel)
			return &saxNote{v, midi}
		}
	}
	voice, _ := sax.VoiceByName(kind.String())
	opt := synth.DefaultOptions()
	v := &saxVoice{kind: kind, p: sax.NewPlayer(voice, opt, rand.New(rand.NewSource(1)))}
	v.press(midi, vel)
	mixer.Add(v)
	curSax = v
	return &saxNote{v, midi}
}
