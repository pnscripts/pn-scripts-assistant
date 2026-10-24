package tools

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

type payingTool struct{ RunCommand }

func (payingTool) Name() string                  { return "buy_credits" }
func (payingTool) Spends(json.RawMessage) string { return "10 € of credits" }

// In goal mode only four things are asked; everything else goes ahead.
func TestGoalModeAsksOnlyTheFourThings(t *testing.T) {
	goal := "/home/ana/brain/goals/7-space-game"
	inside := func(p string) bool { return p == goal || strings.HasPrefix(filepath.Clean(p), goal+"/") }

	cmd := func(dir string, argv ...string) json.RawMessage {
		raw, _ := json.Marshal(commandArgs{Argv: argv, Dir: dir})

		return raw
	}

	cases := []struct {
		name string
		tool Tool
		args json.RawMessage
		want string
	}{
		{"pay", payingTool{}, nil, AskPaying},
		{"send mail", SendEmail{}, json.RawMessage(`{}`), AskSending},
		{"sudo", RunCommand{}, cmd(goal, "sudo", "apt", "install", "x"), AskSystemWide},
		{"stop a service", RunCommand{}, cmd(goal, "systemctl", "stop", "nginx"), AskSystemWide},
		{"run as admin", RunAsAdmin{}, nil, AskSystemWide},
		{"delete outside", RunCommand{}, cmd(goal, "rm", "-rf", "/home/ana/Documents"), AskDeleting},
		{"delete climbing out", RunCommand{}, cmd(goal, "rm", "-r", "../../photos"), AskDeleting},
		{"delete from elsewhere", RunCommand{}, cmd("/home/ana", "rm", "notes.txt"), AskDeleting},
		{"remove software", RemovePart{}, nil, AskDeleting},
		{"delete inside", RunCommand{}, cmd(goal, "rm", "-rf", "build"), ""},
		{"git reset inside", RunCommand{}, cmd(goal, "git", "reset", "--hard"), ""},
		{"build", RunCommand{}, cmd(goal, "godot4", "--headless", "--export-release", "Linux"), ""},
		{"download a model", InstallModel{}, json.RawMessage(`{"name":"qwen3:8b"}`), ""},
		{"write a file", WriteFile{}, json.RawMessage(`{}`), ""},
	}

	for _, c := range cases {
		if got := GoalModeAsks(c.tool, c.args, inside); !strings.HasPrefix(got, c.want) || (c.want == "" && got != "") {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}

	// The sentence agreed to names exactly the four.
	for _, bg := range []bool{false, true} {
		s := GoalModeConsent(bg)
		if bg && !strings.Contains(s, "плащане") || !bg && !strings.Contains(s, "payment") {
			t.Errorf("consent: %s", s)
		}
	}
}

func TestAnInstallNeedingTheAdministratorIsSystemWide(t *testing.T) {
	parts := func() []MachinePart {
		return []MachinePart{{Name: "Whisper", Removable: true}, {Name: "ffmpeg", Removable: false}}
	}

	p := InstallPart{Parts: parts}

	if p.SystemWide(json.RawMessage(`{"name":"Whisper"}`)) {
		t.Error("the program's own part counted as system-wide")
	}

	if !p.SystemWide(json.RawMessage(`{"name":"ffmpeg"}`)) || !p.SystemWide(json.RawMessage(`{"name":"unknown"}`)) {
		t.Error("a system package was installed alone")
	}
}
