package programs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Target is something a source is watched through: a folder, or one file
// in it (Only).
type Target struct {
	Dir  string `json:"dir"`
	Only string `json:"only,omitempty"`
}

/*
 * Scanner reads the sources. Every path it reads is under Root ("" is the
 * real filesystem), so a test builds a machine in a folder; Run runs
 * dpkg-query and apt-config, so a test answers for them.
 */
type Scanner struct {
	Root     string
	Home     string
	DataHome string   // $XDG_DATA_HOME
	DataDirs []string // $XDG_DATA_DIRS
	Path     []string // $PATH, for TryExec

	Run func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// FromEnvironment is the Scanner for this computer and this user.
func FromEnvironment() Scanner {
	home, _ := os.UserHomeDir()

	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" && home != "" {
		dataHome = filepath.Join(home, ".local", "share")
	}

	dirs := filepath.SplitList(os.Getenv("XDG_DATA_DIRS"))
	if len(dirs) == 0 {
		dirs = []string{"/usr/local/share", "/usr/share"}
	}

	return Scanner{
		Home:     home,
		DataHome: dataHome,
		DataDirs: dirs,
		Path:     filepath.SplitList(os.Getenv("PATH")),
		Run:      runCommand,
	}
}

// commandTimeout bounds dpkg-query and apt-config.
const commandTimeout = 30 * time.Second

// mostOutput bounds what a command may print: a system with 10,000
// packages prints about a megabyte.
const mostOutput = 32 << 20

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")

	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	b, readErr := io.ReadAll(io.LimitReader(out, mostOutput+1))

	if err := cmd.Wait(); err != nil {
		return nil, err
	}

	if readErr != nil {
		return nil, readErr
	}

	if len(b) > mostOutput {
		return nil, fmt.Errorf("%s printed more than %d bytes", name, mostOutput)
	}

	return b, nil
}

func (s Scanner) at(path string) string {
	if s.Root == "" {
		return path
	}

	return filepath.Join(s.Root, path)
}

func (s Scanner) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	run := s.Run
	if run == nil {
		run = runCommand
	}

	return run(ctx, name, args...)
}

// Scan is what one source shows now.
func (s Scanner) Scan(ctx context.Context, k Kind) ([]Program, error) {
	var (
		ps  []Program
		err error
	)

	switch k {
	case Deb:
		ps, err = s.debs(ctx)
	case Snap:
		ps, err = s.snaps()
	case Flatpak:
		ps, err = s.flatpaks()
	case App:
		ps, err = s.apps()
	default:
		return nil, fmt.Errorf("no source %q", k)
	}

	for i := range ps {
		ps[i].Name = tidy(ps[i].Name)
		ps[i].ID = tidy(ps[i].ID)
		ps[i].Version = tidy(ps[i].Version)
	}

	return ps, err
}

// MostName is the longest name, id or version kept, in characters.
const MostName = 120

/*
 * tidy keeps what a program says about itself short and on one line: it is
 * written by whoever made the program and reaches the model as data, so a
 * menu entry must not be able to hand it a page of text.
 */
func tidy(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}

		return r
	}, s)

	s = strings.Join(strings.Fields(s), " ")

	if r := []rune(s); len(r) > MostName {
		s = strings.TrimSpace(string(r[:MostName-1])) + "…"
	}

	return s
}

// Targets is what a source is watched through. Folders that do not exist
// yet are included: the watch polls them until they do.
func (s Scanner) Targets(ctx context.Context, k Kind) []Target {
	switch k {
	case Deb:
		status, err := s.dpkgStatus(ctx)
		if err != nil {
			return nil
		}

		return []Target{{Dir: filepath.Dir(s.at(status)), Only: filepath.Base(status)}}
	case Snap:
		return []Target{{Dir: s.at("/var/lib/snapd/snaps")}, {Dir: s.at("/snap")}}
	case Flatpak:
		out := []Target{}
		for _, base := range s.flatpakBases() {
			out = append(out, Target{Dir: base, Only: ".changed"})
		}

		for _, base := range s.flatpakBases() {
			out = append(out, Target{Dir: filepath.Join(base, "app")})
		}

		return out
	case App:
		var out []Target

		for _, dir := range s.appDirs() {
			out = append(out, Target{Dir: dir})

			for _, sub := range subdirs(dir, 3) {
				out = append(out, Target{Dir: sub})
			}
		}

		return out
	}

	return nil
}

// ---- deb ----

const dpkgFormat = "${binary:Package}\t${Version}\t${db:Status-Abbrev}\n"

