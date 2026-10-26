package brain

import (
	"context"
	"encoding/json"
	"time"

	"pn-scripts-assistant/internal/brain/tools"
)

/*
 * Research with sources: the tools and the Research page share one
 * Researching. It searches and reads the way web_search and fetch_url do —
 * the same check of every query for personal details, the same refusal of
 * addresses on this machine or the local network — and only while the
 * privacy setting allows the web, read live so a change in Control applies
 * at once. Its model calls go through the router like every other.
 */
func (b *Brain) researching() tools.Researching {
	search := tools.WebSearch{BraveKey: b.Cfg.BraveKey}

	return tools.Researching{
		Book:   b.DB,
		Search: search.Find,
		Fetch: func(ctx context.Context, address string) (string, error) {
			args, _ := json.Marshal(map[string]string{"url": address})

			return tools.FetchURL{}.Execute(ctx, args)
		},
		Ask: func(ctx context.Context, system, user string) (string, string, error) {
			return b.writeWithModel(ctx, system, user, 6*time.Minute)
		},
		Web: func() bool { return b.Mode.AllowsWeb() },
	}
}

// Researching is the same for the pages.
func (b *Brain) Researching() tools.Researching { return b.researching() }
