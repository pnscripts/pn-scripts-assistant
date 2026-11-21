package brain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"pn-scripts-assistant/internal/brain/agent"
	"pn-scripts-assistant/internal/brain/connect"
	"pn-scripts-assistant/internal/brain/learning"
	"pn-scripts-assistant/internal/brain/mcpimport"
	"pn-scripts-assistant/internal/brain/otherprograms"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/redact"
	"pn-scripts-assistant/internal/brain/sandbox"
	"pn-scripts-assistant/internal/brain/speech"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/tools"
	"pn-scripts-assistant/internal/protocol"
)

/*
 * What another program may hand the assistant (package otherprograms, the
 * second half of its switches): a thing to remember, a reminder, a line for
 * the to-do list, a message for its person in the window or aloud, and the
 * running of an action its person wrote for that.
 *
 * The other way round from the questions in outside.go, and held to the
 * same three things, each of them code:
 *
 *   - Its own switch, off until its person turns it on in the window, read
 *     here again whatever the door read.
 *   - On the record with who handed it over, and no record, nothing done:
 *     what is stored is taken back when its line cannot be written, and
 *     what cannot be taken back (a message, a spoken line, a command) is
 *     written down first.
 *   - Nothing here decides for its person. A thing to remember waits for
 *     their yes in the window and is believed by nobody before it; a
 *     message is a card they read; the one thing that runs is a command
 *     they wrote themselves, by its name, with nothing a program adds.
 *     There is no model on this path at all: a program's words are stored
 *     or shown, never followed.
 */

var (
	// ErrHandoverFull is one thing more than may wait at the desk.
	ErrHandoverFull = errors.New("as many of those as may wait at once are waiting already")

	// ErrNoVoice is a line to say aloud with no voice to say it in.
	ErrNoVoice = errors.New("there is no voice here to say it with")

	// ErrStillSaying is a line to say aloud while the one before waits.
	ErrStillSaying = errors.New("the line before this one has not been said yet")

	// ErrNoSuchAction is an action its person never wrote.
	ErrNoSuchAction = errors.New("there is no action with that name")

	// ErrActionRunning is an action asked for while another runs.
	ErrActionRunning = errors.New("another action is running now")

	// ErrActionStopped is an action refused by "Stop using other
	// programs" or by a program's own card; the sentence says which.
	ErrActionStopped = errors.New("running it is stopped")
)

// What each handing over is said to have come to, on the record. The page
// says these in its own language (programs.js reads them by their shape).
const (
	toldResult     = "Kept for you to accept or discard. Nothing is remembered until you accept it."
	todoResult     = "Put on the list kept for other programs."
	messageResult  = "Shown on a card in my window until it is seen."
	spokenResult   = "To be said aloud at the next gap."
	startedResult  = "Started."
	messageSeen    = "Seen."
	messageNoMore  = "Taken off the desk with its switch."
	noVoiceResult  = "Not said: I have no voice here."
	sayingResult   = "Not said: the line before it was not said yet."
	fullMessages   = "Not shown: as many messages as wait at once were waiting already."
	fullTold       = "Not kept: as many things as wait for your yes at once were waiting already."
	runningResult  = "Not run: another action was running."
	noActionResult = "Not run: there is no action with that name."
	fullReminders  = "Not set: as many reminders of other programs as wait at once were waiting already."
	dayReminders   = "Not set: other programs set as many reminders today as they may in a day."
	fullTodos      = "Not put on the list: as many lines of other programs as it holds were on it already."
	remindersGone  = "Taken away with its switch."
)

// handover is the state of what other programs hand over.
type handover struct {
	// taking holds while one thing is counted and taken, so that the
	// numbers that may wait hold when programs hand over at the same
	// moment.
	taking sync.Mutex

	// The day reminders were last set on, and how many were set on it.
	remindedOn string
	reminded   int

	// saying is a line of another program waiting to be said, or being
	// said: one at a time.
	saying atomic.Bool

	// running holds while an action runs: one at a time.
	running sync.Mutex
}

// sayAloud says a line when there is a gap. A variable so a test hears it.
var sayAloud = func(b *Brain, line string, done func()) {
	go func() {
		defer done()

		b.speakWhenTheresAGap(line)
	}()
}

