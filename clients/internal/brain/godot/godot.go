// Package godot is what the brain knows about the game engine.
//
// Petar asked for it to be able to make complex games with Godot, to check the
// documentation, and to know about updates. Godot is not installed on this
// machine and there is not one project on it, so all three start from nothing.
//
// The shape of the answer matters more than the size of it. A model asked to
// write GDScript will write GDScript whether or not it knows the engine — it
// will invent method names that read exactly like real ones, and the mistake
// only appears when the game is run. So the useful capability is not "learn
// Godot", which cannot be verified, but "look the class up", which can: the
// class reference is a file per class, it is authoritative, and fetching one
// costs a second.
package godot

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Names the engine goes by on a machine, newest convention first.
var names = []string{"godot4", "godot-4", "godot", "Godot", "godot3"}

// Where it lands when it is not installed by a package manager. Godot ships as
// a single executable people put wherever they keep such things.
var places = []string{
	"/usr/local/bin", "/opt/godot", "/opt",
	"~/.local/bin", "~/Applications", "~/bin", "~/Downloads", "~/Desktop", "~/Games",
}

// Engine is an installed Godot.
type Engine struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

/*
 * Find looks for the engine, and says nothing rather than guessing.
 *
 * PATH first, then the handful of places a single-file executable ends up.
 * Started from an applications menu this program gets a short PATH that has
 * no ~/.local/bin on it, which is exactly where somebody who downloaded Godot
 * put it — the same trap the speech recogniser fell into.
 */
func Find() (Engine, bool) {
	all := FindAll()
	if len(all) == 0 {
		return Engine{}, false
	}

	return all[0], true
}

/*
 * FindAll is every Godot on this machine, newest first.
 *
 * Somebody who downloaded 4.7.1 to the desktop and later let this program
 * install 4.7.2 has two, and which one a project gets should be a choice made
 * knowing both — not whichever happened to be looked at first.
 */
func FindAll() []Engine {
	var found []Engine

	seen := map[string]bool{}

	add := func(path string) {
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			real = path
		}

		if seen[real] {
			return
		}

		seen[real] = true
		found = append(found, described(path))
	}

	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			add(path)
		}
	}

	home, _ := os.UserHomeDir()

	for _, dir := range places {
		if strings.HasPrefix(dir, "~/") {
			if home == "" {
				continue
			}

			dir = filepath.Join(home, dir[2:])
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() || !looksLikeGodot(e.Name()) {
				continue
			}

			if info, err := e.Info(); err != nil || info.Mode()&0o111 == 0 {
				continue
			}

			add(filepath.Join(dir, e.Name()))
		}
	}

	// A binary that would not say its version is not one to build with.
	usable := found[:0]

	for _, e := range found {
		if e.Version != "" {
			usable = append(usable, e)
		}
	}

	sort.SliceStable(usable, func(i, j int) bool { return Newer(usable[j].Version, usable[i].Version) })

	return usable
}

// looksLikeGodot recognises the released filenames, which carry the version
// and the platform: Godot_v4.3-stable_linux.x86_64.
func looksLikeGodot(name string) bool {
	lower := strings.ToLower(name)

	if !strings.HasPrefix(lower, "godot") {
		return false
	}

	// Not the exported templates or a zip somebody has not unpacked.
	for _, no := range []string{".zip", ".tpz", ".tar", ".txt", ".log"} {
		if strings.HasSuffix(lower, no) {
			return false
		}
	}

	return true
}

func described(path string) Engine {
	e := Engine{Path: path}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// --version prints one line and exits, and is the only thing here that
	// does not need a project or a display.
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err == nil {
		e.Version = strings.TrimSpace(strings.Split(string(out), "\n")[0])
	}

	return e
}

// version numbers inside a release name, for comparing what is installed with
// what has been released.
var number = regexp.MustCompile(`(\d+)\.(\d+)(?:\.(\d+))?`)

// Newer reports whether released is a later version than installed.
//
// Compared as numbers rather than as strings, because "4.10" is later than
// "4.9" and sorts before it. Anything unparseable answers false: telling
// somebody an update exists when it does not is worse than staying quiet.
func Newer(installed, released string) bool {
	a := number.FindStringSubmatch(installed)
	b := number.FindStringSubmatch(released)

	if a == nil || b == nil {
		return false
	}

	for i := 1; i <= 3; i++ {
		x, _ := strconv.Atoi(a[i])
		y, _ := strconv.Atoi(b[i])

		if x != y {
			return y > x
		}
	}

	return false
}

// Release is what the engine's authors have published.
type Release struct {
	Version string `json:"version"`
	When    string `json:"when,omitempty"`
	Notes   string `json:"notes,omitempty"`
}

/*
 * Latest asks what has been released.
 *
 * One request to the project's own release list. It reaches the network, so
 * every caller has to be somewhere privacy has already been considered — an
 * update check is a small thing to leak and it is still a thing that says this
 * machine exists and runs Godot.
 */
func Latest(ctx context.Context, client *http.Client) (Release, error) {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/godotengine/godot/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}

	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("could not ask what has been released: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("the release list answered %d", resp.StatusCode)
	}

	var body struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		PublishedAt string `json:"published_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Release{}, err
	}

	version := body.TagName
	if version == "" {
		version = body.Name
	}

	if len(body.PublishedAt) >= 10 {
		body.PublishedAt = body.PublishedAt[:10]
	}

	return Release{Version: version, When: body.PublishedAt}, nil
}

// Project is one game on this machine.
type Project struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

/*
 * Projects finds the games under a folder.
 *
 * By project.godot, which is the engine's own marker and sits at the root of
 * every project. Descent stops at one: a Godot project contains addons that
 * are themselves projects, and listing those as somebody's games is the same
 * mistake the document scanner made with node_modules.
 */
func Projects(root string) ([]Project, error) {
	var found []Project

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}

		if name := d.Name(); path != root && (strings.HasPrefix(name, ".") ||
			name == "addons" || name == "node_modules") {
			return fs.SkipDir
		}

		if _, err := os.Stat(filepath.Join(path, "project.godot")); err != nil {
			return nil
		}

		found = append(found, Project{Path: path, Name: nameOf(path)})

		// Everything below is part of this game, not another one.
		return fs.SkipDir
	})

	return found, err
}

// nameOf reads the project's own name, falling back to the folder's.
func nameOf(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, "project.godot"))
	if err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			if after, found := strings.CutPrefix(strings.TrimSpace(line), "config/name="); found {
				if name := strings.Trim(strings.TrimSpace(after), `"`); name != "" {
					return name
				}
			}
		}
	}

	return filepath.Base(dir)
}
