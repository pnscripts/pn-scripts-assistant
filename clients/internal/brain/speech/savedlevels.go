package speech

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

/*
 * Putting right what turning things down used to leave behind.
 *
 * Turning the room down no longer touches anything that is saved (see
 * duck.go). But the old way did, for months, and what it left is still on
 * this machine and on any other that ran it: WirePlumber's saved level for a
 * browser, or for the brain's own espeak voice, at a fifth of a fifth — so
 * that program comes up nearly silent every time it opens a stream, with
 * nothing anywhere to say why. Fixing the cause does not fix that. This does,
 * every time the brain starts.
 *
 * It is also the one place in this program that sets a level WirePlumber
 * saves, on purpose and only to put back what was taken. A test holds it to
 * that.
 */

/*
 * The old way's fingerprint.
 *
 * It read a stream's level from wpctl, which prints two decimals, multiplied
 * by DuckedTo and wrote the result back with three. WirePlumber saves the
 * cube of that. So a saved level it left behind is, once the cube root is
 * taken, a round thousandth no higher than DuckedTo — and the level before it
 * is that divided by DuckedTo. A volume set with a slider is almost never a
 * round thousandth on that scale; one set on purpose this low, by hand, on
 * exactly those terms, is rarer still, and gets the level it had put back
 * rather than something invented.
 */
func leftByOldDucking(saved float64) (was float64, ok bool) {
	if saved <= 0 {
		return 0, false
	}

	level := math.Cbrt(saved)

	if level > DuckedTo+0.0005 {
		return 0, false
	}

	thousandths := level * 1000

	if math.Abs(thousandths-math.Round(thousandths)) > 0.02 {
		return 0, false
	}

	was = math.Round(math.Round(thousandths)/1000/DuckedTo*100) / 100

	if was <= 0 || was > 1 {
		return 0, false
	}

	return was, true
}

// A saved level to put back.
type savedLevel struct {
	// Kind and Value are the key WirePlumber saved it under, which is how a
	// stream opened to put it back is matched to it.
	Kind  string
	Value string

	Channels int
	Was      float64 // on wpctl's scale, what it had before it was turned down
	Now      float64 // on wpctl's scale, what it is saved at
}

/*
 * The keys a stream can be given from outside.
 *
 * WirePlumber keys a stream by the first of media.role, application.id,
 * application.name, media.name and node.name it carries. The stream opened to
 * put a level back can set the first three and carries none of the ones ahead
 * of them, so it lands on the same key; the last two it cannot choose, and a
 * level saved under those is left alone rather than guessed at.
 */
var puttableKinds = map[string]bool{
	"media.role":       true,
	"application.id":   true,
	"application.name": true,
}

// damagedSavedLevels reads WirePlumber's restore-stream state and returns the
// output levels the old ducking left behind.
func damagedSavedLevels(state string) []savedLevel {
	var out []savedLevel

	for _, line := range strings.Split(state, "\n") {
		key, values, found := strings.Cut(strings.TrimSpace(line), "=")

		if !found || !strings.HasPrefix(key, "Output/Audio:") || !strings.HasSuffix(key, ":channelVolumes") {
			continue
		}

		rest := strings.TrimSuffix(strings.TrimPrefix(key, "Output/Audio:"), ":channelVolumes")
		kind, escaped, found := strings.Cut(rest, ":")

		if !found || !puttableKinds[kind] {
			continue
		}

		var levels []float64

		for _, v := range strings.Split(strings.TrimSuffix(values, ";"), ";") {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				levels = nil

				break
			}

			levels = append(levels, f)
		}

		if len(levels) == 0 || !allEqual(levels) {
			continue
		}

		was, ok := leftByOldDucking(levels[0])
		if !ok {
			continue
		}

		out = append(out, savedLevel{
			Kind:     kind,
			Value:    unescapeStateKey(escaped),
			Channels: len(levels),
			Was:      was,
			Now:      math.Cbrt(levels[0]),
		})
	}

	return out
}

func allEqual(levels []float64) bool {
	for _, v := range levels[1:] {
		if math.Abs(v-levels[0]) > 1e-6 {
			return false
		}
	}

	return true
}

// unescapeStateKey undoes WirePlumber's escaping of state keys: a space is
// written \s, "=" \e, "[" \o, "]" \c and a backslash \\.
func unescapeStateKey(s string) string {
	var b strings.Builder

	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])

			continue
		}

		i++

		switch s[i] {
		case 's':
			b.WriteByte(' ')
		case 'e':
			b.WriteByte('=')
		case 'o':
			b.WriteByte('[')
		case 'c':
			b.WriteByte(']')
		default:
			b.WriteByte(s[i])
		}
	}

	return b.String()
}

// wireplumberState is where WirePlumber keeps the levels it restores.
func wireplumberState() string {
	base := os.Getenv("XDG_STATE_HOME")

	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}

		base = filepath.Join(home, ".local", "state")
	}

	return filepath.Join(base, "wireplumber", "restore-stream")
}

/*
 * leftAtAFifth is a stream this program turned down and never got to put
 * back — which, now, only a crash in the middle of a sentence can leave. It
 * lasts until that stream ends, and this ends it sooner.
 */
