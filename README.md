# synth

A saxophone synthesizer in Go. Goal: a basic audio synthesizer (stdlib only) that sounds as close as possible to a saxophone, usable as a library in other applications. `main` is a test bed.

## Layout

- `audio/` — shared by all instruments: `Note`, `Options`, `SampleRate`, `MidiToHz`, `Reverb`/`Finish` (reverb + normalise), `WriteWAV`, `Play`.
- `sax/` — the sax synth (import `synth/sax`). `voice.go` voices + lookup, `synth.go` `Voice.RenderNote` (one note, for per-note/real-time use) and `Render` (sequence + reverb + normalise).
- `piano/` — additive piano (`piano.Render(notes, opts)`, `-piano` flag): inharmonic partials, 3 detuned strings, two-stage decay.
- `abc/` — melody-only ABC notation parser (`abc.Parse(src)` → `Tune.Notes(bpm)` → `[]audio.Note`). C = middle C (MIDI 60). Supports K/L/M/Q/T, accidentals, key signatures & modes, ties, broken rhythm, tuplets, repeats with 1st/2nd endings, dynamics `!p!..!ff!`, inline fields; errors (with line number) on chords/unknown characters. Tests in `abc/abc_test.go`.
- `examples/runs.abc` — four rising runs, each ending on a held note.
- `examples/scale.abc` — a C major scale, a quick smoke test.
- `cmd/keyboard/` — playable one-row keyboard GUI (own module, uses [shirei](https://go.hasen.dev/shirei)): piano and tenor/alto/soprano sax voices, play with mouse/touch/computer keys, or **Load** an ABC file and **Play** it (keys light up). `cd cmd/keyboard && go run .`
- `main.go` — test bed CLI: parses an ABC file and renders it with the chosen voices.

## Run it

```
go run . -abc=examples/scale.abc -voice=tenor,alto   # writes sax_tenor.wav, sax_alto.wav
go run . -abc=tune.abc -voice=all -out=/tmp -bpm=120  # -bpm overrides the tune's tempo
go run . -list                        # voices
go run . -abc=tune.abc -voice=tenor -transpose=-12 -breath=1.5 -reverb=0.3
afplay sax_tenor.wav
```

Output is 16-bit mono, 44.1 kHz. 
## How it works

- Additive synthesis: up to 40 harmonics with 1/n amplitudes (sawtooth-like reed), even partials x0.8.
- Body resonances ("formants"): fixed peaks in the `voices` table, applied to each harmonic by `bodyGain`. Biggest lever on timbre.
- Brightness follows the envelope (more upper harmonics when louder).
- Vibrato (starts ~0.25 s in, ramps over 0.4 s), pitch scoop at the attack (~35 cents flat, settles in ~30 ms).
- Breath noise: low-passed white noise, extra puff at the onset.
- Small Schroeder reverb (4 combs + 2 allpasses), then normalised to 0.7 peak.

## Current settings (what we settled on)

- Voice: **bd2** = "bright but darker": formants `{300,250,1.0} {600,350,0.9} {1800,700,0.9} {2800,900,0.3}`, floor 0.06. Picked over `bright`, `bd1`, `bd3`.
- Envelope (`adsr`): attack 0.05, decay 0.25, sustain 0.94, release 0.12. Earlier version sounded percussive: fast attack, big drop to sustain, and envelope applied twice to the breath noise. All fixed.
- `breathLevel = 1.0` (was 0.5; user wanted more breath). Can go 1.5-2.0 for airier.
- `-transpose=0` by default. ABC pitch is taken literally (C = middle C).
- No built-in melody any more; tunes come from ABC files.
- Voices selected with `-voice`. Other voices (`base`, `bright`, `dark`, `nasal`, `honky`, `buzzy`, `bd1`, `bd3`) are still in the table.

## Real-sax analysis (2026-09-30)

- Analysed `~/Downloads/{alto,tenor,bari}_sax.mp3` (steady-note harmonic levels; tenor is the solid data set, alto only 13 notes).
- Tenor spectrum is explained by one envelope vs **absolute frequency** (harmonic number adds almost nothing). New voice `tenor` in `main.go` uses that measured curve (`Voice.curve`) instead of 1/n + formants. `soprano` is shifted +12 semitones into its own range (`Voice.Shift`) for comparison.
- Fixes the thin high register of `bd2` (2nd harmonic was ~-12 dB, real ~-4 dB). Baritone has much more 1.3-2.4 kHz energy, so a bari voice would need its own curve.

## Learned

- The `floor` parameter barely changes the sound ("buzzy" variant wasn't buzzy). The buzz comes from strong 1.8-2.8 kHz peaks.
- Attack length wasn't the only cause of a percussive feel; the post-peak decay and onset noise burst mattered more.

## Ideas for next time

1. Write real tunes in ABC (e.g. "Autumn Leaves"). ABC todo: chords, slurs → legato, MIDI file reader, multiple voices.
2. Humanise: small timing/pitch variation, tongued onsets on repeated notes, fall-off at end of long notes.
3. Make formants shift slightly with pitch so low and high octaves sound like the same instrument.
4. Second tune: "In a Sentimental Mood".
5. Real-time playback (e.g. with `oto`) and keyboard/MIDI input.
6. Further options: shorter/softer pitch scoop, slower brightness opening, global spectral tilt for darker tone, breath noise colour (cutoff).
7. Longer term: physical model (waveguide + reed nonlinearity), or tune harmonics against a real sax spectrum.
