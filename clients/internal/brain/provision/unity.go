package provision

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/sandbox"
)

/*
 * Unity, as it is really installed: which editors, with which build modules,
 * and whether the licence lets one work without a person at it.
 *
 * The Hub names an editor's folder after its version, and sometimes adds the
 * architecture — "6000.5.4f1-x86_64" next to "6000.5.4f1" — so the version is
 * read out of the name rather than taken as the name. Where one version is
 * installed twice, the one with more build modules is the one to use.
 */

// UnityInstall is one editor.
type UnityInstall struct {
	Path    string   `json:"path"`
	Version string   `json:"version"`
	Folder  string   `json:"folder"`
	Modules []string `json:"modules"`
}

var unityFolder = regexp.MustCompile(`^(\d+\.\d+\.\d+[abfpx]\d+)`)

// UnityInstalls is every editor the Hub put here, best first: newest, then
// the one with the most build support.
func UnityInstalls() []UnityInstall {
	var out []UnityInstall

	for _, root := range UnityEditorsIn() {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}

		for _, e := range entries {
			m := unityFolder.FindStringSubmatch(e.Name())
			if m == nil {
				continue
			}

			editor := filepath.Join(root, e.Name(), "Editor", "Unity")

			if info, err := os.Stat(editor); err != nil || info.Mode()&0o111 == 0 {
				continue
			}

			in := UnityInstall{Path: editor, Version: m[1], Folder: e.Name()}

			modules, _ := os.ReadDir(filepath.Join(root, e.Name(), "Editor", "Data", "PlaybackEngines"))
			for _, mod := range modules {
				if mod.IsDir() {
					in.Modules = append(in.Modules, mod.Name())
				}
			}

			out = append(out, in)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Version != out[j].Version {
			return Newer(out[j].Version, out[i].Version)
		}

		return len(out[i].Modules) > len(out[j].Modules)
	})

	return out
}

// UnityLicenceLife is how long an answer about the licence is kept: asking
// means starting the editor.
const UnityLicenceLife = 30 * time.Minute

// Licence is what an editor's licensing said.
type Licence struct {
	State string    `json:"state"`
	Why   string    `json:"why"`
	At    time.Time `json:"at"`
}

var (
	licences   sync.Mutex
	licenceFor = map[string]Licence{}
)

var (
	unlicensed = []string{"No valid Unity Editor license", "was not found", "Access token is unavailable"}
	licensed   = []string{"License group:", "Successfully activated the entitlement license"}
)

/*
 * UnityLicence is whether the editor at path may run in batch mode, without a
 * person at it.
 *
 * Asked the only way that answers it: the editor is started in batch mode on
 * an empty folder and listened to until its licensing has decided — a few
 * seconds — and then stopped, before it does anything else. Nothing is signed
 * in to, activated or accepted; the editor writes its own licensing log, as
 * it does whenever it starts. Kept for half an hour.
 */
func UnityLicence(path string) Licence {
	licences.Lock()
	if l, ok := licenceFor[path]; ok && time.Since(l.At) < UnityLicenceLife {
		licences.Unlock()

		return l
	}
	licences.Unlock()

	l := probeUnityLicence(path)

	licences.Lock()
	licenceFor[path] = l
	licences.Unlock()

	return l
}

// ForgetUnityLicence drops the kept answer, for when its owner has just
// signed in.
func ForgetUnityLicence() {
	licences.Lock()
	licenceFor = map[string]Licence{}
	licences.Unlock()
}

func probeUnityLicence(path string) Licence {
	l := Licence{State: LicenceUnknown, At: time.Now()}

	// A test binary does not start somebody's Unity as a side effect of
	// proposing a project — only a test that says so.
	if testing.Testing() && os.Getenv("PN_TEST_UNITY_LICENCE") == "" {
		l.Why = "not asked while testing"

		return l
	}

	scratch, err := os.MkdirTemp("", "pn-unity-licence-")
	if err != nil {
		l.Why = err.Error()

		return l
	}

	defer os.RemoveAll(scratch)

	/*
	 * An empty project for it to open: a folder that is not one is refused
	 * before licensing is ever asked, and "could not open the project" says
	 * nothing about the licence.
	 */
	project := filepath.Join(scratch, "probe")
	os.MkdirAll(filepath.Join(project, "Assets"), 0o755)
	os.MkdirAll(filepath.Join(project, "ProjectSettings"), 0o755)

	version := ""
	if m := unityFolder.FindStringSubmatch(filepath.Base(filepath.Dir(filepath.Dir(path)))); m != nil {
		version = m[1]
	}

	os.WriteFile(filepath.Join(project, "ProjectSettings", "ProjectVersion.txt"),
		[]byte("m_EditorVersion: "+version+"\n"), 0o644)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd, err := sandbox.Command(ctx, sandbox.Spec{Dir: scratch}, path, "-batchmode", "-nographics", "-quit",
		"-projectPath", project, "-logFile", "-")
	if err != nil {
		l.Why = err.Error()

		return l
	}

	out, err := cmd.StdoutPipe()
	if err != nil {
		l.Why = err.Error()

		return l
	}

	if err := cmd.Start(); err != nil {
		l.Why = "the editor would not start: " + err.Error()

		return l
	}

	lines := bufio.NewScanner(out)
	lines.Buffer(make([]byte, 64*1024), 1<<20)

	var said []string

	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())

		if strings.Contains(line, "Licensing") || strings.Contains(line, "license") {
			said = append(said, line)
		}

		for _, mark := range unlicensed {
			if strings.Contains(line, mark) && l.State == LicenceUnknown && mark != "Access token is unavailable" {
				l.State, l.Why = LicenceInactive, unityReason(said)
			}
		}

		for _, mark := range licensed {
			if strings.Contains(line, mark) && l.State == LicenceUnknown {
				l.State, l.Why = LicenceActive, "its licensing accepted batch mode"
			}
		}

		if l.State != LicenceUnknown {
			break
		}
	}

	// Stopped here, whatever it was about to do next.
	cancel()
	cmd.Wait()

	if l.State == LicenceUnknown {
		l.Why = "the editor ended without its licensing saying either way"
	}

	return l
}

// unityReason is what the licensing said, in the words that matter.
func unityReason(said []string) string {
	var parts []string

	for _, line := range said {
		for _, mark := range unlicensed {
			if strings.Contains(line, mark) {
				parts = append(parts, strings.TrimSpace(line[strings.Index(line, "]")+1:]))
			}
		}
	}

	why := "no active licence for unattended use"
	if len(parts) > 0 {
		why = strings.Join(dedupe(parts), "; ")
	}

	return why + " — its owner signs in to Unity Hub, which renews the licence, and then this works"
}

func dedupe(list []string) []string {
	seen := map[string]bool{}

	var out []string

	for _, v := range list {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}

	return out
}
