package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"pn-brain/internal/brain/brain"
	"pn-brain/internal/brain/config"
	"pn-brain/internal/brain/progress"
	"pn-brain/internal/brain/store"
	"pn-brain/internal/brain/wake"
	"pn-brain/internal/brain/tools"
)

func newServer(t *testing.T) (*httptest.Server, *store.DB, *brain.Brain) {
	t.Helper()

	root := t.TempDir()

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Owner = "Petar"

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := brain.New(db, cfg, root, filepath.Join(root, "brain.sqlite"), logger)

	ts := httptest.NewServer(New(b, logger).Handler())

	t.Cleanup(func() {
		ts.Close()
		db.Close()
	})

	return ts, db, b
}

func TestListenRefusesNonLoopback(t *testing.T) {
	// A brain bound to a public interface works perfectly for its owner while
	// serving everything it knows to the network around it.
	//
	// Port 0 throughout: with a fixed port this test passed for the wrong
	// reason whenever that port happened to be busy, since a refused bind and a
	// refused address are both just an error here. It hid a real bug — ":port"
	// binds to every interface and was being allowed.
	for _, addr := range []string{"0.0.0.0:0", "192.168.1.10:0", ":0", "[::]:0"} {
		ln, err := Listen(addr)

		if err == nil {
			ln.Close()
			t.Errorf("Listen(%q) was allowed", addr)
		}
	}
}

func TestListenAllowsLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:0", "localhost:0", "[::1]:0"} {
		ln, err := Listen(addr)
		if err != nil {
			t.Errorf("Listen(%q) was refused: %v", addr, err)

			continue
		}

		ln.Close()
	}
}

// The full gate, over HTTP: a recorded action does nothing until approved, then
// does exactly what its summary said.
func TestApprovalGateOverHTTP(t *testing.T) {
	ts, db, _ := newServer(t)

	target := filepath.Join(t.TempDir(), "approved.txt")
	args, _ := json.Marshal(map[string]string{"path": target, "content": "written after approval"})

	id, err := db.RecordInvocation(0, "write_file", string(args), "Create "+target, "mutating")
	if err != nil {
		t.Fatal(err)
	}

	// It must appear as waiting, or the owner can never act on it.
	var pending []store.Invocation
	getJSON(t, ts.URL+"/api/approvals", &pending)

	if len(pending) != 1 || pending[0].ID != id {
		t.Fatalf("pending is %+v", pending)
	}

	if _, err := os.Stat(target); err == nil {
		t.Fatal("the file existed before approval")
	}

	// Approve.
	var result map[string]any
	postJSON(t, ts.URL+"/api/approvals/"+itoa(id)+"/approve", &result)

	if e, ok := result["error"]; ok {
		t.Fatalf("approval reported an error: %v", e)
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("approved action did not run: %v", err)
	}

	if string(content) != "written after approval" {
		t.Errorf("file holds %q", content)
	}

	// It must leave the queue, or it could be run twice.
	getJSON(t, ts.URL+"/api/approvals", &pending)

	if len(pending) != 0 {
		t.Errorf("approved action is still pending: %+v", pending)
	}
}

func TestDenialDoesNotRunTheAction(t *testing.T) {
	ts, db, _ := newServer(t)

	target := filepath.Join(t.TempDir(), "denied.txt")
	args, _ := json.Marshal(map[string]string{"path": target, "content": "should never exist"})

	id, _ := db.RecordInvocation(0, "write_file", string(args), "Create "+target, "mutating")

	var result map[string]any
	postJSON(t, ts.URL+"/api/approvals/"+itoa(id)+"/deny", &result)

	if _, err := os.Stat(target); err == nil {
		t.Fatal("a denied action wrote its file")
	}

	// And it cannot be revived by asking again.
	postJSON(t, ts.URL+"/api/approvals/"+itoa(id)+"/approve", &result)

	if _, err := os.Stat(target); err == nil {
		t.Fatal("a denied action was re-approved and ran")
	}
}

