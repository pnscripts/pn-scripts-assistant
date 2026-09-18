package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"pn-scripts-assistant/internal/brain/tools"
	"pn-scripts-assistant/internal/brain/workspace"
)

/*
 * Every way out of a project an audit tried, each one refused before it runs.
 *
 * The file tools held from the start. What did not: a command copying to
 * "..", a command pointed one folder up, tools that write without saying
 * where, and the project's own settings — which the work being judged could
 * rewrite, including the list of integrations meant to be a second key.
 */
func TestEveryAuditedEscapeIsRefused(t *testing.T) {
	base := t.TempDir()
	proj := filepath.Join(base, "proj")

	os.MkdirAll(filepath.Join(proj, "sub", workspace.Folder), 0o755)
	os.WriteFile(filepath.Join(proj, "a.txt"), []byte("x"), 0o644)
	os.Symlink(base, filepath.Join(proj, "linkout"))

	if err := workspace.Save(proj, workspace.Config{Name: "p", Kind: "game", Outputs: []string{"dist"}}); err != nil {
		t.Fatal(err)
	}

	cfg, _ := workspace.Load(proj)
	scope := workspace.ScopeOf(proj, cfg)

	j := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	cmd := func(dir string, argv ...string) json.RawMessage { return j(map[string]any{"argv": argv, "dir": dir}) }

	refused := []struct {
		name string
		tool tools.Tool
		args json.RawMessage
	}{
		{"write ../", tools.WriteFile{}, j(map[string]string{"path": proj + "/../x.txt"})},
		{"write through a link out", tools.WriteFile{}, j(map[string]string{"path": proj + "/linkout/x.txt"})},
		{"its own settings", tools.WriteFile{}, j(map[string]string{"path": proj + "/.pn-assistant/project.json"})},
		{"its own memory file", tools.EditFile{}, j(map[string]string{"path": proj + "/.pn-assistant/memory.md"})},
		{"a nested project's settings", tools.WriteFile{}, j(map[string]string{"path": proj + "/sub/.pn-assistant/project.json"})},
		{"cp to ..", tools.RunCommand{}, cmd(proj, "cp", "a.txt", "../x.txt")},
		{"cp to sub/../../", tools.RunCommand{}, cmd(proj, "cp", "a.txt", "sub/../../x.txt")},
		{"git -C ..", tools.RunCommand{}, cmd(proj, "git", "-C", "..", "init")},
		{"cp over its settings", tools.RunCommand{}, cmd(proj, "cp", "a.txt", proj+"/.pn-assistant/project.json")},
		{"an absolute path out", tools.RunCommand{}, cmd(proj, "cp", "a.txt", "/tmp/x")},
		{"~ out", tools.RunCommand{}, cmd(proj, "cp", "a.txt", "~/x")},
		{"run one folder up", tools.RunCommand{}, cmd(base, "ls")},
		{"run nowhere in particular", tools.RunCommand{}, cmd("", "ls")},
		{"subtitles to another folder", tools.MakeSubtitles{}, j(map[string]string{"folder": base})},
		{"subtitles with no folder", tools.MakeSubtitles{}, j(map[string]string{})},
		{"put back anything", tools.PutBack{}, j(map[string]string{})},
		{"put back outside", tools.PutBack{}, j(map[string]string{"path": "/home/x/.bashrc"})},
	}

	for _, c := range refused {
		if why := outside(scope, c.tool, c.args); why == "" {
			t.Errorf("%s: allowed", c.name)
		}
	}

	allowed := []struct {
		name string
		tool tools.Tool
		args json.RawMessage
	}{
		{"a file in the project", tools.WriteFile{}, j(map[string]string{"path": proj + "/src/main.js"})},
		{"a picture of a check", tools.WriteFile{}, j(map[string]string{"path": proj + "/.pn-assistant/evidence/x.png"})},
		{"a project skill", tools.WriteFile{}, j(map[string]string{"path": proj + "/.pn-assistant/skills/levels.md"})},
		{"a command in the project", tools.RunCommand{}, cmd(proj, "npm", "test")},
		{"a relative path inside", tools.RunCommand{}, cmd(proj, "cp", "a.txt", "sub/b.txt")},
		{"reading", tools.ReadFile{}, j(map[string]string{"path": "/etc/hostname"})},
	}

	for _, c := range allowed {
		if why := outside(scope, c.tool, c.args); why != "" {
			t.Errorf("%s: refused — %s", c.name, why)
		}
	}
}

// Without the kernel to hold them, a shell or an interpreter handed its
// program as text is refused: nothing else could see what it writes.
func TestInlineProgramsNeedTheKernel(t *testing.T) {
	proj := t.TempDir()
	scope := workspace.ScopeOf(proj, workspace.Config{})

	for _, argv := range [][]string{
		{"bash", "-c", "echo x > ../x"},
		{"python3", "-c", "open('../x','w')"},
		{"node", "-e", "require('fs').writeFileSync('../x','')"},
	} {
		args, _ := json.Marshal(map[string]any{"argv": argv, "dir": proj})

		if why := outside(scope, tools.RunCommand{}, args); why == "" {
			t.Errorf("%v: allowed with no kernel confinement in this test binary", argv)
		}
	}
}