func (s Scanner) debs(ctx context.Context) ([]Program, error) {
	out, err := s.run(ctx, "dpkg-query", "-W", "-f="+dpkgFormat)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, ErrNotHere
		}

		return nil, fmt.Errorf("asking dpkg-query: %w", err)
	}

	return parseDpkg(out), nil
}

/*
 * parseDpkg reads dpkg-query's lines. A package is there when its status
 * (the second letter of Status-Abbrev) is anything but not-installed (n) or
 * config-files (c): one being upgraded, held or half configured is still
 * on the machine, and calling it removed for the minute an upgrade takes
 * would be wrong twice.
 */
func parseDpkg(out []byte) []Program {
	var ps []Program

	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 64<<10), 1<<20)

	for sc.Scan() {
		fields := strings.Split(sc.Text(), "\t")
		if len(fields) < 3 || fields[0] == "" {
			continue
		}

		status := fields[2]
		if len(status) < 2 || status[1] == 'n' || status[1] == 'c' || status[1] == ' ' {
			continue
		}

		ps = append(ps, Program{Kind: Deb, ID: fields[0], Name: fields[0], Version: fields[1]})
	}

	return ps
}

// dpkgStatus is the file dpkg keeps its list in, as apt-config names it:
// asked rather than written here, because it is dpkg's to place.
func (s Scanner) dpkgStatus(ctx context.Context) (string, error) {
	out, err := s.run(ctx, "apt-config", "shell", "S", "Dir::State::status")
	if err != nil {
		return "", err
	}

	line := strings.TrimSpace(string(out))

	v, ok := strings.CutPrefix(line, "S=")
	if !ok {
		return "", errors.New("apt-config named no status file")
	}

	v = strings.Trim(v, `'"`)
	if !filepath.IsAbs(v) {
		return "", errors.New("apt-config named no status file")
	}

	return v, nil
}

// ---- snap ----

// notPrograms are snap types that are part of the system, not programs.
var notPrograms = map[string]bool{"base": true, "core": true, "os": true, "snapd": true, "gadget": true, "kernel": true}

func (s Scanner) snaps() ([]Program, error) {
	root := s.at("/snap")

	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	var ps []Program

	for _, e := range entries {
		if !e.IsDir() || e.Name() == "bin" {
			continue
		}

		dir := filepath.Join(root, e.Name())

		meta, err := readSmall(filepath.Join(dir, "current", "meta", "snap.yaml"))
		if err != nil {
			continue // mid-install, or not a snap
		}

		// A snap with no apps is a runtime other snaps use (gnome-3-38-2004),
		// not a program anybody starts.
		f := yamlTop(meta)
		if _, runs := f["apps"]; !runs || notPrograms[f["type"]] {
			continue
		}

		id := f["name"]
		if id == "" {
			id = e.Name()
		}

		name := f["title"]
		if name == "" {
			name = id
		}

		ps = append(ps, Program{Kind: Snap, ID: id, Name: name, Version: f["version"], Path: dir})
	}

	return ps, nil
}

// yamlTop reads the top-level "key: value" lines of snap.yaml; nothing
// nested is needed, so no YAML library.
func yamlTop(b []byte) map[string]string {
	out := map[string]string{}

	for _, line := range strings.Split(string(b), "\n") {
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}

		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '\'' || v[0] == '"') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}

		out[strings.TrimSpace(k)] = v
	}

	return out
}

// ---- flatpak ----

func (s Scanner) flatpakBases() []string {
	bases := []string{s.at("/var/lib/flatpak")}
	if s.DataHome != "" {
		bases = append(bases, s.at(filepath.Join(s.DataHome, "flatpak")))
	}

	return bases
}

func (s Scanner) flatpaks() ([]Program, error) {
	var ps []Program

	for _, base := range s.flatpakBases() {
		appDir := filepath.Join(base, "app")

		entries, err := os.ReadDir(appDir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}

		if err != nil {
			return nil, err
		}

		for _, e := range entries {
			if p, ok := flatpakApp(filepath.Join(appDir, e.Name()), e.Name()); ok {
				ps = append(ps, p)
			}
		}
	}

	return ps, nil
}

// flatpakApp reads one installed application: current names the arch and
// branch, active in it names the deployed commit.
func flatpakApp(dir, id string) (Program, bool) {
	current, err := os.Readlink(filepath.Join(dir, "current"))
	if err != nil {
		return Program{}, false
	}

	branch := filepath.Join(dir, filepath.FromSlash(current))

	commit, err := os.Readlink(filepath.Join(branch, "active"))
	if err != nil {
		return Program{}, false
	}

	deployed := filepath.Join(branch, filepath.Base(commit))

	p := Program{Kind: Flatpak, ID: id, Path: dir}

	for _, rel := range []string{"metainfo/" + id + ".metainfo.xml", "metainfo/" + id + ".appdata.xml", "appdata/" + id + ".appdata.xml"} {
		if b, err := readSmall(filepath.Join(deployed, "files", "share", filepath.FromSlash(rel))); err == nil {
			p.Name, p.Version = metainfo(b)

			break
		}
	}

	if p.Version == "" {
		c := filepath.Base(commit)
		if len(c) > 12 {
			c = c[:12]
		}

		p.Version = c
	}

	if p.Name == "" {
		p.Name = id[strings.LastIndex(id, ".")+1:]
	}

	return p, true
}

