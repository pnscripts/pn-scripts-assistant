package brain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/autonomy"
	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/provision"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tasks"
)

/*
 * A whole goal, end to end, on a brain of its own (ASSISTANT-GOALMODE-NEXT 4).
 *
 * "Make me a space strategy game" with goal mode on and privacy set to keep
 * everything on this machine. Nothing real is downloaded and no real model
 * answers: a fake Ollama is both the model library (pull, with the checksum
 * line Ollama prints) and the models (chat, answering each call by what it is
 * asked), and a fake tool source serves a toolkit whose published checksum
 * the recipe checks. Everything between them is the program itself: the
 * planner, the specialists and their tools, the router and goal mode's
 * downloads, the goal's folder, the originality checks and the report.
 *
 * What it proves, in the order a goal goes through it:
 *
 *   plan → specialists → provisioning (a model and a tool, both checked and
 *   recorded) → the work written in the goal's own folder → the originality
 *   step, only for the aspects the result has → one final report that says
 *   what was made, what was checked and what was put on the computer — and
 *   nobody was asked anything.
 */

// fakeLibrary is Ollama: a library to pull from and the models that answer.
type fakeLibrary struct {
	mu        sync.Mutex
	root      string
	installed map[string]string
	pulled    []string
	chats     int
	folder    string
	results   []string
}

var goalFolderIn = regexp.MustCompile(`[^\s"'` + "`" + `]*/goals/\d+-[a-z0-9-]+`)

func (f *fakeLibrary) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch r.URL.Path {
	case "/api/tags":
		var list []map[string]any
		for name, digest := range f.installed {
			list = append(list, map[string]any{"name": name, "model": name, "digest": digest, "size": 2 << 30,
				"details": map[string]any{"parameter_size": "3B"}})
		}

		json.NewEncoder(w).Encode(map[string]any{"models": list})
	case "/api/ps":
		json.NewEncoder(w).Encode(map[string]any{"models": []any{}})
	case "/api/version":
		json.NewEncoder(w).Encode(map[string]any{"version": "0.12.0"})
	case "/api/show":
		// What Ollama says about a model: its size from its tag, and a code
		// model's fill-in-the-middle.
		var body struct{ Name, Model string }
		json.NewDecoder(r.Body).Decode(&body)

		name := body.Name + body.Model
		size := "3B"

		if _, tag, ok := strings.Cut(name, ":"); ok {
			size = strings.ToUpper(tag)
		}

		capabilities := []string{"completion", "tools"}
		if strings.Contains(name, "coder") {
			capabilities = append(capabilities, "insert")
		}

		json.NewEncoder(w).Encode(map[string]any{"capabilities": capabilities,
			"details": map[string]any{"parameter_size": size, "family": "fake"}})
	case "/api/pull":
		var body struct {
			Name, Model string
		}
		json.NewDecoder(r.Body).Decode(&body)

		name := body.Name + body.Model
		sum := sha256.Sum256([]byte(name))

		for _, line := range []string{`{"status":"pulling manifest"}`, `{"status":"verifying sha256 digest"}`,
			`{"status":"writing manifest"}`, `{"status":"success"}`} {
			fmt.Fprintln(w, line)
		}

		f.installed[name] = "sha256:" + hex.EncodeToString(sum[:])
		f.pulled = append(f.pulled, name)
	case "/api/embeddings", "/api/embed":
		json.NewEncoder(w).Encode(map[string]any{"embedding": []float64{0.1, 0.2, 0.3}, "embeddings": [][]float64{{0.1, 0.2, 0.3}}})
	case "/api/chat":
		f.chats++
		f.chat(w, r)
	default:
		http.NotFound(w, r)
	}
}

type chatRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

