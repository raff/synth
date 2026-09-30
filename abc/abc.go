// Package abc parses a melody-only subset of ABC notation into audio.Notes.
//
// Supported: header fields X T M L Q K (K ends the header; the key signature
// and modes are honoured), notes with accidentals (^ ^^ _ __ =), octave marks
// (' and ,) and lengths (2, /2, 3/2, //), rests (z, x, Z), bar lines,
// ties (-), broken rhythm (> <), triplets and other tuplets ((3), simple
// repeats (|: :|) with first/second endings ([1 [2), dynamics (!p! !mf! !f! ...),
// inline [K:] [L:] [M:] [Q:] fields, and line continuation. Comments (%),
// chord symbols ("Am"), grace notes ({...}), slurs and other decorations are
// skipped. Chords ([CEG]) and multiple voices are not supported and give an error.
//
// Pitch follows the ABC standard: C is middle C (MIDI 60), c is an octave up.
package abc

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"synth/audio"
)

// DefaultVelocity is used until a dynamics decoration changes it.
const DefaultVelocity = 0.75

// Event is one note or rest; Duration is in whole notes (1/4 = a quarter note).
type Event struct {
	Midi     float64 // 0 = rest
	Duration float64
	Velocity float64
}

// Tune is a parsed melody.
type Tune struct {
	Title  string
	Events []Event
	// WholeNote is the length of a whole note in seconds at the tune's own tempo.
	WholeNote float64
}

// Notes converts the tune to audio.Notes. quarterBPM overrides the tempo of the
// tune if > 0 (beats per minute, counting quarter notes).
func (t *Tune) Notes(quarterBPM float64) []audio.Note {
	whole := t.WholeNote
	if quarterBPM > 0 {
		whole = 4 * 60 / quarterBPM
	}
	notes := make([]audio.Note, len(t.Events))
	for i, e := range t.Events {
		notes[i] = audio.Note{Midi: e.Midi, Duration: e.Duration * whole, Velocity: e.Velocity}
	}
	return notes
}

var dynamics = map[string]float64{
	"ppp": 0.30, "pp": 0.40, "p": 0.55, "mp": 0.65, "mf": 0.75, "f": 0.85, "ff": 0.95, "fff": 1.0,
}

var noteSemis = map[byte]int{'C': 0, 'D': 2, 'E': 4, 'F': 5, 'G': 7, 'A': 9, 'B': 11}

// fifths is the position of each letter on the line of fifths (F = -1 ... B = 5).
var fifths = map[byte]int{'F': -1, 'C': 0, 'G': 1, 'D': 2, 'A': 3, 'E': 4, 'B': 5}

var modeOffset = map[string]int{
	"maj": 0, "ion": 0, "lyd": 1, "mix": -1, "dor": -2, "min": -3, "m": -3, "aeo": -3, "phr": -4, "loc": -5,
}

type parser struct {
	tune      Tune
	line      int
	unit      float64 // default note length, in whole notes
	meter     float64 // bar length in whole notes
	wholeSecs float64
	keyAcc    map[byte]int // key signature: letter -> semitone offset
	barAcc    map[string]int
	velocity  float64

	out        []Event
	repStart   int
	end1Start  int  // index in out where the current first ending starts, or -1
	lastTied   bool // a tie (-) follows the last emitted note
	brokenNext float64
	tuplet     struct {
		left  int
		ratio float64
	}
}

var (
	reFieldLine = regexp.MustCompile(`^([A-Za-z]):\s*(.*)$`)
	reTempo     = regexp.MustCompile(`(\d+)\s*/\s*(\d+)\s*=\s*(\d+(?:\.\d+)?)`)
	reFrac      = regexp.MustCompile(`^(\d+)\s*/\s*(\d+)$`)
)

// Parse reads the first tune in src.
func Parse(src string) (*Tune, error) {
	p := &parser{wholeSecs: 2, velocity: DefaultVelocity, end1Start: -1, barAcc: map[string]int{}}
	p.setKey("C")
	haveBody := false
	haveL := false
	haveT := false
	for i, raw := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		p.line = i + 1
		line := strings.TrimSpace(stripComment(raw))
		if line == "" {
			if haveBody && strings.TrimSpace(raw) == "" {
				break // blank line ends the tune (a comment-only line does not)
			}
			continue
		}
		if m := reFieldLine.FindStringSubmatch(line); m != nil {
			f, val := m[1][0], strings.TrimSpace(m[2])
			if f == 'X' && haveBody {
				break
			}
			if f == 'L' {
				haveL = true
			}
			if err := p.field(f, val, &haveBody, &haveT); err != nil {
				return nil, err
			}
			continue
		}
		if !haveBody {
			continue // text before K: that isn't a field
		}
		if !haveL && p.unit == 0 {
			p.unit = p.defaultUnit()
		}
		if err := p.body(line); err != nil {
			return nil, err
		}
	}
	p.tune.Events = p.out
	p.tune.WholeNote = p.wholeSecs
	return &p.tune, nil
}