// hasVoice reports whether anything can be said aloud. A variable for a test.
var hasVoice = func() bool { return !speech.Silent() && speech.Available() != nil }

// handed writes one handing over on the record, done.
func (b *Brain) handed(tool, by, asked, result string) error {
	return b.AskedFromOutside(tool, by, asked, agent.DeedDone, result)
}

// notHanded writes one that was refused; a refusal that cannot be written
// is still a refusal.
func (b *Brain) notHanded(tool, by, asked, why string) {
	if err := b.AskedFromOutside(tool, by, asked, agent.DeedRefused, why); err != nil {
		b.Log.Warn("could not record what another program handed over", "tool", tool, "error", err)
	}
}

/*
 * RememberForOutside keeps something a program told the assistant to
 * remember where such things wait for a yes, with who told it. Nothing is
 * believed, recalled or acted on until its person accepts it in the window;
 * until then it is a line on a card.
 */
func (b *Brain) RememberForOutside(by, text string) error {
	if by == "" || text == "" {
		return errors.New("something to remember, with nobody named or nothing said")
	}

	if !b.OutsideMay(otherprograms.Remember) {
		return ErrOutsideNotAllowed
	}

	b.handover.taking.Lock()
	defer b.handover.taking.Unlock()

	// Read again now that nothing else is being taken: a switch turned off
	// a moment ago has taken back what waited, and nothing is added behind
	// it.
	if !b.OutsideMay(otherprograms.Remember) {
		return ErrOutsideNotAllowed
	}

	// Counted by the database: behind a long list of the assistant's own
	// lessons a count of its first page was always nought.
	told, err := b.DB.CountLessonsMarked(learning.StatusProposed, otherprograms.Told)
	if err != nil {
		return err
	}

	if told >= otherprograms.MostToldWaiting {
		b.notHanded(otherprograms.RememberTool, by, "", fullTold)

		return ErrHandoverFull
	}

	id, err := b.DB.AddLesson(0, otherprograms.ToldLineIn(b.Cfg.Language, by, text), learning.StatusProposed, otherprograms.Told, "")
	if err != nil {
		return err
	}

	// The label keeps another program's words from standing at the start
	// of what is stored.
	if err := b.handed(otherprograms.RememberTool, by, "Remember: "+text, toldResult); err != nil {
		// No record, nothing kept.
		if undo := b.DB.SetLessonStatus(id, learning.StatusRejected); undo != nil {
			b.Log.Warn("could not take back something another program told me to remember", "id", id, "error", undo)
		}

		return ErrNotOnTheRecord
	}

	return nil
}

// ToldByAnotherProgram reports whether something waiting to be remembered
// was told by another program: its yes is given in the window only.
func ToldByAnotherProgram(l store.Lesson) bool { return l.Confidence == otherprograms.Told }

// LessonsToDecide is what waits to be remembered, as the window lists it:
// what other programs told first, then the assistant's own.
func (b *Brain) LessonsToDecide(limit int) ([]store.Lesson, error) {
	return b.DB.LessonsMarkedFirst(learning.StatusProposed, otherprograms.Told, limit)
}

/*
 * RemindForOutside sets a reminder a program asked for, with who asked in
 * front of its words. It is said aloud when due, like the assistant's own.
 */
