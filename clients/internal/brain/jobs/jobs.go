// Package jobs runs work that outlives the sentence that asked for it.
//
// Everything here used to happen inside one turn: somebody asks, the model
// thinks, a tool runs, an answer comes back, and nothing else could be said in
// between. That is fine for "what time is it" and wrong for "read through that
// folder" — which on this machine is minutes during which its owner is expected
// to sit and wait, unable to ask anything else.
//
// A job is that work, taken out of the turn. The turn ends immediately with
// "started", the conversation carries on, and the answer arrives when it
// arrives — announced rather than returned, because by then the person is
// talking about something else.
package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// State is where a job has got to.
type State string

const (
	Running State = "running"
	Done    State = "done"
	Failed  State = "failed"
	Stopped State = "stopped"
)

// Job is one piece of work happening in the background.
type Job struct {
	ID      int64     `json:"id"`
	What    string    `json:"what"`
	State   State     `json:"state"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended,omitempty"`
	Result  string    `json:"result,omitempty"`
	Err     string    `json:"error,omitempty"`

	/*
	 * Silent is a job that reports itself.
	 *
	 * A task writes its own account when it finishes — what it did, what was
	 * verified and what it only has its own word for — and "Finished going
	 * through your projects." said afterwards is the same news, worse, twice.
	 */
	Silent bool `json:"silent,omitempty"`

	// Queued is a job whose model calls wait their turn in a lane, and so is
	// not counted against AtMost. See StartQueued.
	Queued bool `json:"queued,omitempty"`

	cancel context.CancelFunc
}

// Took is how long it ran for.
func (j Job) Took() time.Duration {
	if j.Ended.IsZero() {
		return time.Since(j.Started)
	}

	return j.Ended.Sub(j.Started)
}

/*
 * Runner holds the work in flight.
 *
 * Bounded, because "in the background" is not a licence to start twenty model
 * calls on four cores. Past the limit a job is refused and says so, which is
 * better than accepting it and delivering it an hour later.
 */
type Runner struct {
	// AtMost is how many may run at once. Zero means the default.
	AtMost int

	// Announce is called when a job ends, with the job. This is how the answer
	// reaches somebody who has long since moved on to another subject.
	Announce func(Job)

	mu   sync.Mutex
	jobs map[int64]*Job
	next int64
}

// DefaultAtMost is deliberately small. Two model calls on four cores already
// make each other slow; a third makes all three useless.
const DefaultAtMost = 2

/*
 * HowManyAtOnce is how much can run beside a conversation, given the machine.
 *
 * The limit was a constant, so a workstation with a graphics card ran exactly
 * as many things at once as a laptop with four cores and none — which is the
 * same mistake the model choice used to make, in a different place. The
 * constraint is real on modest hardware and imaginary on good hardware, and
 * pretending otherwise wastes whichever one it gets wrong.
 *
 * On a machine without a card, one. Every job here is a model call, the model
 * runs on the processor, and two of them do not take twice as long — they take
 * longer than that, because they evict each other from cache and from memory.
 * A single background job that finishes is worth more than three that crawl,
 * and anything running beside a conversation is also stealing from the answer
 * somebody is waiting for.
 *
 * With a card, the picture inverts: the model sits in video memory and a second
 * call costs memory rather than minutes.
 */
func HowManyAtOnce(tier string) int {
	switch tier {
	case "generous":
		return 6
	case "capable":
		return 3
	default:
		return 1
	}
}

/*
 * Start puts work in the background and returns at once.
 *
 * what is in the owner's terms — "reading your documents", not the name of a
 * function — because it is read back to them later, out of the context that
 * produced it.
 */
func (r *Runner) Start(what string, work func(context.Context) (string, error)) (Job, error) {
	return r.start(what, false, false, work)
}

// StartSilent is Start for work that will say what it did in its own words.
func (r *Runner) StartSilent(what string, work func(context.Context) (string, error)) (Job, error) {
	return r.start(what, true, false, work)
}

/*
 * StartQueued is StartSilent for work whose thinking queues in a lane.
 *
 * AtMost exists because every background job was a model call on the
 * processor, and two of those at once are slower than one after the other.
 * A task's calls now wait their turn in the lanes instead, so counting its
 * job here as well limits the same thing twice — and wrongly: on a machine
 * allowed one job, a task handing a step to a specialist was refused the
 * specialist's job by its own, still running, and the step sat waiting for
 * somebody to press a button.
 *
 * Still bounded, by MostQueued, because a limit on thinking is not a licence
 * for a thousand goroutines.
 */