// Arguments imported from the old system are not valid JSON. Approving one must
// fail cleanly rather than panic or write something unintended.
func TestCorruptStoredArgumentsFailSafely(t *testing.T) {
	ts, db, _ := newServer(t)

	id, _ := db.RecordInvocation(0, "write_file",
		"map[content:The word is banana path:output.txt]",
		"Create output.txt", "mutating")

	var result map[string]any
	postJSON(t, ts.URL+"/api/approvals/"+itoa(id)+"/approve", &result)

	if _, ok := result["error"]; !ok {
		t.Fatal("corrupt arguments were accepted without error")
	}

	if _, err := os.Stat("output.txt"); err == nil {
		os.Remove("output.txt")
		t.Fatal("a file was written from unreadable arguments")
	}
}

func TestStatusCarriesEverythingTheInterfaceReads(t *testing.T) {
	ts, _, _ := newServer(t)

	var s map[string]any
	getJSON(t, ts.URL+"/api/status", &s)

	for _, key := range []string{"name", "provider", "model", "privacy", "storage", "capabilities", "memory"} {
		if _, ok := s[key]; !ok {
			t.Errorf("status is missing %q", key)
		}
	}

	memory, _ := s["memory"].(map[string]any)

	for _, key := range []string{"facts", "pending_lessons", "conversations"} {
		if _, ok := memory[key]; !ok {
			t.Errorf("status.memory is missing %q", key)
		}
	}

	privacy, _ := s["privacy"].(map[string]any)
	if privacy["mode"] != "private" {
		t.Errorf("default privacy is %v, want private", privacy["mode"])
	}
}

func TestInterfaceIsServedFromTheBinary(t *testing.T) {
	ts, _, _ := newServer(t)

	for _, path := range []string{"/", "/css/console.css", "/js/console.js", "/js/core3d.js"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s returned %d", path, resp.StatusCode)
		}

		if len(body) == 0 {
			t.Errorf("%s was empty", path)
		}
	}

	// The configured name must reach the page, since that is the only reason
	// it is a template rather than a static file.
	resp, _ := http.Get(ts.URL + "/")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(body), "PN Brain") {
		t.Error("the brain's name did not reach the page")
	}
}

// A tool the model wants but privacy or configuration forbids must not appear
// as usable. Here: with no key, Anthropic is not registered at all.
func TestUnconfiguredProviderIsNotOffered(t *testing.T) {
	ts, _, _ := newServer(t)

	var s struct {
		Providers []struct {
			Name string `json:"name"`
		} `json:"providers"`
	}

	getJSON(t, ts.URL+"/api/status", &s)

	for _, p := range s.Providers {
		if p.Name == "anthropic" {
			t.Error("anthropic was offered without an API key")
		}
	}
}

