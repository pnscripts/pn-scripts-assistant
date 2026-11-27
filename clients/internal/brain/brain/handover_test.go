package brain

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/learning"
	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/store"
)

/*
 * What another program hands the assistant (handover.go), as the brain
 * takes it. For each kind the same four things: with its switch off
 * nothing is taken and nothing is kept; with it on the thing is taken; it
 * is on the record with who handed it over; and nothing in it is a yes
 * given for the assistant's person.
 */

const claude = "Claude Code 2.1.0"

func allowing(t *testing.T, subjects ...otherprograms.Subject) *Brain {
	t.Helper()

	b := testBrain(t)

	for _, s := range subjects {
		if err := b.AllowOutside(s, true); err != nil {
			t.Fatal(err)
		}
	}

	return b
}

// handedOver is the lines on the record for one tool, newest first.
func handedOver(t *testing.T, b *Brain, tool string) []store.Deed {
	t.Helper()

	var out []store.Deed

	for _, d := range outsideDeeds(t, b) {
		if d.Tool == tool {
			out = append(out, d)
		}
	}

	return out
}

// recordAway makes the record unwritable, as a full or damaged disk does.
func recordAway(t *testing.T, b *Brain) {
	t.Helper()

	if _, err := b.DB.SQL().Exec(`ALTER TABLE deeds RENAME TO deeds_away`); err != nil {
		t.Fatal(err)
	}
}

func proposed(t *testing.T, b *Brain) []store.Lesson {
	t.Helper()

	lessons, err := b.DB.LessonsByStatus(learning.StatusProposed, 1000)
	if err != nil {
		t.Fatal(err)
	}

	return lessons
}

/*
 * With its switch off, each handing over is refused by the brain itself,
 * whatever the door read, and leaves nothing behind: no lesson, no
 * reminder, no line, no card, no sound, nothing run.
 */
func TestWithItsSwitchOffNothingIsTakenFromAnotherProgram(t *testing.T) {
	b := testBrain(t)

	said, ran := 0, 0

	hold(t, &sayAloud, func(*Brain, string, func()) { said++ })
	hold(t, &hasVoice, func() bool { return true })
	hold(t, &runAction, func(context.Context, string) (actionEnd, error) { ran++; return actionEnd{}, nil })

	if _, err := b.DB.AddOutsideAction("backup", "true", otherprograms.MostActions, store.Deed{Tool: ActionsTool, Outcome: "done"}); err != nil {
		t.Fatal(err)
	}

	if err := b.RememberForOutside(claude, "x"); !errors.Is(err, ErrOutsideNotAllowed) {
		t.Errorf("a thing to remember with the switch off: %v", err)
	}

	if _, err := b.RemindForOutside(claude, "x", time.Now().Add(time.Hour)); !errors.Is(err, ErrOutsideNotAllowed) {
		t.Errorf("a reminder with the switch off: %v", err)
	}

	if _, err := b.TodoForOutside(claude, "x", nil); !errors.Is(err, ErrOutsideNotAllowed) {
		t.Errorf("a line for the list with the switch off: %v", err)
	}

	if _, err := b.TellForOutside(claude, "x"); !errors.Is(err, ErrOutsideNotAllowed) {
		t.Errorf("a message with the switch off: %v", err)
	}

	if err := b.SayForOutside(claude, "x"); !errors.Is(err, ErrOutsideNotAllowed) {
		t.Errorf("a line to say with the switch off: %v", err)
	}

	if _, err := b.RunActionForOutside(context.Background(), claude, "backup"); !errors.Is(err, ErrOutsideNotAllowed) {
		t.Errorf("an action with the switch off: %v", err)
	}

	reminders, _ := b.DB.Reminders(true, 100)
	todos, _ := b.DB.Todos(otherprograms.TodoList, true)
	cards, _ := b.PendingApprovals()

	if len(proposed(t, b)) != 0 || len(reminders) != 0 || len(todos) != 0 || len(cards) != 0 || said != 0 || ran != 0 {
		t.Errorf("with every switch off something was kept: %d lessons, %d reminders, %d lines, %d cards, %d said, %d run",
			len(proposed(t, b)), len(reminders), len(todos), len(cards), said, ran)
	}

	// One switch on allows that one thing and no other.
	if err := b.AllowOutside(otherprograms.Todo, true); err != nil {
		t.Fatal(err)
	}

	if _, err := b.TodoForOutside(claude, "x", nil); err != nil {
		t.Errorf("a line for the list with its switch on: %v", err)
	}

	if err := b.RememberForOutside(claude, "x"); !errors.Is(err, ErrOutsideNotAllowed) {
		t.Errorf("the switch for the list allowed a thing to remember: %v", err)
	}

	if _, err := b.RunActionForOutside(context.Background(), claude, "backup"); !errors.Is(err, ErrOutsideNotAllowed) || ran != 0 {
		t.Errorf("the switch for the list allowed an action: %v (%d run)", err, ran)
	}
}

