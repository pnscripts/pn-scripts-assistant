package brain

import (
	"context"
	"time"

	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Customer replies for a small business: the tools and the Customers page
 * share one CustomerCare, which keeps drafts on the same desk as every other
 * reply, so a customer's answer waits for its owner's yes on the Mail page
 * like any other. Its model calls go through the router like every other.
 */
func (b *Brain) customerCare() tools.CustomerCare {
	return tools.CustomerCare{
		Book: b.DB,
		Desk: b.Desk,
		Root: b.Root,
		Ask: func(ctx context.Context, system, user string) (string, string, error) {
			return b.writeWithModel(ctx, system, user, 4*time.Minute)
		},
	}
}

// CustomerCare is the same for the pages.
func (b *Brain) CustomerCare() tools.CustomerCare { return b.customerCare() }
