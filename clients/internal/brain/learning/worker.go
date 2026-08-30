package learning

import (
	"context"
	"fmt"
	"log/slog"
	"pn-brain/internal/brain/progress"
	"pn-brain/internal/brain/speech"
	"strings"
	"sync"
	"time"

	"pn-brain/internal/brain/llm"
	"pn-brain/internal/brain/store"
)

// Worker runs the learning pipeline in the background.
//
// This replaces a Redis queue and a separate worker process. A goroutine and a
// channel do the same job here because there is exactly one machine, one brain
// and one process — the queue was infrastructure inherited from a web
// framework, not something this problem needed.
//
// It also fixes a fault the queue had: a worker polling Redis burned CPU
// continuously on an idle machine. Nothing here runs when nothing is happening.
type Worker struct {
	DB        *store.DB
	Extractor Extractor
	Curator   Curator
	Validator Validator
	Log       *slog.Logger

	// Timeout bounds one extraction. A local model on a CPU can take minutes,
	// and a hung call must not pin a goroutine for the life of the process.
	Timeout time.Duration

	queue chan int64
	wg    sync.WaitGroup
	once  sync.Once
}

// QueueDepth is how many conversations may be waiting to be learned from.
//
// Small on purpose. If extraction falls this far behind, dropping the oldest
// request is better than growing without limit: the conversation is still on
// disk and can be learned from later, whereas an unbounded queue on a machine
// that is already too slow ends as memory exhaustion.
const QueueDepth = 32

// Start begins processing. Safe to call more than once.
func (w *Worker) Start(ctx context.Context) {
	w.once.Do(func() {
		if w.Timeout == 0 {
			w.Timeout = 10 * time.Minute
		}

		w.queue = make(chan int64, QueueDepth)
		w.wg.Add(1)

		go w.run(ctx)
	})
}

// Learn asks for a conversation to be considered, without waiting.
//
// Never blocks. A reply to the user must not wait on the brain deciding what to
// remember, and a full queue means learning is behind, not that the answer
// should be delayed.
func (w *Worker) Learn(conversationID int64) {
	if w.queue == nil {
		return
	}

	select {
	case w.queue <- conversationID:
	default:
		w.Log.Warn("learning queue is full; skipping", "conversation", conversationID)
	}
}

// Stop waits for in-flight work to finish.
func (w *Worker) Stop() {
	if w.queue == nil {
		return
	}

	close(w.queue)
	w.wg.Wait()
}

func (w *Worker) run(ctx context.Context) {
	defer w.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case id, open := <-w.queue:
			if !open {
				return
			}

			// Nothing anybody asked for depends on this, so it gives way to
			// anything that does. Learning runs a model call, and it is queued
			// the instant a reply is finished — which is the instant the
			// microphone reopens for the next thing said. The two were
			// competing for four cores, and the one that lost was the
			// transcription somebody was waiting on.
			waitForQuiet(ctx)

			// Said out loud, because it was not.
			//
			// Learning runs a model call and reported nothing while it did, so
			// the interface showed an idle brain on a machine that was clearly
			// working — which reads as stuck. Anything that takes a minute of
			// this machine has to say it is happening.
			progress.SetBackground("learning", "Learning from the last conversation")

			err := w.process(ctx, id)

			progress.Done()

			if err != nil {
				// A failure to learn is not a failure of the brain. It is
				// logged and the conversation is left alone; nothing the user
				// asked for depends on this.
				w.Log.Warn("learning from a conversation failed",
					"conversation", id, "error", err)
			}
		}
	}
}

// process runs one conversation through Extractor, Validator and Curator.
func (w *Worker) process(ctx context.Context, conversationID int64) error {
	ctx, cancel := context.WithTimeout(ctx, w.Timeout)
	defer cancel()

	transcript, err := w.transcript(conversationID)
	if err != nil {
		return err
	}

	if transcript == "" {
		return nil
	}

	proposal, err := w.Extractor.Extract(ctx, transcript)
	if err != nil {
		return err
	}

	// Finding nothing is the common and correct outcome.
	if proposal == nil {
		return nil
	}

	// A lesson drawn from conversation has no filesystem claim to check, so it
	// carries no source and the Validator will leave it for a person.
	status := w.Validator.StatusFor("")

	id, err := w.DB.AddLesson(conversationID, proposal.Lesson, status, proposal.Confidence, "")
	if err != nil {
		return err
	}

	w.Log.Info("learned something worth reviewing",
		"lesson", id, "status", status, "confidence", proposal.Confidence)

	return nil
}

// transcript renders the last few turns for the extractor.
//
// Only the recent exchange, and never the system prompt: handing the model its
// own instructions is what produced seventeen lessons describing the assistant.
func (w *Worker) transcript(conversationID int64) (string, error) {
	history, err := w.DB.History(conversationID)
	if err != nil {
		return "", err
	}

	var turns []store.Message

	for _, m := range history {
		if m.Role == llm.RoleSystem {
			continue
		}

		turns = append(turns, m)
	}

	const recent = 6

	if len(turns) > recent {
		turns = turns[len(turns)-recent:]
	}

	var b strings.Builder

	for _, m := range turns {
		fmt.Fprintf(&b, "%s: %s\n", m.Role, m.Content)
	}

	return strings.TrimSpace(b.String()), nil
}

// PromoteValidated moves everything the Validator trusted into knowledge.
//
// Run on demand rather than continuously: promotion is where duplicates are
// caught, and doing it in one pass means each new fact is compared against a
// store that already includes the ones promoted just before it.
func (w *Worker) PromoteValidated(ctx context.Context, limit int) (promoted, duplicates int, err error) {
	lessons, err := w.DB.LessonsByStatus(StatusValidated, limit)
	if err != nil {
		return 0, 0, err
	}

	for _, l := range lessons {
		id, err := w.Curator.Promote(ctx, l)
		if err != nil {
			return promoted, duplicates, err
		}

		if id == 0 {
			duplicates++

			continue
		}

		promoted++
	}

	return promoted, duplicates, nil
}

// HoldOff is how long learning will wait for the microphone to close.
//
// Bounded rather than indefinite: a stuck recorder must not mean the brain
// silently stops learning altogether, and a minute of waiting is already far
// longer than any turn.
const HoldOff = 60 * time.Second

// waitForQuiet holds until nothing is being recorded, or the wait runs out.
func waitForQuiet(ctx context.Context) {
	deadline := time.Now().Add(HoldOff)

	// Gives way to a microphone that is open and to a reply being worked on.
	// Neither is something the learner should be competing with: one is
	// somebody speaking and the other is somebody waiting.
	for (speech.Recording() || progress.Answering()) && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}