func stripComment(s string) string {
	if i := strings.IndexByte(s, '%'); i >= 0 {
		return s[:i]
	}
	return s
}

func (p *parser) errf(format string, a ...any) error {
	return fmt.Errorf("abc: line %d: %s", p.line, fmt.Sprintf(format, a...))
}

func (p *parser) defaultUnit() float64 {
	if p.meter > 0 && p.meter < 0.75 {
		return 1.0 / 16
	}
	return 1.0 / 8
}

func (p *parser) field(f byte, val string, haveBody, haveT *bool) error {
	switch f {
	case 'T':
		if !*haveT {
			p.tune.Title, *haveT = val, true
		}
	case 'M':
		switch val {
		case "C":
			p.meter = 1
		case "C|":
			p.meter = 1
		case "none":
			p.meter = 0
		default:
			m := reFrac.FindStringSubmatch(val)
			if m == nil {
				return p.errf("bad meter %q", val)
			}
			a, _ := strconv.Atoi(m[1])
			b, _ := strconv.Atoi(m[2])
			if b == 0 {
				return p.errf("bad meter %q", val)
			}
			p.meter = float64(a) / float64(b)
		}
	case 'L':
		m := reFrac.FindStringSubmatch(val)
		if m == nil {
			return p.errf("bad unit note length %q", val)
		}
		a, _ := strconv.Atoi(m[1])
		b, _ := strconv.Atoi(m[2])
		if a == 0 || b == 0 {
			return p.errf("bad unit note length %q", val)
		}
		p.unit = float64(a) / float64(b)
	case 'Q':
		return p.setTempo(val)
	case 'K':
		if err := p.setKey(val); err != nil {
			return p.errf("%v", err)
		}
		if !*haveBody && p.unit == 0 {
			p.unit = p.defaultUnit()
		}
		*haveBody = true
	}
	return nil
}

func (p *parser) setTempo(val string) error {
	if m := reTempo.FindStringSubmatch(val); m != nil {
		a, _ := strconv.ParseFloat(m[1], 64)
		b, _ := strconv.ParseFloat(m[2], 64)
		n, _ := strconv.ParseFloat(m[3], 64)
		if a == 0 || b == 0 || n == 0 {
			return p.errf("bad tempo %q", val)
		}
		p.wholeSecs = (b / a) * 60 / n
		return nil
	}
	if n, err := strconv.ParseFloat(strings.Trim(val, `" `), 64); err == nil && n > 0 {
		p.wholeSecs = 4 * 60 / n
		return nil
	}
	return nil // text-only tempo ("Allegro"): keep the current one
}

func (p *parser) setKey(val string) error {
	p.keyAcc = map[byte]int{}
	v := strings.TrimSpace(val)
	if v == "" || strings.HasPrefix(strings.ToLower(v), "none") || strings.HasPrefix(v, "Hp") || strings.HasPrefix(v, "HP") {
		return nil
	}
	tonic := strings.ToUpper(v[:1])[0]
	pos, ok := fifths[tonic]
	if !ok {
		return fmt.Errorf("bad key %q", val)
	}
	rest := v[1:]
	if strings.HasPrefix(rest, "#") {
		pos += 7
		rest = rest[1:]
	} else if strings.HasPrefix(rest, "b") {
		pos -= 7
		rest = rest[1:]
	}
	rest = strings.ToLower(strings.TrimSpace(rest))
	if f := strings.Fields(rest); len(f) > 0 {
		rest = f[0]
	}
	mode := "maj"
	if rest != "" {
		if strings.HasPrefix(rest, "m") && !strings.HasPrefix(rest, "maj") && !strings.HasPrefix(rest, "mix") {
			mode = "m"
		} else if len(rest) >= 3 {
			mode = rest[:3]
		}
	}
	off, ok := modeOffset[mode]
	if !ok {
		return fmt.Errorf("unknown mode in key %q", val)
	}
	n := pos + off
	sharps := "FCGDAEB"
	flats := "BEADGCF"
	for i := 0; i < n && i < 7; i++ {
		p.keyAcc[sharps[i]] = 1
	}
	for i := 0; i < -n && i < 7; i++ {
		p.keyAcc[flats[i]] = -1
	}
	return nil
}

