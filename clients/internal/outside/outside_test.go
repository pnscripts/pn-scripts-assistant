package outside

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

/*
 * The source contains no address that is not on the list.
 *
 * This is the test that makes outside.go worth having. Without it the list is
 * a comment, and a comment goes stale the first time somebody adds a call to
 * somewhere — which, for this program, is the one thing its owner would most
 * want to know about.
 *
 * It reads the source rather than the built program, so what it proves is
 * bounded and worth stating: no *written-down* address escaped the list. An
 * address assembled at runtime from a setting is not here and cannot be; see
 * the package comment for exactly what is and is not covered.
 */
func TestNothingConnectsSomewhereUndeclared(t *testing.T) {
	root := sourceRoot(t)

	allowed := map[string]bool{}

	for _, p := range Places {
		allowed[p.Host] = true
	}

	for _, n := range Namespaces {
		allowed[n] = true
	}

	for _, l := range Local {
		allowed[l] = true
	}

	found := map[string]string{}

	for _, dir := range []string{"internal", "cmd"} {
		walkGo(t, filepath.Join(root, dir), func(path string, line int, text string) {
			for _, host := range hostsIn(text) {
				if allowed[host] {
					continue
				}

				if _, already := found[host]; !already {
					where, _ := filepath.Rel(root, path)
					found[host] = where + ":" + itoa(line)
				}
			}
		})
	}

	for host, where := range found {
		t.Errorf("%s is reached from %s and is not in outside.Places — "+
			"add it with what it is for, or take the call out", host, where)
	}
}

// And the other way round: a host on the list that nothing uses is a list that
// has stopped describing the program. Reported rather than failed, because a
// host can leave the source in one change and the list in the next, and the
// order of those two is not worth failing a build over.
func TestTheListHasNothingLeftOver(t *testing.T) {
	root := sourceRoot(t)

	used := map[string]bool{}

	for _, dir := range []string{"internal", "cmd"} {
		walkGo(t, filepath.Join(root, dir), func(_ string, _ int, text string) {
			for _, host := range hostsIn(text) {
				used[host] = true
			}
		})
	}

	for _, p := range Places {
		// A redirect target is never written down; that is what makes it one.
		if p.Redirect || used[p.Host] {
			continue
		}

		t.Logf("%s is on the list and nothing in the source reaches it", p.Host)
	}
}

// Every entry says what it is for. A list of bare hostnames would answer
// "what does it talk to" and not "why", and the why is the part somebody
// deciding whether to trust this actually needs.
func TestEveryPlaceSaysWhatItIsFor(t *testing.T) {
	seen := map[string]bool{}

	for _, p := range Places {
		if p.Host == "" || strings.TrimSpace(p.For) == "" {
			t.Errorf("%+v does not say what it is for", p)
		}

		if seen[p.Host] {
			t.Errorf("%s is listed twice", p.Host)
		}

		seen[p.Host] = true
	}
}

// sourceRoot is the module directory, found by walking up to go.mod.
func sourceRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		up := filepath.Dir(dir)
		if up == dir {
			t.Fatal("no go.mod above the test")
		}

		dir = up
	}
}

// walkGo reads every line of every non-test Go file under dir that is not a
// comment. Comments are skipped because this package's own documentation, and
// a good deal of the rest of it, discusses addresses without calling them.
func walkGo(t *testing.T, dir string, each func(path string, line int, text string)) {
	t.Helper()

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		for i, line := range strings.Split(string(raw), "\n") {
			trimmed := strings.TrimLeft(line, " \t")

			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") ||
				strings.HasPrefix(trimmed, "/*") {
				continue
			}

			each(path, i+1, line)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// written matches an address inside a string literal, which is the only place
// one can be that means anything.
var written = regexp.MustCompile(`"https?://([a-zA-Z0-9.\-]+|\[[0-9a-fA-F:]+\])`)

func hostsIn(line string) []string {
	var out []string

	for _, m := range written.FindAllStringSubmatch(line, -1) {
		out = append(out, m[1])
	}

	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var digits []byte

	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	return string(digits)
}
