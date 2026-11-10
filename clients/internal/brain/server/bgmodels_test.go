package server

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/models"
	"pn-scripts-assistant/internal/brain/pace"
	"pn-scripts-assistant/internal/preflight"
)

/*
 * What the server says on Models, Operations and What it runs on, in both
 * languages.
 *
 * Those pages show sentences the server writes: what each part is for and
 * what is lost without it, the verdict on each model size, the note on each
 * row of What it runs on, the advice on how fast answering is. Each now
 * travels with a key, and these tests hold the key and the English together:
 * every key is in i18n.js, its English is the sentence the server writes
 * (so the two cannot drift apart), and its Bulgarian has Bulgarian in it.
 */

var bulgarianLetters = regexp.MustCompile(`[а-яА-Я]`)

// sameSentence checks that i18n.js says what the server says under key,
// with vars filled in, and says it in Bulgarian too.
func sameSentence(t *testing.T, js, key, server string, vars map[string]string) {
	t.Helper()

	en, bg := strings2(t, js, key)

	for name, value := range vars {
		en = strings.ReplaceAll(en, "{"+name+"}", value)
	}

	if en != server {
		t.Errorf("%s: the server says %q and i18n.js %q", key, server, en)
	}

	if !bulgarianLetters.MatchString(bg) {
		t.Errorf("%s in Bulgarian has no Bulgarian in it: %q", key, bg)
	}

	if strings.ContainsAny(en+bg, "–—") {
		t.Errorf("%s uses a dash as punctuation: %q / %q", key, en, bg)
	}
}

func TestEveryPartIsDescribedInBothLanguages(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))
	seen := map[string]string{}

	for _, r := range preflight.Requirements() {
		key := partKey(r.Name)

		if other, clash := seen[key]; clash {
			t.Errorf("%q and %q share the key %q", r.Name, other, key)
		}

		seen[key] = r.Name

		// A name that is a product's (Ollama) reads the same in Bulgarian.
		if en, bg := strings2(t, js, "parts.name."+key); en != r.Name {
			t.Errorf("parts.name.%s: the server says %q and i18n.js %q", key, r.Name, en)
		} else if bg != en && !bulgarianLetters.MatchString(bg) {
			t.Errorf("parts.name.%s in Bulgarian has no Bulgarian in it: %q", key, bg)
		}
		sameSentence(t, js, "parts.why."+key, r.Why, nil)
		sameSentence(t, js, "parts.without."+key, r.Consequence, nil)

		if r.ManualHint != "" {
			sameSentence(t, js, "parts.hint."+key, r.ManualHint, nil)
		}
	}

	if partKey("Voice (listening)") != "voice_listening" || partKey("What each job does (O*NET)") != "what_each_job_does_o_net" {
		t.Errorf("partKey does not make the keys i18n.js names")
	}
}

func TestWhatTheChecksFindIsSaidInBothLanguages(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	for _, detail := range []string{
		"installed",
		"needs Ollama first",
		"piper (neural)",
		"piper (neural), with voices an older version installed",
		"only half of it is here",
		"answering at http://127.0.0.1:11434",
		"whisper-cli is here but has no speech model",
		"0.12.3 · 0.13.0 available",
	} {
		key, args := partDetailKey(detail)
		if key == "" {
			t.Errorf("%q has no key", detail)

			continue
		}

		sameSentence(t, js, "parts.d."+key, detail, args)
	}

	// Versions and program names are not sentences and stay as they are.
	for _, detail := range []string{"nomic-embed-text", "xdotool", "0.12.3", "wpctl, pw-play"} {
		if key, _ := partDetailKey(detail); key != "" {
			t.Errorf("%q is not a sentence but has the key %q", detail, key)
		}
	}

	// Every sentence a check writes in preflight is one of those above.
	src, err := os.ReadFile(filepath.Join("..", "..", "preflight", "requirements_list.go"))
	if err != nil {
		t.Fatal(err)
	}

	for _, m := range regexp.MustCompile(`return (?:OK|Missing|Outdated|Unknown), "([a-z]+(?: [a-z]+)+[^"]*)"`).FindAllStringSubmatch(string(src), -1) {
		if key, _ := partDetailKey(m[1]); key == "" {
			t.Errorf("a check in preflight says %q, which the page cannot say in Bulgarian", m[1])
		}
	}
}

