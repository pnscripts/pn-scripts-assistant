package learning

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * Learning gives way to a question.
 *
 * Seen on a four-core machine with no graphics card: a question asked while
 * the brain was reading its own knowledge folder waited about fourteen minutes
 * for its answer, because the reading's model calls ran beside the answer's on
 * the same cores. These tests hold the rule that fixes it: work nobody is
 * waiting on waits for a question, stops for one that starts, and picks up
 * again afterwards without losing or repeating anything; work somebody asked
 * for never waits on itself.
 *
 * The turn counter is one for the whole program, so none of these run in
 * parallel and each starts with no turn running.
 */

func noTurns(t *testing.T) {
	t.Helper()

	for progress.InATurn() {
		progress.FinishedATurn()
	}

	t.Cleanup(func() {
		for progress.InATurn() {
			progress.FinishedATurn()
		}
	})
}

// within fails the test when ch is not closed in time.
func within(t *testing.T, ch <-chan struct{}, d time.Duration, what string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(d):
		t.Fatalf("%s did not happen within %s", what, d)
	}
}

// never fails the test when ch is closed within d.
func never(t *testing.T, ch <-chan struct{}, d time.Duration, what string) {
	t.Helper()

	select {
	case <-ch:
		t.Fatalf("%s happened, and must not", what)
	case <-time.After(d):
	}
}

func TestBackgroundWorkWaitsForTheQuestionToBeAnswered(t *testing.T) {
	noTurns(t)

	progress.StartedATurn()

	called := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)

		yielding(InBackground(context.Background()), func(context.Context) (int, error) {
			close(called)

			return 1, nil
		})
	}()

	never(t, called, 700*time.Millisecond, "background work started during a question")

	progress.FinishedATurn()

	// Woken by the turn ending, not by a slow poll.
	within(t, called, 300*time.Millisecond, "the work resuming after the answer")
	within(t, done, time.Second, "the work finishing")
}

