//go:build linux

package lowpriority

import (
	"os/exec"
	"strings"
	"testing"
)

// The child really runs at the lower priority, and its output comes back.
func TestABackgroundChildRunsNiced(t *testing.T) {
	out, err := Output(exec.Command("sh", "-c", "sleep 0.3; cut -d' ' -f19 /proc/$$/stat; echo done"))
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Fields(string(out))
	if len(lines) != 2 || lines[0] != "10" || lines[1] != "done" {
		t.Fatalf("got %q, want the nice value 10 and the output", out)
	}
}

// A failing child is reported like exec.Cmd.Output reports it.
func TestAFailingChildSaysSo(t *testing.T) {
	if _, err := Output(exec.Command("sh", "-c", "exit 3")); err == nil {
		t.Fatal("exit 3 was not an error")
	}
}
