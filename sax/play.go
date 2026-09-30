package sax

import (
	"bytes"
	"encoding/binary"
	"math"
	"time"

	"github.com/ebitengine/oto/v3"
)

// Play sends mono samples to the default audio device and blocks until done.
func Play(samples []float64) error {
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   SampleRate,
		ChannelCount: 1,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		return err
	}
	<-ready

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
