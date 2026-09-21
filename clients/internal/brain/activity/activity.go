/*
 * Package activity is how this program says what it is doing, once, to
 * whoever is listening.
 *
 * Before this, everything that wanted to know what was happening asked again
 * every few seconds — thirty-two timers in the interface, each rebuilding a
 * card from a fresh answer. That works on one machine with a window open on
 * it. It does not work for a phone on mobile data, and it cannot answer "what
 * happened while I was away" at all, because a poll only ever returns the
 * present.
 *
 * So: one bus. Something happens, it is written down with a number, and it is
 * handed to every listener. A listener that was away asks for everything
 * after the last number it saw. A listener that is here gets it as it
 * happens. The number is the whole mechanism, and it is the database's to
 * give, so that two things happening at once cannot both be fourth.
 */
package activity

import (
	"sync"

	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/protocol"
)

// Bus carries events to everyone watching, and keeps them for everyone who is
// not. A nil Bus is usable and does nothing, so nothing has to check.
type Bus struct {
	db *store.DB

	mu      sync.Mutex
	next    int
	readers map[int]*reader
}

type reader struct {
	to chan protocol.Envelope

	// behind is set when this reader could not keep up. Its events were
	// dropped rather than blocking everybody else, and it is told so: the
	// honest thing for a reader to do then is ask for what it missed.
	behind bool
}

// New is a bus that keeps what it carries. A nil database is allowed — the
// events are then only handed to whoever is listening at the time.
func New(db *store.DB) *Bus { return &Bus{db: db, readers: map[int]*reader{}} }

/*
 * Say records an event and hands it on, returning it with its number.
 *
 * Recording first, on purpose. If the program stops between the two, the
 * event is still in the record and a client asking what it missed will be
 * told; the other way round, a listener would have seen something that the
 * record then denied.
 */
func (b *Bus) Say(e protocol.Envelope) protocol.Envelope {
	if b == nil {
		return e
	}

	if e.V == 0 {
		e.V = protocol.Version
	}

	if b.db != nil {
		if seq, err := b.db.RecordHappening(e); err == nil {
			e.Seq = seq
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for _, r := range b.readers {
		select {
		case r.to <- e:
		default:
			r.behind = true
		}
	}

	return e
}

/*
 * Watch is a live feed of what happens from now on, and the way to stop
 * listening.
 *
 * Buffered, and a reader that fills its buffer is passed over rather than
 * waited for: one phone on a bad connection must not hold up the work itself.
 * Missing events are visible as a jump in the numbers, and Since fills the
 * gap.
 */
func (b *Bus) Watch(buffer int) (<-chan protocol.Envelope, func()) {
	if b == nil {
		empty := make(chan protocol.Envelope)

		return empty, func() {}
	}

	if buffer <= 0 {
		buffer = 64
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.next++
	id := b.next
	r := &reader{to: make(chan protocol.Envelope, buffer)}
	b.readers[id] = r

	return r.to, func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		if kept, ok := b.readers[id]; ok {
			delete(b.readers, id)
			close(kept.to)
		}
	}
}

// Since is everything after a number, for a listener catching up.
func (b *Bus) Since(after int64, most int) ([]protocol.Envelope, error) {
	if b == nil || b.db == nil {
		return nil, nil
	}

	rows, err := b.db.Happenings(after, most)
	if err != nil {
		return nil, err
	}

	out := make([]protocol.Envelope, 0, len(rows))

	for _, row := range rows {
		out = append(out, row.Envelope())
	}

	return out, nil
}

// Latest is the number of the last event, which is where a listener with no
// history of its own starts.
func (b *Bus) Latest() int64 {
	if b == nil || b.db == nil {
		return 0
	}

	seq, err := b.db.LastHappening()
	if err != nil {
		return 0
	}

	return seq
}
