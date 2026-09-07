// Package selfupdate is the program knowing whether it is out of date.
//
// It is built from source rather than downloaded, which decides the whole
// shape of this: there is no release to fetch and swap, there is a repository
// this copy came from and a commit it was built at. So "is there an update" is
// a question about commits, and "update it" is a pull and a build.
//
// Written honestly about that rather than pretending to be a packaged app. A
// program that says "up to date" because it never looked is worse than one
// that says it cannot tell.
package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// Where this program lives, for asking what has been published since.
const (
	Owner = "pnscripts"
	Repo  = "pn-scripts-assistant"
)

// State is what is known about this copy and the newest one.
type State struct {
	// Running is the commit this binary was built from, short. Empty when the
	// build carries no version control information at all, which happens when
	// it was built from a directory that is not a checkout.
	Running string `json:"running"`

	// Dirty is true when it was built from a working tree with uncommitted
	// changes — which is normal here, and means "newer than any commit".
	Dirty bool `json:"dirty,omitempty"`

	// Built is when, which is the only useful thing to show when there is no
	// revision.
	Built string `json:"built,omitempty"`

	// Latest is the newest commit on the default branch.
	Latest string `json:"latest,omitempty"`

	// Behind is how many commits this copy is missing, when that could be
	// worked out. Zero with Newer false means up to date.
	Behind int `json:"behind,omitempty"`

	Newer bool `json:"newer"`

	// Why explains anything that could not be determined, in a sentence.
	Why string `json:"why,omitempty"`

	// Where is the checkout this could be updated from, when there is one.
	Where string `json:"where,omitempty"`
}

/*
 * Running reads what this binary was built from.
 *
 * From the build information Go embeds rather than a constant somebody has to
 * remember to bump — a version number that is edited by hand is a version
 * number that is wrong, and it is wrong in the direction that matters: it
 * claims to be newer than it is.
 */
func Running() State {
	s := State{}

	info, readable := debug.ReadBuildInfo()
	if !readable {
		s.Why = "this build carries no version information"

		return s
	}

	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if len(setting.Value) >= 7 {
				s.Running = setting.Value[:7]
			}

		case "vcs.modified":
			s.Dirty = setting.Value == "true"

		case "vcs.time":
			if len(setting.Value) >= 10 {
				s.Built = setting.Value[:10]
			}
		}
	}

	if s.Running == "" {
		s.Why = "this copy was built from a folder that is not a checkout, " +
			"so there is nothing to compare against"
	}

	return s
}

/*
 * Check asks what has been published and compares.
 *
 * A built-from-a-dirty-tree copy is never behind: it contains work that is not
 * in any commit, and telling somebody to pull over their own changes is the
 * one answer that can lose work.
 */
func Check(ctx context.Context, client *http.Client) State {
	s := Running()
	s.Where = checkoutHere()

	if s.Running == "" {
		return s
	}

	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	latest, err := newestCommit(ctx, client, s.Where)
	if err != nil {
		s.Why = err.Error()

		return s
	}

	s.Latest = latest

	if s.Dirty {
		s.Why = "this copy has changes that are not in any commit, so it is " +
			"ahead of what is published rather than behind it"

		return s
	}

	if strings.HasPrefix(latest, s.Running) || strings.HasPrefix(s.Running, latest) {
		return s
	}

	s.Newer = true
	s.Behind = behind(ctx, client, s.Running)

	return s
}

/*
 * newestCommit is the tip of the published branch.
 *
 * Asked of git first, and this is not an optimisation. The repository is
 * private, so the public API answers 404 and the honest report becomes "cannot
 * tell" for the one thing somebody most wants told. Git already has the
 * credentials — the same key that cloned it — and it is asking the same server
 * the update would pull from, which makes it the more truthful answer as well
 * as the one that works.
 *
 * The API is the fallback, for a copy that was moved away from its checkout
 * and belongs to a repository anybody can read.
 */
func newestCommit(ctx context.Context, client *http.Client, where string) (string, error) {
	if where != "" {
		out, err := git(ctx, where, "ls-remote", "origin", "HEAD")
		if err == nil {
			if sha, _, found := strings.Cut(strings.TrimSpace(out), "\t"); found && len(sha) >= 7 {
				return sha[:7], nil
			}
		}
	}

	var body struct {
		SHA string `json:"sha"`
	}

	if err := ask(ctx, client,
		fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/HEAD", Owner, Repo),
		&body); err != nil {
		return "", err
	}

	if len(body.SHA) < 7 {
		return "", fmt.Errorf("could not read what has been published")
	}

	return body.SHA[:7], nil
}

