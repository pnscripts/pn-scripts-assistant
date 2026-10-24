package autonomy

import (
	"context"
	"fmt"
	"time"
)

/*
 * Taking away what goal mode downloaded once it has gone unused (owner
 * 2026-10-24, ASSISTANT-GOALMODE-NEXT 3): a model of several gigabytes kept
 * for one goal a year ago is a disk filling up for nothing.
 *
 * Only what the ledger says goal mode put here, never anything its owner
 * installed; never while it is still in use; and each removal is proven
 * before its entry is dropped, so a failure is tried again next time rather
 * than forgotten.
 */

// UnusedFor is how long something of goal mode's may lie unused before it is
// removed: the consent says thirty days.
const UnusedFor = 30 * 24 * time.Hour

// Removers take away one kind of thing; a nil one leaves that kind alone.
type Removers struct {
	Model       func(ctx context.Context, name string) error
	Tool        func(id, title string) error
	Integration func(id string) error
}

// Tidied is what one round removed, and what it could not.
type Tidied struct {
	Removed []Entry
	Failed  []string
	Freed   int64
}

// Tidy removes what goal mode put here and nobody has used for idle, leaving
// out whatever inUse says is still needed.
func Tidy(ctx context.Context, ledger *Ledger, idle time.Duration, inUse func(Entry) bool, rm Removers, now time.Time) Tidied {
	var out Tidied

	for _, e := range ledger.Unused(idle, inUse, now) {
		if ctx.Err() != nil {
			break
		}

		key := e.ID
		if key == "" {
			key = e.Name
		}

		var err error

		switch {
		case e.Kind == "model" && rm.Model != nil:
			err = rm.Model(ctx, e.Name)
		case e.Kind == "tool" && rm.Tool != nil:
			err = rm.Tool(key, e.Name)
		case e.Kind == "integration" && rm.Integration != nil:
			err = rm.Integration(key)
		default:
			continue
		}

		if err != nil {
			out.Failed = append(out.Failed, fmt.Sprintf("%s %s: %v", e.Kind, e.Name, err))

			continue
		}

		if err := ledger.Forget(e.Kind, e.Name); err != nil {
			out.Failed = append(out.Failed, fmt.Sprintf("%s %s: removed, but the record could not be updated: %v", e.Kind, e.Name, err))

			continue
		}

		out.Removed = append(out.Removed, e)
		out.Freed += e.Bytes
	}

	return out
}