func TestEveryRowOfWhatItRunsOnIsSaidInBothLanguages(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	parts := []Part{
		brainPart(),
		ollamaPart(ctx, "http://127.0.0.1:1"),
		voicePart(),
		earsPart(),
		cloudPart(true, "claude-opus-5"),
		cloudPart(false, ""),
	}

	parts = append(parts, modelParts(ctx, "http://127.0.0.1:1", "", "")...)

	for _, p := range parts {
		if p.NoteKey == "" {
			t.Errorf("%s: the note %q has no key", p.Name, p.Note)

			continue
		}

		switch p.NoteKey {
		case "whisperNone", "whisperNoModel":
			// The reason is said whole in either language.
			en, _ := strings2(t, js, "parts.note."+p.NoteKey)
			if !strings.HasPrefix(p.Note, strings.SplitN(en, ". ", 2)[0]) {
				t.Errorf("%s: %q does not begin like %q", p.NoteKey, p.Note, en)
			}

		default:
			sameSentence(t, js, "parts.note."+p.NoteKey, strings.TrimSpace(p.Note), p.Args)
		}

		if p.NameKey != "" {
			sameSentence(t, js, "parts.called."+p.NameKey, p.Name, nil)
		}

		status := regexp.MustCompile(`\W+`).ReplaceAllString(p.Status, "_")
		sameSentence(t, js, "parts.status."+status, p.Status, nil)
	}

	// The notes no row above happened to carry on this machine.
	for key, note := range map[string]string{
		"ollama":        "Runs the models on this machine.",
		"ollamaMissing": "Without it the brain cannot think. Install it from ollama.com.",
		"model":         "Downloaded once, runs here, costs nothing to use.",
		"voice":         "The voice.",
		"plainVoice":    "A plainer system voice. Piper sounds considerably better if you install it.",
		"voiceMissing":  "Nothing installed that can speak, so replies are text only.",
		"whisper":       "Hearing you speak.",
	} {
		sameSentence(t, js, "parts.note."+key, note, nil)
	}

	for _, status := range []string{"running", "installed", "answering", "remembering", "configured", "not_found", "not_configured", "unknown"} {
		if _, bg := strings2(t, js, "parts.status."+status); !bulgarianLetters.MatchString(bg) {
			t.Errorf("parts.status.%s has no Bulgarian", status)
		}
	}
}

func TestEveryVerdictOnAModelIsSaidInBothLanguages(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	machines := []models.Power{
		reportedMachine,
		{Cores: 2, RAMBytes: 4 << 30},
		{Cores: 8, RAMBytes: 32 << 30, Accelerated: true, VRAMBytes: 8 << 30},
	}

	seen := map[string]bool{}

	for _, p := range machines {
		for _, gb := range []float64{1, 3, 5, 7, 12, 40, 200} {
			for _, embedding := range []bool{false, true} {
				_, _, verdict, key := judge(gb, embedding, p)
				seen[key] = true

				sameSentence(t, js, "mdl.verdict."+key, verdict, nil)
			}
		}
	}

	if len(seen) != 8 {
		t.Errorf("found %d of the 8 verdicts; are there more now?", len(seen))
	}

	for _, can := range []string{"tools", "vision", "thinking", "embedding"} {
		if _, bg := strings2(t, js, "mdl.can."+can); !bulgarianLetters.MatchString(bg) {
			t.Errorf("mdl.can.%s has no Bulgarian", can)
		}
	}

	// The machine in figures says what Describe says in words.
	for _, p := range machines {
		f := powerFigures(p)
		key := "mdl.machineNoCard"

		if p.Accelerated {
			key = "mdl.machineCard"
		}

		sameSentence(t, js, key, p.Describe(), map[string]string{
			"cores":  figure(f["cores"]),
			"memory": figure(f["memory_gb"]),
			"card":   figure(f["card_gb"]),
		})
	}
}

