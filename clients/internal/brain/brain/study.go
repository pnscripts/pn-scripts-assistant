package brain

import (
	"context"
	"time"

	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Learning with the assistant as a tutor: the tools and the Learning page
 * share one Tutoring, whose model calls go the way every model call goes —
 * through the router, where privacy is enforced, to the model kept for work
 * that deserves care. A lesson is longer than a rewrite, so it is given
 * longer.
 */
func (b *Brain) tutoring() tools.Tutoring {
	return tools.Tutoring{
		Book: b.DB,
		Ask: func(ctx context.Context, system, user string) (string, string, error) {
			return b.writeWithModel(ctx, system, user, 8*time.Minute)
		},
	}
}

// Tutoring is the same for the pages.
func (b *Brain) Tutoring() tools.Tutoring { return b.tutoring() }