func (r *Runner) StartQueued(what string, work func(context.Context) (string, error)) (Job, error) {
	return r.start(what, true, true, work)
}

// MostQueued is how many queued jobs may exist at once. Far more than the
// lanes will let think, on purpose: the lanes decide the pace, and this only
// stops something having gone badly wrong from becoming a thousand of them.
const MostQueued = 24

func (r *Runner) start(what string, silent, queued bool, work func(context.Context) (string, error)) (Job, error) {
	r.mu.Lock()

	if r.jobs == nil {
		r.jobs = map[int64]*Job{}
	}

	limit := r.AtMost
	if limit <= 0 {
		limit = DefaultAtMost
	}

	running, waiting := 0, 0

	for _, j := range r.jobs {
		switch {
		case j.State != Running:
		case j.Queued:
			waiting++
		default:
			running++
		}
	}

	if queued && waiting >= MostQueued {
		r.mu.Unlock()

		return Job{}, fmt.Errorf("already %d pieces of work waiting their turn; "+
			"ask again when some of them have finished", waiting)
	}

	if !queued && running >= limit {
		r.mu.Unlock()

		return Job{}, fmt.Errorf(
			"already doing %d things in the background, which is as much as this "+
				"machine manages at once; ask again when one has finished", running)
	}

	r.next++

	ctx, cancel := context.WithCancel(context.Background())

	job := &Job{
		ID:      r.next,
		What:    what,
		State:   Running,
		Started: time.Now(),
		Silent:  silent,
		Queued:  queued,
		cancel:  cancel,
	}

	r.jobs[job.ID] = job

	/*
	 * Copied while the lock is still held, and returned instead of the job.
	 *
	 * The goroutine below writes to the same struct the moment the work
	 * finishes, and returning *job read it without the lock — a race the
	 * detector finds immediately once anything actually looks at what Start
	 * gave back. Nothing did, until a task started a job and asked for its
	 * number in the next line.
	 */
	snapshot := *job

	r.mu.Unlock()

	go func() {
		/*
		 * A job that panics fails; it does not take the program down.
		 *
		 * This runs on a goroutine of its own, and an unrecovered panic there
		 * is the whole process — no request survives it, nothing is written,
		 * and what somebody sees is their assistant vanishing while it was
		 * working on something for them. A nil field in a task conductor did
		 * exactly that once. Work running unattended is precisely the work
		 * nobody is watching closely enough to explain a disappearance.
		 */
		var (
			result string
			err    error
		)

		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("this stopped unexpectedly: %v", r)
				}
			}()

			result, err = work(ctx)
		}()

		r.mu.Lock()

		job.Ended = time.Now()
		job.Result = result

		switch {
		case ctx.Err() != nil:
			job.State = Stopped
		case err != nil:
			job.State = Failed
			job.Err = err.Error()
		default:
			job.State = Done
		}

		finished := *job
		announce := r.Announce

		r.mu.Unlock()

		// Outside the lock: announcing means speaking, which takes seconds and
		// must not hold up anything else finishing.
		if announce != nil && !finished.Silent {
			announce(finished)
		}
	}()

	return snapshot, nil
}

// List reports what is happening and what recently happened, newest first.
func (r *Runner) List() []Job {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Job, 0, len(r.jobs))

	for _, j := range r.jobs {
		out = append(out, *j)
	}

	// Newest first, which is the order somebody asking "what are you doing"
	// means.
	for i := 0; i < len(out); i++ {
		for k := i + 1; k < len(out); k++ {
			if out[k].Started.After(out[i].Started) {
				out[i], out[k] = out[k], out[i]
			}
		}
	}

	return out
}

// Stop ends one job early.
func (r *Runner) Stop(id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.jobs[id]
	if !ok {
		return fmt.Errorf("there is no job %d", id)
	}

	if job.State != Running {
		return fmt.Errorf("job %d already finished", id)
	}

	job.cancel()

	return nil
}

/*
 * Forget drops jobs that ended a while ago.
 *
 * Only the finished ones, and only after long enough that somebody might still
 * ask about them. A list that grows forever is a leak; one that forgets while
 * its owner is still interested is worse.
 */
func (r *Runner) Forget(olderThan time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, j := range r.jobs {
		if j.State != Running && time.Since(j.Ended) > olderThan {
			delete(r.jobs, id)
		}
	}
}