// hold puts a stand-in in a variable for the length of a test.
func hold[T any](t *testing.T, where *T, with T) {
	t.Helper()

	was := *where
	*where = with

	t.Cleanup(func() { *where = was })
}

/*
 * Something a program tells the assistant to remember is not remembered:
 * it waits with who told it, is on the record, and only a yes on its card
 * in the window makes it something the assistant knows. Every other way of
 * saying yes leaves it waiting.
 */
func TestWhatAProgramToldIsRememberedOnlyByAYesInTheWindow(t *testing.T) {
	b := allowing(t, otherprograms.Remember)
	b.Learner.Curator.Embedder = fixedEmbedder{}

	if err := b.RememberForOutside(claude, "Alex deploys on Fridays."); err != nil {
		t.Fatal(err)
	}

	waiting := proposed(t, b)
	if len(waiting) != 1 || waiting[0].Content != "Told by another program (Claude Code 2.1.0): Alex deploys on Fridays." ||
		waiting[0].Confidence != otherprograms.Told || waiting[0].Source != "" || !ToldByAnotherProgram(waiting[0]) {
		t.Fatalf("what waits: %+v", waiting)
	}

	told := waiting[0]

	deeds := handedOver(t, b, otherprograms.RememberTool)
	if len(deeds) != 1 || deeds[0].AskedBy != claude || deeds[0].Outcome != "done" || deeds[0].Asked != "Remember: Alex deploys on Fridays." ||
		!strings.Contains(deeds[0].Result, "Nothing is remembered until you accept it") {
		t.Fatalf("the record: %+v", deeds)
	}

	if n, _ := b.DB.CountFacts(); n != 0 {
		t.Fatalf("%d things are known before anybody said yes", n)
	}

	// A lesson of the assistant's own beside it, to see the difference.
	own, _ := b.DB.AddLesson(0, "Alex works from Sofia", learning.StatusProposed, "high", "")

	// Said out loud or typed: "remember them".
	if _, handled := b.handleLessonInstruction(context.Background(), "Okay, remember them."); !handled {
		t.Fatal("the sentence was not taken as an instruction")
	}

	// By number, as decide_waiting does, and listed for it.
	if _, err := b.DecideLessonItem(context.Background(), told.ID, true); !errors.Is(err, ErrToldInTheWindow) {
		t.Errorf("a yes by number: %v", err)
	}

	if items, _ := b.PendingLessons(); len(items) != 0 {
		t.Errorf("what a program told is offered to a sentence to decide: %+v", items)
	}

	// The whole queue worked through in the background.
	if _, err := b.workThroughTheQueue(context.Background(), true, 1); err != nil {
		t.Fatal(err)
	}

	// And the plain decision every other caller uses.
	if _, err := b.DecideLesson(context.Background(), told.ID, true); !errors.Is(err, ErrToldInTheWindow) {
		t.Errorf("a yes from anywhere but the card: %v", err)
	}

	if l, _ := b.DB.Lesson(told.ID); l == nil || l.Status != learning.StatusProposed {
		t.Fatalf("what a program told did not stay waiting: %+v", l)
	}

	if l, _ := b.DB.Lesson(own); l == nil || l.Status != learning.StatusPromoted {
		t.Fatalf("the assistant's own lesson was not remembered by the same sentence: %+v", l)
	}

	if n, _ := b.DB.CountFacts(); n != 1 {
		t.Fatalf("%d things are known; only the assistant's own lesson should be", n)
	}

	// The card in the window.
	decision, err := b.DecideLessonInTheWindow(context.Background(), told.ID, true)
	if err != nil || decision.Status != learning.StatusPromoted {
		t.Fatalf("a yes on its card: %+v, %v", decision, err)
	}

	facts, _ := b.DB.AllFacts()

	found := false

	for _, f := range facts {
		// It keeps who told it, for whoever reads it later.
		if f.Content == "Told by another program (Claude Code 2.1.0): Alex deploys on Fridays." {
			found = true
		}
	}

	if !found {
		t.Errorf("what was accepted is not known with who told it: %+v", facts)
	}
}

