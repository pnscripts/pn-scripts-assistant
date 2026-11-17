package mcpimport

import "testing"

// Windows does not care how a file name is capitalised, so neither does
// telling which program an entry starts: CMD.EXE is a shell like cmd.exe.
func TestAProgramIsToldWhateverTheCaseOfItsName(t *testing.T) {
	fs, err := Read(Place{ClaudeDesktop, "/c.json"}, []byte(`{"mcpServers": {
		"shouting": {"command": "C:\\Windows\\System32\\CMD.EXE", "args": ["/c", "npx", "-y", "some-server"]},
		"mixed":    {"command": "PowerShell.Exe", "args": ["-Command", "some-server"]}
	}}`), home)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"shouting", "mixed"} {
		if f := one(t, fs, name); f.Problem != ProblemShell {
			t.Errorf("%s: problem %q, want %q", name, f.Problem, ProblemShell)
		}
	}
}