func (b *Brain) RemindForOutside(by, text string, at time.Time) (store.Reminder, error) {
	if by == "" || text == "" {
		return store.Reminder{}, errors.New("a reminder with nobody named or nothing said")
	}

	if !b.OutsideMay(otherprograms.Remind) {
		return store.Reminder{}, ErrOutsideNotAllowed
	}

	b.handover.taking.Lock()
	defer b.handover.taking.Unlock()

	// Read again now that nothing else is being taken: a switch turned off
	// a moment ago has taken back what waited, and nothing is added behind
	// it.
	if !b.OutsideMay(otherprograms.Remind) {
		return store.Reminder{}, ErrOutsideNotAllowed
	}

	// Only so many of theirs wait to be said, and only so many are set in
	// a day: each is said aloud when it is due.
	waiting, err := b.remindersOfPrograms()
	if err != nil {
		return store.Reminder{}, err
	}

	if len(waiting) >= otherprograms.MostRemindersWaiting {
		b.notHanded(otherprograms.RemindTool, by, "", fullReminders)

		return store.Reminder{}, ErrHandoverFull
	}

	if today := time.Now().Format("2006-01-02"); b.handover.remindedOn != today {
		b.handover.remindedOn, b.handover.reminded = today, 0
	}

	if b.handover.reminded >= otherprograms.MostRemindersADay {
		b.notHanded(otherprograms.RemindTool, by, "", dayReminders)

		return store.Reminder{}, ErrHandoverFull
	}

	r, err := b.DB.AddReminder(otherprograms.FromLineIn(b.Cfg.Language, by, text), at)
	if err != nil {
		return store.Reminder{}, err
	}

	result := "Reminder set for " + at.Local().Format("2006-01-02 15:04") + "."

	if err := b.handed(otherprograms.RemindTool, by, "Reminder: "+text, result); err != nil {
		if undo := b.DB.ForgetReminder(r.ID); undo != nil {
			b.Log.Warn("could not take back a reminder another program set", "id", r.ID, "error", undo)
		}

		return store.Reminder{}, ErrNotOnTheRecord
	}

	b.handover.reminded++

	return r, nil
}

// remindersOfPrograms is the reminders other programs set that were not
// said yet. Their words are sealed, so they are told apart here.
func (b *Brain) remindersOfPrograms() ([]store.Reminder, error) {
	all, err := b.DB.Reminders(false, 100000)
	if err != nil {
		return nil, err
	}

	var theirs []store.Reminder

	for _, r := range all {
		if otherprograms.FromAnotherProgram(r.What) {
			theirs = append(theirs, r)
		}
	}

	return theirs, nil
}

// takeBackReminders takes away the reminders of other programs that were
// not said yet, when their switch is turned off: off means nothing of
// theirs is said from then on.
func (b *Brain) takeBackReminders() {
	b.handover.taking.Lock()
	defer b.handover.taking.Unlock()

	theirs, err := b.remindersOfPrograms()
	if err != nil {
		b.Log.Warn("could not look for reminders of other programs to take away", "error", err)

		return
	}

	for _, r := range theirs {
		if err := b.DB.ForgetReminder(r.ID); err != nil {
			b.Log.Warn("could not take away a reminder another program set", "id", r.ID, "error", err)
		}
	}

	if len(theirs) > 0 {
		b.Log.Info("reminders of other programs were taken away with their switch", "reminders", len(theirs), "why", remindersGone)
	}
}

// TodoForOutside puts a line a program handed over on the list kept for
// other programs, with who handed it over.
func (b *Brain) TodoForOutside(by, text string, due *time.Time) (store.Todo, error) {
	if by == "" || text == "" {
		return store.Todo{}, errors.New("a line for the list with nobody named or nothing said")
	}

	if !b.OutsideMay(otherprograms.Todo) {
		return store.Todo{}, ErrOutsideNotAllowed
	}

	b.handover.taking.Lock()
	defer b.handover.taking.Unlock()

	// Read again now that nothing else is being taken: a switch turned off
	// a moment ago has taken back what waited, and nothing is added behind
	// it.
	if !b.OutsideMay(otherprograms.Todo) {
		return store.Todo{}, ErrOutsideNotAllowed
	}

	// Only so many lines of theirs, not ticked, are on the list at once.
	open := 0

	for _, list := range []string{otherprograms.TodoList, otherprograms.TodoListBG} {
		lines, err := b.DB.Todos(list, false)
		if err != nil {
			return store.Todo{}, err
		}

		open += len(lines)
	}

	if open >= otherprograms.MostTodosWaiting {
		b.notHanded(otherprograms.TodoTool, by, "", fullTodos)

		return store.Todo{}, ErrHandoverFull
	}

	t, err := b.DB.AddTodo(otherprograms.TodoListIn(b.Cfg.Language), otherprograms.FromLineIn(b.Cfg.Language, by, text), due)
	if err != nil {
		return store.Todo{}, err
	}

	if err := b.handed(otherprograms.TodoTool, by, "To do: "+text, todoResult); err != nil {
		if undo := b.DB.RemoveTodo(t.ID); undo != nil {
			b.Log.Warn("could not take back a line another program put on the list", "id", t.ID, "error", undo)
		}

		return store.Todo{}, ErrNotOnTheRecord
	}

	return t, nil
}

