package machinecare

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

/*
 * "Can I restart?" — answered from the machine, not from memory.
 *
 * A restart is fine when nothing would be cut off and nothing would come
 * back broken. Cut off: the assistant's own running work, a package manager
 * in the middle of installing. Broken: a newer kernel waiting whose
 * out-of-tree drivers (DKMS — a USB Wi-Fi adapter's, a graphics card's)
 * have not been built for it, so the machine would come up without them.
 * Worth knowing but not in the way: work in project folders that is not
 * committed or not pushed — a restart keeps it, a dead disk would not.
 */

// Item is one thing the answer rests on. Key and Args are for the page to
// say it in its own language; Say is the same in English.
type Item struct {
	Key      string            `json:"key"`
	Args     map[string]string `json:"args,omitempty"`
	Say      string            `json:"say"`
	Blocking bool              `json:"blocking"`
}

// Readiness is the answer. Known is false on a system it is not read on.
type Readiness struct {
	Known bool   `json:"known"`
	Ready bool   `json:"ready"`
	Items []Item `json:"items"`
}

// Repo is one project folder's state in version control.
type Repo struct {
	Dir         string
	Uncommitted int
	Unpushed    int
	NoUpstream  bool

	// Unchecked is a repository whose settings would run programs, which
	// is therefore not asked: see repoState.
	Unchecked bool
}

// Inputs is everything the answer is made from.
type Inputs struct {
	Running        string   // the kernel running now
	Installed      []string // kernels installed, by version
	RebootRequired bool     // the system says a restart is wanted
	DKMS           string   // what `dkms status` printed, or empty
	Installing     []string // package managers at work
	Busy           []string // the assistant's own work in progress
	Repos          []Repo
}

// Judge is the answer for in.
func Judge(in Inputs) Readiness {
	var items []Item

	add := func(key, say string, blocking bool, args map[string]string) {
		items = append(items, Item{Key: key, Say: say, Blocking: blocking, Args: args})
	}

	newest := Newest(in.Installed)

	if newest != "" && newest != in.Running {
		add("kernel_waiting", "A restart starts the newer kernel "+newest+" (running "+in.Running+").", false,
			map[string]string{"new": newest, "now": in.Running})

		for _, module := range MissingDKMS(in.DKMS, in.Running, newest) {
			add("dkms_missing", "The driver "+module+" is not built for "+newest+": after a restart it would be missing. "+
				"Rebuild it first (sudo dkms autoinstall -k "+newest+").", true, map[string]string{"module": module, "kernel": newest})
		}
	} else if in.RebootRequired {
		add("reboot_wanted", "The system asks for a restart to finish an update.", false, nil)
	}

	for _, p := range in.Installing {
		add("installing", p+" is installing or updating software; wait until it has finished.", true, map[string]string{"what": p})
	}

	for _, b := range in.Busy {
		add("busy", "The assistant is still working on: "+b+".", true, map[string]string{"what": b})
	}

	for _, r := range in.Repos {
		switch {
		case r.Unchecked:
			add("repo_unchecked", "Not checked: the git settings in "+r.Dir+" would run programs.", false,
				map[string]string{"dir": r.Dir})
		case r.Uncommitted > 0:
			add("uncommitted", "Changed files not committed in "+r.Dir+": "+strconv.Itoa(r.Uncommitted)+".", false,
				map[string]string{"n": strconv.Itoa(r.Uncommitted), "dir": r.Dir})
		case r.Unpushed > 0:
			add("unpushed", "Commits not pushed in "+r.Dir+": "+strconv.Itoa(r.Unpushed)+".", false,
				map[string]string{"n": strconv.Itoa(r.Unpushed), "dir": r.Dir})
		}
	}

	ready := true

	for _, i := range items {
		if i.Blocking {
			ready = false
		}
	}

	if items == nil {
		items = []Item{}
	}

	return Readiness{Known: true, Ready: ready, Items: items}
}

