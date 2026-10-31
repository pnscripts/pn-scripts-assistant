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

/*
 * A shell's own program is judged, on Linux and on Windows.
 *
 * Found by the 1.0 security review: "bash -c 'rm -rf ~'" read as bash, and on
 * Windows every deletion is a cmd or PowerShell command, so nothing there was
 * ever asked about. An encoded PowerShell command cannot be read and is asked
 * about as what it might be.
 */
func TestAShellsOwnProgramIsJudged(t *testing.T) {
	asked := [][]string{
		{"bash", "-c", "rm -rf ~/Documents"},
		{"sh", "-lc", "cd x && rm -rf y"},
		{"bash", "-c", "echo $(shred secret.txt)"},
		{"bash", "-c", "find . | xargs rm"},
		{"/usr/bin/env", "rm", "-rf", "x"},
		{"timeout", "5", "pkill", "x"},
		{`C:\Windows\System32\cmd.exe`, "/c", "del /s /q C:\\Users\\ana\\Documents"},
		{"cmd", "/C", "rd /s /q build"},
		{"powershell", "-NoProfile", "-Command", "Remove-Item -Recurse $HOME\\Documents"},
		{"pwsh", "-c", "Get-ChildItem | ForEach-Object { Remove-Item $_ }"},
		{"powershell.exe", "Stop-Process -Name explorer"},
		{"powershell", "-ExecutionPolicy", "Bypass", "-EncodedCommand", "UgBlAG0AbwB2AGUA"},
		{"taskkill", "/F", "/IM", "x.exe"},
		{"reg", "delete", "HKCU\\Software\\X"},
		{"Format-Volume", "-DriveLetter", "D"},
	}

	for _, argv := range asked {
		if Destructive(argv) == "" {
			t.Errorf("%v is not asked about", argv)
		}
	}

	everyday := [][]string{
		{"bash", "-c", "npm install && npm run build"},
		{"cmd", "/c", "dir && echo done"},
		{"powershell", "-NoProfile", "-Command", "Get-ChildItem | Select-Object Name"},
		{"powershell", "-File", "build.ps1"},
		{"bash", "build.sh"},
		{"env", "GOOS=windows", "go", "build"},
	}

	for _, argv := range everyday {
		if why := Destructive(argv); why != "" {
			t.Errorf("%v is treated as destructive: %s", argv, why)
		}
	}

	if _, hidden, _ := Inline([]string{"pwsh", "-enc", "AAAA"}); !hidden {
		t.Error("an encoded command was read as visible")
	}

	if why := Destructive([]string{"pwsh", "-enc", "AAAA"}); why != HiddenCommand {
		t.Errorf("an encoded command: %q", why)
	}
}

// On Windows a command written into cmd's or PowerShell's argument is never
// held by the kernel, so it is unconfinable like sh -c.
func TestWindowsShellsAreUnconfinable(t *testing.T) {
	for _, argv := range [][]string{
		{"cmd.exe", "/c", "copy a b"}, {"powershell", "-Command", "Set-Content x y"}, {"pwsh", "Set-Content x y"},
	} {
		raw, _ := json.Marshal(commandArgs{Argv: argv})

		if Unconfinable(RunCommand{}, raw) == "" {
			t.Errorf("%v is treated as confinable", argv)
		}
	}
}