// metainfo is the untranslated name and the newest release of an AppStream
// file (releases are listed newest first).
func metainfo(b []byte) (name, version string) {
	var doc struct {
		Names []struct {
			Lang string `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
			Text string `xml:",chardata"`
		} `xml:"name"`
		Releases []struct {
			Version string `xml:"version,attr"`
		} `xml:"releases>release"`
	}

	if xml.Unmarshal(b, &doc) != nil {
		return "", ""
	}

	for _, n := range doc.Names {
		if n.Lang == "" {
			name = strings.TrimSpace(n.Text)

			break
		}
	}

	if len(doc.Releases) > 0 {
		version = doc.Releases[0].Version
	}

	return name, version
}

// ---- menu entries ----

// appDirs is where menu entries are looked for, first wins: the user's own,
// then $XDG_DATA_DIRS, then the snap and Flatpak exports, which a session
// adds to $XDG_DATA_DIRS but a program started outside one may not have.
func (s Scanner) appDirs() []string {
	var bases []string
	if s.DataHome != "" {
		bases = append(bases, s.DataHome)
	}

	bases = append(bases, s.DataDirs...)
	bases = append(bases, "/var/lib/snapd/desktop", "/var/lib/flatpak/exports/share")

	if s.DataHome != "" {
		bases = append(bases, filepath.Join(s.DataHome, "flatpak", "exports", "share"))
	}

	seen := map[string]bool{}

	var out []string

	for _, b := range bases {
		if b == "" || !filepath.IsAbs(b) {
			continue
		}

		d := s.at(filepath.Join(filepath.Clean(b), "applications"))
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}

	return out
}

func (s Scanner) apps() ([]Program, error) {
	taken := map[string]bool{}

	var ps []Program

	for _, dir := range s.appDirs() {
		for _, file := range desktopFiles(dir) {
			rel, _ := filepath.Rel(dir, file)
			id := strings.ReplaceAll(filepath.ToSlash(rel), "/", "-")

			if taken[id] {
				continue
			}

			b, err := readSmall(file)
			if err != nil {
				continue
			}

			taken[id] = true

			e := ParseDesktop(id, bytes.NewReader(b))
			if !e.Listed() || (e.TryExec != "" && !s.found(e.TryExec)) {
				continue
			}

			e.Program.Path = file
			ps = append(ps, e.Program)
		}
	}

	return ps, nil
}

// found is whether a program named by TryExec is there.
func (s Scanner) found(name string) bool {
	if filepath.IsAbs(name) {
		return isFile(s.at(name))
	}

	for _, dir := range s.Path {
		if filepath.IsAbs(dir) && isFile(s.at(filepath.Join(dir, name))) {
			return true
		}
	}

	return false
}

func isFile(path string) bool {
	st, err := os.Stat(path)

	return err == nil && !st.IsDir()
}

// desktopFiles is every .desktop file in dir and its subfolders (three
// deep), sorted, so the result never depends on the order of a listing.
func desktopFiles(dir string) []string {
	var out []string

	var walk func(d string, depth int)
	walk = func(d string, depth int) {
		entries, err := os.ReadDir(d)
		if err != nil {
			return
		}

		for _, e := range entries {
			path := filepath.Join(d, e.Name())

			switch {
			case e.IsDir() && depth < 3:
				walk(path, depth+1)
			case !e.IsDir() && strings.HasSuffix(e.Name(), ".desktop"):
				out = append(out, path)
			}
		}
	}

	walk(dir, 0)
	sort.Strings(out)

	return out
}

func subdirs(dir string, depth int) []string {
	if depth == 0 {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var out []string

	for _, e := range entries {
		if e.IsDir() {
			sub := filepath.Join(dir, e.Name())
			out = append(out, sub)
			out = append(out, subdirs(sub, depth-1)...)
		}
	}

	return out
}

// mostFile bounds a menu entry or a metadata file.
const mostFile = 1 << 20

func readSmall(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer f.Close()

	b, err := io.ReadAll(io.LimitReader(f, mostFile+1))
	if err != nil {
		return nil, err
	}

	if len(b) > mostFile {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, mostFile)
	}

	return b, nil
}
