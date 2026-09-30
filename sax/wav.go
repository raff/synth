package sax

import (
	"encoding/binary"
	"math"
	"os"
)

// WriteWAV writes mono 16-bit PCM at SampleRate.
func WriteWAV(path string, samples []float64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	dataLen := uint32(len(samples) * 2)
	hdr := []any{
		[4]byte{'R', 'I', 'F', 'F'}, 36 + dataLen,
		[4]byte{'W', 'A', 'V', 'E'},
		[4]byte{'f', 'm', 't', ' '}, uint32(16), uint16(1), uint16(1),
		uint32(SampleRate), uint32(SampleRate * 2), uint16(2), uint16(16),
		[4]byte{'d', 'a', 't', 'a'}, dataLen,
	}
	for _, v := range hdr {
		if err := binary.Write(f, binary.LittleEndian, v); err != nil {
			return err
		}
	}
	pcm := make([]int16, len(samples))
	for i, s := range samples {
		pcm[i] = int16(math.Max(-1, math.Min(1, s)) * 32767)
	}
	return binary.Write(f, binary.LittleEndian, pcm)
}
