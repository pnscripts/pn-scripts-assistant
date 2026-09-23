package logs

import (
	"bytes"
	"errors"
	"log"
	"log/slog"
	"os"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/redact"
)

/*
 * A log is the file somebody sends when they ask for help.
 *
 * It is read by other people and attached to messages without being read
 * first, so a key printed by an approved command must not be in it. Written
 * against the real secret shapes rather than a made-up one.
 */
func TestSecretsDoNotReachTheLog(t *testing.T) {
	redact.Add("hunter2-the-mail-password")

	var kept bytes.Buffer

	logger := slog.New(Scrub(slog.NewTextHandler(&kept, nil)))

	logger.Info("ran a command",
		"command", "curl -H 'Authorization: Bearer abcdefghijklmnop12345'",
		"key", "sk-ant-api03-ZZZZYYYYXXXXWWWWVVVVUUUU",
		"mail", "hunter2-the-mail-password",
		"error", errors.New("github token ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123 was refused"),
		"port", 8896)

	// The message itself, too: an error wrapped three deep ends up there.
	logger.Warn("could not reach https://example.test/?access_token=abcdefghijklmnop")

	written := kept.String()

	for _, secret := range []string{
		"abcdefghijklmnop12345",
		"sk-ant-api03-ZZZZYYYYXXXXWWWWVVVVUUUU",
		"hunter2-the-mail-password",
		"ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123",
	} {
		if strings.Contains(written, secret) {
			t.Errorf("the log holds %q:\n%s", secret, written)
		}
	}

	// And it is still a log: what happened is still legible.
	for _, want := range []string{"ran a command", "port=8896", redact.Mark} {
		if !strings.Contains(written, want) {
			t.Errorf("the log lost %q:\n%s", want, written)
		}
	}
}

// Attributes carried on a child logger are scrubbed as well: the value is
// attached once and written on every line after it.
func TestSecretsInACarriedAttributeAreTakenOut(t *testing.T) {
	var kept bytes.Buffer

	logger := slog.New(Scrub(slog.NewTextHandler(&kept, nil))).
		With("url", "https://example.test/hook?token=abcdefghijklmnopqrst")

	logger.Info("starting")

	if strings.Contains(kept.String(), "abcdefghijklmnopqrst") {
		t.Errorf("a carried attribute kept its secret:\n%s", kept.String())
	}
}

/*
 * The standard library's logger goes to the same place.
 *
 * One line in the speech server uses it. Before this it went to the terminal
 * only — not to the file anybody reads afterwards, and past no scrubbing at
 * all.
 */
func TestTheStandardLoggerIsScrubbedToo(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)

	_, where := Open(slog.LevelInfo)

	if where == "" {
		t.Fatal("no log file")
	}

	log.Printf("a webhook failed: https://example.test/?api_key=abcdefghijklmnop")

	written, err := os.ReadFile(where)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(written), "a webhook failed") {
		t.Errorf("the standard logger did not reach the file:\n%s", written)
	}

	if strings.Contains(string(written), "abcdefghijklmnop") {
		t.Errorf("the standard logger kept its secret:\n%s", written)
	}
}