func TestAQuestionStopsALongCallWhichIsThenDoneAgain(t *testing.T) {
	noTurns(t)

	var (
		mu       sync.Mutex
		attempts int
		causes   []error
	)

	first := make(chan struct{})

	call := func(ctx context.Context) (string, error) {
		mu.Lock()
		attempts++
		n := attempts
		mu.Unlock()

		if n == 1 {
			// A model call that runs for minutes: it only ends when told to.
			close(first)
			<-ctx.Done()

			mu.Lock()
			causes = append(causes, context.Cause(ctx))
			mu.Unlock()

			return "", ctx.Err()
		}

		return "a fact", nil
	}

	type result struct {
		out string
		err error
	}

	got := make(chan result, 1)

	go func() {
		out, err := yielding(InBackground(context.Background()), call)
		got <- result{out, err}
	}()

	within(t, first, time.Second, "the first call starting")

	progress.StartedATurn()

	// The long call ends at once rather than at its own pace.
	deadline := time.Now().Add(time.Second)

	for {
		mu.Lock()
		stopped := len(causes) == 1
		mu.Unlock()

		if stopped {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("a question did not stop the background call")
		}

		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	cause := causes[0]
	mu.Unlock()

	if !errors.Is(cause, ErrGaveWay) {
		t.Errorf("stopped for %v, want ErrGaveWay", cause)
	}

	// And it is not tried again while the question is still being answered.
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	during := attempts
	mu.Unlock()

	if during != 1 {
		t.Errorf("tried %d times during the question, want once", during)
	}

	progress.FinishedATurn()

	select {
	case r := <-got:
		if r.err != nil || r.out != "a fact" {
			t.Errorf("after the question: %q, %v", r.out, r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the work was not done again after the question")
	}

	if attempts != 2 {
		t.Errorf("%d attempts, want 2", attempts)
	}
}

/*
 * Reading a folder because somebody asked for it in the conversation is the
 * question itself. If it waited for the turn to end it would wait on itself,
 * for GiveWayAtMost, with the person watching.
 */
func TestWorkSomebodyAskedForDoesNotWaitOnItself(t *testing.T) {
	noTurns(t)

	progress.StartedATurn()

	start := time.Now()

	out, err := yielding(context.Background(), func(ctx context.Context) (int, error) {
		// Another question starting meanwhile does not stop it either.
		progress.StartedATurn()

		if ctx.Err() != nil {
			return 0, ctx.Err()
		}

		return 7, nil
	})

	if err != nil || out != 7 {
		t.Fatalf("asked-for work: %d, %v", out, err)
	}

	if took := time.Since(start); took > 200*time.Millisecond {
		t.Errorf("asked-for work waited %s", took)
	}
}

// A conversation that keeps going does not keep the work going round for
// ever: after MostRestarts it is left for another time, and says why.
func TestBackgroundWorkStopsTryingAfterSoManyQuestions(t *testing.T) {
	noTurns(t)

	attempts := 0

	var questions sync.WaitGroup

	_, err := yielding(InBackground(context.Background()), func(ctx context.Context) (int, error) {
		attempts++

		// Every attempt is met by a new question, which is answered at once.
		questions.Add(1)

		go func() {
			defer questions.Done()

			progress.StartedATurn()
			<-ctx.Done()
			progress.FinishedATurn()
		}()

		<-ctx.Done()

		return 0, ctx.Err()
	})

	// Every question answered before the next test counts turns.
	questions.Wait()

	if !errors.Is(err, ErrGaveWay) {
		t.Errorf("error %v does not say it gave way", err)
	}

	if attempts != MostRestarts+1 {
		t.Errorf("%d attempts, want %d", attempts, MostRestarts+1)
	}
}

// A turn that never ends must not mean the brain never learns again.
func TestATurnThatNeverEndsDoesNotStopLearningForEver(t *testing.T) {
	noTurns(t)

	was := GiveWayAtMost
	GiveWayAtMost = 300 * time.Millisecond

	t.Cleanup(func() { GiveWayAtMost = was })

	progress.StartedATurn()

	start := time.Now()

	out, err := yielding(InBackground(context.Background()), func(ctx context.Context) (int, error) {
		// Not stopped by the turn it already waited out.
		time.Sleep(100 * time.Millisecond)

		return 3, ctx.Err()
	})

	if err != nil || out != 3 {
		t.Fatalf("after waiting out a stuck turn: %d, %v", out, err)
	}

	if took := time.Since(start); took < 300*time.Millisecond || took > 2*time.Second {
		t.Errorf("took %s, want about GiveWayAtMost and the call", took)
	}
}

// Shutting down while giving way ends the wait at once.
func TestStoppingTheProgramEndsTheWait(t *testing.T) {
	noTurns(t)

	progress.StartedATurn()

	ctx, cancel := context.WithCancel(InBackground(context.Background()))
	done := make(chan struct{})

	called := false

	go func() {
		defer close(done)

		yielding(ctx, func(ctx context.Context) (int, error) {
			called = ctx.Err() == nil

			return 0, ctx.Err()
		})
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	within(t, done, time.Second, "the wait ending on shutdown")

	if called {
		t.Error("work began after the program was told to stop")
	}
}

// --- through the real learner ---

// slowEmbedder embeds like the real one, except that it can be held: a held
// call only ends when its context does, as a model call would.
type slowEmbedder struct {
	mu      sync.Mutex
	hold    bool
	held    chan struct{}
	causes  []error
	embeds  int
	started int
}

func (*slowEmbedder) EmbedModel() string { return "test" }

func (e *slowEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	e.mu.Lock()
	e.started++
	hold := e.hold

	if hold {
		e.hold = false
	}

	e.mu.Unlock()

	if hold {
		close(e.held)
		<-ctx.Done()

		e.mu.Lock()
		e.causes = append(e.causes, context.Cause(ctx))
		e.mu.Unlock()

		return nil, ctx.Err()
	}

	e.mu.Lock()
	e.embeds++
	e.mu.Unlock()

	sum := sha256.Sum256([]byte(text))
	vec := make([]float32, 8)

	for i := range vec {
		vec[i] = float32(sum[i]) - 128
	}

	return vec, nil
}

func testWorker(t *testing.T, embedder llm.Embedder, provider llm.Provider) *Worker {
	t.Helper()

	db, err := store.Open(filepath.Join(t.TempDir(), "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	return &Worker{
		DB:        db,
		Extractor: Extractor{Provider: provider, Owner: "Alex"},
		Curator:   Curator{DB: db, Embedder: embedder},
		Log:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func lessonCount(t *testing.T, db *store.DB) int {
	t.Helper()

	counts, err := db.CountLessonsByStatus()
	if err != nil {
		t.Fatal(err)
	}

	n := 0
	for _, c := range counts {
		n += c
	}

	return n
}

/*
 * A folder being read in the background meets a question halfway through.
 *
 * The embedding in flight is stopped, nothing more is read while the answer is
 * written, and afterwards the reading finishes: every observation known once,
 * none lost, none stored twice, none counted as failed.
 */
func TestReadingAFolderGivesWayToAQuestionAndFinishesAfterwards(t *testing.T) {
	noTurns(t)

	dir := t.TempDir()

	var obs Observations

	for _, name := range []string{"invoices.md", "garden.md", "bike.md"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("notes"), 0o600); err != nil {
			t.Fatal(err)
		}

		obs = append(obs, Observation{
			Content: "Alex keeps notes about " + name + " in " + path,
			Source:  "document:" + path,
		})
	}

	embedder := &slowEmbedder{held: make(chan struct{})}
	w := testWorker(t, embedder, nil)

	// The second observation's embedding is the one a question meets.
	type result struct {
		rep IngestReport
		err error
	}

	got := make(chan result, 1)

	go func() {
		rep, err := w.Ingest(InBackground(context.Background()), obs, func(r IngestReport) {
			// After the first is done, hold the next.
			if r.Promoted == 1 {
				embedder.mu.Lock()
				embedder.hold = true
				embedder.mu.Unlock()
			}
		})
		got <- result{rep, err}
	}()

	within(t, embedder.held, 2*time.Second, "the second embedding starting")

	progress.StartedATurn()

	// Stopped for the question, not left to run beside it.
	deadline := time.Now().Add(time.Second)

	for {
		embedder.mu.Lock()
		stopped := len(embedder.causes) == 1
		embedder.mu.Unlock()

		if stopped {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("the embedding in flight was not stopped for the question")
		}

		time.Sleep(10 * time.Millisecond)
	}

	if !errors.Is(embedder.causes[0], ErrGaveWay) {
		t.Errorf("stopped for %v", embedder.causes[0])
	}

	// Nothing more is embedded while the answer is being written.
	embedder.mu.Lock()
	before := embedder.started
	embedder.mu.Unlock()

	time.Sleep(500 * time.Millisecond)

	embedder.mu.Lock()
	after := embedder.started
	embedder.mu.Unlock()

	if after != before {
		t.Errorf("%d embeddings started during the question", after-before)
	}

	progress.FinishedATurn()

	var r result

	select {
	case r = <-got:
	case <-time.After(3 * time.Second):
		t.Fatal("the reading did not finish after the question")
	}

	if r.err != nil {
		t.Fatal(r.err)
	}

	if r.rep.Promoted != 3 || r.rep.Failed != 0 || r.rep.Duplicates != 0 {
		t.Errorf("after giving way: %+v, want 3 promoted, none failed or duplicate", r.rep)
	}

	if n := lessonCount(t, w.DB); n != 3 {
		t.Errorf("%d lessons stored for 3 observations", n)
	}

	if facts, _ := w.DB.CountFacts(); facts != 3 {
		t.Errorf("%d facts for 3 observations", facts)
	}
}

// heldProvider answers the extractor, holding its first call until it is
// stopped, as a model on a processor would hold for minutes.
type heldProvider struct {
	mu     sync.Mutex
	calls  int
	first  chan struct{}
	causes []error
}

func (*heldProvider) Name() string                   { return llm.Local }
func (*heldProvider) Available(context.Context) bool { return true }

func (p *heldProvider) Chat(ctx context.Context, _ llm.Request) (llm.Response, error) {
	p.mu.Lock()
	p.calls++
	n := p.calls
	p.mu.Unlock()

	if n == 1 {
		close(p.first)
		<-ctx.Done()

		p.mu.Lock()
		p.causes = append(p.causes, context.Cause(ctx))
		p.mu.Unlock()

		return llm.Response{}, ctx.Err()
	}

	return llm.Response{Content: `{"lesson": "Alex keeps the invoices of the bike shop in a folder called Ledger", "confidence": "high"}`}, nil
}

/*
 * Learning from the last conversation, which starts the moment a reply ends,
 * meets the next question. Its model call is stopped, and once the question
 * is answered the conversation is learned from all the same.
 */
func TestLearningFromAConversationGivesWayToTheNextQuestion(t *testing.T) {
	noTurns(t)

	provider := &heldProvider{first: make(chan struct{})}
	w := testWorker(t, &slowEmbedder{held: make(chan struct{})}, provider)

	id, err := w.DB.NewConversation("bike shop")
	if err != nil {
		t.Fatal(err)
	}

	w.DB.AddMessage(id, llm.RoleUser, "", "", "Where do I keep the bike shop invoices?")
	w.DB.AddMessage(id, llm.RoleAssistant, "", "", "In the folder called Ledger.")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		w.Stop()
	})

	w.Start(ctx)
	w.Learn(id)

	within(t, provider.first, 2*time.Second, "learning starting")

	progress.StartedATurn()

	deadline := time.Now().Add(time.Second)

	for {
		provider.mu.Lock()
		stopped := len(provider.causes) == 1
		provider.mu.Unlock()

		if stopped {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("the learning call was not stopped for the question")
		}

		time.Sleep(10 * time.Millisecond)
	}

	if !errors.Is(provider.causes[0], ErrGaveWay) {
		t.Errorf("stopped for %v", provider.causes[0])
	}

	time.Sleep(300 * time.Millisecond)

	if n := lessonCount(t, w.DB); n != 0 {
		t.Errorf("%d lessons written during the question", n)
	}

	progress.FinishedATurn()

	deadline = time.Now().Add(3 * time.Second)

	for lessonCount(t, w.DB) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the conversation was not learned from after the question")
		}

		time.Sleep(20 * time.Millisecond)
	}

	if n := lessonCount(t, w.DB); n != 1 {
		t.Errorf("%d lessons from one conversation", n)
	}

	provider.mu.Lock()
	calls := provider.calls
	provider.mu.Unlock()

	if calls != 2 {
		t.Errorf("%d model calls, want the stopped one and one more", calls)
	}
}

/*
 * What a document says waits for a person, except in the brain's own
 * knowledge folder: the program's guides, and what its owner put there on
 * purpose. Anywhere else a document may be anybody's writing.
 */
func TestTheBrainsOwnKnowledgeIsTrustedAndNothingElse(t *testing.T) {
	root := t.TempDir()

	SetBrainRoot(root)
	t.Cleanup(func() { SetBrainRoot("") })

	guide := filepath.Join(root, "knowledge", "guides", "mail.md")
	if err := os.MkdirAll(filepath.Dir(guide), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(guide, []byte("guide"), 0o600); err != nil {
		t.Fatal(err)
	}

	elsewhere := filepath.Join(t.TempDir(), "contract.md")
	if err := os.WriteFile(elsewhere, []byte("contract"), 0o600); err != nil {
		t.Fatal(err)
	}

	var v Validator

	for source, want := range map[string]string{
		ReadingSource(guide):     StatusValidated,
		ReadingSource(elsewhere): StatusProposed,
		ReadingSource(filepath.Join(root, "knowledge", "gone.md")): StatusRejected,
		// Inside the brain but not its knowledge: its memory is never trusted
		// as something read.
		"reading:" + BrainPrefix + "memory/notes.md": StatusProposed,
	} {
		if got := v.StatusFor(source); got != want {
			t.Errorf("%s: %s, want %s", source, got, want)
		}
	}

	if ReadingSource(guide) != "reading:"+OwnKnowledge+"guides/mail.md" {
		t.Errorf("a guide is remembered as %s", ReadingSource(guide))
	}
}

/*
 * A task the agent works through shows an answer on screen without anybody
 * having asked a question here. Learning steps aside for it briefly, as it
 * always did, but a task that runs for an hour does not stop it for an hour.
 */
func TestATaskOnScreenIsWaitedForOnlyBriefly(t *testing.T) {
	noTurns(t)

	was := HoldOff
	HoldOff = 300 * time.Millisecond

	t.Cleanup(func() {
		HoldOff = was
		progress.Done()
	})

	progress.Begin()
	progress.Set("thinking", "Thinking")

	start := time.Now()

	out, err := yielding(InBackground(context.Background()), func(context.Context) (int, error) {
		return 5, nil
	})

	took := time.Since(start)

	if err != nil || out != 5 {
		t.Fatalf("%d, %v", out, err)
	}

	if took < 250*time.Millisecond || took > 900*time.Millisecond {
		t.Errorf("waited %s for a task on screen, want about HoldOff", took)
	}
}

/*
 * Inside the knowledge folder, only what is the brain's own is trusted.
 *
 * Copies of imported agent skills live there too (knowledge/skills), and
 * those are somebody else's writing: a skill folder from anywhere on the
 * disk, or from a download. And a source is a string, so one that climbs out
 * of the folder with "../" must not be trusted because of how it starts.
 */
func TestOnlyTheBrainsOwnWritingInItsKnowledgeIsTrusted(t *testing.T) {
	root := filepath.Join(t.TempDir(), "brain")

	SetBrainRoot(root)
	t.Cleanup(func() { SetBrainRoot("") })

	write := func(path string) {
		t.Helper()

		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte("text"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write(filepath.Join(root, "knowledge", "guides", "mail.md"))
	write(filepath.Join(root, "knowledge", "my-notes.md"))
	write(filepath.Join(root, "knowledge", "skills", "someones-skill", "SKILL.md"))
	write(filepath.Join(root, "knowledge", "pack-skills", "bookkeeping", "SKILL.md"))
	write(filepath.Join(root, "memory", "notes.md"))
	write(filepath.Join(filepath.Dir(root), "outside.md"))

	var v Validator

	for source, want := range map[string]string{
		"reading:" + OwnKnowledge + "guides/mail.md": StatusValidated,
		"reading:" + OwnKnowledge + "my-notes.md":    StatusValidated,

		// Somebody else's skill, copied into the brain: a person decides.
		"reading:" + OwnKnowledge + "skills/someones-skill/SKILL.md":   StatusProposed,
		"reading:" + OwnKnowledge + "pack-skills/bookkeeping/SKILL.md": StatusProposed,

		// Out of the folder by "..": not the brain's own knowledge.
		"reading:" + OwnKnowledge + "../memory/notes.md":                       StatusProposed,
		"reading:" + OwnKnowledge + "../../outside.md":                         StatusProposed,
		"reading:" + OwnKnowledge + "guides/../../memory/notes.md":             StatusProposed,
		"reading:" + OwnKnowledge + "guides/../skills/someones-skill/SKILL.md": StatusProposed,
	} {
		if got := v.StatusFor(source); got != want {
			t.Errorf("%s: %s, want %s", source, got, want)
		}
	}
}

/*
 * A question that starts while learning is already waiting is waited for too,
 * and costs no attempt.
 *
 * The signal for "a new question started" was taken before the wait. A second
 * question that began during the wait (while the first was still being
 * answered) had therefore already fired it by the time the wait ended, so the
 * call was cancelled the moment it started: an attempt spent on nothing, and
 * three such conversations in a row and the work was dropped for the day.
 */
func TestAQuestionStartedDuringTheWaitDoesNotSpendAnAttempt(t *testing.T) {
	noTurns(t)

	progress.StartedATurn() // the question being answered when learning wants to run

	var (
		mu       sync.Mutex
		attempts int
		causes   []error
	)

	done := make(chan struct{})

	go func() {
		defer close(done)

		yielding(InBackground(context.Background()), func(ctx context.Context) (int, error) {
			mu.Lock()
			attempts++
			mu.Unlock()

			// A call that takes a moment, during which nobody asks anything.
			select {
			case <-ctx.Done():
				mu.Lock()
				causes = append(causes, context.Cause(ctx))
				mu.Unlock()

				return 0, ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return 1, nil
			}
		})
	}()

	// Learning is waiting for the first question. A second one starts, then
	// the first is answered, then the second.
	time.Sleep(200 * time.Millisecond)
	progress.StartedATurn()
	time.Sleep(100 * time.Millisecond)
	progress.FinishedATurn()
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	early := attempts
	mu.Unlock()

	if early != 0 {
		t.Fatalf("background work started while the second question was being answered")
	}

	progress.FinishedATurn()

	within(t, done, 2*time.Second, "the work finishing after both questions")

	mu.Lock()
	defer mu.Unlock()

	if attempts != 1 || len(causes) != 0 {
		t.Errorf("%d attempts, stopped by %v; want one attempt that nothing stopped", attempts, causes)
	}
}
