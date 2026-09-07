package brain

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/learning"
	"pn-scripts-assistant/internal/brain/places"

	"pn-scripts-assistant/internal/brain/store"
)

// Reply time on a CPU is close to linear in prompt size, and recall is the
// biggest part of that prompt.
func TestRecallIsTrimmedForThePrompt(t *testing.T) {
	long := `Petar has a PHP/Composer project called "ps_emailsubscription" at ` +
		`/media/petar/DEV/Projects/divacon.bg/modules/ps_emailsubscription, last modified 2025-08-19. ` +
		`README excerpt: # E-mail subscription form [![Build Status](https://travis-ci.com/x.svg)](https://t) ` +
		strings.Repeat("more badge noise ", 40)

	out := formatRecall([]store.Scored{{Fact: store.Fact{Content: long}}})

	if len([]rune(out)) > RecalledFactLimit+200 {
		t.Errorf("recall preamble is %d runes; trimming did not happen", len([]rune(out)))
	}

	// The part that answers questions must survive.
	for _, want := range []string{"ps_emailsubscription", "/media/petar/DEV/Projects", "2025-08-19"} {
		if !strings.Contains(out, want) {
			t.Errorf("trimming removed %q, which is the useful part", want)
		}
	}

	// The part that is noise must not.
	if strings.Contains(out, "travis-ci") {
		t.Error("a build-status badge reached the prompt")
	}
}

// A fact cut mid-word reads as though it were complete and slightly wrong.
func TestTrimPrefersASentenceBoundary(t *testing.T) {
	s := "Petar has a Go project called xplorer at /home/petar/x, last modified 2026-01-01. " +
		strings.Repeat("trailing detail that should go away ", 20)

	got := trimForPrompt(s, 120)

	if !strings.HasSuffix(got, ".") {
		t.Errorf("did not end at a sentence: %q", got)
	}

	if strings.Contains(got, "trailing detail") {
		t.Errorf("kept the tail: %q", got)
	}
}

// Short facts must pass through untouched, or every website fact would gain a
// pointless ellipsis.
func TestShortFactsAreNotTrimmed(t *testing.T) {
	s := "Petar regularly uses the website pnscripts.local (1160 visits in browser history)."

	if got := trimForPrompt(s, RecalledFactLimit); got != s {
		t.Errorf("a short fact was altered:\n  got  %q\n  want %q", got, s)
	}
}

// A greeting is composed rather than generated, so it must arrive instantly.
// Asking the model would cost most of a minute before it said hello.
func TestGreetingIsImmediate(t *testing.T) {
	b := testBrain(t)

	started := time.Now()
	greeting := b.Greet()

	if took := time.Since(started); took > 200*time.Millisecond {
		t.Errorf("greeting took %v; it must not wait on a model", took)
	}

	if greeting.Text == "" {
		t.Fatal("greeting is empty")
	}
}

// What is worth saying on opening is what is waiting, not that it exists.
func TestGreetingMentionsWhatNeedsTheOwner(t *testing.T) {
	b := testBrain(t)

	b.DB.AddLesson(0, "something the model guessed", "proposed", "low", "")
	b.DB.AddLesson(0, "another guess", "proposed", "low", "")

	text := b.Greet().Text

	if !strings.Contains(text, "2") {
		t.Errorf("greeting does not mention the two waiting lessons: %q", text)
	}
}

// With nothing waiting it should say what it has, not just announce itself.
func TestGreetingFallsBackToWhatItKnows(t *testing.T) {
	b := testBrain(t)

	for i := 0; i < 3; i++ {
		b.DB.AddFact("project", "a thing it knows", []float32{1, 0, 0})
	}

	text := b.Greet().Text

	if !strings.Contains(text, "3") {
		t.Errorf("greeting does not mention what it knows: %q", text)
	}
}

// An empty brain should say so and point somewhere, rather than claim to know
// nothing useful and stop.
func TestEmptyBrainSaysHowToTeachIt(t *testing.T) {
	b := testBrain(t)

	text := b.Greet().Text

	if !strings.Contains(strings.ToLower(text), "folder") {
		t.Errorf("greeting gives an empty brain nothing to act on: %q", text)
	}
}