/*
 * A message from another program.
 *
 * It is shown where the questions that wait for its person are shown: a
 * card with the name the program gave and one button, "Seen". It asks for
 * nothing and nothing is carried out for it; the card is there so that the
 * message is read by a person and by nobody else. In particular it never
 * enters a conversation: what is said there is read by the model on its
 * next turn, and a program's words must not arrive there as the
 * assistant's own.
 */

// IsMessage reports whether a waiting card is a message another program
// left, which is seen and not decided.
func IsMessage(inv store.Invocation) bool { return inv.Tool == otherprograms.TellTool }

// actionsWaiting is the questions that wait for a yes or a no: everything
// waiting but the messages other programs left, which are only seen.
func (b *Brain) actionsWaiting() []store.Invocation {
	pending, err := b.DB.PendingInvocations()
	if err != nil {
		return nil
	}

	out := pending[:0:0]

	for _, inv := range pending {
		if !IsMessage(inv) {
			out = append(out, inv)
		}
	}

	return out
}

// TellForOutside leaves a message of another program on a card in the
// window, until its person has seen it.
func (b *Brain) TellForOutside(by, text string) (int64, error) {
	if by == "" || text == "" {
		return 0, errors.New("a message with nobody named or nothing said")
	}

	if !b.OutsideMay(otherprograms.Tell) {
		return 0, ErrOutsideNotAllowed
	}

	b.handover.taking.Lock()
	defer b.handover.taking.Unlock()

	// Read again now that nothing else is being taken: a switch turned off
	// a moment ago has taken back what waited, and nothing is added behind
	// it.
	if !b.OutsideMay(otherprograms.Tell) {
		return 0, ErrOutsideNotAllowed
	}

	pending, err := b.DB.PendingInvocations()
	if err != nil {
		return 0, err
	}

	waiting := 0

	for _, inv := range pending {
		if IsMessage(inv) {
			waiting++
		}
	}

	if waiting >= otherprograms.MostMessagesWaiting {
		b.notHanded(otherprograms.TellTool, by, "", fullMessages)

		return 0, ErrHandoverFull
	}

	// Shown cannot be taken back, so it is written down first.
	if err := b.handed(otherprograms.TellTool, by, "Message: "+text, messageResult); err != nil {
		return 0, ErrNotOnTheRecord
	}

	arguments, err := json.Marshal(map[string]string{"message": text})
	if err != nil {
		return 0, err
	}

	en, bg := "Message: "+text, "Съобщение: "+text

	id, err := b.DB.RecordInvocationAskedBy(0, otherprograms.TellTool, string(arguments), en, bg, string(tools.Safe), by)
	if err != nil {
		b.notHanded(otherprograms.TellTool, by, "", "Not shown: the card could not be made.")

		return 0, err
	}

	b.Happens.Say(protocol.New(protocol.ApprovalRequired, en).
		With(map[string]any{"invocation": id, "tool": otherprograms.TellTool, "asked_by": by}))

	return id, nil
}

// seenMessage closes a message its person has seen (decide, after the row
// moved from waiting). Either button means the same: there is nothing to
// refuse.
func (b *Brain) seenMessage(inv store.Invocation) (store.Invocation, error) {
	inv.Status, inv.Result = store.InvocationDone, messageSeen

	if err := b.DB.CompleteInvocation(inv.ID, inv.Status, inv.Result); err != nil {
		b.Log.Warn("could not close a message that was seen", "id", inv.ID, "error", err)
	}

	return inv, nil
}

