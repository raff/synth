// Command synth is a test bed for the sax package: it renders an ABC melody with
// one or more saxophone voices to WAV files.
//
//	go run . -abc=examples/scale.abc -voice=tenor,alto
//	go run . -abc=examples/runs.abc -voice=all -out=/tmp/sax -bpm=120
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
	abcFile := flag.String("abc", "", "ABC notation file to play (required)")
	bpm := flag.Float64("bpm", 0, "tempo in quarter-note beats per minute (0 = the tune's own)")
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
	if *abcFile == "" {
		fmt.Fprintln(os.Stderr, "missing -abc=<file> (see examples/)")
		os.Exit(2)
	}
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
	notes := tune.Notes(*bpm)
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