/*
 * behind counts the commits between this copy and the newest.
 *
 * A number rather than "there is an update", because they mean different
 * things to somebody deciding whether to interrupt what they are doing. Zero
 * when it cannot be worked out, and the caller says "an update" instead.
 */
func behind(ctx context.Context, client *http.Client, from string) int {
	// Counted locally when there is a checkout, for the same reason the tip is
	// asked of git: a private repository answers nothing to an anonymous API.
	if where := checkoutHere(); where != "" {
		if _, err := git(ctx, where, "fetch", "--quiet", "origin"); err == nil {
			out, err := git(ctx, where, "rev-list", "--count", from+"..origin/HEAD")

			if err == nil {
				if n, err := strconv.Atoi(strings.TrimSpace(out)); err == nil {
					return n
				}
			}
		}
	}

	var body struct {
		BehindBy int `json:"behind_by"`
	}

	if err := ask(ctx, client, fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/compare/%s...HEAD", Owner, Repo, from), &body); err != nil {
		return 0
	}

	return body.BehindBy
}

func ask(ctx context.Context, client *http.Client, url string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach the place this program is published: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("asking what is published answered %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(into)
}

/*
 * checkoutHere finds the source this copy could be rebuilt from.
 *
 * Beside the running binary and upwards, because that is where it is: the
 * program is built into dist/ inside its own checkout. Returns empty when
 * there is none, which is the honest answer for a copy somebody moved
 * somewhere else — it can still say an update exists and cannot install it.
 */
func checkoutHere() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}

	at := filepath.Dir(exe)

	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(at, ".git")); err == nil {
			return at
		}

		parent := filepath.Dir(at)

		if parent == at {
			break
		}

		at = parent
	}

	return ""
}

/*
 * Update pulls and rebuilds, and says what it did.
 *
 * Deliberately not clever. It refuses on a tree with local changes rather than
 * stashing them, refuses when the build fails rather than leaving a half
 * replaced binary, and writes the new one beside the old before moving it into
 * place — the same discipline as every other download here, and for the same
 * reason: the thing being replaced is the program doing the replacing.
 */
func Update(ctx context.Context, note func(string)) (string, error) {
	if note == nil {
		note = func(string) {}
	}

	where := checkoutHere()
	if where == "" {
		return "", fmt.Errorf(
			"this copy was not built from a checkout I can find, so I cannot " +
				"update it from here")
	}

	if dirty(ctx, where) {
		return "", fmt.Errorf(
			"there are uncommitted changes in %s. I will not pull over them — "+
				"commit or put them aside first", where)
	}

	note("fetching what has been published")

	if out, err := git(ctx, where, "pull", "--ff-only"); err != nil {
		return "", fmt.Errorf("could not pull: %w\n%s", err, out)
	}

	note("building — this takes a minute")

	/*
	 * Built beside the running binary and moved into place.
	 *
	 * A build straight over the running program leaves it truncated if the
	 * compile fails halfway, and the thing truncated is the thing that would
	 * have to fix it.
	 */
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}

	next := exe + ".new"

	build := exec.CommandContext(ctx, "go", "build", "-C", "clients", "-o", next, "./cmd/brain")
	build.Dir = where
	build.Env = append(os.Environ(), "CGO_ENABLED=1")

	if out, err := build.CombinedOutput(); err != nil {
		os.Remove(next)

		return "", fmt.Errorf("the build failed, so nothing was replaced: %w\n%s",
			err, lastLines(string(out)))
	}

	if err := os.Rename(next, exe); err != nil {
		os.Remove(next)

		return "", fmt.Errorf("could not put the new program in place: %w", err)
	}

	return "Updated. Close and open it again to run the new version.", nil
}

func dirty(ctx context.Context, where string) bool {
	out, err := git(ctx, where, "status", "--porcelain")

	return err == nil && strings.TrimSpace(out) != ""
}

func git(ctx context.Context, where string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = where

	out, err := cmd.CombinedOutput()

	return string(out), err
}

func lastLines(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")

	if len(lines) > 15 {
		lines = lines[len(lines)-15:]
	}

	return strings.Join(lines, "\n")
}