// body parses one line of music.
func (p *parser) body(s string) error {
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\\':
			i++
		case c == '"': // chord symbol / annotation
			j := strings.IndexByte(s[i+1:], '"')
			if j < 0 {
				return p.errf("unterminated \"")
			}
			i += j + 2
		case c == '!' || c == '+':
			j := strings.IndexByte(s[i+1:], c)
			if j < 0 {
				return p.errf("unterminated %c", c)
			}
			name := s[i+1 : i+1+j]
			if v, ok := dynamics[name]; ok {
				p.velocity = v
			}
			i += j + 2
		case c == '{': // grace notes
			j := strings.IndexByte(s[i:], '}')
			if j < 0 {
				return p.errf("unterminated {")
			}
			i += j + 1
		case c == '|' || c == ':' || c == '[' && i+1 < len(s) && (s[i+1] == '|' || s[i+1] >= '0' && s[i+1] <= '9'):
			n, err := p.bar(s[i:])
			if err != nil {
				return err
			}
			i += n
		case c == '[':
			n, err := p.inlineField(s[i:])
			if err != nil {
				return err
			}
			i += n
		case c == '(':
			if i+1 < len(s) && s[i+1] >= '2' && s[i+1] <= '9' {
				n, err := p.tupletStart(s[i+1:])
				if err != nil {
					return err
				}
				i += 1 + n
			} else {
				i++ // slur
			}
		case c == ')' || c == '~' || c == '.' || c == 'H' || c == 'L' || c == 'M' || c == 'O' ||
			c == 'P' || c == 'S' || c == 'T' || c == 'u' || c == 'v':
			i++
		case c == '>' || c == '<':
			j := i
			for j < len(s) && s[j] == c {
				j++
			}
			if err := p.broken(c, j-i); err != nil {
				return err
			}
			i = j
		case c == '-':
			p.lastTied = len(p.out) > 0 && p.out[len(p.out)-1].Midi != 0
			i++
		case c == 'z' || c == 'x' || c == 'Z':
			n, err := p.note(s[i:], true)
			if err != nil {
				return err
			}
			i += n
		case c == '^' || c == '_' || c == '=' || c >= 'A' && c <= 'G' || c >= 'a' && c <= 'g':
			n, err := p.note(s[i:], false)
			if err != nil {
				return err
			}
			i += n
		default:
			return p.errf("unsupported character %q", string(c))
		}
	}
	return nil
}

// length parses a length multiplier such as "", "2", "/", "/4", "3/2".
func length(s string) (float64, int) {
	i := 0
	num := 1.0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > 0 {
		num, _ = strconv.ParseFloat(s[:i], 64)
	}
	den := 1.0
	for i < len(s) && s[i] == '/' {
		i++
		j := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i > j {
			d, _ := strconv.ParseFloat(s[j:i], 64)
			den *= d
		} else {
			den *= 2
		}
	}
	return num / den, i
}

func (p *parser) note(s string, rest bool) (int, error) {
	i := 0
	var midi float64
	restKind := byte(0)
	if rest {
		restKind = s[0]
		i = 1
	} else {
		acc, hasAcc := 0, false
		switch {
		case strings.HasPrefix(s, "^^"):
			acc, hasAcc, i = 2, true, 2
		case strings.HasPrefix(s, "__"):
			acc, hasAcc, i = -2, true, 2
		case s[0] == '^':
			acc, hasAcc, i = 1, true, 1
		case s[0] == '_':
			acc, hasAcc, i = -1, true, 1
		case s[0] == '=':
			acc, hasAcc, i = 0, true, 1
		}
		if i >= len(s) || !(s[i] >= 'A' && s[i] <= 'G' || s[i] >= 'a' && s[i] <= 'g') {
			return 0, p.errf("accidental without a note")
		}
		letter := s[i]
		i++
		up := strings.ToUpper(string(letter))[0]
		octave := 4
		if letter >= 'a' {
			octave = 5
		}
		for i < len(s) && (s[i] == '\'' || s[i] == ',') {
			if s[i] == '\'' {
				octave++
			} else {
				octave--
			}
			i++
		}
		key := fmt.Sprintf("%c%d", up, octave)
		if hasAcc {
			p.barAcc[key] = acc
		} else if a, ok := p.barAcc[key]; ok {
			acc = a
		} else {
			acc = p.keyAcc[up]
		}
		midi = float64(12*(octave+1) + noteSemis[up] + acc)
	}
	mult, n := length(s[i:])
	i += n
	dur := mult * p.unit
	if restKind == 'Z' {
		if p.meter == 0 {
			return 0, p.errf("Z needs a meter")
		}
		dur = mult * p.meter
	}
	dur = p.applyTiming(dur)
	ev := Event{Midi: midi, Duration: dur, Velocity: p.velocity}
	if !rest && p.lastTied && len(p.out) > 0 && p.out[len(p.out)-1].Midi == midi {
		p.out[len(p.out)-1].Duration += dur
	} else {
		p.out = append(p.out, ev)
	}
	p.lastTied = false
	return i, nil
}

