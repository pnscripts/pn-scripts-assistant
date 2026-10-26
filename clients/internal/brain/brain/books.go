package brain

import (
	"context"
	"time"

	"pn-scripts-assistant/internal/brain/tools"
)

// bookkeeping is what the bookkeeping tools and the Invoices page share: a
// model reads an invoice's text through the router like every other model
// call; the arithmetic and the rules are the books package's.
func (b *Brain) bookkeeping() tools.Bookkeeping {
	return tools.Bookkeeping{
		Book: b.DB,
		Ask: func(ctx context.Context, system, user string) (string, string, error) {
			return b.writeWithModel(ctx, system, user, 3*time.Minute)
		},
	}
}

// Bookkeeping is the same for the pages.
func (b *Brain) Bookkeeping() tools.Bookkeeping { return b.bookkeeping() }
