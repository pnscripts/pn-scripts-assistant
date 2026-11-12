package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/vault"
)

/*
 * The lines about what it is doing are said in Bulgarian.
 *
 * The Go code writes each step for a person in English, and the page says it
 * in the page's language by recognising its shape: status.js lists the
 * strings of i18n.js whose English is a shape the Go code writes. Each line
 * below is one the Go code writes, with the file it is written in and a piece
 * of its format, so a change of wording there fails here rather than turning
 * the Bulgarian page English without anybody noticing.
 */
func TestEveryStepLineHasItsBulgarian(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))
	status := readAsset(t, filepath.Join("js", "status.js"))
	cyrillic := regexp.MustCompile(`[а-яА-Я]`)

	list := regexp.MustCompile(`(?s)const STEP_KEYS = \[(.*?)\];`).FindStringSubmatch(status)
	if list == nil {
		t.Fatal("status.js has no STEP_KEYS")
	}

	type shape struct {
		key     string
		pattern *regexp.Regexp
	}

	var shapes []shape

	for _, m := range regexp.MustCompile(`'([\w.]+)'`).FindAllStringSubmatch(list[1], -1) {
		en, bg := strings2(t, js, m[1])
		if !cyrillic.MatchString(bg) {
			t.Errorf("%s has no Bulgarian in it: %q", m[1], bg)
		}

		var pieces []string
		for _, piece := range regexp.MustCompile(`\{\d\}`).Split(en, -1) {
			pieces = append(pieces, regexp.QuoteMeta(piece))
		}

		shapes = append(shapes, shape{m[1], regexp.MustCompile("^" + strings.Join(pieces, "(.+?)") + "$")})

		if strings.Count(en, "{") != strings.Count(bg, "{") {
			t.Errorf("%s does not put every value in both languages: %q, %q", m[1], en, bg)
		}
	}

	lines := []struct{ line, file, format string }{
		{"Thinking", "agent/agent.go", `l.doing("thinking", "Thinking")`},
		{"Doing it", "agent/agent.go", `"Doing it"`},
		{"Answering", "agent/agent.go", `"Answering"`},
		{"Waiting for you: Write notes.txt", "agent/agent.go", `"Waiting for you: "+summary`},
		{"Approved: Write notes.txt", "progress/decided.go", `Approved: "Approved: "`},
		{"Refused: Write notes.txt", "progress/decided.go", `Refused:  "Refused: "`},
		{"Started by itself: Ask Claude Code", "progress/decided.go", `ByItself: "Started by itself: "`},
		{"qwen3:4b · reading 2.4k words", "agent/agent.go", `"%s · reading %s"`},
		{"12 words in 3.1s", "agent/agent.go", `"%d words in %s"`},
		{"120 words in 1m 04s · 1.9 a second", "agent/agent.go", `"%d words in %s · %.1f a second"`},
		{"nothing back in 80ms", "agent/agent.go", `"nothing back"`},
		{"14 lines back in 1.2s", "agent/agent.go", `"%d lines back"`},
		{"failed: permission denied", "agent/agent.go", `"failed: "`},
		{"Listening", "speech/server.go", `"Listening"`},
		{"Making out the words", "speech/server.go", `"Making out the words"`},
		{"Speaking", "speech/aloud.go", `"Speaking"`},
		{"still talking, waiting for the rest", "speech/server.go", `"still talking, waiting for the rest"`},
		{"joined 3 pieces", "speech/server.go", `"joined "`},
		{"steady noise, not a voice, so ignored", "speech/server.go", `"steady noise, not a voice, so ignored"`},
		{"you cut in, so my own voice was dropped first", "speech/server.go", `"you cut in, so my own voice was dropped first"`},
		{"2.3s of speech · peak 812 · room 40", "speech/server.go", `"%.1fs of speech · peak %d · room %d"`},
		{"that was the film, not you, so ignored", "speech/server.go", `"that was the film, not you, so ignored"`},
		{"that was my own voice, so ignored", "speech/server.go", `"that was my own voice, so ignored"`},
		{"heard: good morning", "speech/server.go", `"heard: "`},
		{"no words made out", "speech/server.go", `"no words made out"`},
		{"kept the recording: turn-12.wav", "speech/server.go", `"kept the recording: "`},
		{"unfamiliar: Petar", "speech/server.go", `"unfamiliar: "`},
		{"Learning from the last conversation", "learning/worker.go", `"Learning from the last conversation"`},
		{"Reading /home/you/notes.txt", "brain/learn_intent.go", `"Reading "+target.Path`},
		{"Learning from notes: 3000 things, about 50 minutes", "brain/learn_intent.go", `"Learning from %s: %d things, about %d minutes"`},
		{"Learning from notes: 12 of 3000", "brain/learn_intent.go", `"Learning from %s: %d of %d"`},
		{"Reading example.org/page: passage 2 of 9", "brain/learn_intent.go", `"Reading %s: passage %d of %d"`},
		{"Going through the waiting list: 4 of 90", "brain/intent.go", `"Going through the waiting list: %d of %d"`},
		{"Reading Projects", "brain/placewatch.go", `"Reading "+p.Name`},
		{"Could not read Projects", "brain/placewatch.go", `"Could not read "+p.Name`},
		{"1 new thing from Projects, 40 still to read", "brain/placewatch.go", `"%s from %s, %d still to read"`},
		{"12 new things from Projects, 40 still to read", "brain/placewatch.go", `"%d new things"`},
		{"1 new thing from Projects, and that is all of it", "brain/placewatch.go", `", and that is all of it"`},
		{"12 new things from Projects, and that is all of it", "brain/placewatch.go", `", and that is all of it"`},
		{"Testing qwen3:4b", "server/server.go", `"Testing "+body.Name`},
		{"Re-indexing memories with nomic-embed-text: 40 of 900", "server/server.go", `"Re-indexing memories with %s: %d of %d"`},
		{"Fetching what tells voices apart", "server/voiceprint.go", `"Fetching what tells voices apart"`},
		{"Listening to film.mkv", "tools/subtitles.go", `"Listening to "`},
		{"reading the Godot reference for signals", "tools/games.go", `"reading the " + adapter.Title() + " reference for "`},
		{"reading the Node2D reference", "tools/godot.go", `"reading the " + a.Class + " reference"`},
		{"checking tetris", "tools/games.go", `"checking "`},
		{"running tetris for a few seconds", "tools/games.go", `" for a few seconds"`},
		{"building tetris", "tools/games.go", `"building "`},
		{"exporting tetris", "tools/godot.go", `"exporting "`},
		{"running tetris", "tools/godot.go", `"running "`},
		{"Claude Code is working in /home/you/tetris", "brain/codingagents.go", `" is working in "`},
	}

	for _, l := range lines {
		raw, err := os.ReadFile(filepath.Join("..", l.file))
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(string(raw), l.format) {
			t.Errorf("%s no longer writes %s: say the new line here and in i18n.js", l.file, l.format)
		}

		said := false

		for _, s := range shapes {
			said = said || s.pattern.MatchString(l.line)
		}

		if !said {
			t.Errorf("%q, which %s writes, has no string in i18n.js the page can say it with", l.line, l.file)
		}
	}
}

