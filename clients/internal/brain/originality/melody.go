package originality

import (
	"encoding/binary"
	"errors"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

/*
 * The melody check: the notes a piece plays, as the steps between them, and
 * the longest run of steps it shares with a known theme.
 *
 * Steps rather than notes, so a theme played in another key is still the
 * theme; a run of seven steps is eight notes — about as many as anybody needs
 * to hear to name a tune. Runs that only walk up or down a scale, or repeat
 * one note, are not counted: every melody has those.
 */

// MinRun is the fewest shared steps that are reported.
const MinRun = 6

var (
	noteName = regexp.MustCompile(`\b([A-Ga-g])([#b♯♭]?)(-?[0-8])\b`)
	// In code only capitals count: "a1" and "e2" are names of variables.
	codeNote = regexp.MustCompile(`\b([A-G])([#b]?)([0-8])\b`)
)

// Note is a note name like "C#4" or "Bb3" as a MIDI number (C4 = 60).
func Note(name string) (int, bool) {
	m := noteName.FindStringSubmatch(name)
	if m == nil || m[0] != strings.TrimSpace(name) {
		return 0, false
	}

	return noteOf(m[1], m[2], m[3])
}

func noteOf(letter, accidental, octave string) (int, bool) {
	base := map[string]int{"c": 0, "d": 2, "e": 4, "f": 5, "g": 7, "a": 9, "b": 11}[strings.ToLower(letter)]

	switch accidental {
	case "#", "♯":
		base++
	case "b", "♭":
		base--
	}

	o, err := strconv.Atoi(octave)
	if err != nil {
		return 0, false
	}

	return (o+1)*12 + base, true
}

func notesOf(names string) []int {
	var out []int

	for _, f := range strings.Fields(names) {
		if n, ok := Note(f); ok {
			out = append(out, n)
		}
	}

	return out
}

// Steps are the intervals between notes, in semitones.
func Steps(notes []int) []int {
	if len(notes) < 2 {
		return nil
	}

	out := make([]int, len(notes)-1)

	for i := range out {
		out[i] = notes[i+1] - notes[i]
	}

	return out
}

// telling is whether a run of steps could identify a tune: at least three
// different steps, and not only a walk along a scale.
func telling(run []int) bool {
	kinds := map[int]bool{}
	scale := true

	for _, s := range run {
		kinds[s] = true

		if s > 2 || s < -2 {
			scale = false
		}
	}

	if len(kinds) < 3 {
		return false
	}

	if scale {
		// Up and down a scale both: still a walk; one leap or turn is enough
		// to be a tune, and that needs a direction change plus a repeat.
		ups, downs := 0, 0

		for _, s := range run {
			if s > 0 {
				ups++
			} else if s < 0 {
				downs++
			}
		}

		return ups > 0 && downs > 0 && len(kinds) >= 4
	}

	return true
}

// longestShared is the longest telling run of steps both have, and where it
// starts in ours.
func longestShared(ours, theirs []int) (int, int) {
	best, at := 0, -1

	for i := range ours {
		for j := range theirs {
			k := 0
			for i+k < len(ours) && j+k < len(theirs) && ours[i+k] == theirs[j+k] {
				k++
			}

			if k > best && telling(ours[i:i+k]) {
				best, at = k, i
			}
		}
	}

	return best, at
}

// MelodyMatch is a known theme found in a melody.
type MelodyMatch struct {
	Theme  Theme
	Steps  int // shared steps; the notes are one more
	Where  string
	Risk   Risk
	Played []int // our notes in the shared run
}

// MelodyRisk turns a shared run into what to do.
func MelodyRisk(steps int) Risk {
	switch {
	case steps >= 8:
		return High
	case steps >= MinRun:
		return Medium
	}

	return None
}

// CompareMelody finds the known themes in a sequence of notes.
func CompareMelody(where string, notes []int) []MelodyMatch {
	ours := Steps(notes)

	var out []MelodyMatch

	for _, t := range Themes {
		run, at := longestShared(ours, Steps(notesOf(t.Notes)))
		if risk := MelodyRisk(run); risk != None {
			out = append(out, MelodyMatch{Theme: t, Steps: run, Where: where, Risk: risk,
				Played: append([]int(nil), notes[at:at+run+1]...)})
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Steps > out[j].Steps })

	return out
}

var (
	brackets = regexp.MustCompile(`[\[\{(]([^\[\]{}()]*)[\]\})]`)
	number   = regexp.MustCompile(`-?\d+(?:\.\d+)?`)
)

/*
 * NotesInCode finds the melodies written in a program: note names in order
 * ("E4", 'D#5'), lists of MIDI numbers ([64, 62, 60]) and lists of
 * frequencies ([329.63, 293.66]). A list counts when it has at least eight
 * entries and every one is a playable note; a frequency must lie on a note.
 * Each is returned on its own, so two lists never join into a false tune.
 */
func NotesInCode(text string) [][]int {
	var out [][]int

	var named []int

	for _, m := range codeNote.FindAllStringSubmatch(text, -1) {
		if n, ok := noteOf(m[1], m[2], m[3]); ok {
			named = append(named, n)
		}
	}

	if len(named) >= MinRun+1 {
		out = append(out, named)
	}

	for _, m := range brackets.FindAllStringSubmatch(text, -1) {
		values := number.FindAllString(m[1], -1)
		if len(values) < 8 {
			continue
		}

		// Only lists that are nothing but numbers.
		rest := strings.Map(func(r rune) rune {
			if strings.ContainsRune(" ,;\t\n\r", r) {
				return -1
			}

			return r
		}, number.ReplaceAllString(m[1], ""))
		if rest != "" {
			continue
		}

		if notes, ok := asNotes(values); ok {
			out = append(out, notes)
		}
	}

	return out
}

func asNotes(values []string) ([]int, bool) {
	notes := make([]int, 0, len(values))
	allInts, allFreq := true, true

	for _, v := range values {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, false
		}

		if f != math.Trunc(f) || f < 21 || f > 108 {
			allInts = false
		}

		if f < 27.5 || f > 4200 {
			allFreq = false
		} else {
			midi := 69 + 12*math.Log2(f/440)
			if math.Abs(midi-math.Round(midi)) > 0.3 {
				allFreq = false
			}
		}
	}

	for _, v := range values {
		f, _ := strconv.ParseFloat(v, 64)

		switch {
		case allInts:
			notes = append(notes, int(f))
		case allFreq:
			notes = append(notes, int(math.Round(69+12*math.Log2(f/440))))
		default:
			return nil, false
		}
	}

	return notes, true
}

