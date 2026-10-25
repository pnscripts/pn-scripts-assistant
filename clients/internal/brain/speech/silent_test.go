package speech

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * No test may make a sound. Every speaking program a test could reach is
 * replaced on PATH by one that leaves a mark; speaking through every entry
 * point must leave none, and what would have been said is still known.
 */
func TestATestBinaryNeverReachesTheSpeakers(t *testing.T) {
	if !Silent() {
		t.Fatal("a test binary is not silent")
	}

	dir := t.TempDir()
	mark := filepath.Join(dir, "played")

	for _, name := range []string{"spd-say", "espeak-ng", "espeak", "piper", "pw-play", "aplay", "paplay",
		"pw-record", "arecord", "wpctl", "pw-cli", "systemctl"} {
		script := "#!/bin/sh\necho " + name + " >> " + mark + "\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("PATH", dir)

	ctx := context.Background()

	_ = Speak(ctx, "Добър ден, тест едно.")
	_ = SpeakAndWait(ctx, "Добър ден, тест две.")
	_ = (&Piper{}).Speak(ctx, "Добър ден, тест три.")
	_ = Record(ctx, 1, "", filepath.Join(dir, "x.wav"))
	_ = recordPipeWire(ctx, 1, "", filepath.Join(dir, "y.wav"))
	_ = reloadAudio(ctx)
	moveEverythingPlaying(ctx)
	_ = openAndSetLevel(ctx, savedLevel{Channels: 2})

	if raw, err := os.ReadFile(mark); err == nil {
		t.Fatalf("a test reached a real sound program: %s", strings.TrimSpace(string(raw)))
	}

	said := strings.Join(Silenced(), " | ")
	for _, want := range []string{"тест едно", "тест две", "тест три"} {
		if !strings.Contains(said, want) {
			t.Errorf("%q was not kept as what would have been said: %s", want, said)
		}
	}
}

// Any process can be made silent: previews and harnesses set this.
func TestTheSilentSwitchWorksOutsideTests(t *testing.T) {
	t.Setenv("PN_TEST_REAL_AUDIO", "1")

	if Silent() {
		t.Skip("PN_SCRIPTS_ASSISTANT_SILENT is set for this run, which is the point")
	}

	t.Setenv("PN_SCRIPTS_ASSISTANT_SILENT", "1")

	if !Silent() {
		t.Error("PN_SCRIPTS_ASSISTANT_SILENT=1 did not silence the process")
	}
}