// Where the key is kept is named by a string of i18n.js, for every keyring.
func TestTheRecoveryNoticeSaysWhereTheKeyIsInBothLanguages(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))
	cyrillic := regexp.MustCompile(`[а-яА-Я]`)

	file := vault.FileKeyring{Dir: "/home/you/.config/pn-scripts-assistant"}

	cases := []struct{ name, key, dir string }{
		{file.Name(), "vault.keyring.file", file.Dir},
		{"this computer's keyring (Secret Service)", "vault.keyring.secret", ""},
		{"the macOS Keychain", "vault.keyring.macos", ""},
		{"Windows (DPAPI, for your account on this computer)", "vault.keyring.windows", ""},
		{(&vault.MemoryKeyring{}).Name(), "vault.keyring.memory", ""},
	}

	for _, c := range cases {
		key, dir := keyringWords(c.name)
		if key != c.key || dir != c.dir {
			t.Errorf("%q is named %q (%q), want %q (%q)", c.name, key, dir, c.key, c.dir)
		}

		en, bg := strings2(t, js, key)
		if !cyrillic.MatchString(bg) {
			t.Errorf("%s has no Bulgarian in it: %q", key, bg)
		}

		if said := strings.ReplaceAll(en, "{dir}", c.dir); said != c.name {
			t.Errorf("%s says %q in English, the vault %q", key, said, c.name)
		}
	}

	// The names the vault gives on each system, read from its source so a
	// change there is a change here.
	for _, file := range []string{"keyring_linux.go", "keyring_darwin.go", "keyring_windows.go"} {
		raw, err := os.ReadFile(filepath.Join("..", "vault", file))
		if err != nil {
			t.Fatal(err)
		}

		name := regexp.MustCompile(`Name\(\) string\s*\{ return "([^"]+)" \}`).FindStringSubmatch(string(raw))
		if name == nil {
			t.Fatalf("%s names its keyring some other way now", file)
		}

		if key, _ := keyringWords(name[1]); key == "" {
			t.Errorf("%q (%s) has no string in i18n.js", name[1], file)
		}
	}

	if got := warningWords(file.Warning()); got != "vault.warning.file" {
		t.Errorf("the key file's warning is named %q", got)
	}

	if en, bg := strings2(t, js, "vault.warning.file"); en != file.Warning() || !cyrillic.MatchString(bg) {
		t.Errorf("vault.warning.file says %q, the vault %q", en, file.Warning())
	}
}