// NotesInMIDI reads a standard MIDI file: one melody per track (the highest
// note at each moment), drums left out.
func NotesInMIDI(data []byte) ([][]int, error) {
	if len(data) < 14 || string(data[:4]) != "MThd" {
		return nil, errors.New("not a MIDI file")
	}

	pos := 8 + int(binary.BigEndian.Uint32(data[4:8]))

	var out [][]int

	for pos+8 <= len(data) {
		kind, size := string(data[pos:pos+4]), int(binary.BigEndian.Uint32(data[pos+4:pos+8]))
		pos += 8

		if pos+size > len(data) {
			return out, errors.New("the MIDI file is cut short")
		}

		if kind == "MTrk" {
			if notes := track(data[pos : pos+size]); len(notes) > MinRun {
				out = append(out, notes)
			}
		}

		pos += size
	}

	return out, nil
}

func track(t []byte) []int {
	type on struct{ at, note int }

	var (
		ons     []on
		now, i  int
		running byte
	)

	vlq := func() int {
		v := 0

		for i < len(t) {
			b := t[i]
			i++
			v = v<<7 | int(b&0x7f)

			if b&0x80 == 0 {
				break
			}
		}

		return v
	}

	for i < len(t) {
		now += vlq()

		if i >= len(t) {
			break
		}

		status := t[i]

		switch {
		case status == 0xff:
			i += 2
			if i > len(t) {
				return nil
			}
			n := vlq()
			i += n

			continue
		case status == 0xf0 || status == 0xf7:
			i++
			n := vlq()
			i += n

			continue
		case status&0x80 != 0:
			running = status
			i++
		}

		data := 2
		if kind := running & 0xf0; kind == 0xc0 || kind == 0xd0 {
			data = 1
		}

		if i+data > len(t) {
			break
		}

		if running&0xf0 == 0x90 && running&0x0f != 9 && t[i+1] > 0 {
			ons = append(ons, on{now, int(t[i])})
		}

		i += data
	}

	sort.SliceStable(ons, func(a, b int) bool { return ons[a].at < ons[b].at })

	var notes []int

	for k := 0; k < len(ons); k++ {
		top := ons[k].note

		for k+1 < len(ons) && ons[k+1].at == ons[k].at {
			k++
			top = max(top, ons[k].note)
		}

		notes = append(notes, top)
	}

	return notes
}

// NoteName is a MIDI number as a name, for reports.
func NoteName(n int) string {
	names := []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}

	return names[((n%12)+12)%12] + strconv.Itoa(n/12-1)
}