// Discarding needs no window: nothing is believed by a no.
func TestWhatAProgramToldCanBeDiscardedFromAnywhere(t *testing.T) {
	b := allowing(t, otherprograms.Remember)

	if err := b.RememberForOutside(claude, "x is y"); err != nil {
		t.Fatal(err)
	}

	told := proposed(t, b)[0]

	if _, err := b.DecideLesson(context.Background(), told.ID, false); err != nil {
		t.Fatalf("a no from anywhere: %v", err)
	}

	if l, _ := b.DB.Lesson(told.ID); l.Status != learning.StatusRejected {
		t.Errorf("not discarded: %+v", l)
	}
}

func TestOnlySoManyThingsToldWaitAtOnce(t *testing.T) {
	b := allowing(t, otherprograms.Remember)

	for i := 0; i < otherprograms.MostToldWaiting; i++ {
		if err := b.RememberForOutside(claude, "fact "+string(rune('a'+i))); err != nil {
			t.Fatalf("thing %d: %v", i, err)
		}
	}

	if err := b.RememberForOutside(claude, "one more"); !errors.Is(err, ErrHandoverFull) {
		t.Fatalf("one over: %v", err)
	}

	if n := len(proposed(t, b)); n != otherprograms.MostToldWaiting {
		t.Errorf("%d wait", n)
	}

	// The refusal is on the record, without what was told.
	last := handedOver(t, b, otherprograms.RememberTool)[0]
	if last.Outcome != "refused" || last.Asked != "" || !strings.Contains(last.Result, "were waiting already") {
		t.Errorf("the refusal on the record: %+v", last)
	}
}

/*
 * No record, nothing kept: what is stored is taken back when its line
 * cannot be written, and what cannot be taken back does not happen.
 */
func TestWhatCannotBeRecordedIsNotTaken(t *testing.T) {
	b := allowing(t, otherprograms.Remember, otherprograms.Remind, otherprograms.Todo, otherprograms.Tell, otherprograms.Speak, otherprograms.Actions)

	said, ran := 0, 0

	hold(t, &sayAloud, func(_ *Brain, _ string, done func()) { said++; done() })
	hold(t, &hasVoice, func() bool { return true })
	hold(t, &runAction, func(context.Context, string) (actionEnd, error) { ran++; return actionEnd{}, nil })

	if _, err := b.AddOutsideAction("backup", "true"); err != nil {
		t.Fatal(err)
	}

	recordAway(t, b)

	if err := b.RememberForOutside(claude, "x"); !errors.Is(err, ErrNotOnTheRecord) {
		t.Errorf("a thing to remember: %v", err)
	}

	if _, err := b.RemindForOutside(claude, "x", time.Now().Add(time.Hour)); !errors.Is(err, ErrNotOnTheRecord) {
		t.Errorf("a reminder: %v", err)
	}

	if _, err := b.TodoForOutside(claude, "x", nil); !errors.Is(err, ErrNotOnTheRecord) {
		t.Errorf("a line for the list: %v", err)
	}

	if _, err := b.TellForOutside(claude, "x"); !errors.Is(err, ErrNotOnTheRecord) {
		t.Errorf("a message: %v", err)
	}

	if err := b.SayForOutside(claude, "x"); !errors.Is(err, ErrNotOnTheRecord) {
		t.Errorf("a line to say: %v", err)
	}

	if _, err := b.RunActionForOutside(context.Background(), claude, "backup"); !errors.Is(err, ErrNotOnTheRecord) {
		t.Errorf("an action: %v", err)
	}

	reminders, _ := b.DB.Reminders(true, 100)
	todos, _ := b.DB.Todos(otherprograms.TodoList, true)
	cards, _ := b.PendingApprovals()

	if len(proposed(t, b)) != 0 || len(reminders) != 0 || len(todos) != 0 || len(cards) != 0 || said != 0 || ran != 0 {
		t.Errorf("without a record something was kept: %d lessons, %d reminders, %d lines, %d cards, %d said, %d run",
			len(proposed(t, b)), len(reminders), len(todos), len(cards), said, ran)
	}

	// And a second line can be said afterwards: the first did not leave
	// the voice taken.
	if b.handover.saying.Load() {
		t.Error("a line that was never said still holds the voice")
	}
}

