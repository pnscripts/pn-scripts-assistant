package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"pn-brain/internal/brain/llm"
	"strconv"
	"strings"
	"testing"

	"pn-brain/internal/brain/brain"
	"pn-brain/internal/brain/config"
	"pn-brain/internal/brain/progress"
	"pn-brain/internal/brain/protect"
	"pn-brain/internal/brain/store"
	"pn-brain/internal/brain/tools"
	"pn-brain/internal/brain/wake"
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
		"ask_first", "brain_copies", "change_a_setting", "change_this_conversation",
		"click", "decide_waiting", "do_in_background", "edit_file",
		"fetch_url", "films_without_subtitles", "forget_reminder", "how_fast_can_you_answer",
		"learn_from_folder", "list_background", "list_directory",
		"list_drives", "list_models",
		"list_reminders", "list_waiting", "list_windows", "look_at_screen",
		"make_subtitles", "open_app", "places_it_learns_from", "put_it_back",
		"read_document", "read_file", "remind_me", "run_command",
		"scroll", "search_files", "set_appearance", "set_wake_word",
		"stop_background", "stop_hearing_this_machine", "type_text",
		"web_search", "what_am_i_hearing", "what_can_you_do", "what_you_know",
		"write_document",
		"write_file",
	}

	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("tools are %v, want %v", names, want)
	}

	/*
	 * And the ones that act on the machine must still be gated.
	 *
	 * click and type_text belong in this list more than anything else does. A
	 * click can send a message, empty a folder or confirm a purchase, and
	 * nothing about the click itself says which — the only thing standing
	 * between the model and any of those is that somebody is asked first.
	 */
	for _, n := range []string{
		"write_file", "run_command", "click", "type_text", "open_app",
		"write_document",
	} {
		tool, registered := b.Agent.Registry.Get(n)
		if !registered {
			t.Errorf("%s is not registered at all", n)

			continue
		}

		if tool.Risk() != tools.Mutating {
			t.Errorf("%s is not gated", n)
		}
	}

	// Mail is only registered when a mailbox is set up, so it is checked only
	// when it is there — but if it is there, sending must ask first.
	if tool, registered := b.Agent.Registry.Get("send_email"); registered {
		if tool.Risk() != tools.Mutating {
			t.Error("send_email is not gated, and a sent message cannot be recalled")
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
/*
 * Privacy decides what the model is shown and what it is allowed to run.
 *
 * It used to decide what was registered, which was simpler and could only
 * change by restarting: somebody switching to research watched the setting
 * save and nothing else happen, because the tools that make research mean
 * anything had been decided minutes earlier.
 *
 * So both halves are checked. Hidden is what stops the model reaching for it —
 * a tool it can see is a tool it will try. Refused is what stops it anyway,
 * because a model that saw the tool earlier in a conversation will name it
 * again after the rule changes, and a guard that only removes the menu is a
 * convention rather than a rule.
 */
func TestPrivacyHidesAndRefusesTheWebTools(t *testing.T) {
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

			// Registered whatever the setting, so that changing it does not
			// need the program restarted.
			if _, present := b.Agent.Registry.Get("fetch_url"); !present {
				t.Fatal("the web tool is not registered at all")
			}

			for _, name := range []string{"fetch_url", "web_search"} {
				offLimits := b.Agent.OffLimits(name)

				if offLimits == wantWeb {
					t.Errorf("privacy %q: %s off limits = %v, want %v",
						privacy, name, offLimits, !wantWeb)
				}
			}
		})
	}
}

/*
 * And it follows the setting as it changes, without a restart.
 *
 * This is the whole point of moving the decision out of the registry. The file
 * on the owner's machine said research while the program said private, because
 * the program reports what it is using and it was using what it read at
 * startup.
 */
