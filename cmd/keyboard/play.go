// ABC playback: the Load button picks a .abc file, the Play button plays it
// with the selected voice through the same mixer as the keys, lighting the keys.
package main

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"go.hasen.dev/shirei"

	"synth/abc"
	synth "synth/audio"
)

var (
	tune    []synth.Note
	playing atomic.Bool
	// playVoice mirrors appData.voice so a running tune follows instrument changes.
	playVoice atomic.Int32
	stop      atomic.Bool

	soundingMu sync.Mutex
	sounding   = map[int]bool{} // midi notes the tune is playing now
)

func tuneSounding(midi int) bool {
	soundingMu.Lock()
	defer soundingMu.Unlock()
	return sounding[midi]
}

func setSounding(midi int, on bool) {
	soundingMu.Lock()
	sounding[midi] = on
	soundingMu.Unlock()
	shirei.RequestNextFrame()
}

func loadABC(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	t, err := abc.Parse(string(src))
	if err != nil {
		return fmt.Errorf("%s: %v", path, err)
	}
	tune = t.Notes(0)
	return nil
}

// togglePlay starts the tune, or stops it if it is playing.
func togglePlay() {
	if playing.Load() {
		stop.Store(true)
		return
	}
	playing.Store(true)
	stop.Store(false)
	go func() {
		defer func() { playing.Store(false); shirei.RequestNextFrame() }()
		for _, n := range tune {
			if stop.Load() {
				return
			}
			d := time.Duration(n.Duration * float64(time.Second))
			if n.Midi == 0 {
				time.Sleep(d)
				continue
			}
			m := int(n.Midi)
			v := makeVoice(VoiceKind(playVoice.Load()), m)
			mixer.Add(v)
			setSounding(m, true)
			time.Sleep(d)
			v.Release()
			setSounding(m, false)
		}
	}()
}