// applyTiming applies a pending broken-rhythm factor and tuplet ratio.
func (p *parser) applyTiming(d float64) float64 {
	if p.brokenNext != 0 {
		d *= p.brokenNext
		p.brokenNext = 0
	}
	if p.tuplet.left > 0 {
		d *= p.tuplet.ratio
		p.tuplet.left--
	}
	return d
}

func (p *parser) broken(c byte, n int) error {
	if len(p.out) == 0 {
		return p.errf("broken rhythm with no previous note")
	}
	short := 1.0
	for k := 0; k < n; k++ {
		short /= 2
	}
	long := 2 - short
	prev, next := long, short
	if c == '<' {
		prev, next = short, long
	}
	p.out[len(p.out)-1].Duration *= prev
	p.brokenNext = next
	return nil
}

var tupletQ = map[int]int{2: 3, 3: 2, 4: 3, 5: 2, 6: 2, 7: 2, 8: 3, 9: 2}

// tupletStart parses "(p", "(p:q" or "(p:q:r" after the '('. s begins at p.
func (p *parser) tupletStart(s string) (int, error) {
	parts := []int{int(s[0] - '0')}
	i := 1
	for len(parts) < 3 && i+1 < len(s) && s[i] == ':' && s[i+1] >= '0' && s[i+1] <= '9' {
		parts = append(parts, int(s[i+1]-'0'))
		i += 2
	}
	n := parts[0]
	q := tupletQ[n]
	if len(parts) > 1 {
		q = parts[1]
	}
	r := n
	if len(parts) > 2 {
		r = parts[2]
	}
	p.tuplet.left = r
	p.tuplet.ratio = float64(q) / float64(n)
	return i, nil
}

func (p *parser) inlineField(s string) (int, error) {
	j := strings.IndexByte(s, ']')
	if j < 0 || len(s) < 3 || s[2] != ':' {
		return 0, p.errf("chords are not supported (found %q)", firstN(s, 8))
	}
	var haveBody, haveT bool = true, true
	if err := p.field(s[1], strings.TrimSpace(s[3:j]), &haveBody, &haveT); err != nil {
		return 0, err
	}
	return j + 1, nil
}

func firstN(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// bar handles bar lines, repeat signs and ending markers starting at s[0].
// It returns the number of bytes consumed.
func (p *parser) bar(s string) (int, error) {
	i := 0
	for i < len(s) && strings.IndexByte(":|[]", s[i]) >= 0 {
		if s[i] == '[' && i > 0 {
			break // "[" after a bar token starts an ending or inline field
		}
		i++
	}
	tok := s[:i]
	hasBar := strings.Contains(tok, "|") || tok == "::"
	repeatEnd := hasBar && strings.HasPrefix(tok, ":")
	repeatStart := hasBar && strings.HasSuffix(tok, ":")
	// ending marker: "1", "2" directly after the bar token
	ending := 0
	if i < len(s) && s[i] >= '1' && s[i] <= '9' {
		ending = int(s[i] - '0')
		i++
	}
	p.barAcc = map[string]int{}
	if repeatEnd {
		p.repeat()
	}
	if repeatStart {
		p.repStart = len(p.out)
		p.end1Start = -1
	}
	if ending == 1 {
		p.end1Start = len(p.out)
	}
	if i == 0 {
		i = 1
	}
	return i, nil
}

// repeat replays the section since the last repeat start, skipping a first ending.
func (p *parser) repeat() {
	end := len(p.out)
	if p.end1Start >= 0 {
		end = p.end1Start
	}
	sec := append([]Event(nil), p.out[p.repStart:end]...)
	p.out = append(p.out, sec...)
	p.repStart = len(p.out)
	p.end1Start = -1
	p.lastTied = false
}
