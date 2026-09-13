package lanes

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
)

// slow is a model that takes a moment and counts how many are answering at
// once, which is the whole question.
type slow struct {
	name      string
	now, most atomic.Int32
	lingerFor time.Duration
}

func (s *slow) Name() string                   { return s.name }
func (s *slow) Available(context.Context) bool { return true }

func (s *slow) Chat(ctx context.Context, _ llm.Request) (llm.Response, error) {
	at := s.now.Add(1)

	for {
		most := s.most.Load()
		if at <= most || s.most.CompareAndSwap(most, at) {
			break
		}
	}

	time.Sleep(s.lingerFor)
	s.now.Add(-1)

	return llm.Response{Content: "ok"}, nil
}

func askAtOnce(l *Lanes, model llm.Provider, n int) {
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			Provider{Provider: model, Lanes: l, Seat: Seat{Who: "someone"}}.
				Chat(context.Background(), llm.Request{})
		}()
	}

	wg.Wait()
}

// Two agents on this machine's model do not think at the same time. Two
// calls on a processor take longer than twice as long.
func TestTheLocalModelAnswersOneAtATime(t *testing.T) {
	model := &slow{name: llm.Local, lingerFor: 20 * time.Millisecond}

	askAtOnce(New(6), model, 5)

	if model.most.Load() != 1 {
		t.Fatalf("%d local calls ran at once, want 1", model.most.Load())
	}
}

// Hosted calls run side by side, as many as the pool allows and no more.
func TestHostedModelsWorkInParallel(t *testing.T) {
	model := &slow{name: "anthropic", lingerFor: 40 * time.Millisecond}

	askAtOnce(New(3), model, 8)

	if got := model.most.Load(); got != 3 {
		t.Fatalf("%d hosted calls ran at once, want 3", got)
	}
}

// A hosted pool is never one: that would make using somebody else's computer
// as slow as the processor is at running its own.
func TestAHostedPoolIsNeverSingleFile(t *testing.T) {
	if New(1).Now()[1].Size != FewestHosted {
		t.Error("a hosted pool was made smaller than the fewest it may be")
	}
}

/*
 * Waiting is said, and in order.
 *
 * The first to wait is the first let in, and while it waits the lanes say who
 * has the lane and who is behind — which is the difference between a queue
 * and something that looks stuck.
 */
func TestWhoIsWaitingIsSaidAndServedInOrder(t *testing.T) {
	l := New(2)

	leave, _ := l.Enter(context.Background(), llm.Local, Seat{Who: "researcher"})

	order := make(chan string, 2)

	for _, who := range []string{"writer", "designer"} {
		who := who

		go func() {
			done, _ := l.Enter(context.Background(), llm.Local, Seat{Who: who})
			order <- who
			done()
		}()

		// So the second really asks after the first.
		for len(l.Now()[0].Waiting) == 0 || (who == "designer" && len(l.Now()[0].Waiting) < 2) {
			time.Sleep(time.Millisecond)
		}
	}

	here := l.Now()[0]

	if len(here.In) != 1 || here.In[0].Who != "researcher" {
		t.Errorf("the lane does not say who has it: %+v", here.In)
	}

	if len(here.Waiting) != 2 || here.Waiting[0].Who != "writer" {
		t.Errorf("the lane does not say who is waiting, in order: %+v", here.Waiting)
	}

	leave()

	if first := <-order; first != "writer" {
		t.Errorf("%s went first; the writer had waited longer", first)
	}

	<-order
}

// A task stopped while it waits leaves the queue, and does not later hold a
// lane nobody is using.
func TestGivingUpLeavesTheQueue(t *testing.T) {
	l := New(2)

	leave, _ := l.Enter(context.Background(), llm.Local, Seat{Who: "first"})

	ctx, cancel := context.WithCancel(context.Background())

	gaveUp := make(chan error)

	go func() {
		_, err := l.Enter(ctx, llm.Local, Seat{Who: "stopped"})
		gaveUp <- err
	}()

	for len(l.Now()[0].Waiting) == 0 {
		time.Sleep(time.Millisecond)
	}

	cancel()

	if err := <-gaveUp; err == nil {
		t.Fatal("waiting did not give up with its context")
	}

	leave()

	if here := l.Now()[0]; len(here.In) != 0 || len(here.Waiting) != 0 {
		t.Errorf("a stopped task is still in the lane: %+v", here)
	}
}

// Measured after start, the pool may grow while work is queued, and whoever
// now fits is let in straight away.
func TestAGrowingPoolLetsTheQueueIn(t *testing.T) {
	l := New(2)

	for i := 0; i < 2; i++ {
		l.Enter(context.Background(), "openai", Seat{Who: "busy"})
	}

	in := make(chan struct{})

	go func() {
		l.Enter(context.Background(), "openai", Seat{Who: "third"})
		close(in)
	}()

	for len(l.Now()[1].Waiting) == 0 {
		time.Sleep(time.Millisecond)
	}

	l.SetHosted(3)

	select {
	case <-in:
	case <-time.After(time.Second):
		t.Fatal("the third was not let in when the pool grew")
	}
}