func TestAReminderAndALineOfTheListSayWhoHandedThemOver(t *testing.T) {
	b := allowing(t, otherprograms.Remind, otherprograms.Todo)

	at := time.Now().Add(90 * time.Minute)

	r, err := b.RemindForOutside(claude, "call the bank", at)
	if err != nil {
		t.Fatal(err)
	}

	reminders, _ := b.DB.Reminders(false, 10)
	if len(reminders) != 1 || reminders[0].ID != r.ID || reminders[0].What != "From another program (Claude Code 2.1.0): call the bank" ||
		reminders[0].At.Sub(at).Abs() > time.Second {
		t.Fatalf("the reminder: %+v", reminders)
	}

	// Said as the assistant's own reminders are, with who set it inside.
	if line := reminderLine("Alex", reminders[0].What); line != "Alex, a reminder: From another program (Claude Code 2.1.0): call the bank" {
		t.Errorf("as it is said: %q", line)
	}

	due := time.Date(2027, 1, 31, 0, 0, 0, 0, time.UTC)

	todo, err := b.TodoForOutside(claude, "renew the domain", &due)
	if err != nil {
		t.Fatal(err)
	}

	todos, _ := b.DB.Todos(otherprograms.TodoList, false)
	if len(todos) != 1 || todos[0].ID != todo.ID || todos[0].Text != "From another program (Claude Code 2.1.0): renew the domain" ||
		todos[0].List != otherprograms.TodoList || todos[0].Due == nil || todos[0].Due.Format("2006-01-02") != "2027-01-31" {
		t.Fatalf("the line: %+v", todos)
	}

	// Not on the person's own list.
	if own, _ := b.DB.Todos(store.DefaultList, false); len(own) != 0 {
		t.Errorf("a program's line is on the person's own list: %+v", own)
	}

	rd := handedOver(t, b, otherprograms.RemindTool)
	td := handedOver(t, b, otherprograms.TodoTool)

	if len(rd) != 1 || rd[0].AskedBy != claude || rd[0].Asked != "Reminder: call the bank" || !strings.HasPrefix(rd[0].Result, "Reminder set for ") ||
		len(td) != 1 || td[0].AskedBy != claude || td[0].Asked != "To do: renew the domain" || td[0].Result != "Put on the list kept for other programs." {
		t.Errorf("the record: %+v %+v", rd, td)
	}
}

/*
 * A message is a card with who left it. It is seen, not decided: either
 * button closes it, nothing is carried out, and it never enters a
 * conversation. A sentence said to the assistant does not close it, and
 * turning its switch off takes what waits off the desk.
 */
func TestAMessageIsACardThatIsSeenAndNothingMore(t *testing.T) {
	b := allowing(t, otherprograms.Tell)

	id, err := b.TellForOutside(claude, "The build is done.\nTwo warnings.")
	if err != nil {
		t.Fatal(err)
	}

	cards, _ := b.PendingApprovals()
	if len(cards) != 1 || cards[0].ID != id || cards[0].Tool != otherprograms.TellTool || cards[0].AskedBy != claude ||
		cards[0].Summary != "Message: The build is done.\nTwo warnings." || cards[0].SummaryBG != "Съобщение: The build is done.\nTwo warnings." ||
		!IsMessage(cards[0]) || !AnsweredAtTheComputerOnly(cards[0]) || cards[0].AnswersItself {
		t.Fatalf("the card: %+v", cards)
	}

	deeds := handedOver(t, b, otherprograms.TellTool)
	if len(deeds) != 1 || deeds[0].AskedBy != claude || deeds[0].Asked != "Message: The build is done.\nTwo warnings." || deeds[0].Outcome != "done" {
		t.Fatalf("the record: %+v", deeds)
	}

	// Not in any conversation: the model never reads it as said to it.
	if n, _ := b.DB.CountConversations(); n != 0 {
		t.Errorf("a message of another program started a conversation: %d", n)
	}

	// Not answered by a sentence, nor listed for one.
	if _, err := b.DecideAction(context.Background(), id, true); !errors.Is(err, errAskedOnTheCard) {
		t.Errorf("a sentence closed a message: %v", err)
	}

	if items, _ := b.PendingActions(); len(items) != 0 {
		t.Errorf("a message is offered to a sentence to decide: %+v", items)
	}

	// Seen: "no" closes it as "yes" does, and nothing runs.
	seen, err := b.Decide(context.Background(), id, false)
	if err != nil || seen.Status != store.InvocationDone || seen.Result != "Seen." {
		t.Fatalf("seen: %+v, %v", seen, err)
	}

	if cards, _ := b.PendingApprovals(); len(cards) != 0 {
		t.Errorf("still waiting: %+v", cards)
	}

	if again := handedOver(t, b, otherprograms.TellTool); len(again) != 1 {
		t.Errorf("seeing a message wrote the record again: %+v", again)
	}

	// Only so many wait; one more is refused, on the record.
	for i := 0; i < otherprograms.MostMessagesWaiting; i++ {
		if _, err := b.TellForOutside(claude, "message"); err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
	}

	if _, err := b.TellForOutside(claude, "one more"); !errors.Is(err, ErrHandoverFull) {
		t.Fatalf("one over: %v", err)
	}

	if last := handedOver(t, b, otherprograms.TellTool)[0]; last.Outcome != "refused" || last.Asked != "" {
		t.Errorf("the refusal on the record: %+v", last)
	}

	// The switch off: what waits goes with it.
	if err := b.AllowOutside(otherprograms.Tell, false); err != nil {
		t.Fatal(err)
	}

	if cards, _ := b.PendingApprovals(); len(cards) != 0 {
		t.Errorf("messages wait behind a switch that is off: %d", len(cards))
	}
}