func leftAtAFifth(s stream) bool {
	if len(s.Volumes) == 0 || len(s.Gain) != len(s.Volumes) {
		return false
	}

	for i, v := range s.Volumes {
		if v <= 0 || math.Abs(s.Gain[i]-v*DuckedTo) > 0.01*math.Max(v, 0.01) {
			return false
		}
	}

	return true
}

/*
 * The machine, as functions, so the tests are not run against this one.
 *
 * Every test of this touched the real sound of whoever ran the suite once
 * already; these are what they replace.
 */
var (
	readSavedLevels = func() string {
		raw, err := os.ReadFile(wireplumberState())
		if err != nil {
			return ""
		}

		return string(raw)
	}

	putSavedLevelBack = openAndSetLevel
)

/*
 * PutBackAnythingLeftDown runs when the brain starts, and says what it put
 * back so that can be logged. Silent when there is nothing to do, which is
 * almost always.
 */
func PutBackAnythingLeftDown() []string {
	var said []string

	// The note the old way kept, which described levels it could not finish
	// putting back and was wrong as often as it was right. What it was for is
	// done below, from what WirePlumber actually saved.
	if home, err := os.UserHomeDir(); err == nil {
		os.Remove(filepath.Join(home, ".pn-brain", "turned-down.json"))

		// And the folder it was kept in, which held nothing else that is still
		// used — removed only when that leaves nothing in it.
		os.Remove(filepath.Join(home, ".pn-brain"))
	}

	if !haveGainControl() {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for _, s := range streamsNow(ctx) {
		if leftAtAFifth(s) && putGain(ctx, s, s.Volumes) {
			said = append(said, fmt.Sprintf("%s was still turned down from last time; put back", s.Name))
		}
	}

	for _, d := range damagedSavedLevels(readSavedLevels()) {
		if err := putSavedLevelBack(ctx, d); err != nil {
			said = append(said, fmt.Sprintf("%s is saved at %.0f%% and could not be put back: %v",
				d.Value, d.Now*100, err))

			continue
		}

		said = append(said, fmt.Sprintf("%s was saved at %.0f%% by the old way of turning the room down; put back to %.0f%%",
			d.Value, d.Now*100, d.Was*100))
	}

	return said
}

/*
 * openAndSetLevel opens a silent stream carrying the saved key and sets its
 * level, which WirePlumber then saves in place of the damaged one.
 *
 * There is no other way to reach a saved level: WirePlumber keeps its state in
 * memory and writes over the file, so editing the file does nothing, and it
 * only restores and saves a level for a stream that exists. So one is made —
 * silence, through aplay, which unlike pw-play carries no media role of its own
 * and so is saved under exactly the key given to it.
 */
func openAndSetLevel(ctx context.Context, d savedLevel) error {
	if _, err := exec.LookPath("aplay"); err != nil {
		return fmt.Errorf("aplay is not installed")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "aplay", "-q", "-D", "pipewire", "-")
	cmd.Stdin = bytes.NewReader(silentWAV(d.Channels, 22050, 2*time.Second))
	cmd.Env = append(os.Environ(), "PIPEWIRE_PROPS="+streamProps(d))

	dieWithParent(cmd)

	if err := cmd.Start(); err != nil {
		return err
	}

	defer cmd.Wait()

	pid := cmd.Process.Pid

	for tries := 0; tries < 15; tries++ {
		time.Sleep(100 * time.Millisecond)

		raw, err := exec.CommandContext(ctx, "pw-dump").Output()
		if err != nil {
			continue
		}

		for _, s := range readStreams(raw) {
			if s.Process != pid {
				continue
			}

			if err := exec.CommandContext(ctx, "wpctl", "set-volume", strconv.Itoa(s.ID),
				strconv.FormatFloat(d.Was, 'f', 2, 64)).Run(); err != nil {
				return err
			}

			// WirePlumber writes its state a second after the last change; the
			// silence lasts longer than that, so the stream is still there when
			// it does.
			return nil
		}
	}

	return fmt.Errorf("the stream to set it on never appeared")
}

// streamProps is the PIPEWIRE_PROPS the silent stream is opened with.
func streamProps(d savedLevel) string {
	value := strings.ReplaceAll(strings.ReplaceAll(d.Value, `\`, `\\`), `"`, `\"`)

	return fmt.Sprintf(`{ %s = "%s" }`, d.Kind, value)
}

// silentWAV is a WAV file of silence.
func silentWAV(channels, rate int, length time.Duration) []byte {
	if channels < 1 {
		channels = 1
	}

	frames := int(float64(rate) * length.Seconds())
	data := frames * channels * 2

	var b bytes.Buffer

	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(36+data))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, uint32(16))
	binary.Write(&b, binary.LittleEndian, uint16(1))
	binary.Write(&b, binary.LittleEndian, uint16(channels))
	binary.Write(&b, binary.LittleEndian, uint32(rate))
	binary.Write(&b, binary.LittleEndian, uint32(rate*channels*2))
	binary.Write(&b, binary.LittleEndian, uint16(channels*2))
	binary.Write(&b, binary.LittleEndian, uint16(16))
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(data))
	b.Write(make([]byte, data))

	return b.Bytes()
}