// testBrain is an otherwise empty brain on a temporary database.
func testBrain(t *testing.T) *Brain {
	t.Helper()

	root := t.TempDir()

	db, err := store.Open(filepath.Join(root, "brain.sqlite"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { db.Close() })

	cfg := config.Default()
	cfg.Owner = "Petar"

	return New(db, cfg, root, filepath.Join(root, "brain.sqlite"),
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// The greeting says four things are waiting, so "remember them" is the obvious
// next sentence. Before this the model answered "I will remember these
// instructions" and did nothing at all.
func TestRememberThemActuallyRemembers(t *testing.T) {
	b := testBrain(t)
	b.Learner.Curator.Embedder = fixedEmbedder{}

	b.DB.AddLesson(0, "Petar prefers Laravel over Python", "proposed", "high", "")
	b.DB.AddLesson(0, "Petar works from Sofia", "proposed", "high", "")

	answer, handled := b.handleLessonInstruction(context.Background(), "Okay, remember them.")

	if !handled {
		t.Fatal("the instruction was not recognised")
	}

	if !strings.Contains(answer, "2") {
		t.Errorf("answer does not say what happened: %q", answer)
	}

	pending, _ := b.DB.CountPendingLessons()
	if pending != 0 {
		t.Errorf("%d lessons still waiting after being told to remember them", pending)
	}

	facts, _ := b.DB.CountFacts()
	if facts != 2 {
		t.Errorf("%d facts stored, want 2", facts)
	}
}

func TestForgetThemDiscards(t *testing.T) {
	b := testBrain(t)

	b.DB.AddLesson(0, "a guess", "proposed", "low", "")

	answer, handled := b.handleLessonInstruction(context.Background(), "forget them")

	if !handled {
		t.Fatal("not recognised")
	}

	if !strings.Contains(strings.ToLower(answer), "discard") {
		t.Errorf("answer is %q", answer)
	}

	if pending, _ := b.DB.CountPendingLessons(); pending != 0 {
		t.Errorf("%d still pending", pending)
	}

	if facts, _ := b.DB.CountFacts(); facts != 0 {
		t.Errorf("%d facts stored despite being told to forget", facts)
	}
}

// A false positive silently rewrites what the brain believes about its owner,
// so the match has to be narrow.
func TestOrdinarySentencesAreNotInstructions(t *testing.T) {
	b := testBrain(t)
	b.DB.AddLesson(0, "a guess", "proposed", "low", "")

	notInstructions := []string{
		"Remind me to forget about the meeting tomorrow morning",
		"What do you remember about my Go projects?",
		"I keep them in the drawer next to my desk downstairs",
		"Do you remember when we discussed the Laravel upgrade last week?",
		"Can you help me remember my password",
		"Tell me what you would like to remember and why",
	}

	for _, message := range notInstructions {
		if _, handled := b.handleLessonInstruction(context.Background(), message); handled {
			t.Errorf("treated as an instruction: %q", message)
		}
	}

	if pending, _ := b.DB.CountPendingLessons(); pending != 1 {
		t.Error("the queue was changed by a sentence that was not an instruction")
	}
}

// With nothing waiting, "remember that" is somebody talking about something
// else entirely.
func TestInstructionIsIgnoredWithAnEmptyQueue(t *testing.T) {
	b := testBrain(t)

	if _, handled := b.handleLessonInstruction(context.Background(), "remember them"); handled {
		t.Error("handled an instruction with an empty queue")
	}
}

// fixedEmbedder gives every distinct text its own direction.
//
// Summing character codes was not enough: two sentences that both begin
// "Petar " came out similar enough to be treated as the same fact. A hash
// spreads them properly, which is what a real embedder does for genuinely
// different statements.
type fixedEmbedder struct{}

func (fixedEmbedder) EmbedModel() string { return "fixed" }

func (fixedEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	sum := sha256.Sum256([]byte(text))

	vec := make([]float32, 8)

	for i := range vec {
		vec[i] = float32(sum[i]) - 128
	}

	return vec, nil
}

/*
 * A queue worked a hundred at a time must not report the hundred as the whole.
 *
 * Petar's queue held 9,197 lessons. Every count in the interface and every
 * answer in the conversation was a report on one page of it: the badge read
 * 100 because that is what the fetch returns, and the reply read "all 100"
 * because that is what the batch contained. Both were true of the page and
 * false of the queue, which is the only reading anybody cares about.
 */
func TestALargeQueueSaysWhatIsLeft(t *testing.T) {
	b := testBrain(t)

	for i := 0; i < queuePage+5; i++ {
		b.DB.AddLesson(0, fmt.Sprintf("a guess number %d", i), "proposed", "low", "")
	}

	answer, handled := b.handleLessonInstruction(context.Background(), "forget them")

	if !handled {
		t.Fatal("not recognised")
	}

	left, _ := b.DB.CountPendingLessons()
	if left != 5 {
		t.Fatalf("%d left after one page, want 5", left)
	}

	if !strings.Contains(answer, "5 more are still waiting") {
		t.Errorf("answer does not say what is left: %q", answer)
	}

	// "Discarded all 100" over five that are still there is the same lie in
	// friendlier words.
	if strings.Contains(answer, "all") {
		t.Errorf("called one page of the queue all of it: %q", answer)
	}
}

// The ordinary case must not grow a footnote: four lessons, four gone, nothing
// behind them, so the answer ends where it always did.
func TestAQueueThatIsFinishedSaysNothingMore(t *testing.T) {
	b := testBrain(t)

	for i := 0; i < 4; i++ {
		b.DB.AddLesson(0, fmt.Sprintf("a guess number %d", i), "proposed", "low", "")
	}

	answer, _ := b.handleLessonInstruction(context.Background(), "forget them")

	if strings.Contains(answer, "still waiting") {
		t.Errorf("mentioned a remainder that does not exist: %q", answer)
	}

	if !strings.Contains(answer, "all 4") {
		t.Errorf("would not say all when it really was all: %q", answer)
	}
}

/*
 * The sentence Petar actually said, three times, in three sessions.
 *
 * "I want you to start to remember one by one everything that is in the
 * waiting list" went to the model every time, because the matcher only looked
 * at sentences of six words or fewer. The model answered that it had processed
 * a large number of lessons, having processed none — which is the exact
 * failure this file exists to prevent, arriving through the door that was left
 * open for long sentences.
 */
func TestNamingTheListMakesALongSentenceAnInstruction(t *testing.T) {
	for _, said := range []string{
		`I want you to start to remember one by one everything that is in the waiting list, the things that are in the waiting list.`,
		`remember everything in the waiting list`,
		`I want you to remember everything in "WAITING FOR YOU"`,
		`please keep all the things in the review queue`,
	} {
		got := readLessonInstruction(said)

		if !got.Found || !got.Accept {
			t.Errorf("%q was not read as an instruction to keep: %+v", said, got)
		}

		if !got.All {
			t.Errorf("%q asks for all of it, read as a hundred: %+v", said, got)
		}
	}

	// And the other verb, on the same shape of sentence.
	got := readLessonInstruction("discard everything in the waiting list")

	if !got.Found || got.Accept || !got.All {
		t.Errorf("discarding the whole list was read as %+v", got)
	}
}

/*
 * A question about the list is not an instruction about it.
 *
 * The cost of getting this wrong is nine thousand judgements made on somebody's
 * behalf, from a sentence that was asking what was there.
 */
func TestAQuestionAboutTheListChangesNothing(t *testing.T) {
	for _, said := range []string{
		"what is in the waiting list?",
		"do you remember the waiting list",
		"how many things are in the review queue",
		"can you show me the waiting list",
		"what's waiting for you",
		"tell me what is in the waiting list",
	} {
		if got := readLessonInstruction(said); got.Found {
			t.Errorf("%q was treated as an instruction: %+v", said, got)
		}
	}
}

// "Remember them" is still the short form, and still means what is in front of
// you rather than nine thousand.
func TestTheShortFormIsStillOnePage(t *testing.T) {
	got := readLessonInstruction("okay, remember them")

	if !got.Found || !got.Accept {
		t.Fatalf("the short form stopped working: %+v", got)
	}

	if got.All {
		t.Error(`"remember them" was read as the whole queue`)
	}
}

/*
 * The whole queue, not the first hundred of it, and once each.
 *
 * Walking with LessonsByStatus would have been the obvious thing and is wrong
 * twice over: accepting a lesson takes it out of the results, so "the first
 * hundred proposed" returns a different hundred each time with everything
 * shifted up, and anything that cannot be stored stays at the front and is met
 * again on every pass, forever.
 */
func TestTheWholeQueueIsWalkedOnce(t *testing.T) {
	b := testBrain(t)

	const many = queuePage*2 + 7

	for i := 0; i < many; i++ {
		b.DB.AddLesson(0, fmt.Sprintf("a guess number %d", i), "proposed", "low", "")
	}

	// Discarding rather than accepting, so this is a test of the walk and not
	// of the embedder.
	answer, err := b.workThroughTheQueue(context.Background(), false, many)
	if err != nil {
		t.Fatal(err)
	}

	left, _ := b.DB.CountPendingLessons()
	if left != 0 {
		t.Errorf("%d still waiting after going through all of them", left)
	}

	if !strings.Contains(answer, fmt.Sprint(many)) {
		t.Errorf("the report does not say how many: %q", answer)
	}
}

// Stopping part way is not failing, and the answer says which it was.
func TestStoppingPartWayIsReportedAsStopping(t *testing.T) {
	b := testBrain(t)

	for i := 0; i < 5; i++ {
		b.DB.AddLesson(0, fmt.Sprintf("a guess number %d", i), "proposed", "low", "")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	answer, err := b.workThroughTheQueue(ctx, false, 5)
	if err != nil {
		t.Fatal(err)
	}

	if left, _ := b.DB.CountPendingLessons(); left != 5 {
		t.Errorf("a cancelled walk still went through %d of them", 5-left)
	}

	if !strings.Contains(answer, "nothing in the waiting list") &&
		!strings.Contains(answer, "Stopped") {
		t.Errorf("a cancelled walk reported itself as finished: %q", answer)
	}
}

/*
 * The instruction reaches the background, and the answer says so.
 *
 * The end-to-end of the sentence Petar said: it is recognised, a job is
 * started, and he is told it will take a while rather than being handed a
 * finished-sounding sentence about a queue that has not moved.
 */
func TestTheWholeListInstructionStartsAJobAndSaysSo(t *testing.T) {
	b := testBrain(t)

	for i := 0; i < 250; i++ {
		b.DB.AddLesson(0, fmt.Sprintf("a guess number %d", i), "proposed", "low", "")
	}

	answer, handled := b.handleLessonInstruction(context.Background(),
		"I want you to start to remember one by one everything that is in the waiting list")

	if !handled {
		t.Fatal("the instruction was not recognised")
	}

	if !strings.Contains(answer, "250") {
		t.Errorf("the answer does not say how many it is working through: %q", answer)
	}

	if !strings.Contains(strings.ToLower(answer), "background") {
		t.Errorf("the answer does not say it is happening behind the conversation: %q", answer)
	}

	// And it really is a job, listed where somebody can stop it.
	running := b.Jobs.List()

	if len(running) != 1 {
		t.Fatalf("%d background jobs, want 1", len(running))
	}

	if !strings.Contains(running[0].What, "250") {
		t.Errorf("the job does not name the size of the queue: %q", running[0].What)
	}

	b.Jobs.Stop(running[0].ID)
}

/*
 * "I am learning from that", said every ninety seconds for an afternoon.
 *
 * A place is read a few files at a time over hours, and each bite was a fresh
 * step for the page to narrate. What Petar asked for instead is one sentence
 * when a folder is finished, saying what came out of it.
 */
func TestFinishingAFolderSaysWhatCameOutOfIt(t *testing.T) {
	b := testBrain(t)

	// Two waiting from this folder, one from somewhere else.
	b.DB.AddLesson(0, "something from a document", "proposed", "high",
		"document:/home/petar/Documents/a.pdf")
	b.DB.AddLesson(0, "something else from a document", "proposed", "high",
		"reading:/home/petar/Documents/b.pdf")
	b.DB.AddLesson(0, "from another drive entirely", "proposed", "high",
		"document:/media/drive/c.pdf")

	said := b.finishedReading(
		places.Place{Name: "your documents", Path: "/home/petar/Documents"},
		places.Pass{
			Place: places.Place{Name: "your documents", Learned: 31},
			Known: 4,
		})

	for _, want := range []string{
		"finished reading your documents",
		"I learned 31 things",
		"4 more I already knew",
		"2 of them are waiting for you",
	} {
		if !strings.Contains(said, want) {
			t.Errorf("the line does not say %q:\n  %s", want, said)
		}
	}

	// And never the present tense that was the complaint.
	if strings.Contains(said, "I am learning") {
		t.Errorf("still narrating the middle: %s", said)
	}
}

// A folder that held nothing says so, because "finished" on its own cannot be
// told apart from "finished and found nothing worth having".
func TestAnEmptyFolderSaysThatItWasEmpty(t *testing.T) {
	b := testBrain(t)

	said := b.finishedReading(
		places.Place{Name: "your desktop", Path: "/home/petar/Desktop"},
		places.Pass{Place: places.Place{Name: "your desktop", Learned: 0}})

	if !strings.Contains(said, "nothing in it worth remembering") {
		t.Errorf("an empty folder was reported as a success: %s", said)
	}

	if strings.Contains(said, "waiting for you") {
		t.Errorf("invented something to review: %s", said)
	}
}

/*
 * The queue is a person's attention, and it is finite.
 *
 * The one rule here that does not depend on guessing whose writing a file is.
 * Every other rule is a judgement about content, and judgements are wrong
 * sometimes: this queue reached 9,199, then 2,895, each time because a rule
 * that was right about yesterday's disk met a folder nobody had thought of.
 * On somebody else's machine it will meet another one.
 *
 * So the reading stops at a number a person can imagine finishing, and starts
 * again by itself when they have.
 */
func TestReadingStopsWhenThePersonHasEnoughToDo(t *testing.T) {
	b := testBrain(t)

	// There has to be something that could read, or it stops for that reason
	// instead and the check never runs.
	b.Learner = &learning.Worker{DB: b.DB}

	for i := 0; i < AsMuchAsAnyoneWillReview; i++ {
		b.DB.AddLesson(0, fmt.Sprintf("a guess number %d", i), "proposed", "low", "")
	}

	if b.lookAtOnePlace(context.Background()) {
		t.Error("kept reading with a full queue")
	}

	// And it says why, because a brain that has quietly stopped reading looks
	// exactly like one that has finished, and those mean opposite things.
	said, err := whatItSaid(b)
	if err != nil {
		t.Fatal(err)
	}

	var explained bool

	for _, m := range said {
		if strings.Contains(m.Content, "waiting for you") &&
			strings.Contains(m.Content, "stopped reading") {
			explained = true
		}
	}

	if !explained {
		t.Errorf("stopped reading without saying so: %+v", said)
	}
}

// And it says it once. Checked every ten minutes for as long as the queue
// stays full, and a program that repeats itself every ten minutes is one
// somebody turns off.
func TestTheFullQueueIsExplainedOnce(t *testing.T) {
	b := testBrain(t)
	b.Learner = &learning.Worker{DB: b.DB}

	for i := 0; i < AsMuchAsAnyoneWillReview; i++ {
		b.DB.AddLesson(0, fmt.Sprintf("a guess number %d", i), "proposed", "low", "")
	}

	for i := 0; i < 4; i++ {
		b.lookAtOnePlace(context.Background())
	}

	said, _ := whatItSaid(b)

	var times int

	for _, m := range said {
		if strings.Contains(m.Content, "stopped reading") {
			times++
		}
	}

	if times != 1 {
		t.Errorf("said it %d times, want once", times)
	}
}

/*
 * whatItSaid reads back the lines the brain wrote on its own.
 *
 * Through the conversation it actually chose, not conversation 0 — writing
 * there is what made these lines vanish for as long as they have existed:
 * messages.conversation_id is NOT NULL with a foreign key, so the database
 * refused every one and the error was swallowed. See sayWhenFinished.
 */
func whatItSaid(b *Brain) ([]store.Message, error) {
	latest, err := b.DB.LatestConversation()
	if err != nil || latest == nil {
		return nil, err
	}

	return b.DB.History(latest.ID)
}
