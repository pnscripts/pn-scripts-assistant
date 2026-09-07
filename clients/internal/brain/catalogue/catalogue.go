// Package catalogue is every model that can be installed, from the place that
// knows.
//
// It began as nine models written into a file, which is a list of what one
// person had heard of — wrong within a month, and wrong in the direction that
// matters: somebody looking for the model they read about yesterday does not
// find it and concludes the program cannot run it.
//
// So the list comes from ollama's own library page, which is where the answer
// actually lives. Two hundred and thirty-nine of them at the time of writing,
// with what each one is, what it can do, and the sizes it comes in.
package catalogue

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Where the list lives.
const libraryURL = "https://ollama.com/library"

/*
 * HowLongItKeeps is how long a fetched list is reused.
 *
 * The library changes a few times a week and this page is opened far more
 * often than that, so asking every time would be a network round trip to
 * learn nothing. Long enough to be free, short enough that a model published
 * this morning appears today.
 */
const HowLongItKeeps = 6 * time.Hour

// Model is one entry in the library.
type Model struct {
	Name string `json:"name"`

	// What it is, in its authors' own words rather than anybody's summary.
	What string `json:"what"`

	// Can is what it is capable of: tools, vision, thinking, embedding.
	// Empty means an ordinary chat model.
	Can []string `json:"can,omitempty"`

	// Sizes are the parameter counts it is published in — "1b", "3b", "70b".
	// The size is what decides whether a machine can run it at all, so a model
	// with no sizes listed is one this cannot judge.
	Sizes []string `json:"sizes,omitempty"`

	// Pulls is roughly how many times it has been downloaded, which is the
	// only signal here about whether anybody found it useful.
	Pulls string `json:"pulls,omitempty"`
}

var (
	mu     sync.Mutex
	cached []Model
	when   time.Time
)

/*
 * Fetch returns the library, from memory when it has been read recently.
 *
 * An error is only returned when there is nothing to return at all: a fetch
 * that fails while a list from an hour ago is in hand answers with the list.
 * The alternative is an empty page because a website was briefly slow, which
 * is a worse answer than a slightly old one.
 */
func Fetch(ctx context.Context, client *http.Client) ([]Model, error) {
	mu.Lock()
	fresh := time.Since(when) < HowLongItKeeps && len(cached) > 0
	held := cached
	mu.Unlock()

	if fresh {
		return held, nil
	}

	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, libraryURL, nil)
	if err != nil {
		return held, err
	}

	resp, err := client.Do(req)
	if err != nil {
		if len(held) > 0 {
			return held, nil
		}

		return nil, fmt.Errorf("could not reach the model library: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if len(held) > 0 {
			return held, nil
		}

		return nil, fmt.Errorf("the model library answered %d", resp.StatusCode)
	}

	// Bounded: this is a page from the internet and it is being held in memory.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return held, err
	}

	found := parse(string(body))

	if len(found) == 0 {
		if len(held) > 0 {
			return held, nil
		}

		return nil, fmt.Errorf("the model library page could not be read")
	}

	mu.Lock()
	cached = found
	when = time.Now()
	mu.Unlock()

	return found, nil
}

/*
 * The page is HTML rather than an API, so this reads it as HTML.
 *
 * Deliberately loose. A parser that insists on the exact markup breaks the
 * first time somebody changes a class name, and the failure would be an empty
 * list on somebody's screen — so each piece is optional and a model with a
 * name is kept even when everything else about it was unreadable.
 */
var (
	entry = regexp.MustCompile(`(?s)<a href="/library/([a-z0-9._\-]+)".*?</li>`)
	blurb = regexp.MustCompile(`(?s)<p class="[^"]*max-w-lg[^"]*">(.*?)</p>`)
	chip  = regexp.MustCompile(`(?s)<span[^>]*class="[^"]*(?:indigo|#ddf4ff)[^"]*"[^>]*>(.*?)</span>`)
	pulls = regexp.MustCompile(`(?s)([\d.,]+[KMB]?)\s*(?:Pulls|Downloads)`)
	tags  = regexp.MustCompile(`<[^>]+>`)
)

// knownSize is a parameter count: 1b, 3b, 70b, 1.5b.
var knownSize = regexp.MustCompile(`^\d+(\.\d+)?b$`)

func parse(page string) []Model {
	var out []Model

	seen := map[string]bool{}

	for _, m := range entry.FindAllStringSubmatch(page, -1) {
		name, block := m[1], m[0]

		if seen[name] {
			continue
		}

		seen[name] = true

		model := Model{Name: name}

		if b := blurb.FindStringSubmatch(block); b != nil {
			model.What = clean(b[1])
		}

		for _, c := range chip.FindAllStringSubmatch(block, -1) {
			word := strings.ToLower(clean(c[1]))

			switch {
			case word == "":
			case knownSize.MatchString(word):
				model.Sizes = append(model.Sizes, word)
			default:
				model.Can = append(model.Can, word)
			}
		}

		if p := pulls.FindStringSubmatch(block); p != nil {
			model.Pulls = p[1]
		}

		out = append(out, model)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

func clean(s string) string {
	s = tags.ReplaceAllString(s, "")
	s = html.UnescapeString(s)

	return strings.Join(strings.Fields(s), " ")
}

/*
 * MemoryFor estimates what one size wants while it is running, in gigabytes.
 *
 * An estimate and said to be one. What a model needs depends on the
 * quantisation ollama picks, the context length and what else is loaded, and
 * the useful thing here is not a precise number — it is the difference between
 * "this fits" and "this machine will swap itself to a standstill", which is
 * two orders of magnitude and does not need three significant figures.
 *
 * Ollama's default is roughly four bits a parameter, so about 0.6GB per
 * billion, plus a bit over a gigabyte for the context and the runtime.
 */
func MemoryFor(size string) float64 {
	size = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(size)), "b")

	billions, err := strconv.ParseFloat(size, 64)
	if err != nil || billions <= 0 {
		return 0
	}

	return billions*0.6 + 1.2
}

// Embedding reports whether a model is for memory rather than for talking.
func (m Model) Embedding() bool {
	for _, c := range m.Can {
		if c == "embedding" {
			return true
		}
	}

	return strings.Contains(m.Name, "embed")
}