func TestALineIsSaidAloudWithWhoSaysItOneAtATime(t *testing.T) {
	b := allowing(t, otherprograms.Speak)

	var (
		mu      sync.Mutex
		lines   []string
		release []func()
	)

	hold(t, &sayAloud, func(_ *Brain, line string, done func()) {
		mu.Lock()
		lines, release = append(lines, line), append(release, done)
		mu.Unlock()
	})

	// No voice: refused and recorded, nothing said.
	hold(t, &hasVoice, func() bool { return false })

	if err := b.SayForOutside(claude, "hello"); !errors.Is(err, ErrNoVoice) {
		t.Fatalf("with no voice: %v", err)
	}

	if d := handedOver(t, b, otherprograms.SpeakTool); len(d) != 1 || d[0].Outcome != "refused" || d[0].Result != "Not said: I have no voice here." || len(lines) != 0 {
		t.Fatalf("with no voice: %+v, said %v", d, lines)
	}

	hasVoice = func() bool { return true }

	if err := b.SayForOutside(claude, "The build is done."); err != nil {
		t.Fatal(err)
	}

	if len(lines) != 1 || lines[0] != "Another program (Claude Code 2.1.0) says: The build is done." {
		t.Fatalf("what is said: %q", lines)
	}

	if d := handedOver(t, b, otherprograms.SpeakTool)[0]; d.Outcome != "done" || d.AskedBy != claude || d.Asked != "Say: The build is done." || d.Result != "To be said aloud at the next gap." {
		t.Errorf("the record: %+v", d)
	}

	// A second line before the first was said.
	if err := b.SayForOutside(claude, "And another thing."); !errors.Is(err, ErrStillSaying) || len(lines) != 1 {
		t.Fatalf("a second line at once: %v, said %v", err, lines)
	}

	release[0]()

	if err := b.SayForOutside(claude, "And another thing."); err != nil || len(lines) != 2 {
		t.Fatalf("a second line after the first: %v, said %v", err, lines)
	}
}

/*
 * An action is its person's own command, written at the desk. A program
 * gives a name and gets that command, as written; a name that is not
 * there runs nothing; one runs at a time; and each run is on the record
 * before it starts and says how it ended.
 */