func figure(v any) string {
	n, _ := v.(int)

	return strconv.Itoa(n)
}

func TestEveryProviderIsDescribedInBothLanguages(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	src, err := os.ReadFile("providers.go")
	if err != nil {
		t.Fatal(err)
	}

	entry := regexp.MustCompile(`Name:\s+"(\w+)",\s+What:\s+((?:"(?:[^"\\]|\\.)*"\s*\+?\s*)+),`)
	piece := regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)

	found := entry.FindAllStringSubmatch(string(src), -1)
	if len(found) < 4 {
		t.Fatalf("found %d providers in providers.go; has the list moved?", len(found))
	}

	for _, m := range found {
		var what strings.Builder

		for _, p := range piece.FindAllStringSubmatch(m[2], -1) {
			what.WriteString(p[1])
		}

		sameSentence(t, js, "mdl.prov."+m[1], what.String(), nil)
	}

	for _, key := range []string{"forbidden", "answered", "silent"} {
		if !regexp.MustCompile(`"why_key":\s+"` + key + `"`).MatchString(string(src)) {
			t.Errorf("providers.go no longer sends the key %s", key)
		}

		if _, bg := strings2(t, js, "mdl.test."+key); !bulgarianLetters.MatchString(bg) {
			t.Errorf("mdl.test.%s has no Bulgarian", key)
		}
	}
}

func TestTheAdviceOnHowFastItAnswersIsSaidInBothLanguages(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	slow := pace.Milestones{Hearing: 900, Thinking: 9000, Speaking: 700, ToFirst: 12000, Model: "qwen3:8b"}

	machines := []pace.Machine{
		{Cores: 4, Working: "qwen3:8b"},
		{Cores: 4, Working: "qwen3:8b", Suggest: "llama3.2:3b"},
		{Cores: 4, Working: "qwen3:8b", Quick: "llama3.2:3b"},
		{Cores: 8, HasGPU: true, GPUName: "a GeForce", Working: "qwen3:8b"},
	}

	keys := map[string]bool{}

	for _, m := range machines {
		for _, f := range pace.Findings(slow, 5, m) {
			keys[f.Key] = true

			sameSentence(t, js, "ops.pace."+f.Key+".because", f.Because, f.Args)
			sameSentence(t, js, "ops.pace."+f.Key+".change", f.Change, f.Args)
		}
	}

	for _, want := range []string{"speaking", "hearing", "noQuick", "fetchQuick", "quick", "card"} {
		if !keys[want] {
			t.Errorf("no finding said %s", want)
		}
	}

	for _, typical := range []pace.Milestones{{ToFirst: 600}, {ToFirst: 2000}, slow} {
		for _, over := range []int{0, 4} {
			key, args := pace.VerdictKey(typical, over)
			sameSentence(t, js, "ops.pace.verdict."+key, pace.Verdict(typical, over), args)
		}
	}
}

func TestTheGraphicsNotesAreSaidInBothLanguages(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))

	src, err := os.ReadFile(filepath.Join("..", "machine", "nvidia.go"))
	if err != nil {
		t.Fatal(err)
	}

	notes := regexp.MustCompile(`return nil, "([^"]+)"`).FindAllStringSubmatch(string(src), -1)
	if len(notes) < 3 {
		t.Fatalf("found %d notes in nvidia.go; has the reader moved?", len(notes))
	}

	for _, m := range notes {
		note := m[1]
		if strings.HasSuffix(note, ": ") {
			note += "it timed out"
		}

		key, why := gpuNoteKey(note)
		if key == "" {
			t.Errorf("the note %q has no key", note)

			continue
		}

		sameSentence(t, js, "ops.gpu."+key, note, map[string]string{"why": why})
	}
}
