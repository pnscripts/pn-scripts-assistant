package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// MaxSearchResults keeps the list readable and the context small.
const MaxSearchResults = 8

// WebSearch finds pages without reading them.
//
// Two engines, chosen by whether a key exists. Brave has a proper API and is
// used when configured; without one it falls back to scraping DuckDuckGo's HTML
// endpoint, which needs no account. That fallback is deliberately fragile-by-
// nature — it parses a page nobody promised to keep stable — so it reports a
// clear failure rather than silently returning nothing when the markup changes.
//
// Searching is the one thing that genuinely leaves the machine in research
// mode: the model composes the query, and the query is a disclosure. That is
// stated plainly in the privacy description rather than buried here.
type WebSearch struct {
	BraveKey string
	Client   *http.Client
}

func (WebSearch) Name() string { return "web_search" }

func (WebSearch) Description() string {
	/*
	 * Named for the questions it answers, not only for what it does.
	 *
	 * "Search the web" describes the mechanism, and a small model choosing
	 * between thirty tools matches on the subject of the question instead. One
	 * asked for the weather in Sofia and chose to photograph the screen. The
	 * examples are here so the obvious cases need no inference at all.
	 */
	return "Search the web for anything current or factual the brain does not " +
		"already know: weather, news, prices, opening times, sport, travel, " +
		"documentation, or any question about what is happening now. " +
		"Returns titles, URLs and snippets. " +
		"Follow up with fetch_url to read a specific page."
}

func (WebSearch) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "What to search for"}
		},
		"required": ["query"]
	}`)
}

func (WebSearch) Risk() Risk { return Safe }

func (WebSearch) Summarize(raw json.RawMessage) string {
	var a struct {
		Query string `json:"query"`
	}
	argsOf(raw, &a)

	return "Search the web for: " + a.Query
}

// SearchResult is one hit.
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

func (s WebSearch) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Query string `json:"query"`
	}

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	query := strings.TrimSpace(a.Query)
	if query == "" {
		return "", fmt.Errorf("no search query was given")
	}

	/*
	 * Checked here, where it leaves, rather than asked for in the prompt.
	 *
	 * The model writes this query, and searching is the one thing this program
	 * does that reaches off the machine. A rule that personal information is
	 * never shared is only a rule if something enforces it at the point of
	 * sending; in the prompt it is a request, and a request can be argued with
	 * by the user, by a page the model has read, or by its own confusion.
	 */
	if err := CheckQuery(query); err != nil {
		return "", err
	}

	var (
		results []SearchResult
		err     error
	)

	if s.BraveKey != "" {
		results, err = s.brave(ctx, query)
	} else {
		results, err = s.duckDuckGo(ctx, query)
	}

	if err != nil {
		return "", err
	}

	if len(results) == 0 {
		return "No results.", nil
	}

	var b strings.Builder

	for i, r := range results {
		fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, r.Title, r.URL)

		if r.Snippet != "" {
			fmt.Fprintf(&b, "   %s\n", r.Snippet)
		}
	}

	return b.String(), nil
}

func (s WebSearch) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}

	return &http.Client{Timeout: 30 * time.Second}
}

func (s WebSearch) brave(ctx context.Context, query string) ([]SearchResult, error) {
	endpoint := "https://api.search.brave.com/res/v1/web/search?count=" +
		fmt.Sprint(MaxSearchResults) + "&q=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", s.BraveKey)

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach Brave search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("Brave rejected the API key")
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Brave search returned %d", resp.StatusCode)
	}

	var body struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	out := make([]SearchResult, 0, len(body.Web.Results))

	for _, r := range body.Web.Results {
		out = append(out, SearchResult{
			Title:   stripTags(r.Title),
			URL:     r.URL,
			Snippet: stripTags(r.Description),
		})

		if len(out) >= MaxSearchResults {
			break
		}
	}

	return out, nil
}

var (
	ddgResult  = regexp.MustCompile(`(?is)<a[^>]+class="[^"]*result__a[^"]*"[^>]+href="([^"]+)"[^>]*>(.*?)</a>`)
	ddgSnippet = regexp.MustCompile(`(?is)<a[^>]+class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>`)
)

// duckDuckGo scrapes the HTML endpoint, which needs no account.
func (s WebSearch) duckDuckGo(ctx context.Context, query string) ([]SearchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://html.duckduckgo.com/html/",
		strings.NewReader("q="+url.QueryEscape(query)))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; PN Brain personal assistant)")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach DuckDuckGo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("DuckDuckGo returned %d", resp.StatusCode)
	}

	page, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}

	links := ddgResult.FindAllStringSubmatch(string(page), MaxSearchResults)
	snippets := ddgSnippet.FindAllStringSubmatch(string(page), MaxSearchResults)

	// Nothing parsed from a successful response means the markup changed. Say
	// so, rather than reporting "no results" and letting the model conclude the
	// web has nothing on the subject.
	if len(links) == 0 {
		return nil, fmt.Errorf(
			"could not read DuckDuckGo's results; the page layout has probably changed. " +
				"Setting a Brave API key avoids this")
	}

	out := make([]SearchResult, 0, len(links))

	for i, m := range links {
		r := SearchResult{Title: stripTags(m[2]), URL: unwrapRedirect(m[1])}

		if i < len(snippets) {
			r.Snippet = stripTags(snippets[i][1])
		}

		out = append(out, r)
	}

	return out, nil
}

// unwrapRedirect pulls the real destination out of DuckDuckGo's tracking link,
// so the model is given somewhere it can actually fetch.
func unwrapRedirect(href string) string {
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}

	parsed, err := url.Parse(href)
	if err != nil {
		return href
	}

	if target := parsed.Query().Get("uddg"); target != "" {
		return target
	}

	return href
}

func stripTags(s string) string {
	return strings.TrimSpace(manySpaces.ReplaceAllString(
		htmlTags.ReplaceAllString(strings.NewReplacer(
			"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`,
			"&#39;", "'", "&nbsp;", " ", "&#x27;", "'",
		).Replace(s), ""), " "))
}