// Newest is the highest kernel version among list ("6.8.0-149-generic").
func Newest(list []string) string {
	sorted := append([]string{}, list...)
	sort.Slice(sorted, func(i, j int) bool { return lessVersion(sorted[i], sorted[j]) })

	if len(sorted) == 0 {
		return ""
	}

	return sorted[len(sorted)-1]
}

// lessVersion compares two versions number by number: 6.8.0-149 > 6.8.0-94.
func lessVersion(a, b string) bool {
	na, nb := numbers(a), numbers(b)

	for i := 0; i < len(na) && i < len(nb); i++ {
		if na[i] != nb[i] {
			return na[i] < nb[i]
		}
	}

	return len(na) < len(nb)
}

func numbers(v string) []int {
	var out []int

	for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r < '0' || r > '9' }) {
		n, _ := strconv.Atoi(part)
		out = append(out, n)
	}

	return out
}

/*
 * MissingDKMS is the DKMS modules installed for the running kernel and not
 * for the newer one, read from `dkms status` lines such as
 * "rtl8821au/5.12.5.2, 6.8.0-149-generic, x86_64: installed" (older dkms
 * writes "rtl8821au, 5.12.5.2, 6.8.0-149-generic, x86_64: installed").
 */
func MissingDKMS(status, running, newer string) []string {
	now, next := map[string]bool{}, map[string]bool{}

	for _, line := range strings.Split(status, "\n") {
		colon := strings.LastIndex(line, ":")
		if colon < 0 || !strings.Contains(line[colon:], "installed") {
			continue
		}

		parts := strings.Split(line[:colon], ",")
		if len(parts) < 3 {
			continue
		}

		module := strings.TrimSpace(strings.SplitN(parts[0], "/", 2)[0])
		kernel := strings.TrimSpace(parts[len(parts)-2])

		switch kernel {
		case running:
			now[module] = true
		case newer:
			next[module] = true
		}
	}

	var out []string

	for m := range now {
		if !next[m] {
			out = append(out, m)
		}
	}

	sort.Strings(out)

	return out
}

/*
 * Read gathers the inputs from this machine: the kernels in /boot, the
 * system's own "restart wanted" note, dkms when it is installed, package
 * managers at work, and the given project folders' state in git. busy is
 * the assistant's own work, from the caller. Every command has a few
 * seconds and no more.
 */
func Read(ctx context.Context, busy []string, folders []string) Readiness {
	if runtime.GOOS != "linux" {
		return Readiness{Items: []Item{}}
	}

	in := Inputs{Busy: busy}

	if raw, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		in.Running = strings.TrimSpace(string(raw))
	}

	if matches, _ := filepath.Glob("/boot/vmlinuz-*"); len(matches) > 0 {
		for _, m := range matches {
			in.Installed = append(in.Installed, strings.TrimPrefix(filepath.Base(m), "vmlinuz-"))
		}
	}

	if _, err := os.Stat("/var/run/reboot-required"); err == nil {
		in.RebootRequired = true
	}

	if path, err := exec.LookPath("dkms"); err == nil {
		in.DKMS = output(ctx, path, "status")
	}

	if procs, err := Processes(); err == nil {
		in.Installing = installing(procs)
	}

	for _, dir := range folders {
		if r, ok := repoState(ctx, dir); ok {
			in.Repos = append(in.Repos, r)
		}
	}

	return Judge(in)
}

// installing is the package managers at work among procs. The update
// service waiting for the machine to shut down is not at work.
func installing(procs []Proc) []string {
	seen := map[string]bool{}

	var out []string

	for _, p := range procs {
		fields := strings.Fields(p.Command)
		if len(fields) == 0 || strings.Contains(p.Command, "--wait-for-signal") {
			continue
		}

		name := filepath.Base(fields[0])
		if name == "python3" && len(fields) > 1 {
			name = filepath.Base(fields[1])
		}

		switch name {
		case "apt", "apt-get", "dpkg", "unattended-upgrade", "packagekitd-install", "snap-install":
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}

	return out
}

func output(ctx context.Context, program string, args ...string) string {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, program, args...).Output()
	if err != nil && len(out) == 0 {
		return ""
	}

	return string(out)
}

