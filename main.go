// Command synth is a test bed for the sax package: it renders a melody with
// one or more saxophone voices to WAV files.
//
//	go run . -voice=tenor,alto
//	go run . -voice=all -out=/tmp/sax -bpm=120
//	go run . -abc=examples/runs.abc -voice=tenor
//	go run . -list
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"synth/abc"
	"synth/sax"
)

func main() {
	voiceFlag := flag.String("voice", "tenor", "comma-separated voice names, or \"all\"")
	list := flag.Bool("list", false, "list available voices and exit")
	out := flag.String("out", ".", "output directory")
	abcFile := flag.String("abc", "", "play this ABC notation file instead of the demo melody")
	bpm := flag.Float64("bpm", 0, "tempo in quarter-note beats per minute (0 = the tune's own, or 100 for the demo)")
	opt := sax.DefaultOptions()
	flag.Float64Var(&opt.Transpose, "transpose", opt.Transpose, "semitones added to every note")
	flag.Float64Var(&opt.Breath, "breath", opt.Breath, "breath noise level")
	flag.Float64Var(&opt.Reverb, "reverb", opt.Reverb, "reverb wet mix 0..1")
	flag.Parse()

	if *list {
		fmt.Println(strings.Join(sax.VoiceNames(), "\n"))
		return
	}

	names := sax.VoiceNames()
	if *voiceFlag != "all" {
		names = strings.Split(*voiceFlag, ",")
	}
	var notes []sax.Note
	if *abcFile != "" {
		src, err := os.ReadFile(*abcFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		tune, err := abc.Parse(string(src))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", *abcFile, err)
			os.Exit(1)
		}
		notes = tune.Notes(*bpm)
	} else {
		b := *bpm
		if b == 0 {
			b = 100
		}
		notes = demoMelody(b)
	}
	for _, name := range names {
		name = strings.TrimSpace(name)
		v, ok := sax.VoiceByName(name)
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown voice %q (try -list)\n", name)
			os.Exit(1)
		}
		path := filepath.Join(*out, "sax_"+v.Name+".wav")
		if err := sax.WriteWAV(path, sax.Render(notes, v, opt)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("wrote", path)
	}
}