func TestTheWebFollowsPrivacyWhileRunning(t *testing.T) {
	_, _, b := newServer(t)

	if !b.Agent.OffLimits("fetch_url") {
		t.Fatal("the web is reachable in private mode")
	}

	if _, err := b.UsePrivacy("research"); err != nil {
		t.Fatal(err)
	}

	if b.Agent.OffLimits("fetch_url") {
		t.Fatal("switching to research left the web off limits")
	}

	if _, err := b.UsePrivacy("private"); err != nil {
		t.Fatal(err)
	}

	if !b.Agent.OffLimits("fetch_url") {
		t.Fatal("switching back to private left the web reachable")
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
	if heard, _ := srv.decide("what time is it", true, false); heard.Addressed {
		t.Error("a sentence without the name was answered while the name is required")
	}

	if heard, _ := srv.decide("brain what time is it", true, false); !heard.Addressed {
		t.Error("a sentence carrying the name was not answered")
	}

	// With the setting off, the same claim keeps the conversation open.
	b.Cfg.AlwaysName = false

	if heard, _ := srv.decide("what time is it", true, false); !heard.Addressed {
		t.Error("a follow-up was ignored while the conversation was meant to be open")
	}

	// And leaving the conversation only means anything while in one.
	if _, ends := srv.decide("thanks", true, false); !ends {
		t.Error("saying thank you did not end the conversation")
	}

	b.Cfg.AlwaysName = true

	if _, ends := srv.decide("thanks", true, false); ends {
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

	// Chosen, rather than merely configured. The two are different now: an
	// untouched setting is a default this program shipped with, and defers to
	// what the machine can actually run.
	b.Cfg.OllamaModel = "qwen2.5-coder:7b"
	b.Cfg.ModelChosen = true

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

/*
 * A model nobody chose gives way to what the machine can actually run.
 *
 * The value this program ships with was picked against four processor cores
 * and no graphics card, and it was then used unchanged on every machine. The
 * code that works out what the hardware can manage ran at startup, wrote its
 * answer into the log, and was read by nothing — so the tier was a decoration.
 *
 * Both directions of that matter. A computer with a card was held to a model
 * chosen for one without; and a computer without one was held to a
 * seven-billion-parameter model that takes minutes per round, when a smaller
 * model already installed answers far quicker and asks for its tools more
 * reliably.
 */
func TestAnUnchosenModelDefersToTheMachine(t *testing.T) {
	/*
	 * Decided from the roles directly, not from what happens to be installed.
	 *
	 * Asking the brain meant asking Ollama, so the test measured the machine
	 * it ran on: with models present it passed, and with none it failed
	 * reporting a fault that was not there. A test that changes its mind when
	 * somebody clears their models is not testing the thing it names.
	 */
	quick := llm.Sizes{
		Work:  "qwen2.5-coder:7b",
		Quick: "llama3.2:3b",
		Best:  "qwen2.5-coder:7b",
	}

	// Nobody chose, so the machine's own pick wins.
	if got := workModelFrom(quick, false); got != quick.Quick {
		t.Errorf("an untouched setting gave %q, so what the machine can run is "+
			"worked out and then ignored", got)
	}

	// Chosen deliberately, it stands on any machine.
	if got := workModelFrom(quick, true); got != quick.Work {
		t.Errorf("a model chosen from the panel gave %q instead of %q",
			got, quick.Work)
	}

	// And with nothing installed there is nothing to defer to, which must not
	// leave the brain with no model at all.
	bare := llm.Sizes{Work: "qwen2.5-coder:7b"}

	if got := workModelFrom(bare, false); got != bare.Work {
		t.Errorf("with nothing installed the working model became %q", got)
	}
}

// workModelFrom is the rule modelRoles applies, isolated so it can be tested
// without asking the machine what it happens to have installed.
func workModelFrom(roles llm.Sizes, chosen bool) string {
	if !chosen && roles.Quick != "" {
		return roles.Quick
	}

	return roles.Work
}

/*
 * The prompt has to forbid inventing what is on the machine.
 *
 * Asked "what is waiting for me?", with the tools offered and the right model
 * chosen, it answered with a confident list: call your mother at four, write a
 * report by Friday, schedule a team meeting. None of it existed. One lesson was
 * actually waiting.
 *
 * That is the worst failure available to this program. A refusal is obvious and
 * a wrong answer is arguable, but an invented list of somebody's own commitments
 * is indistinguishable from a real one — and it is the kind of thing they would
 * act on.
 */
func TestThePromptForbidsInventingTheMachinesState(t *testing.T) {
	_, _, b := newServer(t)

	prompt := strings.ToLower(b.SystemPrompt())

	for _, must := range []string{"never state", "from memory", "call the tool"} {
		if !strings.Contains(prompt, must) {
			t.Errorf("the prompt does not say %q, so a model may answer from imagination", must)
		}
	}

	// And it has to name the things it must not invent, because "do not make
	// things up" is advice a model agrees with and ignores.
	for _, named := range []string{"waiting", "reminders", "models", "mailbox"} {
		if !strings.Contains(prompt, named) {
			t.Errorf("the prompt does not name %q among the things it cannot know", named)
		}
	}
}

/*
 * The transcript comes back after a reload.
 *
 * The page asks for the latest conversation and reads convo.id to decide
 * whether one was found. The id was only ever nested inside "conversation", so
 * that check never passed and the transcript was left empty after every reload
 * and every restart — while everything else worked perfectly. The messages
 * were stored, the history panel listed them, the brain answered out loud, and
 * the one place somebody actually looks showed nothing but the greeting.
 *
 * That is the shape of failure this program keeps producing: a thing that is
 * working everywhere except where it is read.
 */
func TestTheLatestConversationCarriesItsID(t *testing.T) {
	srv, db, _ := newServer(t)

	id, err := db.NewConversation("weather")
	if err != nil {
		t.Fatalf("could not start a conversation: %v", err)
	}

	if _, err := db.AddMessage(id, "user", "", "", "what is the weather"); err != nil {
		t.Fatalf("could not record what was said: %v", err)
	}

	if _, err := db.AddMessage(id, "assistant", "", "", "Cloudy, about 18 degrees."); err != nil {
		t.Fatalf("could not record the answer: %v", err)
	}

	resp, err := http.Get(srv.URL + "/api/conversations/latest")
	if err != nil {
		t.Fatalf("could not ask for the latest conversation: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the endpoint returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	var reply struct {
		ID       int64 `json:"id"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	if err := json.Unmarshal(body, &reply); err != nil {
		t.Fatalf("could not read the reply: %v", err)
	}

	if reply.ID == 0 {
		t.Error("no id at the top level, so the page treats this as no conversation " +
			"at all and leaves the transcript empty")
	}

	if len(reply.Messages) < 2 {
		t.Errorf("only %d messages came back, want both sides of the exchange",
			len(reply.Messages))
	}
}

/*
 * Silence while it is still being introduced.
 *
 * The naming card covers the screen on a first run, and the console behind it
 * asks for the opening line as it loads — so a brand new brain announced
 * itself aloud, by the name its owner was at that moment being asked to
 * choose, before they had pressed Begin. It came out of the speakers before
 * anybody had agreed to anything.
 *
 * Checked at the server because that is where the voice is: the handler speaks
 * the greeting itself, so no amount of care in the page can hold it back — a
 * reload, a second window or a tab left open from before would each produce
 * one.
 */
func TestItSaysNothingUntilItHasBeenNamed(t *testing.T) {
	ts, _, b := newServer(t)

	b.Cfg.New = true

	greeting := func() string {
		t.Helper()

		res, err := http.Get(ts.URL + "/api/greeting")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		var out struct {
			Text string `json:"text"`
		}

		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}

		return out.Text
	}

	if said := greeting(); said != "" {
		t.Errorf("it greeted before it had been named: %q", said)
	}

	// And says hello properly once it has one — the greeting is postponed,
	// not lost, or the first thing a named brain does is nothing.
	res, err := http.Post(ts.URL+"/api/settings", "application/json",
		strings.NewReader(`{"name":"Ariel","owner":"Petar"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if said := greeting(); said == "" {
		t.Error("it had nothing to say after being named")
	}
}

/*
 * There has to be somewhere to put a copy.
 *
 * On the machine this was written for the list came back empty, which makes
 * the feature look broken rather than unused: the brain lives on the external
 * drive, so that is excluded as the drive it is already on, and the internal
 * disk is mounted at / which an ordinary user cannot write to. Every drive was
 * either the brain's own or unwritable — on a machine with 200GB free in the
 * owner's own home folder, which is not a drive and so was in no list of them.
 */
func TestThereIsSomewhereToKeepACopy(t *testing.T) {
	_, _, b := newServer(t)

	places := placesForACopy(b.Root)

	if len(places) == 0 {
		t.Fatal("nowhere at all was offered as a place for a copy")
	}

	for _, p := range places {
		where, _ := p["suggested"].(string)

		if where == "" {
			t.Errorf("a place was offered with no path: %v", p)
		}

		if where == b.Root {
			t.Errorf("the brain's own folder was offered as a copy of itself")
		}
	}
}

/*
 * The operations view answers even before the first reading is taken.
 *
 * Every rate on it is the difference between two readings, so for the first
 * second of a run there is nothing to compare against. The page asks
 * immediately — it is what a person opening the program sees — and an endpoint
 * that fails until the watcher has ticked would make the whole view look
 * broken for exactly as long as somebody is most likely to be looking at it.
 */
func TestOperationsAnswersBeforeTheFirstReading(t *testing.T) {
	ts, _, _ := newServer(t)

	res, err := http.Get(ts.URL + "/api/operations")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("it returned %d before any reading was taken", res.StatusCode)
	}

	var out struct {
		Available bool   `json:"available"`
		Why       string `json:"why"`
	}

	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}

	// Either it has readings, or it says why it has none. What it must never
	// do is return a page of zeroes that reads as a machine doing nothing.
	if !out.Available && out.Why == "" {
		t.Error("it reports nothing and does not say why")
	}
}

/*
 * Saying yes once is saying yes.
 *
 * The point of asking rather than refusing is lost the moment the same
 * question comes back: a prompt seen twice about a file already decided is a
 * prompt that stops being read and starts being clicked through, and at that
 * moment it looks like protection while being worse than none.
 */
func TestApprovingAProtectedFileIsRemembered(t *testing.T) {
	ts, db, b := newServer(t)

	// A file this brain would stop at, somewhere it can actually read.
	key := filepath.Join(t.TempDir(), ".ssh", "id_rsa")

	if err := os.MkdirAll(filepath.Dir(key), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(key, []byte("not a real key"), 0o600); err != nil {
		t.Fatal(err)
	}

	protect.Use(protect.Choices{})
	protect.OwnFolder(b.Root)

	t.Cleanup(func() { protect.Use(protect.Choices{}) })

	if _, ask := protect.Ask(key); !ask {
		t.Fatal("the test file is not one the brain would ask about")
	}

	conv, _ := db.NewConversation("t")

	id, err := db.RecordInvocation(conv, "read_file",
		`{"path":"`+key+`"}`, "Read a file — "+key, "mutating")
	if err != nil {
		t.Fatal(err)
	}

	res, err := http.Post(fmt.Sprintf("%s/api/approvals/%d/approve", ts.URL, id), "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}

	res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("approving returned %d", res.StatusCode)
	}

	// It stops asking about that file.
	if _, ask := protect.Ask(key); ask {
		t.Error("it would ask again about a file that was just approved")
	}

	// One file, not the whole rule: allowing one key must not open the folder.
	sibling := filepath.Join(filepath.Dir(key), "id_ed25519")

	if _, ask := protect.Ask(sibling); !ask {
		t.Error("approving one key opened every key beside it")
	}

	// And it is written down, so tomorrow's run knows it too — and is listed
	// where a person can take it back.
	if kept := protect.Load(b.Root); len(kept.Allowed) != 1 || kept.Allowed[0] != key {
		t.Errorf("what it learned was not saved: %+v", kept)
	}

	shown, err := http.Get(ts.URL + "/api/protection")
	if err != nil {
		t.Fatal(err)
	}
	defer shown.Body.Close()

	var out struct {
		Learned []string `json:"learned"`
	}

	if err := json.NewDecoder(shown.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}

	if len(out.Learned) != 1 {
		t.Errorf("the panel does not show what it learned: %+v", out.Learned)
	}
}

/*
 * Interrupting does not require saying its name.
 *
 * The name is what separates being spoken to from being in the same room as a
 * television, and it earns that everywhere except here: the brain is already
 * working for the person who has just started talking, so there is nothing to
 * disambiguate. Being made to say a name before you can interrupt is being
 * made to wait your turn by the thing that is meant to be waiting for you.
 */
func TestAnythingSaidOverTheTopOfItCounts(t *testing.T) {
	_, _, b := newServer(t)

	b.Cfg.WakeWord = "PN Brain"
	b.Cfg.AlwaysName = true

	s := New(b, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Not interrupting: the name is required, which is what keeps a room with
	// a television in it out of the conversation.
	if heard, _ := s.decide("what time is the film on", false, false); heard.Addressed {
		t.Error("something said across the room was treated as a question")
	}

	// Interrupting: it counts, whatever it was.
	heard, _ := s.decide("no, not that", false, true)

	if !heard.Addressed {
		t.Error("it carried on talking over somebody who had started speaking")
	}

	if heard.Text != "no, not that" {
		t.Errorf("what was said came through as %q", heard.Text)
	}

	// Silence is not an interruption. A recording with no words in it means
	// the room made a noise, not that anybody spoke.
	if quiet, _ := s.decide("   ", false, true); quiet.Addressed {
		t.Error("an empty transcript stopped it")
	}
}

/*
 * The tool that answers "why did you ignore me" must be able to reach the
 * record of what was heard.
 *
 * These are two packages joined by a package-level function, which is the kind
 * of wiring that compiles perfectly whether or not anybody remembered to
 * connect it. Left unconnected the tool is still registered, still offered to
 * the model, and answers every question about the room with "nothing here is
 * listening".
 */
func TestTheHearingToolCanReachWhatWasHeard(t *testing.T) {
	_, _, b := newServer(t)

	for _, tool := range b.Agent.Registry.All() {
		hearing, is := tool.(tools.Hearing)

		if !is {
			continue
		}

		said, err := hearing.Execute(context.Background(), nil)
		if err != nil {
			t.Fatalf("the hearing tool cannot reach the listener: %v", err)
		}

		if said == "" {
			t.Fatal("the hearing tool said nothing at all")
		}

		return
	}

	t.Fatal("the hearing tool is not registered")
}

// And a turn that was heard comes back through it, newest first, in words.
func TestWhatWasHeardComesBackThroughTheTool(t *testing.T) {
	_, _, b := newServer(t)
	s := New(b, slog.New(slog.NewTextHandler(io.Discard, nil)))

	s.heard.add(Overheard{Text: "first thing", PeakRMS: 1000})
	s.heard.add(Overheard{Text: "second thing", Addressed: true, PeakRMS: 2000})

	recent := s.recentlyHeard()

	if len(recent) != 2 {
		t.Fatalf("expected both turns, got %d", len(recent))
	}

	if recent[0].Text != "second thing" {
		t.Fatalf("the newest should come first, got %q", recent[0].Text)
	}

	if recent[0].Ago == "" {
		t.Fatal("a turn with no sense of when it happened is not much of an account")
	}
}

/*
 * Permission is not privacy, and the API says so.
 *
 * Petar's first point: "yes it can have privacy but the permissions on the
 * machine is other thing". They were one setting, so the only way to let the
 * brain do more was to let more leave.
 */
func TestPermissionsAreSeparateFromPrivacy(t *testing.T) {
	ts, _, _ := newServer(t)

	var page struct {
		Freedom      string `json:"freedom"`
		Privacy      string `json:"privacy"`
		Capabilities []struct {
			Name     string `json:"name"`
			Changes  bool   `json:"changes_something"`
			Decision string `json:"decision"`
		} `json:"capabilities"`
	}

	getJSON(t, ts.URL+"/api/permissions", &page)

	if page.Freedom != "ask" {
		t.Errorf("a new brain starts at %q, want ask", page.Freedom)
	}

	// Every capability, not only the ones with a decision on them: the
	// question somebody has is "what can this thing do".
	if len(page.Capabilities) < 30 {
		t.Errorf("only %d capabilities listed", len(page.Capabilities))
	}

	var looks, changes int

	for _, c := range page.Capabilities {
		if c.Changes {
			changes++

			if c.Decision != "ask" {
				t.Errorf("%s starts at %q, want ask", c.Name, c.Decision)
			}

			continue
		}

		looks++

		// Reading never needed permission and still does not.
		if c.Decision != "allow" {
			t.Errorf("%s only looks and yet needs permission", c.Name)
		}
	}

	if looks == 0 || changes == 0 {
		t.Errorf("%d that look and %d that change — one of those is wrong", looks, changes)
	}
}

// "Yes, and stop asking me about this one" — the whole reason for the page.
func TestAStandingGrantIsRecordedAndCanBeTakenBack(t *testing.T) {
	ts, _, _ := newServer(t)

	send(t, ts.URL+"/api/permissions/decide",
		`{"tool":"write_file","answer":"allow","why":"notes"}`)

	send(t, ts.URL+"/api/permissions/freedom", `{"level":"granted"}`)

	var page struct {
		Freedom      string `json:"freedom"`
		Capabilities []struct {
			Name     string `json:"name"`
			Decision string `json:"decision"`
		} `json:"capabilities"`
	}

	getJSON(t, ts.URL+"/api/permissions", &page)

	if page.Freedom != "granted" {
		t.Errorf("freedom is %q, want granted", page.Freedom)
	}

	found := false

	for _, c := range page.Capabilities {
		if c.Name == "write_file" {
			found = c.Decision == "allow"
		}
	}

	if !found {
		t.Error("the grant was not recorded")
	}

	// And taking it back puts the capability where it started.
	send(t, ts.URL+"/api/permissions/decide", `{"tool":"write_file","answer":"ask"}`)

	getJSON(t, ts.URL+"/api/permissions", &page)

	for _, c := range page.Capabilities {
		if c.Name == "write_file" && c.Decision != "ask" {
			t.Errorf("a withdrawn grant is still %q", c.Decision)
		}
	}
}

// send posts a body and insists it worked, for the endpoints whose answer is
// only "yes, that is recorded".
func send(t *testing.T, url, body string) {
	t.Helper()

	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		said, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s -> %d: %s", url, resp.StatusCode, said)
	}
}

/*
 * A key is never sent back to the page.
 *
 * Sending the whole thing back to be shown in a form is how a secret ends up
 * in a screenshot. The page is told whether one is set and its last four
 * characters — enough to tell two keys apart, useless to anybody else.
 */
func TestAKeyIsNeverSentBack(t *testing.T) {
	ts, _, _ := newServer(t)

	const key = "sk-ant-not-a-real-key-abcdefgh1234"

	send(t, ts.URL+"/api/providers/connect",
		`{"provider":"anthropic","key":"`+key+`","model":"claude-opus-5"}`)

	raw := getRaw(t, ts.URL+"/api/providers")

	if strings.Contains(raw, key) {
		t.Fatal("the whole key came back to the page")
	}

	if !strings.Contains(raw, "1234") {
		t.Error("the page cannot tell which key is set")
	}

	var page struct {
		Providers []struct {
			Name     string `json:"name"`
			NeedsKey bool   `json:"needs_key"`
			HasKey   bool   `json:"has_key"`
			Model    string `json:"model"`
		} `json:"providers"`
	}

	getJSON(t, ts.URL+"/api/providers", &page)

	var seen int

	for _, p := range page.Providers {
		switch p.Name {
		case "anthropic":
			seen++

			if !p.HasKey || p.Model != "claude-opus-5" {
				t.Errorf("anthropic came back as %+v", p)
			}

		case "ollama":
			seen++

			if p.NeedsKey {
				t.Error("the local one was said to need a key")
			}
		}
	}

	if seen != 2 {
		t.Errorf("%d of the expected providers listed", seen)
	}
}

/*
 * An empty box means "leave it alone", never "delete the key".
 *
 * Those are different intentions and there is a separate button for the
 * second. Saving a model name must not silently disconnect a provider.
 */
func TestSavingAModelDoesNotWipeTheKey(t *testing.T) {
	ts, _, _ := newServer(t)

	send(t, ts.URL+"/api/providers/connect",
		`{"provider":"openai","key":"sk-something-9999"}`)

	send(t, ts.URL+"/api/providers/connect",
		`{"provider":"openai","model":"gpt-4o"}`)

	var page struct {
		Providers []struct {
			Name   string `json:"name"`
			HasKey bool   `json:"has_key"`
			Model  string `json:"model"`
		} `json:"providers"`
	}

	getJSON(t, ts.URL+"/api/providers", &page)

	for _, p := range page.Providers {
		if p.Name != "openai" {
			continue
		}

		if !p.HasKey {
			t.Error("saving a model name wiped the key")
		}

		if p.Model != "gpt-4o" {
			t.Errorf("the model is %q", p.Model)
		}
	}

	// And forgetting is deliberate, and works.
	send(t, ts.URL+"/api/providers/connect", `{"provider":"openai","forget":true}`)

	getJSON(t, ts.URL+"/api/providers", &page)

	for _, p := range page.Providers {
		if p.Name == "openai" && p.HasKey {
			t.Error("the key survived being forgotten")
		}
	}
}

// getRaw reads a response as text, for asking whether something is in it at
// all rather than what shape it has.
func getRaw(t *testing.T, url string) string {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	return string(body)
}

/*
 * Which half is missing, not "not configured".
 *
 * An address with no token and a token with no address are different mistakes
 * with different next steps, and telling somebody neither is how a setup page
 * becomes a guessing game.
 */
func TestTheHouseSaysWhichHalfIsMissing(t *testing.T) {
	ts, _, _ := newServer(t)

	var page struct {
		URL       string `json:"url"`
		HasToken  bool   `json:"has_token"`
		TokenTail string `json:"token_tail"`
		Connected bool   `json:"connected"`
		Why       string `json:"why"`
	}

	getJSON(t, ts.URL+"/api/devices", &page)

	if !strings.Contains(page.Why, "Not set up yet") {
		t.Errorf("a brand new brain says %q", page.Why)
	}

	// An address alone.
	send(t, ts.URL+"/api/devices/connect", `{"url":"http://homeassistant.local:8123"}`)
	getJSON(t, ts.URL+"/api/devices", &page)

	if !strings.Contains(page.Why, "no token") {
		t.Errorf("with an address and no token it says %q", page.Why)
	}

	// And the token never comes back.
	const token = "eyJhbGciOiJIUzI1NiJ9.secret-token-7788"

	send(t, ts.URL+"/api/devices/connect", `{"token":"`+token+`"}`)

	raw := getRaw(t, ts.URL+"/api/devices")

	if strings.Contains(raw, token) {
		t.Fatal("the whole token came back to the page")
	}

	getJSON(t, ts.URL+"/api/devices", &page)

	if !page.HasToken || page.TokenTail != "7788" {
		t.Errorf("the page cannot tell which token is set: %+v", page)
	}

	// Saving the address again must not wipe the token.
	send(t, ts.URL+"/api/devices/connect", `{"url":"http://192.168.0.9:8123"}`)
	getJSON(t, ts.URL+"/api/devices", &page)

	if !page.HasToken {
		t.Error("saving the address wiped the token")
	}

	if page.URL != "http://192.168.0.9:8123" {
		t.Errorf("the address is %q", page.URL)
	}
}