// takeBackMessages takes the messages that wait off the desk, when their
// switch is turned off.
func (b *Brain) takeBackMessages() {
	pending, err := b.DB.PendingInvocations()
	if err != nil {
		b.Log.Warn("could not look for messages of other programs to take off the desk", "error", err)

		return
	}

	for _, inv := range pending {
		if !IsMessage(inv) {
			continue
		}

		if moved, err := b.DB.DecideInvocation(inv.ID, store.InvocationDenied); err != nil || !moved {
			continue
		}

		if err := b.DB.CompleteInvocation(inv.ID, store.InvocationDenied, messageNoMore); err != nil {
			b.Log.Warn("could not close a message taken off the desk", "id", inv.ID, "error", err)
		}

		progress.Decided(inv.ID, progress.Refused)

		b.Happens.Say(protocol.New(protocol.ApprovalReceived, "").
			With(map[string]any{"invocation": inv.ID, "tool": otherprograms.TellTool, "approved": false}))
	}
}

// SpokenLine is a line of another program as it is said aloud: that it is
// another program's first, then the name it gave, so nobody takes it for
// the assistant's own or for somebody the program named itself after.
func SpokenLine(by, text string) string { return otherprograms.SaidLine(by, text) }

/*
 * SayForOutside says a line of another program aloud, in the assistant's
 * voice, when there is a gap in what is being said and heard. One at a
 * time: a second line while the first has not been said is refused.
 */
func (b *Brain) SayForOutside(by, text string) error {
	if by == "" || text == "" {
		return errors.New("a line to say with nobody named or nothing said")
	}

	if !b.OutsideMay(otherprograms.Speak) {
		return ErrOutsideNotAllowed
	}

	if !hasVoice() {
		b.notHanded(otherprograms.SpeakTool, by, "", noVoiceResult)

		return ErrNoVoice
	}

	if !b.handover.saying.CompareAndSwap(false, true) {
		b.notHanded(otherprograms.SpeakTool, by, "", sayingResult)

		return ErrStillSaying
	}

	// Said cannot be taken back, so it is written down first.
	if err := b.handed(otherprograms.SpeakTool, by, "Say: "+text, spokenResult); err != nil {
		b.handover.saying.Store(false)

		return ErrNotOnTheRecord
	}

	sayAloud(b, otherprograms.SaidLineIn(b.Cfg.Language, by, text), func() { b.handover.saying.Store(false) })

	return nil
}

/*
 * The actions its person wrote for other programs to run.
 *
 * An action is a name and a command. Both are written at the desk, in the
 * assistant's own window (the server takes them with the window's key and
 * from nowhere else); a program gives a name and gets that command run as
 * it was written, with nothing of its own added: no arguments, no
 * environment, no folder. So what a program can make happen is exactly the
 * list its person wrote, and writing the list is the approval.
 */

// ActionsTool is the name the record keeps a change of the list under.
const ActionsTool = "other_programs_action"

/*
 * actionTimeout is how long an action may run: under the minute after which
 * the programs that ask give up on a call (the Model Context Protocol's own
 * client), so that how it ended is always told. An action that takes longer
 * is written to start its work and return. A variable for a test.
 */
var actionTimeout = 45 * time.Second

// ActionTimeout is how long an action may run, for whoever waits on one.
func ActionTimeout() time.Duration { return actionTimeout }

// mostActionOutput is how much of what an action printed is handed back,
// in characters.
const mostActionOutput = 4000

// OutsideActions is every action its person wrote.
func (b *Brain) OutsideActions() ([]store.OutsideAction, error) { return b.DB.OutsideActions() }

// ErrNotAnAction is a name or a command that cannot be kept, with why.
type ErrNotAnAction struct{ Why string }

func (e ErrNotAnAction) Error() string { return e.Why }

// Why an action cannot be kept, as codes the page says in its language.
const (
	ActionBadName    = "name"
	ActionBadCommand = "command"
	ActionTaken      = "taken"
	ActionsFull      = "full"
)

