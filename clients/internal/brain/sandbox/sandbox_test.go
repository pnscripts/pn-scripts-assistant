package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if Main() {
		return
	}

	Enable()
	os.Exit(m.Run())
}

func needLandlock(t *testing.T) {
	t.Helper()

	if _, err := ABI(); err != nil {
		t.Skip(err)
	}
}

/*
 * The three escapes an audit found — cp to "..", a shell, an interpreter's
 * one-liner — each tried under confinement: the file inside is written, and
 * nothing outside is.
 */
func TestTheKernelKeepsACommandInItsProject(t *testing.T) {
	needLandlock(t)

	base := t.TempDir()
	project := filepath.Join(base, "project")
	os.MkdirAll(project, 0o755)
	os.WriteFile(filepath.Join(project, "a.txt"), []byte("x"), 0o644)

	// The temporary folder is where base lives, so it is left out here: the
	// point is that only the project may be written.
	spec := Spec{Dir: project, Writable: []string{project, "/dev"}}

	for _, argv := range [][]string{
		{"cp", "a.txt", "../escape-cp.txt"},
		{"bash", "-c", "echo x > ../escape-bash.txt"},
		{"python3", "-c", "open('../escape-py.txt','w').write('x')"},
		{"sh", "-c", "echo inside > inside.txt"},
	} {
		cmd, err := Command(context.Background(), spec, argv...)
		if err != nil {
			t.Fatal(err)
		}

		cmd.Run()
	}

	entries, _ := os.ReadDir(base)
	for _, e := range entries {
		if e.Name() != "project" {
			t.Errorf("written outside the project: %s", e.Name())
		}
	}

	if _, err := os.Stat(filepath.Join(project, "inside.txt")); err != nil {
		t.Error("writing inside the project was refused too")
	}
}

func TestAnEnvironmentCarriesNoCredentials(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-secret")
	t.Setenv("SSH_AUTH_SOCK", "/run/agent")
	t.Setenv("GITHUB_TOKEN", "ghp_secret")
	t.Setenv("CLAUDE_CODE_MESSAGING_TOKEN", "x")

	for name, env := range map[string][]string{"clean": Clean(), "scrubbed": Scrubbed(os.Environ())} {
		joined := strings.Join(env, "\n")

		for _, secret := range []string{"sk-ant-secret", "ghp_secret", "CLAUDE_CODE_MESSAGING_TOKEN"} {
			if strings.Contains(joined, secret) {
				t.Errorf("%s environment carries %s", name, secret)
			}
		}
	}

	if strings.Contains(strings.Join(Clean(), "\n"), "SSH_AUTH_SOCK") {
		t.Error("the clean environment lends out the SSH agent")
	}
}

func TestStoppingKillsWhatTheCommandStarted(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "late")

	ctx, cancel := context.WithCancel(context.Background())

	cmd, _ := Command(ctx, Spec{Dir: dir}, "sh", "-c", "(sleep 2; touch "+marker+") & wait")
	cmd.Start()
	cancel()
	cmd.Wait()

	// The grandchild would have written the marker by now had it lived.
	cmd2, _ := Command(context.Background(), Spec{Dir: dir}, "sleep", "2.5")
	cmd2.Run()

	if _, err := os.Stat(marker); err == nil {
		t.Error("a child of a stopped command went on running")
	}
}