func TestAnActionRunsAsItsPersonWroteItAndIsOnTheRecord(t *testing.T) {
	b := allowing(t, otherprograms.Actions)

	var ran []string

	hold(t, &runAction, func(_ context.Context, command string) (actionEnd, error) {
		ran = append(ran, command)

		return actionEnd{output: "copied 3 files"}, nil
	})

	a, err := b.AddOutsideAction(" Backup Now ", "rsync -a ~/work /mnt/backup")
	if err != nil || a.Name != "backup-now" || a.Command != "rsync -a ~/work /mnt/backup" {
		t.Fatalf("the action as kept: %+v, %v", a, err)
	}

	run, err := b.RunActionForOutside(context.Background(), claude, "backup-now")
	if err != nil || run.Name != "backup-now" || run.Exit != 0 || run.Output != "copied 3 files" {
		t.Fatalf("the run: %+v, %v", run, err)
	}

	if len(ran) != 1 || ran[0] != "rsync -a ~/work /mnt/backup" {
		t.Fatalf("what ran: %q", ran)
	}

	d := handedOver(t, b, otherprograms.ActionTool)
	if len(d) != 1 || d[0].AskedBy != claude || d[0].Asked != "Action: backup-now" || d[0].Outcome != "done" ||
		!strings.HasPrefix(d[0].Result, "Ended with exit code 0 after ") || !d[0].Changes {
		t.Fatalf("the record: %+v", d)
	}

	// What a program adds to the name is not run: there is no such action.
	for _, name := range []string{"backup-now; rm -rf ~", "backup-now --delete", "../backup-now", "nothing-here", "", "?"} {
		if _, err := b.RunActionForOutside(context.Background(), claude, name); !errors.Is(err, ErrNoSuchAction) {
			t.Errorf("the name %q: %v", name, err)
		}
	}

	if len(ran) != 1 {
		t.Fatalf("something ran for a name that is not an action: %q", ran)
	}

	if last := handedOver(t, b, otherprograms.ActionTool)[0]; last.Outcome != "refused" || last.Result != "Not run: there is no action with that name." {
		t.Errorf("the refusal on the record: %+v", last)
	}

	// A run that fails is said as it was.
	runAction = func(context.Context, string) (actionEnd, error) { return actionEnd{output: "disk full", exit: 3}, nil }

	run, err = b.RunActionForOutside(context.Background(), claude, "backup-now")
	if err != nil || run.Exit != 3 || run.Output != "disk full" {
		t.Fatalf("a failing run: %+v, %v", run, err)
	}

	if last := handedOver(t, b, otherprograms.ActionTool)[0]; last.Outcome != "failed" || !strings.HasPrefix(last.Result, "Ended with exit code 3 after ") {
		t.Errorf("a failing run on the record: %+v", last)
	}

	// One at a time.
	inside, leave := make(chan struct{}), make(chan struct{})

	runAction = func(context.Context, string) (actionEnd, error) {
		close(inside)
		<-leave

		return actionEnd{}, nil
	}

	first := make(chan error, 1)

	go func() {
		_, err := b.RunActionForOutside(context.Background(), claude, "backup-now")
		first <- err
	}()

	<-inside

	if _, err := b.RunActionForOutside(context.Background(), claude, "backup-now"); !errors.Is(err, ErrActionRunning) {
		t.Errorf("a second action while one runs: %v", err)
	}

	// While it runs the record says it was started.
	var started bool

	for _, d := range handedOver(t, b, otherprograms.ActionTool) {
		if d.Outcome == "waiting" && d.Result == "Started." {
			started = true
		}
	}

	if !started {
		t.Error("a running action is not on the record before it ends")
	}

	close(leave)

	if err := <-first; err != nil {
		t.Fatal(err)
	}
}

// The real thing once: a command of this computer, through the shell.
func TestAnActionReallyRuns(t *testing.T) {
	b := allowing(t, otherprograms.Actions)

	if _, err := b.AddOutsideAction("say-hello", "echo hello from the action && exit 4"); err != nil {
		t.Fatal(err)
	}

	run, err := b.RunActionForOutside(context.Background(), claude, "say-hello")
	if err != nil || run.Exit != 4 || strings.TrimSpace(run.Output) != "hello from the action" {
		t.Fatalf("the run: %+v, %v", run, err)
	}

	hold(t, &actionTimeout, 300*time.Millisecond)

	if _, err := b.AddOutsideAction("sleeps", "sleep 20"); err != nil {
		t.Fatal(err)
	}

	began := time.Now()

	run, err = b.RunActionForOutside(context.Background(), claude, "sleeps")
	if err != nil || !run.TimedOut || time.Since(began) > 10*time.Second {
		t.Fatalf("an action that runs too long: %+v, %v after %s", run, err, time.Since(began))
	}

	if last := handedOver(t, b, otherprograms.ActionTool)[0]; last.Outcome != "failed" || !strings.HasPrefix(last.Result, "Stopped: it was still running after") {
		t.Errorf("on the record: %+v", last)
	}
}

