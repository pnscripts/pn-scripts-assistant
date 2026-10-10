package tools

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

/*
 * The command reaches the system's asker unchanged, and what the asker says
 * comes back. "env" stands in for pkexec here: a real one would put a
 * password dialog in front of whoever is at this machine.
 */
func TestRunAsAdminGoesThroughTheSystemsAsker(t *testing.T) {
	out, err := RunAsAdmin{Elevate: []string{"env", "PN_ELEVATED=yes"}}.Execute(context.Background(),
		json.RawMessage(`{"argv":["sh","-c","echo elevated:$PN_ELEVATED; echo 'a b'"]}`))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "elevated:yes") || !strings.Contains(out, "a b") {
		t.Errorf("got %q", out)
	}
}

func TestRunAsAdminUsesTheSystemsOwnDialog(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux")
	}

	argv, err := RunAsAdmin{}.elevated([]string{"apt-get", "install", "-y", "ffmpeg"})
	if err != nil {
		t.Skip(err)
	}

	if argv[0] != "pkexec" || strings.Join(argv[1:], " ") != "apt-get install -y ffmpeg" {
		t.Errorf("not asked through polkit: %v", argv)
	}

	if strings.Contains(strings.Join(argv, " "), "sudo") {
		t.Error("sudo cannot ask from here")
	}
}
