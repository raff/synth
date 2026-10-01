package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

var (
	otoOnce sync.Once
	otoCtx  *oto.Context
	otoErr  error
)

// context returns the process-wide oto context; oto allows only one.
func context() (*oto.Context, error) {
	otoOnce.Do(func() {
		var ready chan struct{}
		otoCtx, ready, otoErr = oto.NewContext(&oto.NewContextOptions{
			SampleRate:   SampleRate,
			ChannelCount: 1,
			Format:       oto.FormatSignedInt16LE,
		})
		if otoErr == nil {
			<-ready
		}
	})
	return otoCtx, otoErr
}

// Play sends mono samples to the default audio device and blocks until done.
func Play(samples []float64) error {
	ctx, err := context()
	if err != nil {
		return err
	}

	pcm := make([]int16, len(samples))
	for i, s := range samples {
		pcm[i] = int16(math.Max(-1, math.Min(1, s)) * 32767)
	}
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, pcm)

	p := ctx.NewPlayer(&buf)
	p.Play()
	for p.IsPlaying() {
		time.Sleep(20 * time.Millisecond)
	}
	return p.Close()
}