/*
 * repoState is a folder's uncommitted files and unpushed commits, when it is
 * a git working tree.
 *
 * Git reads the folder's own settings, and some of them run programs (a file
 * system monitor, filters, hooks, includes): a folder an agent or a download
 * wrote into must not get to run anything because somebody asked whether it
 * is safe to restart. A repository whose settings name any of them is not
 * asked at all, and git is started with the monitor and hooks off and no
 * locks taken, whatever the settings say.
 */
func repoState(ctx context.Context, dir string) (Repo, bool) {
	git, err := exec.LookPath("git")
	if err != nil {
		return Repo{}, false
	}

	config, found := gitConfig(dir)
	if !found {
		return Repo{}, false
	}

	if RunsPrograms(config) {
		return Repo{Dir: dir, Unchecked: true}, true
	}

	run := func(args ...string) string {
		return output(ctx, git, append([]string{"-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null",
			"--no-optional-locks", "-C", dir}, args...)...)
	}

	if strings.TrimSpace(run("rev-parse", "--is-inside-work-tree")) != "true" {
		return Repo{}, false
	}

	r := Repo{Dir: dir}

	scanner := bufio.NewScanner(strings.NewReader(run("status", "--porcelain")))
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			r.Uncommitted++
		}
	}

	if ahead := strings.TrimSpace(run("rev-list", "--count", "@{upstream}..HEAD")); ahead != "" {
		r.Unpushed, _ = strconv.Atoi(ahead)
	} else {
		r.NoUpstream = true
	}

	return r, true
}

// gitConfig is the text of a folder's own git settings — .git/config, or for
// a worktree the config of the repository it belongs to — and whether the
// folder is a git working tree at its top.
func gitConfig(dir string) (string, bool) {
	gitPath := filepath.Join(dir, ".git")

	info, err := os.Lstat(gitPath)
	if err != nil {
		return "", false
	}

	var configs []string

	if info.IsDir() {
		configs = append(configs, filepath.Join(gitPath, "config"))
	} else if raw, err := os.ReadFile(gitPath); err == nil && strings.HasPrefix(string(raw), "gitdir:") {
		gitdir := strings.TrimSpace(strings.TrimPrefix(string(raw), "gitdir:"))
		if !filepath.IsAbs(gitdir) {
			gitdir = filepath.Join(dir, gitdir)
		}

		configs = append(configs, filepath.Join(gitdir, "config.worktree"))

		common := gitdir
		if c, err := os.ReadFile(filepath.Join(gitdir, "commondir")); err == nil {
			common = strings.TrimSpace(string(c))
			if !filepath.IsAbs(common) {
				common = filepath.Join(gitdir, common)
			}
		}

		configs = append(configs, filepath.Join(common, "config"))
	} else {
		return "", false
	}

	var text strings.Builder

	for _, c := range configs {
		if raw, err := os.ReadFile(c); err == nil {
			text.Write(raw)
			text.WriteString("\n")
		}
	}

	return text.String(), true
}

// RunsPrograms is whether git settings name anything that makes git run a
// program while it only looks: a file system monitor, hooks elsewhere,
// filters, text conversion, external diffs, includes of other settings.
func RunsPrograms(config string) bool {
	low := strings.ToLower(config)

	for _, word := range []string{"fsmonitor", "hookspath", "[filter", "textconv", "[include", "external",
		"sshcommand", "askpass", "pager", "editor", "[credential", "untrackedcache"} {
		if strings.Contains(low, word) {
			return true
		}
	}

	return false
}
