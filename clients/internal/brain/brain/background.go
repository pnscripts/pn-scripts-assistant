package brain

import (
	"context"
	"fmt"
	"time"

	"pn-brain/internal/brain/jobs"
	"pn-brain/internal/brain/speech"
	"pn-brain/internal/brain/tools"
)

/*
 * Work happening behind the conversation.
 *
 * The adapter between the runner and the tools, and the part that decides what
 * happens when something finishes while its owner is mid-sentence about
 * something else entirely.
 */

type backgroundOf struct{ b *Brain }

func (g backgroundOf) Start(what string, work func(context.Context) (string, error)) (int64, error) {
	job, err := g.b.Jobs.Start(what, work)

	return job.ID, err
}

func (g backgroundOf) List() []tools.BackgroundJob {
	rows := g.b.Jobs.List()

	out := make([]tools.BackgroundJob, 0, len(rows))

	for _, j := range rows {
		out = append(out, tools.BackgroundJob{
			ID: j.ID, What: j.What, State: string(j.State),
			Took: j.Took(), Result: j.Result, Err: j.Err,
		})
	}

	return out
}

func (g backgroundOf) Stop(id int64) error { return g.b.Jobs.Stop(id) }

/*
 * announce says a finished job out loud, if anybody is listening.
 *
 * Waiting for a gap first. Something finishing is not a reason to talk over
 * whoever is speaking, and an announcement that lands in the middle of a
 * sentence is worse than one that arrives ten seconds later — the whole point
 * of doing the work in the background was to stay out of the way.
 *
 * Written to the transcript either way, so it is never only spoken: somebody
 * who was out of the room still finds out.
 */
func (b *Brain) announce(job jobs.Job) {
	b.Log.Info("background work finished", "what", job.What, "state", job.State)

	b.sayWhenFinished(describeJob(job))
}

/*
 * sayWhenFinished puts a line where somebody will find it, and reads it out if
 * they are listening.
 *
 * The shared half of announcing anything that happened without being asked
 * for. A finished job used this and the background reading did not, which is
 * how the reading came to say "I am learning from that" over and over instead:
 * it had no way to say anything at the end, so all it could do was narrate the
 * middle, and the middle is thousands of files long.
 */
func (b *Brain) sayWhenFinished(line string) {
	if line == "" {
		return
	}

	/*
	 * Into the latest conversation, not into nothing.
	 *
	 * This wrote to conversation 0, and messages.conversation_id is NOT NULL
	 * with a foreign key to conversations — so every one of these was refused
	 * by the database and swallowed by the Debug line below it. The comment
	 * above says the line goes in the transcript either way so that somebody
	 * who was out of the room still finds out; that has been false for as long
	 * as it has been written down, and it took a test asking to read one back
	 * to notice, because the only symptom is a sentence that is never there.
	 *
	 * A brain that has just finished reading a drive on its own may never have
	 * been spoken to, so there is not always a conversation to join. Starting
	 * one is right: something happened, and it is the first thing in it.
	 */
	id, err := b.somewhereToSayIt()
	if err != nil {
		b.Log.Warn("nowhere to record something that finished", "error", err)

		return
	}

	if _, err := b.DB.AddMessage(id, "assistant", "", "", line); err != nil {
		b.Log.Warn("could not record something that finished", "error", err)
	}

	/*
	 * Spoken only when the conversation is being held out loud.
	 *
	 * Somebody typing does not want the machine to start talking at them
	 * because a job finished; the line is in the transcript, which is where
	 * they are looking.
	 */
	if !b.spokenConversation() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for i := 0; i < 60; i++ {
		if !speech.Speaking() && !speech.Recording() {
			break
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}

	_ = speech.SpeakAndWait(ctx, line)
}

/*
 * spokenConversation reports whether anybody is listening rather than reading.
 *
 * From whether the last exchange was spoken, which the brain is told directly,
 * rather than from the microphone being open at this instant — between turns it
 * is not, and that is precisely when a job is most likely to finish.
 *
 * Somebody typing does not want the machine to start talking at them because a
 * job finished. The line goes in the transcript either way, which is where they
 * are looking.
 */
func (b *Brain) spokenConversation() bool {
	b.mu.Lock()
	last := b.lastSpokenAt
	b.mu.Unlock()

	// Within a few minutes of the last spoken turn counts as still talking.
	return !last.IsZero() && time.Since(last) < 5*time.Minute
}

// describeJob is what gets said, in the terms it was asked for.
func describeJob(job jobs.Job) string {
	switch job.State {
	case jobs.Failed:
		return fmt.Sprintf("That %s did not work: %s", job.What, job.Err)
	case jobs.Stopped:
		return fmt.Sprintf("I stopped %s.", job.What)
	default:
		return fmt.Sprintf("Finished %s.", job.What)
	}
}

/*
 * somewhereToSayIt is the conversation an unprompted line belongs in.
 *
 * The one being held, when there is one, so that "I have finished reading your
 * documents" arrives where somebody is already looking. Otherwise a new one,
 * because work that finished while nobody was talking to the brain is still
 * the first thing it has to say when they come back.
 */
func (b *Brain) somewhereToSayIt() (int64, error) {
	latest, err := b.DB.LatestConversation()
	if err != nil {
		return 0, err
	}

	if latest != nil && latest.ID != 0 {
		return latest.ID, nil
	}

	return b.DB.NewConversation("")
}
