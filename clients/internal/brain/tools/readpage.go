package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"pn-scripts-assistant/internal/brain/browse"
)

/*
 * ReadAPage opens a page the way a browser opens it.
 *
 * Separate from fetch_url rather than replacing it, and the difference is
 * cost. The plain fetch is an HTTP request: milliseconds, no display needed,
 * and right for anything that serves its words. This starts a whole browser in
 * a child process, runs the page's scripts and waits for it to settle — which
 * is most of a minute and the only thing that works on a site that sends an
 * empty shell and a script tag.
 *
 * So the description tells the model when each is worth it, and the cheap one
 * stays the default. A model that reached for this every time would make every
 * question about anything on the web take a minute.
 */
type ReadAPage struct{}

func (ReadAPage) Name() string { return "read_a_page" }

func (ReadAPage) Description() string {
	return "Open a web page in a real browser, run its scripts, and read what a person " +
		"would see. Slower than fetch_url and needs a screen — use it only when " +
		"fetch_url came back empty, came back as a shell with no content, or when the " +
		"page is known to build itself with scripts."
}

func (ReadAPage) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {"type": "string", "description": "The address, beginning with http:// or https://."}
		},
		"required": ["url"]
	}`)
}

/*
 * Safe, on the same reasoning as fetch_url.
 *
 * It reads and changes nothing on this machine. What it does do is run
 * somebody else's code, which is why it runs in a child process with none of
 * this brain's environment and nothing kept between pages — no cookies, no
 * storage, no cache. A browser that remembered would be a browser that could
 * be logged in as somebody.
 */
func (ReadAPage) Risk() Risk { return Safe }

type pageArgs struct {
	URL string `json:"url"`
}

func (ReadAPage) Summarize(raw json.RawMessage) string {
	var a pageArgs

	json.Unmarshal(raw, &a)

	return "Open " + a.URL + " in a browser"
}

func (ReadAPage) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a pageArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("could not read the address: %w", err)
	}

	url := strings.TrimSpace(a.URL)

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("give an address beginning with http:// or https://")
	}

	/*
	 * The same guard the plain fetch uses, and for a sharper reason.
	 *
	 * A page told to open 127.0.0.1 would reach this program's own unauthenticated
	 * interface — and unlike a fetch, this one runs the page's scripts, so a
	 * page that got in could keep going. One guard, used by both.
	 */
	var guard FetchURL

	if err := guard.GuardTarget(url); err != nil {
		return "", err
	}

	text, err := browse.Page(ctx, url)
	if err != nil {
		return "", err
	}

	return text, nil
}