// chat answers each call by what it is: the plan, a step's work, a check of
// a step, an originality check.
func (f *fakeLibrary) chat(w http.ResponseWriter, r *http.Request) {
	var req chatRequest
	json.NewDecoder(r.Body).Decode(&req)

	var all strings.Builder

	lastUser, usedTool := "", false

	for _, m := range req.Messages {
		all.WriteString(m.Content + "\n")

		switch m.Role {
		case "user":
			lastUser = m.Content
		case "tool":
			usedTool = true
			f.results = append(f.results, oneLineOf(m.Content, 200))
		}
	}

	text := all.String()

	if f.folder == "" {
		if found := goalFolderIn.FindString(text); found != "" {
			f.folder = found
		}
	}

	say := func(content string, calls ...map[string]any) {
		msg := map[string]any{"role": "assistant", "content": content}
		if len(calls) > 0 {
			msg["tool_calls"] = calls
		}

		json.NewEncoder(w).Encode(map[string]any{"model": req.Model, "message": msg, "done": true})
	}

	// The step this call is for: "Do only this step, and nothing beyond it:"
	// in the step prompt, or "Your task:" in a project's writing prompt.
	step := lastUser
	for _, marker := range []string{"Do only this step, and nothing beyond it:", "Your task:"} {
		if _, after, ok := strings.Cut(lastUser, marker); ok {
			step = after

			break
		}
	}

	call := func(name string, args map[string]any) map[string]any {
		return map[string]any{"function": map[string]any{"name": name, "arguments": args}}
	}

	switch {
	case strings.Contains(text, "Break the job below into steps"):
		say(`{"name":"Space strategy game","done_when":"a playable prototype and its design are in the goal's folder",
"steps":[
 {"do":"Install spacekit, the toolkit this game is built with","kind":"do","who":"assistant"},
 {"do":"Write the game design: the core rule of drifting star lanes that close behind every fleet","kind":"do","who":"writer","changes":true},
 {"do":"Build the playable prototype in main.js","kind":"do","who":"developer","changes":true},
 {"do":"Compose the music and sound effects as code","kind":"do","who":"music_producer","changes":true}]}`)
	case strings.Contains(text, "Decide whether one step of a job"):
		evidence := ""
		if _, after, ok := strings.Cut(text, "What the tools returned:\n"); ok {
			for _, line := range strings.Split(after, "\n") {
				if line = strings.TrimSpace(line); len(line) > 6 {
					evidence = line

					break
				}
			}
		}

		raw, _ := json.Marshal(evidence)
		say(`{"met":true,"evidence":` + string(raw) + `,"why":"the tool returned it"}`)
	case usedTool:
		say("Done: what the step asked for is written and checked.")
	case strings.Contains(step, "Install spacekit"):
		say("", call("install_requirement", map[string]any{"recipe": "spacekit"}))
	case strings.Contains(step, "Write the game design"):
		say("", call("write_file", map[string]any{"path": filepath.Join(f.folder, "DESIGN.md"),
			"content": "# Lanecraft\n\nStar lanes close behind every fleet that uses them, so each move reshapes the map.\n"}))
	case strings.Contains(step, "Build the playable prototype"):
		say("", call("write_file", map[string]any{"path": filepath.Join(f.folder, "main.js"),
			"content": "// Lanecraft prototype\nconst lanes = new Map();\nexport function move(fleet, lane) { lanes.delete(lane); return fleet; }\n"}))
	case strings.Contains(step, "Compose the music"):
		say("", call("write_file", map[string]any{"path": filepath.Join(f.folder, "music.js"),
			"content": "// An original motif in D dorian, written for this game\nexport const motif = [62, 65, 69, 67, 64, 60];\n"}))
	case strings.Contains(step, tasks.OriginalityMarker):
		say("OK: compared with the three closest known works; the core rule, the look and the motif are this game's own.")
	default:
		say("Done.")
	}
}