func TestTheListOfActionsIsWrittenWithCare(t *testing.T) {
	b := testBrain(t)

	for name, command := range map[string]string{
		"":         "true",
		"rm -rf /": "true",
		"име":      "true",
		"ok":       "",
		"ok2":      "one\ntwo",
		"ok3":      strings.Repeat("a", otherprograms.MostActionCommand+1),
		"ok4":      "echo ‮gnp.exe",
	} {
		var not ErrNotAnAction

		if _, err := b.AddOutsideAction(name, command); !errors.As(err, &not) {
			t.Errorf("the action %q / %q was kept: %v", name, command, err)
		}
	}

	if _, err := b.AddOutsideAction("backup", "true"); err != nil {
		t.Fatal(err)
	}

	var not ErrNotAnAction

	if _, err := b.AddOutsideAction("Backup", "false"); !errors.As(err, &not) || not.Why != ActionTaken {
		t.Errorf("a second action of one name: %v", err)
	}

	actions, _ := b.OutsideActions()
	if len(actions) != 1 || actions[0].Command != "true" {
		t.Fatalf("the list: %+v", actions)
	}

	// Writing and taking away are on the record, as changes, by the
	// action's name and never its command: the record is read by programs
	// on this computer and by paired devices (ASSISTANT-ACTION-COMMAND-RECORD-1127).
	deeds, _ := b.DB.Deeds(store.DeedFilter{Limit: 10})
	if len(deeds) != 1 || deeds[0].Tool != ActionsTool || !deeds[0].Changes || deeds[0].Asked != "" ||
		!strings.HasSuffix(deeds[0].Summary, ": backup") || !strings.HasSuffix(deeds[0].SummaryBG, ": backup") {
		t.Fatalf("the record of writing one: %+v", deeds)
	}

	if err := b.RemoveOutsideAction(actions[0].ID); err != nil {
		t.Fatal(err)
	}

	if err := b.RemoveOutsideAction(actions[0].ID); !errors.Is(err, store.ErrNoAction) {
		t.Errorf("taking away what is gone: %v", err)
	}

	deeds, _ = b.DB.Deeds(store.DeedFilter{Limit: 10})
	if len(deeds) != 2 || !strings.Contains(deeds[0].Summary, "taken away: backup") {
		t.Fatalf("the record of taking one away: %+v", deeds)
	}

	for i := 0; i < otherprograms.MostActions; i++ {
		if _, err := b.AddOutsideAction("a"+string(rune('a'+i)), "true"); err != nil {
			t.Fatalf("action %d: %v", i, err)
		}
	}

	if _, err := b.AddOutsideAction("one-more", "true"); !errors.As(err, &not) || not.Why != ActionsFull {
		t.Errorf("one action over: %v", err)
	}
}

// "Stop using other programs" stops the actions too, and says so on the record.
func TestStopUsingOtherProgramsStopsTheActions(t *testing.T) {
	b := allowing(t, otherprograms.Actions)

	ran := 0

	hold(t, &runAction, func(context.Context, string) (actionEnd, error) { ran++; return actionEnd{}, nil })

	if _, err := b.AddOutsideAction("backup", "true"); err != nil {
		t.Fatal(err)
	}

	if _, ok := b.Agent.Registry.Get("run_command"); !ok {
		t.Skip("this brain has no run_command tool")
	}

	if err := b.StopUsingPrograms(); err != nil {
		t.Fatal(err)
	}

	if _, err := b.RunActionForOutside(context.Background(), claude, "backup"); !errors.Is(err, ErrActionStopped) || ran != 0 {
		t.Fatalf("with other programs stopped: %v (%d run)", err, ran)
	}

	if last := handedOver(t, b, otherprograms.ActionTool)[0]; last.Outcome != "refused" || !strings.HasPrefix(last.Result, "Not run: ") {
		t.Errorf("on the record: %+v", last)
	}
}

// The list a program's lines go on is named in the language the
// assistant is used in.
func TestTheListForOtherProgramsIsNamedInThePersonsLanguage(t *testing.T) {
	b := allowing(t, otherprograms.Todo)
	b.Cfg.Language = "bg"

	todo, err := b.TodoForOutside(claude, "подновяване на домейна", nil)
	if err != nil || todo.List != "от други програми" || todo.Text != "От друга програма (Claude Code 2.1.0): подновяване на домейна" {
		t.Fatalf("the line in Bulgarian: %+v, %v", todo, err)
	}

	// And it counts as a program's, for the number that may be open.
	if !otherprograms.FromAnotherProgram(todo.Text) {
		t.Error("a line in Bulgarian is not told from the person's own")
	}
}
