/*
 * Package sandbox is how a program this assistant starts is started: with an
 * environment made for it, in a process group of its own, and — when the work
 * belongs to a project — unable to write anywhere but the project.
 *
 * Checking a command's arguments before it runs catches "cp a ../b". It does
 * not catch a build script, a game's own code or `python -c` doing the same
 * thing from inside, and an audit showed three files written outside a
 * project that way. So the last word is the kernel's: Landlock, which Linux
 * has had since 5.13, is told before the program starts which folders it may
 * write in, and everything else is refused by the kernel itself, to the
 * program and to everything it starts.
 *
 * Landlock can only be applied by a process to itself, and Go cannot run code
 * between fork and exec. So this program starts a copy of itself with a
 * marker argument; that copy restricts itself and then becomes the command.
 * A binary that has not said it can do that (Enable) — a test binary, say —
 * runs commands without the kernel's part, and says so.
 */
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// helperArg marks the copy of this program that confines itself and execs.
const helperArg = "__pn_confine"

var enabled bool

// Enable says this binary handles helperArg in Main, so commands may be
// confined by the kernel.
func Enable() { enabled = true }

// Enabled is whether confinement by the kernel is in effect for commands.
func Enabled() bool {
	if !enabled {
		return false
	}

	_, err := ABI()

	return err == nil
}

// Spec is how one program is started.
type Spec struct {
	Dir string

	// Env is added to the clean environment.
	Env []string

	// Writable, when given, is every folder the program may write in. Empty
	// means the program is not confined.
	Writable []string

	// Inherit keeps the caller's environment, minus anything that looks like
	// a credential, instead of a clean one: for a command its owner is
	// approving from a conversation, which may well need their SSH agent.
	Inherit bool
}

/*
 * Command is an exec.Cmd for argv under spec.
 *
 * Cancelling ctx kills the whole process group, not only the program named:
 * an engine that has started a shader compiler and a licensing client leaves
 * nothing behind when it is stopped.
 */
func Command(ctx context.Context, spec Spec, argv ...string) (*exec.Cmd, error) {
	if len(argv) == 0 {
		return nil, errors.New("no command")
	}

	env := Clean(spec.Env...)
	if spec.Inherit {
		env = Scrubbed(os.Environ(), spec.Env...)
	}

	name, args := argv[0], argv[1:]

	if len(spec.Writable) > 0 && Enabled() {
		self, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("could not find this program to confine a command with: %w", err)
		}

		dirs := existing(spec.Writable)

		wrapped := []string{helperArg, strconv.Itoa(len(dirs))}
		wrapped = append(wrapped, dirs...)
		wrapped = append(wrapped, "--")
		wrapped = append(wrapped, argv...)

		name, args = self, wrapped
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = spec.Dir
	cmd.Env = env
	/*
	 * Its own process group, so that stopping it stops what it started.
	 *
	 * A build tool starts compilers, a game engine starts a renderer, and
	 * killing only the program that was launched leaves those behind holding
	 * the project open. How a group is made and how it is killed differ
	 * between systems — see ownGroup and killGroup — and writing the Unix
	 * form here is what kept this program from compiling for Windows.
	 */
	OwnGroup(cmd)

	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}

		return KillGroup(cmd.Process.Pid)
	}
	cmd.WaitDelay = 3 * time.Second

	return cmd, nil
}

// existing is the folders that exist, made real: a rule on a path that is
// not there cannot be made, and a link is judged where it leads.
func existing(dirs []string) []string {
	var out []string

	seen := map[string]bool{}

	for _, d := range dirs {
		if d == "" {
			continue
		}

		real, err := filepath.EvalSymlinks(d)
		if err != nil {
			continue
		}

		if !seen[real] {
			seen[real] = true
			out = append(out, real)
		}
	}

	sort.Strings(out)

	return out
}

