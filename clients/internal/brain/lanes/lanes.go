/*
 * Package lanes is how much thinking happens at once, and who is waiting.
 *
 * Several agents working on one job is only an organisation if some of them
 * can work at the same time, and whether they can is not a setting — it is a
 * fact about where the thinking happens. The program runs on more than one
 * machine, so the answer has to come from the machine rather than from a
 * number somebody keeps up to date.
 *
 * Two kinds of lane, because there are two kinds of model.
 *
 * This machine is one lane. The model sits in one slot, and two calls on a
 * processor do not take twice as long, they take longer than that — they
 * evict each other from cache and from memory. So local work goes one call at
 * a time, in the order it asked, whatever else is going on.
 *
 * Hosted services are several lanes. A call to somebody else's computer costs
 * this one nothing but a socket, so as many run at once as the machine can
 * keep up with the results of: jobs.HowManyAtOnce, the same number that
 * already decides background work, and never fewer than two — a hosted lane of
 * one would make a machine without a graphics card as slow at using somebody
 * else's as it is at running its own.
 *
 * A lane is held for one model call, not one step. A step is several calls
 * with tools between them, and holding the local lane through a tool that
 * reads a folder for a minute would stop every other agent for nothing.
 *
 * And it says who is waiting and why, because work that is queued looks
 * exactly like work that is stuck unless something says which it is.
 */
package lanes

import (
	"context"
	"sort"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/llm"
)

const (
	// Here is the lane for the model on this machine.
	Here = "this machine"

	// Hosted is the lanes for somebody else's.
	Hosted = "hosted"

	// FewestHosted is the least a hosted pool is ever given. See the package
	// comment.
	FewestHosted = 2
)

// LaneOf is which lane a provider's calls go in.
func LaneOf(provider string) string {
	if provider == llm.Local || provider == "" {
		return Here
	}

	return Hosted
}

// Seat is one piece of work in a lane or waiting for one.
type Seat struct {
	Who   string    `json:"who"`
	What  string    `json:"what"`
	Since time.Time `json:"since"`

	// Key is what the work is known by elsewhere — a task step, say — so the
	// interface can put "waiting for this machine" beside the right thing.
	Key string `json:"key,omitempty"`
}

// Lane is one kind of lane as it stands.
type Lane struct {
	Name    string `json:"name"`
	Size    int    `json:"size"`
	In      []Seat `json:"in"`
	Waiting []Seat `json:"waiting"`
}

type waiter struct {
	seat  Seat
	ready chan struct{}
}

type lane struct {
	size    int
	in      map[*Seat]bool
	waiting []*waiter
}

// Lanes is every lane there is. The zero value is not usable; see New.
type Lanes struct {
	mu    sync.Mutex
	lanes map[string]*lane
}

// New makes a local lane of one and a hosted pool of the given size.
func New(hosted int) *Lanes {
	l := &Lanes{lanes: map[string]*lane{
		Here:   {size: 1, in: map[*Seat]bool{}},
		Hosted: {size: 1, in: map[*Seat]bool{}},
	}}

	l.SetHosted(hosted)

	return l
}

/*
 * SetHosted changes how many hosted calls may run at once.
 *
 * Asked once the machine has been measured, which is after the program has
 * started — so it may grow while work is already queued, and anybody waiting
 * who now fits is let in at once rather than at the next release.
 */
func (l *Lanes) SetHosted(n int) {
	if n < FewestHosted {
		n = FewestHosted
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.lanes[Hosted].size = n
	l.admit(l.lanes[Hosted])
}

// admit lets waiters in, oldest first, while there is room. Called with the
// lock held.
func (l *Lanes) admit(ln *lane) {
	for len(ln.waiting) > 0 && len(ln.in) < ln.size {
		next := ln.waiting[0]
		ln.waiting = ln.waiting[1:]

		ln.in[&next.seat] = true
		close(next.ready)
	}
}

/*
 * Enter waits for room in the lane a provider's calls belong in, and returns
 * what to call when the call is finished.
 *
 * Waits in order. A task that has been waiting for the local model for two
 * minutes is not overtaken by one that asked a second ago, which is the
 * difference between a queue and a scramble.
 *
 * Gives up when the context does, taking its place out of the queue — a task
 * stopped from the interface while it waits must not be let in afterwards to
 * hold a lane nobody is using.
 */
func (l *Lanes) Enter(ctx context.Context, provider string, seat Seat) (func(), error) {
	name := LaneOf(provider)

	if seat.Since.IsZero() {
		seat.Since = time.Now()
	}

	l.mu.Lock()

	ln := l.lanes[name]

	if len(ln.in) < ln.size && len(ln.waiting) == 0 {
		held := &seat
		ln.in[held] = true

		l.mu.Unlock()

		return l.leaving(ln, held), nil
	}

	w := &waiter{seat: seat, ready: make(chan struct{})}
	ln.waiting = append(ln.waiting, w)

	l.mu.Unlock()

	select {
	case <-w.ready:
		return l.leaving(ln, &w.seat), nil
	case <-ctx.Done():
		l.mu.Lock()
		defer l.mu.Unlock()

		select {
		case <-w.ready:
			// Let in at the same moment as it gave up: give the place back.
			delete(ln.in, &w.seat)
			l.admit(ln)
		default:
			for i, other := range ln.waiting {
				if other == w {
					ln.waiting = append(ln.waiting[:i], ln.waiting[i+1:]...)

					break
				}
			}
		}

		return func() {}, ctx.Err()
	}
}

func (l *Lanes) leaving(ln *lane, held *Seat) func() {
	var once sync.Once

	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()

			delete(ln.in, held)
			l.admit(ln)
		})
	}
}

// Now is every lane: who is in it, and who is waiting, oldest first.
func (l *Lanes) Now() []Lane {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := []Lane{}

	for _, name := range []string{Here, Hosted} {
		ln := l.lanes[name]
		view := Lane{Name: name, Size: ln.size, In: []Seat{}, Waiting: []Seat{}}

		for seat := range ln.in {
			view.In = append(view.In, *seat)
		}

		sort.Slice(view.In, func(i, j int) bool { return view.In[i].Since.Before(view.In[j].Since) })

		for _, w := range ln.waiting {
			view.Waiting = append(view.Waiting, w.seat)
		}

		out = append(out, view)
	}

	return out
}

/*
 * Provider is a provider whose calls take their turn in a lane.
 *
 * Wrapped where the work asks for a provider, so nothing about how a step,
 * a plan or a check talks to a model has to know that lanes exist — and so
 * nothing can forget to queue.
 */
type Provider struct {
	llm.Provider

	Lanes *Lanes
	Seat  Seat
}

func (p Provider) Chat(ctx context.Context, req llm.Request) (llm.Response, error) {
	if p.Lanes == nil {
		return p.Provider.Chat(ctx, req)
	}

	leave, err := p.Lanes.Enter(ctx, p.Provider.Name(), p.Seat)
	if err != nil {
		return llm.Response{}, err
	}

	defer leave()

	return p.Provider.Chat(ctx, req)
}