// AddOutsideAction keeps an action its person wrote, with its line on the
// record. From the window only (the server sees to that).
func (b *Brain) AddOutsideAction(name, command string) (store.OutsideAction, error) {
	clean := otherprograms.ActionName(name)
	if clean == "" {
		return store.OutsideAction{}, ErrNotAnAction{ActionBadName}
	}

	// One line, as typed: what runs is what its person read on the page.
	command = strings.TrimSpace(command)
	if command == "" || len([]rune(command)) > otherprograms.MostActionCommand ||
		otherprograms.Words(command, otherprograms.MostActionCommand) != command || strings.ContainsAny(command, "\r\n") {
		return store.OutsideAction{}, ErrNotAnAction{ActionBadCommand}
	}

	a, err := b.DB.AddOutsideAction(clean, command, otherprograms.MostActions, store.Deed{
		Tool:      ActionsTool,
		Summary:   "An action other programs may run was written: " + clean,
		SummaryBG: "Написано е действие, което другите програми могат да изпълняват: " + clean,
		Asked:     "Command: " + command,
		Outcome:   agent.DeedDone,
		Changes:   true,
	})

	switch {
	case errors.Is(err, store.ErrActionExists):
		return a, ErrNotAnAction{ActionTaken}
	case errors.Is(err, store.ErrActionsFull):
		return a, ErrNotAnAction{ActionsFull}
	}

	return a, err
}

// RemoveOutsideAction takes an action away, with its line on the record.
func (b *Brain) RemoveOutsideAction(id int64) error {
	return b.DB.RemoveOutsideAction(id, func(a store.OutsideAction) store.Deed {
		return store.Deed{
			Tool:      ActionsTool,
			Summary:   "An action other programs could run was taken away: " + a.Name,
			SummaryBG: "Премахнато е действие, което другите програми можеха да изпълняват: " + a.Name,
			Outcome:   agent.DeedDone,
			Changes:   true,
		}
	})
}

// ActionRun is how one run of an action ended.
type ActionRun struct {
	Name   string `json:"name"`
	Exit   int    `json:"exit"`
	Millis int64  `json:"millis"`

	// Output is what it printed, cut to mostActionOutput characters; Cut
	// is whether there was more.
	Output string `json:"output"`
	Cut    bool   `json:"cut,omitempty"`

	// TimedOut is an action stopped for running too long.
	TimedOut bool `json:"timed_out,omitempty"`
}

// shellFor is the command line of an action as it is started: through the
// shell, as its person would type it in a terminal.
func shellFor(command string) []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/C", command}
	}

	return []string{"/bin/sh", "-c", command}
}

// actionEnd is how one command ended.
type actionEnd struct {
	output   string
	cut      bool
	exit     int
	timedOut bool
}

// mostActionHeld is how much of what a command prints is held while it
// runs, in bytes; what is over is dropped as it comes.
const mostActionHeld = 64 << 10

// held keeps the first so many bytes written to it and counts the rest.
type held struct {
	mu   sync.Mutex
	kept []byte
	over bool
}

func (h *held) Write(p []byte) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if room := mostActionHeld - len(h.kept); room >= len(p) {
		h.kept = append(h.kept, p...)
	} else {
		if room > 0 {
			h.kept = append(h.kept, p[:room]...)
		}

		h.over = true
	}

	return len(p), nil
}

/*
 * runAction runs one command line and reports how it ended, read from the
 * process itself: its exit code, never what it printed. Started with
 * nothing of this program's (no keys in its environment), in a process
 * group of its own, with nothing to read from. A command that starts work
 * in the background and returns is over when it returns, whatever that
 * work still holds open. A variable so a test runs nothing.
 */
var runAction = func(ctx context.Context, command string) (actionEnd, error) {
	ctx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()

	cmd, err := sandbox.Command(ctx, sandbox.Spec{Inherit: true}, shellFor(command)...)
	if err != nil {
		return actionEnd{}, err
	}

	var out held

	cmd.Stdin = nil
	cmd.Stdout = &out
	cmd.Stderr = &out

	err = cmd.Run()

	out.mu.Lock()
	end := actionEnd{output: string(out.kept), cut: out.over}
	out.mu.Unlock()

	var exited *exec.ExitError

	switch {
	case ctx.Err() == context.DeadlineExceeded:
		end.timedOut, end.exit = true, -1
	case err == nil || errors.Is(err, exec.ErrWaitDelay):
		// Ended, with something it started still holding its output.
		if cmd.ProcessState != nil {
			end.exit = cmd.ProcessState.ExitCode()
		}
	case errors.As(err, &exited):
		end.exit = exited.ExitCode()
	default:
		return end, err
	}

	return end, nil
}

