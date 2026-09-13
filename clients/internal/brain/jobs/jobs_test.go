package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

/*
 * Work that outlives the sentence asking for it.
 *
 * The point of this package is that "read through that folder" does not mean
 * its owner sits in silence for four minutes unable to say anything else. So
 * the thing to prove is that starting returns at once and the answer arrives
 * afterwards, by itself.
 */
func TestWorkCarriesOnWhileTheConversationDoes(t *testing.T) {
	var (
		announced Job
		wg        sync.WaitGroup
	)

	wg.Add(1)

	r := &Runner{Announce: func(j Job) { announced = j; wg.Done() }}

	release := make(chan struct{})

	started := time.Now()

	job, err := r.Start("reading your documents", func(context.Context) (string, error) {
		<-release

		return "read 12 files", nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Starting must not wait for the work.
	if time.Since(started) > 200*time.Millisecond {
		t.Errorf("starting took %s; it is meant to return immediately", time.Since(started))
	}

	if job.State != Running {
		t.Errorf("a job that has not finished is %q", job.State)
	}

	// And the conversation can see what is happening meanwhile.
	if list := r.List(); len(list) != 1 || list[0].State != Running {
		t.Errorf("the running job is not visible: %+v", list)
	}

	close(release)
	wg.Wait()

	if announced.State != Done || announced.Result != "read 12 files" {
		t.Errorf("finished as %+v", announced)
	}
}

// A failure is reported as one rather than silently dropped.
func TestFailureIsAnnouncedToo(t *testing.T) {
	var wg sync.WaitGroup

	wg.Add(1)

	var got Job

	r := &Runner{Announce: func(j Job) { got = j; wg.Done() }}

	if _, err := r.Start("checking the disk", func(context.Context) (string, error) {
		return "", errors.New("no such folder")
	}); err != nil {
		t.Fatal(err)
	}

	wg.Wait()

	if got.State != Failed || got.Err != "no such folder" {
		t.Errorf("a failure came back as %+v", got)
	}
}

/*
 * Bounded, because "in the background" is not a licence to start twenty model
 * calls on four cores.
 *
 * Past the limit a job is refused and says so, which is better than accepting
 * it and delivering it an hour later.
 */
func TestOnlySoMuchAtOnce(t *testing.T) {
	r := &Runner{AtMost: 2}

	release := make(chan struct{})

	defer close(release)

	block := func(context.Context) (string, error) { <-release; return "", nil }

	for i := 0; i < 2; i++ {
		if _, err := r.Start("something", block); err != nil {
			t.Fatalf("job %d was refused: %v", i, err)
		}
	}

	if _, err := r.Start("one too many", block); err == nil {
		t.Error("a third job was accepted on a machine that manages two")
	}
}

// Stopping one ends it early and says so.
func TestStoppingSomethingEarly(t *testing.T) {
	var wg sync.WaitGroup

	wg.Add(1)

	var got Job

	r := &Runner{Announce: func(j Job) { got = j; wg.Done() }}

	job, err := r.Start("a long search", func(ctx context.Context) (string, error) {
		<-ctx.Done()

		return "", ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := r.Stop(job.ID); err != nil {
		t.Fatal(err)
	}

	wg.Wait()

	if got.State != Stopped {
		t.Errorf("a stopped job came back as %q", got.State)
	}

	// And stopping something that is not there says so rather than pretending.
	if err := r.Stop(9999); err == nil {
		t.Error("stopping a job that does not exist reported success")
	}
}

/*
 * How much runs at once follows the machine.
 *
 * It was a constant, so a workstation with a graphics card ran exactly as many
 * background jobs as a laptop with four cores and none. The constraint is real
 * on modest hardware and imaginary on good hardware, and a single number gets
 * one of them wrong whichever value it takes.
 */
func TestHowMuchRunsAtOnceFollowsTheMachine(t *testing.T) {
	modest := HowManyAtOnce("modest")
	capable := HowManyAtOnce("capable")
	generous := HowManyAtOnce("generous")

	if modest != 1 {
		t.Errorf("a machine with no graphics card runs %d jobs beside a "+
			"conversation; every one of them is a model call on the same "+
			"processor the answer is waiting on", modest)
	}

	if capable <= modest || generous <= capable {
		t.Errorf("better hardware does not get more: modest %d, capable %d, "+
			"generous %d", modest, capable, generous)
	}

	// An unknown tier is treated as the modest one, because the safe reading
	// of "I do not know what this machine is" is the cautious one.
	if HowManyAtOnce("something else") != modest {
		t.Error("an unrecognised machine was given more than the cautious limit")
	}
}

/*
 * A job that panics fails; it does not take the program down with it.
 *
 * Background work runs on a goroutine of its own, where an unrecovered panic
 * is the whole process — the window disappears, nothing is written down, and
 * what somebody sees is their assistant vanishing while it was doing something
 * for them. A nil field in the task conductor did exactly that on the first
 * task ever run on a real machine.
 */
func TestAJobThatPanicsDoesNotTakeTheProgramWithIt(t *testing.T) {
	told := make(chan Job, 1)

	r := &Runner{Announce: func(j Job) { told <- j }}

	if _, err := r.Start("reading your documents", func(context.Context) (string, error) {
		var nothing *struct{ field int }

		return "", fmt.Errorf("unreachable %d", nothing.field)
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case finished := <-told:
		if finished.State != Failed {
			t.Errorf("the job ended %q, want failed", finished.State)
		}

		if !strings.Contains(finished.Err, "stopped unexpectedly") {
			t.Errorf("what it says happened: %q", finished.Err)
		}

	case <-time.After(5 * time.Second):
		t.Fatal("the job never finished")
	}
}