func TestChatRejectsEmptyMessage(t *testing.T) {
	ts, _, _ := newServer(t)

	resp, err := http.Post(ts.URL+"/api/chat", "application/json",
		strings.NewReader(`{"message":"   "}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty message returned %d", resp.StatusCode)
	}
}

// The agent registry must expose exactly the tools that were built, so a tool
// cannot be silently dropped or added.
func TestRegisteredTools(t *testing.T) {
	_, _, b := newServer(t)

	var names []string
	for _, tool := range b.Agent.Registry.All() {
		names = append(names, tool.Name())
	}

	// set_appearance, and now the two queue tools, are here for the same
	// reason: being asked to do something and answering "Understood" without
	// doing it is the failure this list exists to stop. Told to approve
	// everything waiting, the brain said it would and then did nothing at all,
	// because it had no way to reach its own queue.
	want := []string{
		"decide_waiting", "edit_file", "forget_reminder", "list_directory",
		"list_models", "list_reminders", "list_waiting", "look_at_screen", "read_document",
		"read_file", "remind_me", "run_command", "search_files",
		"set_appearance", "set_wake_word", "write_document", "write_file",
	}

	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("tools are %v, want %v", names, want)
	}

	// And the dangerous ones must still be gated.
	for _, n := range []string{"write_file", "run_command"} {
		tool, _ := b.Agent.Registry.Get(n)

		if tool.Risk() != tools.Mutating {
			t.Errorf("%s is not gated", n)
		}
	}
}

func getJSON(t *testing.T, url string, into any) {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
}

func postJSON(t *testing.T, url string, into any) {
	t.Helper()

	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// A tool the model can see is a tool it will try. In private mode the web tool
// is not registered at all, rather than registered and refused — an assistant
// that keeps proposing something it may never do is worse than one that simply
// cannot.
func TestWebToolExistsOnlyWhenPrivacyAllowsIt(t *testing.T) {
	cases := map[string]bool{"private": false, "research": true, "open": true}

	for privacy, wantWeb := range cases {
		t.Run(privacy, func(t *testing.T) {
			root := t.TempDir()

			db, err := store.Open(filepath.Join(root, "brain.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			cfg := config.Default()
			cfg.Privacy = privacy

			b := brain.New(db, cfg, root, filepath.Join(root, "brain.sqlite"),
				slog.New(slog.NewTextHandler(io.Discard, nil)))

			_, hasWeb := b.Agent.Registry.Get("fetch_url")

			if hasWeb != wantWeb {
				t.Errorf("privacy %q: fetch_url present = %v, want %v", privacy, hasWeb, wantWeb)
			}
		})
	}
}

// A lesson a person accepts goes through the same duplicate check as an
// automatic promotion. Without that, the one path a human touches would be the
// only path that can create duplicates.
func TestAcceptingALessonStillChecksForDuplicates(t *testing.T) {
	ts, db, b := newServer(t)

	// Stand in for the embedder so the test needs no model. Two texts sharing
	// their first word embed identically, which is enough to exercise the
	// threshold.
	b.Learner.Curator.Embedder = fakeEmbedder{}

	first, _ := db.AddLesson(0, "Petar prefers Laravel for backend work", "proposed", "high", "")
	second, _ := db.AddLesson(0, "Petar prefers Laravel over Python", "proposed", "high", "")

	var out map[string]any
	postJSON(t, ts.URL+"/api/lessons/"+itoa(first)+"/accept", &out)

	if out["status"] != "promoted" {
		t.Fatalf("first accept gave %v", out)
	}

	postJSON(t, ts.URL+"/api/lessons/"+itoa(second)+"/accept", &out)

	if out["duplicate"] != true {
		t.Errorf("a restatement of a known fact was stored again: %v", out)
	}

	facts, _ := db.CountFacts()
	if facts != 1 {
		t.Errorf("%d facts stored, want 1", facts)
	}
}

func TestLessonDecisionsAreFinal(t *testing.T) {
	ts, db, _ := newServer(t)

	id, _ := db.AddLesson(0, "something the model guessed", "proposed", "low", "")

	var out map[string]any
	postJSON(t, ts.URL+"/api/lessons/"+itoa(id)+"/reject", &out)

	if out["status"] != "rejected" {
		t.Fatalf("reject gave %v", out)
	}

	postJSON(t, ts.URL+"/api/lessons/"+itoa(id)+"/accept", &out)

	if _, isError := out["error"]; !isError {
		t.Error("a rejected lesson was accepted on a second request")
	}
}

// Only model inferences wait for a person. A scanned observation is checked
// against disk and promotes itself, so it must not clutter the review list.
func TestOnlyProposedLessonsAreOfferedForReview(t *testing.T) {
	ts, db, _ := newServer(t)

	db.AddLesson(0, "a guess", "proposed", "low", "")
	db.AddLesson(0, "a scanned project", "validated", "high", "project:/tmp")
	db.AddLesson(0, "already stored", "promoted", "high", "")
	db.AddLesson(0, "thrown out", "rejected", "low", "")

	var lessons []store.Lesson
	getJSON(t, ts.URL+"/api/lessons", &lessons)

	if len(lessons) != 1 || lessons[0].Content != "a guess" {
		t.Fatalf("review list holds %+v", lessons)
	}
}

// fakeEmbedder maps text to a vector by its first word, so restatements of the
// same fact collide and unrelated facts do not.
type fakeEmbedder struct{}

func (fakeEmbedder) EmbedModel() string { return "fake" }

func (fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	words := strings.Fields(strings.ToLower(text))

	vec := make([]float32, 8)

	for i, w := range words {
		if i >= 2 {
			break
		}

		for _, r := range w {
			vec[int(r)%8] += 1
		}
	}

	return vec, nil
}

// Interface changes appeared not to work, repeatedly, because the scripts were
// served with no cache instruction at all. A browser given none caches
// heuristically and indefinitely, so the window ran code from several builds
// earlier while the server served the current file to nobody.
func TestAssetsCarryCacheValidators(t *testing.T) {
	ts, _, _ := newServer(t)

	for _, path := range []string{"/js/console.js", "/js/core3d.js", "/css/console.css"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}

		resp.Body.Close()

		if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s Cache-Control = %q, want no-cache", path, got)
		}

		if resp.Header.Get("ETag") == "" {
			t.Errorf("%s has no ETag, so a browser cannot revalidate it", path)
		}
	}
}

// Revalidation has to actually work, or no-cache just means fetching
// everything every time.
func TestUnchangedAssetsRevalidateCheaply(t *testing.T) {
	ts, _, _ := newServer(t)

	first, err := http.Get(ts.URL + "/js/console.js")
	if err != nil {
		t.Fatal(err)
	}

	first.Body.Close()
	tag := first.Header.Get("ETag")

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/js/console.js", nil)
	req.Header.Set("If-None-Match", tag)

	second, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	second.Body.Close()

	if second.StatusCode != http.StatusNotModified {
		t.Errorf("revalidation returned %d, want 304", second.StatusCode)
	}
}

// Two different files must not share an identity, or updating one would leave
// the other stale.
func TestAssetsHaveDistinctETags(t *testing.T) {
	ts, _, _ := newServer(t)

	seen := map[string]string{}

	for _, path := range []string{"/js/console.js", "/js/core3d.js", "/css/console.css"} {
		resp, _ := http.Get(ts.URL + path)
		resp.Body.Close()

		tag := resp.Header.Get("ETag")

		if other, clash := seen[tag]; clash {
			t.Errorf("%s and %s share the ETag %s", path, other, tag)
		}

		seen[tag] = path
	}
}

/*
 * Being asked for a name once, and then never again.
 *
 * The interface shows the naming overlay when the status says first_run, and
 * the status says first_run when there was no settings file to read. The part
 * worth pinning down is the end of it: Save has a value receiver, so it cannot
 * clear the flag on the running config by itself. The handler has to, and if it
 * ever stops, the overlay comes back on every reload — which reads as the brain
 * having forgotten its own name, on a machine where forgetting nothing is the
 * whole point.
 */
func TestFirstRunAsksForANameOnlyOnce(t *testing.T) {
	ts, _, b := newServer(t)

	b.Cfg.New = true

	status := func() map[string]any {
		t.Helper()

		res, err := http.Get(ts.URL + "/api/status")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		var out map[string]any
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}

		return out
	}

	if first, _ := status()["first_run"].(bool); !first {
		t.Fatal("a brain with no settings file should ask to be introduced")
	}

	res, err := http.Post(ts.URL+"/api/settings", "application/json",
		strings.NewReader(`{"name":"Ariel","owner":"Petar"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("naming it returned %d", res.StatusCode)
	}

	after := status()

	if first, _ := after["first_run"].(bool); first {
		t.Error("it asked to be introduced again after being given a name")
	}

	if name, _ := after["name"].(string); name != "Ariel" {
		t.Errorf("it is called %q, not the name it was given", name)
	}

	// And on the next start, which is a fresh Load of the file just written.
	reloaded, err := config.Load(b.Root)
	if err != nil {
		t.Fatal(err)
	}

	if reloaded.New {
		t.Error("it would ask to be introduced again on the next start")
	}

	if reloaded.Name != "Ariel" {
		t.Errorf("the name did not survive a restart: %q", reloaded.Name)
	}
}

/*
 * A brain that has a name should answer to it.
 *
 * Naming it on first run and then finding it ignores that name reads as
 * broken, not as a setting left at its default — so the name it is given
 * becomes the name it listens for.
 *
 * The part that needs care is not doing that over the top of somebody's work.
 * What it answers to is a list, because a microphone writes a name down
 * differently every so often and the fix is to add what it actually wrote.
 * A rename must not throw that list away.
 */
func TestWhatItAnswersToFollowsItsName(t *testing.T) {
	name := func(t *testing.T, ts *httptest.Server, body string) {
		t.Helper()

		res, err := http.Post(ts.URL+"/api/settings", "application/json",
			strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Fatalf("settings returned %d", res.StatusCode)
		}
	}

	t.Run("an unset one takes the name", func(t *testing.T) {
		ts, _, b := newServer(t)
		b.Cfg.WakeWord = ""

		name(t, ts, `{"name":"Ariel"}`)

		if b.Cfg.WakeWord != "Ariel" {
			t.Errorf("it answers to %q, not to the name it was just given", b.Cfg.WakeWord)
		}
	})

	t.Run("renaming carries it along", func(t *testing.T) {
		ts, _, b := newServer(t)
		b.Cfg.Name = "Ariel"
		b.Cfg.WakeWord = "Ariel"

		name(t, ts, `{"name":"Tau"}`)

		if b.Cfg.WakeWord != "Tau" {
			t.Errorf("renamed to Tau but still answers to %q", b.Cfg.WakeWord)
		}
	})

	t.Run("a tuned list survives a rename", func(t *testing.T) {
		ts, _, b := newServer(t)
		b.Cfg.Name = "Ariel"
		b.Cfg.WakeWord = "Ariel, Arielle, Aerial"

		name(t, ts, `{"name":"Tau"}`)

		if b.Cfg.WakeWord != "Ariel, Arielle, Aerial" {
			t.Errorf("a rename overwrote the names it had been taught: %q", b.Cfg.WakeWord)
		}
	})

	t.Run("asking for both, the explicit one wins", func(t *testing.T) {
		ts, _, b := newServer(t)
		b.Cfg.WakeWord = ""

		name(t, ts, `{"name":"Tau","wake_word":"Tau, Tao"}`)

		if b.Cfg.WakeWord != "Tau, Tao" {
			t.Errorf("it answers to %q, not to what was asked for", b.Cfg.WakeWord)
		}
	})
}

/*
 * Being called by name every time.
 *
 * Staying engaged after one exchange is what a conversation wants and what a
 * room with other people in it cannot have: for the next minute, everything
 * anybody says is treated as addressed, so the brain answers the television and
 * the person sitting next to it.
 *
 * Decided on this side rather than by trusting the page not to claim it, since
 * this is the setting that stops that happening.
 */
func TestCallingItByNameEveryTime(t *testing.T) {
	_, _, b := newServer(t)
	srv := New(b, slog.New(slog.NewTextHandler(io.Discard, nil)))

	b.Cfg.WakeWord = "Brain"
	b.Cfg.AlwaysName = true

	// The page claims it is still in a conversation. With the name required,
	// that claim buys nothing.
	if heard, _ := srv.decide("what time is it", true); heard.Addressed {
		t.Error("a sentence without the name was answered while the name is required")
	}

	if heard, _ := srv.decide("brain what time is it", true); !heard.Addressed {
		t.Error("a sentence carrying the name was not answered")
	}

	// With the setting off, the same claim keeps the conversation open.
	b.Cfg.AlwaysName = false

	if heard, _ := srv.decide("what time is it", true); !heard.Addressed {
		t.Error("a follow-up was ignored while the conversation was meant to be open")
	}

	// And leaving the conversation only means anything while in one.
	if _, ends := srv.decide("thanks", true); !ends {
		t.Error("saying thank you did not end the conversation")
	}

	b.Cfg.AlwaysName = true

	if _, ends := srv.decide("thanks", true); ends {
		t.Error("a conversation that was never open was ended")
	}
}

// What a brain answers to before anybody has said otherwise.
//
// One ordinary word, because that is what transcription reliably gets right:
// whisper writes this program's own name down as "Piembring" one time and
// "Piendren" the next, so no list of spellings ever converges on it.
func TestTheNameItShipsListeningFor(t *testing.T) {
	if config.DefaultWakeWord == "" {
		t.Fatal("it ships answering to everything it hears, which suits a headset, not a room")
	}

	if got := wake.Names(config.DefaultWakeWord); len(got) == 0 {
		t.Fatalf("the default wake word yields no names to match: %q", config.DefaultWakeWord)
	}

	if !wake.Listen("brain, what time is it", config.DefaultWakeWord, false).Addressed {
		t.Error("it does not answer to the name it ships with")
	}

	// And naming it replaces the shipped default rather than leaving a brain
	// called Ariel that only answers to Brain.
	ts, _, b := newServer(t)
	b.Cfg.WakeWord = config.DefaultWakeWord

	res, err := http.Post(ts.URL+"/api/settings", "application/json",
		strings.NewReader(`{"name":"Ariel"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if b.Cfg.WakeWord != "Ariel" {
		t.Errorf("named Ariel but still answers to %q", b.Cfg.WakeWord)
	}
}

/*
 * The mailbox password is never sent back to the page.
 *
 * A page that echoed it would put it in the page source, in the accessibility
 * tree, and in every screenshot of this window — including the ones this
 * program can now take of its own screen and hand to a model.
 */
func TestTheMailPasswordNeverLeavesTheSettingsFile(t *testing.T) {
	ts, _, b := newServer(t)

	b.Cfg.MailUser = "petar@example.com"
	b.Cfg.MailHost = "imap.example.com"
	b.Cfg.MailPassword = "hunter2-app-password"

	res, err := http.Get(ts.URL + "/api/mail")
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)

	if strings.Contains(string(body), "hunter2") {
		t.Fatalf("the password came back to the page: %s", body)
	}

	var out map[string]any

	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}

	if has, _ := out["has_password"].(bool); !has {
		t.Error("the page cannot tell that a password is saved")
	}
}

/*
 * Saving the form with the password box empty keeps the saved one.
 *
 * The form never shows the password, so somebody changing only the server name
 * submits a blank box — and clearing it there would silently break the mailbox
 * they just finished setting up.
 */
func TestABlankPasswordBoxKeepsTheSavedOne(t *testing.T) {
	ts, _, b := newServer(t)

	b.Cfg.MailPassword = "already-saved"

	res, err := http.Post(ts.URL+"/api/mail", "application/json",
		strings.NewReader(`{"user":"petar@example.com","host":"imap.example.com","password":""}`))
	if err != nil {
		t.Fatal(err)
	}

	res.Body.Close()

	if b.Cfg.MailPassword != "already-saved" {
		t.Errorf("the saved password was wiped by an empty box: %q", b.Cfg.MailPassword)
	}

	if b.Cfg.MailUser != "petar@example.com" {
		t.Errorf("the rest of the form did not save: %q", b.Cfg.MailUser)
	}
}

/*
 * The core is coloured from this endpoint, so the endpoint has to carry it.
 *
 * The handler builds its own object field by field rather than marshalling the
 * step, so adding a field to the step changes nothing here — which is how the
 * whole distinction between background and foreground work came to be correct
 * everywhere except on the wire, where it is the only place it matters.
 */
func TestProgressSaysWhetherAnybodyIsWaiting(t *testing.T) {
	ts, _, _ := newServer(t)

	progress.Done()
	progress.SetBackground("learning", "Learning from the last conversation")

	defer progress.Done()

	res, err := http.Get(ts.URL + "/api/progress")
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()

	var out map[string]any

	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}

	if _, carried := out["background"]; !carried {
		t.Fatal("the page is never told which kind of work this is")
	}

	if background, _ := out["background"].(bool); !background {
		t.Error("learning after a conversation is reported as work somebody is waiting for")
	}

	// And a real turn is reported as one.
	progress.Begin()

	res2, err := http.Get(ts.URL + "/api/progress")
	if err != nil {
		t.Fatal(err)
	}

	defer res2.Body.Close()

	var turn map[string]any

	if err := json.NewDecoder(res2.Body).Decode(&turn); err != nil {
		t.Fatal(err)
	}

	if background, _ := turn["background"].(bool); background {
		t.Error("a turn its owner is waiting for was reported as background work")
	}
}

/*
 * Changing the model on the Models page changes what the brain uses.
 *
 * Which small and which reasoning model exist has to be discovered, and that is
 * worked out once. The working model is a choice its owner makes and remakes,
 * and remembering it alongside the discovered ones meant the page said one
 * thing while the brain went on using another until it was restarted.
 */
func TestChoosingAModelTakesEffectAtOnce(t *testing.T) {
	_, _, b := newServer(t)

	b.Cfg.OllamaModel = "qwen2.5-coder:7b"

	before, _, _ := b.Roles()
	if before != "qwen2.5-coder:7b" {
		t.Fatalf("it started on %q", before)
	}

	b.Cfg.OllamaModel = "qwen3:latest"

	after, _, _ := b.Roles()
	if after != "qwen3:latest" {
		t.Errorf("the model was changed to qwen3:latest but it still uses %q", after)
	}
}