/*
 * RunActionForOutside runs the action its person wrote under name, for the
 * program that asked. One at a time. It is on the record before it starts,
 * and the line says how it ended once it has.
 */
func (b *Brain) RunActionForOutside(ctx context.Context, by, name string) (ActionRun, error) {
	if by == "" {
		return ActionRun{}, errors.New("an action asked for with nobody named")
	}

	if !b.OutsideMay(otherprograms.Actions) {
		return ActionRun{}, ErrOutsideNotAllowed
	}

	name = otherprograms.ActionName(name)

	actions, err := b.DB.OutsideActions()
	if err != nil {
		return ActionRun{}, err
	}

	var action *store.OutsideAction

	for i := range actions {
		if name != "" && actions[i].Name == name {
			action = &actions[i]
		}
	}

	if action == nil {
		// Told with the names there are, so only when it is on the record.
		if err := b.AskedFromOutside(otherprograms.ActionTool, by, "", agent.DeedRefused, noActionResult); err != nil {
			return ActionRun{}, ErrNotOnTheRecord
		}

		return ActionRun{}, ErrNoSuchAction
	}

	asked := "Action: " + action.Name

	/*
	 * "Stop using other programs", and a program's own card set to never:
	 * what its person closed stays closed for a command they wrote
	 * earlier, as it does for the assistant's own.
	 */
	if tool, known := b.Agent.Registry.Get("run_command"); known {
		raw, _ := json.Marshal(map[string]any{"argv": shellFor(action.Command)})

		why := b.stoppedFor(tool)
		if why == "" {
			why = b.refusedByItsCard(ctx, tool, raw)
		}

		if why != "" {
			b.notHanded(otherprograms.ActionTool, by, asked, "Not run: "+why)

			return ActionRun{Name: action.Name}, fmt.Errorf("%w: %s", ErrActionStopped, why)
		}
	}

	if !b.handover.running.TryLock() {
		b.notHanded(otherprograms.ActionTool, by, asked, runningResult)

		return ActionRun{Name: action.Name}, ErrActionRunning
	}
	defer b.handover.running.Unlock()

	// Run cannot be taken back, so it is written down first.
	en, bg := otherprograms.Asked(otherprograms.ActionTool)

	deed, err := b.DB.AddDeed(store.Deed{
		Tool: otherprograms.ActionTool, Summary: en, SummaryBG: bg, Asked: asked,
		Result: startedResult, Outcome: agent.DeedWaiting, AskedBy: by, Changes: true,
	})
	if err != nil {
		return ActionRun{Name: action.Name}, ErrNotOnTheRecord
	}

	began := time.Now()

	// Not the request's own context: a program that stops waiting does not
	// stop a command half way; its limit of time does.
	runCtx := context.WithoutCancel(ctx)

	end, runErr := runAction(runCtx, action.Command)

	run := ActionRun{Name: action.Name, Millis: time.Since(began).Milliseconds(), Exit: end.exit, TimedOut: end.timedOut, Cut: end.cut}
	outcome, result := agent.DeedDone, ""

	// What it printed goes to another program and on to whatever model
	// that uses: keys and tokens in it are taken out, as everywhere a
	// command's words are kept or passed on.
	output := redact.Text(strings.ToValidUTF8(end.output, ""))

	switch {
	case runErr != nil:
		run.Exit = -1
		outcome, result = agent.DeedFailed, "Could not be started."
		output = ""
	case end.timedOut:
		outcome, result = agent.DeedFailed, fmt.Sprintf("Stopped: it was still running after %s.", actionTimeout)
	default:
		result = fmt.Sprintf("Ended with exit code %d after %.1f s.", run.Exit, float64(run.Millis)/1000)

		if run.Exit != 0 {
			outcome = agent.DeedFailed
		}
	}

	if all := []rune(output); len(all) > mostActionOutput {
		output, run.Cut = string(all[:mostActionOutput]), true
	}

	run.Output = strings.TrimSpace(output)

	if err := b.DB.FinishDeed(deed, outcome, result, run.Millis); err != nil {
		b.Log.Warn("could not write how an action for another program ended", "action", action.Name, "error", err)
	}

	return run, nil
}

