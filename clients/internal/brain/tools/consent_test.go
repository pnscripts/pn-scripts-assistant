package tools

import (
	"encoding/json"
	"testing"
)

// What cannot be taken back is always asked; the everyday is not.
func TestDestructiveCommandsAreKnown(t *testing.T) {
	asked := [][]string{
		{"rm", "old.txt"}, {"rm", "-rf", "build"}, {"/bin/rm", "-r", "x"}, {"sudo", "apt", "install", "x"},
		{"git", "reset", "--hard", "HEAD~1"}, {"git", "clean", "-fdx"}, {"git", "push", "--force"},
		{"git", "-C", "repo", "push", "-f"}, {"git", "branch", "-D", "topic"}, {"git", "stash", "clear"},
		{"find", ".", "-name", "*.tmp", "-delete"}, {"chmod", "-R", "777", "."}, {"dd", "if=/dev/zero", "of=x"},
		{"mkfs.ext4", "/dev/sdb1"}, {"pkill", "godot"}, {"systemctl", "stop", "ollama"},
	}

	for _, argv := range asked {
		if Destructive(argv) == "" {
			t.Errorf("%v is not asked about", argv)
		}
	}

	everyday := [][]string{
		{"ls", "-la"}, {"git", "status"}, {"git", "push"}, {"git", "commit", "-m", "x"}, {"npm", "test"},
		{"go", "build", "./..."}, {"chmod", "+x", "run.sh"}, {"find", ".", "-name", "*.go"}, {"mkdir", "-p", "a/b"},
		{"git", "restore", "--staged", "x"},
	}

	for _, argv := range everyday {
		if why := Destructive(argv); why != "" {
			t.Errorf("%v is treated as destructive: %s", argv, why)
		}
	}
}

// On "never stop" these still ask, each saying why.
func TestTheOwnersOwnActionsAlwaysAsk(t *testing.T) {
	for _, c := range []struct {
		tool Tool
		args string
	}{
		{SendEmail{}, `{"to":"x@example.com"}`},
		{InstallPart{}, `{"part":"xvfb"}`},
		{InstallModel{}, `{"model":"qwen2.5-coder:14b"}`},
		{RemovePart{}, `{"part":"xvfb"}`},
		{RunCommand{}, `{"argv":["rm","-rf","build"],"dir":"/p"}`},
	} {
		if ConsentFor(c.tool, json.RawMessage(c.args)) == "" {
			t.Errorf("%s is not always asked", c.tool.Name())
		}
	}

	if ConsentFor(RunCommand{}, json.RawMessage(`{"argv":["npm","test"],"dir":"/p"}`)) != "" {
		t.Error("an everyday command is always asked")
	}
}
