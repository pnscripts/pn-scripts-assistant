package brain

import (
	"context"
	"time"

	"pn-scripts-assistant/internal/brain/tools"
)

// tables is what the table tools and the Tables page share: a model turns
// a question into a recipe, through the router like every other model call;
// the arithmetic is the sheet package's.
func (b *Brain) tables() tools.Tables {
	return tools.Tables{
		Ask: func(ctx context.Context, system, user string) (string, string, error) {
			return b.writeWithModel(ctx, system, user, 3*time.Minute)
		},
	}
}

// Tables is the same for the pages.
func (b *Brain) Tables() tools.Tables { return b.tables() }