/*
 * Clean is an environment with nothing of this program's in it: where to find
 * programs, whose home it is, the language, and the XDG folders when somebody
 * has pointed them elsewhere. No display, no bus, no SSH agent, no keys.
 */
func Clean(extra ...string) []string {
	home, _ := os.UserHomeDir()

	path := os.Getenv("PATH")
	if path == "" {
		path = "/usr/local/bin:/usr/bin:/bin"
	}

	if home != "" {
		local := filepath.Join(home, ".local", "bin")

		if !strings.Contains(":"+path+":", ":"+local+":") {
			path = local + ":" + path
		}
	}

	env := []string{"PATH=" + path, "HOME=" + home, "TERM=dumb",
		"LANG=" + orElse(os.Getenv("LANG"), "C.UTF-8")}

	for _, name := range []string{"USER", "LOGNAME", "TMPDIR", "XDG_CONFIG_HOME", "XDG_DATA_HOME",
		"XDG_CACHE_HOME", "GOPATH", "GOCACHE", "GOMODCACHE", "LC_ALL"} {
		if v := os.Getenv(name); v != "" {
			env = append(env, name+"="+v)
		}
	}

	return merge(env, extra)
}

// credential is a variable name that carries a secret, by the names people
// give them.
var credential = regexp.MustCompile(`(?i)(TOKEN|SECRET|PASSWORD|PASSWD|API_?KEY|ACCESS_?KEY|PRIVATE_?KEY|CREDENTIAL|AUTH)` +
	`|^(ANTHROPIC|OPENAI|CLAUDE_CODE|CODEX|CURSOR|GEMINI|GOOGLE_API|AWS|AZURE|GH|GITHUB)_`)

// Scrubbed is an environment with every credential-looking variable taken
// out, and extra added.
func Scrubbed(env []string, extra ...string) []string {
	var out []string

	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")

		if credential.MatchString(name) {
			continue
		}

		out = append(out, kv)
	}

	return merge(out, extra)
}

// merge adds extra to env, extra winning for a name in both.
func merge(env, extra []string) []string {
	if len(extra) == 0 {
		return env
	}

	given := map[string]bool{}

	for _, kv := range extra {
		name, _, _ := strings.Cut(kv, "=")
		given[name] = true
	}

	out := make([]string, 0, len(env)+len(extra))

	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")

		if !given[name] {
			out = append(out, kv)
		}
	}

	return append(out, extra...)
}

func orElse(v, fallback string) string {
	if v == "" {
		return fallback
	}

	return v
}

/*
 * ForWork is the folders work on a project may write in: the project's own,
 * the temporary folder, the device files, and the caches the tools it runs
 * keep in the home folder — npm's, Go's, Godot's and Unity's settings and
 * caches. Not the home folder itself, not another project, not the brain.
 */
func ForWork(project ...string) []string {
	home, _ := os.UserHomeDir()

	dirs := append([]string{}, project...)
	dirs = append(dirs, os.TempDir(), "/dev", "/var/tmp")

	if home == "" {
		return dirs
	}

	for _, rel := range []string{".cache", ".npm", "go", ".config/godot", ".local/share/godot",
		".config/unity3d", ".local/share/unity3d", ".cargo/registry", ".local/share/uv"} {
		dirs = append(dirs, filepath.Join(home, rel))
	}

	for _, v := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"} {
		if d := os.Getenv(v); d != "" {
			dirs = append(dirs, filepath.Join(d, "godot"))
		}
	}

	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		dirs = append(dirs, d)
	}

	return dirs
}

type writableKey struct{}

// WithWritable is ctx carrying where work in it may write.
func WithWritable(ctx context.Context, dirs []string) context.Context {
	return context.WithValue(ctx, writableKey{}, append([]string{}, dirs...))
}

// WritableFrom is where work in ctx may write, when it was said.
func WritableFrom(ctx context.Context) ([]string, bool) {
	dirs, ok := ctx.Value(writableKey{}).([]string)

	return dirs, ok && len(dirs) > 0
}