// goalBrain is a brain in a folder of its own, its home and settings in
// another, with goal mode agreed to and privacy keeping everything here.
func goalBrain(t *testing.T, ollamaURL string, log io.Writer) *Brain {
	t.Helper()

	scratch := t.TempDir()

	t.Setenv("HOME", filepath.Join(scratch, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(scratch, "config"))
	t.Setenv("OLLAMA_MODELS", filepath.Join(scratch, "models"))
	t.Setenv("PN_SCRIPTS_ASSISTANT_AGENT_SKILLS", "-")

	for _, dir := range []string{"home", "config", "models", "brain"} {
		os.MkdirAll(filepath.Join(scratch, dir), 0o755)
	}

	root := filepath.Join(scratch, "brain")

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	cfg := config.Default()
	cfg.Owner = "Alex"
	cfg.OllamaURL = ollamaURL
	cfg.Privacy = "private"
	cfg.GoalMode = true

	return New(db, cfg, root, filepath.Join(root, "brain.sqlite"), slog.New(slog.NewTextHandler(log, nil)))
}

func TestAGoalGoesFromPlanToReportOnItsOwn(t *testing.T) {
	library := &fakeLibrary{installed: map[string]string{}}

	ollama := httptest.NewServer(http.HandlerFunc(library.serve))
	defer ollama.Close()

	// The toolkit, as its official source serves it, with its checksum.
	kit := []byte("#!/bin/sh\necho spacekit 1.4.2\n")
	sum := sha256.Sum256(kit)
	published := hex.EncodeToString(sum[:])

	toolSource := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(kit) }))
	defer toolSource.Close()

	var log bytes.Buffer

	b := goalBrain(t, ollama.URL, &log)

	bin := filepath.Join(b.Root, "..", "home", ".local", "bin", "spacekit")

	spacekit := provision.Recipe{
		ID: "spacekit", Title: "Spacekit", Kind: provision.Tool, Why: "builds the game",
		Source: "spacekit.example (official releases)", Size: "1MB",
		Find: func() (string, string, bool) {
			if _, err := os.Stat(bin); err != nil {
				return "", "", false
			}

			return bin, "1.4.2", true
		},
		Install: func(ctx context.Context, w io.Writer) error {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, toolSource.URL+"/spacekit-1.4.2", nil)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			got, _ := io.ReadAll(resp.Body)
			if s := sha256.Sum256(got); hex.EncodeToString(s[:]) != published {
				return fmt.Errorf("checksum does not match the published one")
			}

			fmt.Fprintln(w, "checksum matches the published one")
			os.MkdirAll(filepath.Dir(bin), 0o755)

			return os.WriteFile(bin, got, 0o755)
		},
		Remove: func(io.Writer) error { return os.Remove(bin) },
	}

	b.recipesOnce.Do(func() { b.recipes = provision.NewBook(spacekit) })

	conv, err := b.DB.NewConversation("goal")
	if err != nil {
		t.Fatal(err)
	}

	started := time.Now()

	task, ok, err := b.Tasks.Take(context.Background(), conv, "Make me a space strategy game", "ollama", true)
	if err != nil || !ok {
		t.Fatalf("the goal did not start: %v %v\n%s", ok, err, log.String())
	}

	done := waitForTask(t, b.DB, task.ID, 90*time.Second)

	steps, _ := b.DB.Steps(task.ID)

	describe := func() string {
		var s strings.Builder
		fmt.Fprintf(&s, "task %q state %s project %q\n", done.Name, done.State, done.Project)

		for _, st := range steps {
			fmt.Fprintf(&s, "- [%s] %s → %s: %s\n", st.State, st.Assignee, oneLineOf(st.Instruction, 90), oneLineOf(st.Answer, 160))
		}

		fmt.Fprintf(&s, "report:\n%s\n", done.Report)

		for _, r := range library.results {
			fmt.Fprintf(&s, "  tool said: %s\n", r)
		}

		for id := task.ID; id < task.ID+6; id++ {
			if rows, err := b.DB.EvidenceFor(id); err == nil {
				for _, e := range rows {
					fmt.Fprintf(&s, "  evidence task %d step %d %s %s ok=%v: %s\n", id, e.StepID, e.Kind, oneLineOf(e.Subject, 60), e.OK, oneLineOf(e.Detail, 200))
				}
			}
		}

		return s.String()
	}

	if done.State != store.TaskDone {
		t.Fatalf("the goal did not finish:\n%s\nlog:\n%s", describe(), lastLogLines(log.String(), 60))
	}

	// Plan → specialists: each step went to the specialist the plan named.
	who := map[string]string{}
	for _, st := range steps {
		who[oneLineOf(st.Instruction, 30)] = st.Assignee
	}

	for _, want := range []string{"assistant", "writer", "developer", "music_producer"} {
		if !strings.Contains(fmt.Sprint(who), want) {
			t.Errorf("no step went to %s:\n%s", want, describe())
		}
	}

	// Provisioning: a model pulled because none was here (with privacy set to
	// this computer only — the consent says downloads still happen), and the
	// toolkit installed from its source with its checksum; both recorded.
	if len(library.pulled) == 0 {
		t.Errorf("no model was downloaded:\n%s", describe())
	}

	ledger := autonomy.LedgerAt(b.Root)
	if !ledger.Mine("tool", "Spacekit") || !ledger.Mine("model", library.pulled[0]) {
		t.Errorf("the ledger does not record what was put here: %+v", ledger.Entries())
	}

	// Workspace: the work is in the goal's own folder, inside the brain.
	if !strings.HasPrefix(done.Project, filepath.Join(b.Root, "goals")+string(filepath.Separator)) {
		t.Errorf("the goal worked in %q, not a folder of its own", done.Project)
	}

	for _, file := range []string{"DESIGN.md", "main.js", "music.js"} {
		if _, err := os.Stat(filepath.Join(done.Project, file)); err != nil {
			t.Errorf("%s was not written in the goal's folder: %v\n%s", file, err, describe())
		}
	}

	// The originality step: look, sound (a step made music) and game rules,
	// side by side — no business strategist for a "strategy" game.
	var checks []string
	for _, st := range steps {
		if strings.HasPrefix(st.Instruction, tasks.OriginalityMarker) {
			checks = append(checks, st.Assignee)

			if st.State != store.StepDone {
				t.Errorf("check by %s is %s", st.Assignee, st.State)
			}
		}
	}

	if strings.Join(checks, ",") != "visual_reviewer,sound_reviewer,game_design_reviewer" {
		t.Errorf("checked by %v:\n%s", checks, describe())
	}

	// One report: the checks, and what was put on the computer.
	for _, want := range []string{tasks.OriginalitySection, "Prepared for this job:", "Spacekit", library.pulled[0],
		"as agreed for goals worked on alone"} {
		if !strings.Contains(done.Report, want) {
			t.Errorf("the report does not say %q:\n%s", want, done.Report)
		}
	}

	// And nobody was asked anything.
	waiting, _ := b.DB.PendingInvocations()
	if len(waiting) > 0 {
		t.Errorf("%d things wait for the owner: %+v", len(waiting), waiting)
	}

	if since := ledger.Since(started); len(since) < 2 {
		t.Errorf("prepared since the goal started: %+v", since)
	}

	// The whole run, for anybody reading the test with -v.
	t.Logf("%s", describe())
}

// waitForTask waits for a task to settle, finished or not.
func waitForTask(t *testing.T, db *store.DB, id int64, most time.Duration) *store.Task {
	t.Helper()

	deadline := time.Now().Add(most)

	for {
		task, err := db.Task(id)
		if err != nil {
			t.Fatal(err)
		}

		switch task.State {
		case store.TaskDone, store.TaskBlocked, store.TaskStopped:
			return task
		case store.TaskWaiting:
			// Waiting on its own specialists is work; waiting on the owner
			// is a question goal mode should not have asked.
			if pending, _ := db.PendingInvocations(); len(pending) > 0 {
				return task
			}
		}

		if time.Now().After(deadline) {
			t.Fatalf("the goal is still %q after %s", task.State, most)
		}

		time.Sleep(20 * time.Millisecond)
	}
}

func oneLineOf(s string, most int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > most {
		return string(r[:most]) + "…"
	}

	return s
}

func lastLogLines(log string, n int) string {
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return strings.Join(lines, "\n")
}