/*
 * Connecting another program to the assistant (package connect): the entry
 * that starts "pn-scripts-assistant mcp" is written into that program's
 * own settings, after its person read which file and what entry, in the
 * window.
 */

// ConnectTool is the name the record keeps a connection under.
const ConnectTool = "connect_program"

// ownProgram is the full path of the running program. A variable so a
// test names one.
var ownProgram = os.Executable

// thisProgram is the full path of the running program, when it is a file
// that is there: after an update the system can name a file that is gone,
// and an entry that starts nothing must not be offered or written.
func thisProgram() (string, error) {
	program, err := ownProgram()
	if err != nil {
		return "", fmt.Errorf("where this program is could not be found: %w", err)
	}

	if info, err := os.Stat(program); err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("this program is not where it was started from (%s): close the assistant and start it again", program)
	}

	return program, nil
}

// ConnectPlans is what connecting each program would do, to be shown.
func (b *Brain) ConnectPlans() ([]connect.Plan, error) {
	program, err := thisProgram()
	if err != nil {
		return nil, err
	}

	out := make([]connect.Plan, 0, len(connect.Clients))

	for _, c := range connect.Clients {
		plan, err := connect.PlanFor(c, b.mcpEnv(), program)
		if err != nil {
			return nil, err
		}

		out = append(out, plan)
	}

	return out, nil
}

/*
 * Connect writes the assistant into one program's settings, for the plan
 * its person was shown (seen) and no other. It is on the record before
 * the file is touched, and the line says how it ended: with the file and
 * where its copy from before is, or that nothing was written. From the
 * window only (the server sees to that).
 */
func (b *Brain) Connect(client mcpimport.Client, seen string) (connect.Done, error) {
	program, err := thisProgram()
	if err != nil {
		return connect.Done{}, err
	}

	if !connect.Known(client) {
		return connect.Done{}, fmt.Errorf("%q is not a program this connects to", client)
	}

	// Looked at first, so that a request for a plan nobody was shown, or
	// for what is there already, is refused with nothing begun and nothing
	// to write down. Write looks again before it writes.
	plan, err := connect.PlanFor(client, b.mcpEnv(), program)

	switch {
	case err != nil:
		return connect.Done{}, err
	case plan.Seen != seen:
		return connect.Done{}, connect.ErrChanged
	case plan.State == connect.StateUnread:
		return connect.Done{}, connect.ErrUnread
	case plan.State == connect.StateSame:
		return connect.Done{}, connect.ErrSame
	}

	// No record, nothing written.
	deed, err := b.DB.AddDeed(store.Deed{
		Tool:      ConnectTool,
		Summary:   "Connect " + client.Title() + " to me, so that it can start me as an MCP server",
		SummaryBG: "Свързване на " + client.Title() + " с мен, за да може да ме стартира като MCP сървър",
		Result:    startedResult,
		Outcome:   agent.DeedWaiting,
		Changes:   true,
	})
	if err != nil {
		return connect.Done{}, ErrNotOnTheRecord
	}

	done, err := connect.Write(client, b.mcpEnv(), program, seen, time.Now())

	outcome, result := agent.DeedDone, "Written into "+done.File+"."

	switch {
	case err != nil:
		outcome, result = agent.DeedFailed, "Nothing was written: "+err.Error()+"."
	case done.Backup != "":
		result += " The file as it was is kept as " + done.Backup + "."
	}

	if ferr := b.DB.FinishDeed(deed, outcome, result, 0); ferr != nil {
		b.Log.Warn("could not write how connecting another program ended", "client", string(client), "error", ferr)
	}

	return done, err
}

// HearInstead gives every line said aloud for another program to hear
// instead of saying it, as though there were a voice, until the function it
// returns is called. For the tests of other packages: nothing in the
// program calls it.
func HearInstead(hear func(line string)) (restore func()) {
	said, voice := sayAloud, hasVoice

	sayAloud = func(_ *Brain, line string, done func()) {
		hear(line)
		done()
	}
	hasVoice = func() bool { return true }

	return func() { sayAloud, hasVoice = said, voice }
}
