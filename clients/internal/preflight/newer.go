package preflight

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

/*
 * Noticing that a newer release exists.
 *
 * Offering to fetch what is current is not much use if nobody can tell whether
 * they are behind: the page said "ok" beside an Ollama from a year ago and
 * meant only that one was present. What somebody wants to know is whether to
 * bother, and that is a question about two version numbers.
 *
 * Asked of the redirect rather than of the API. GitHub answers
 * /releases/latest with a 302 to /releases/tag/v0.33.2, so the tag arrives in
 * a header with no token, no JSON and no rate limit worth thinking about —
 * where api.github.com allows sixty unauthenticated calls an hour and then
 * starts refusing, which would make this work on a quiet machine and fail on a
 * busy one for reasons nobody could see.
 */

// HowLongToWaitForAVersion caps the check.
//
// Short on purpose: this is a nicety on a page whose real job is installing
// things, and a slow network must not make it look stuck.
const HowLongToWaitForAVersion = 6 * time.Second

/*
 * LatestTag returns the version a project's latest release is tagged with.
 *
 * Empty when it cannot be found, which is not an error worth surfacing: no
 * internet, a redirect that changed shape, or GitHub being down all mean the
 * same thing here — nothing can be said about whether an update exists, and
 * saying nothing is the correct outcome.
 */
func LatestTag(ctx context.Context, releasesLatestURL string) string {
	if tag, known := rememberedTag(releasesLatestURL); known {
		return tag
	}

	tag := askForTag(ctx, releasesLatestURL)

	rememberTag(releasesLatestURL, tag)

	return tag
}

/*
 * How long an answer is kept.
 *
 * The setup page asks for its state every two seconds, so without this the
 * check would put a request to GitHub on the wire every two seconds — each
 * with a six second timeout that could stack up behind it — to answer a
 * question whose answer changes a few times a year. That is not a nicety
 * misbehaving, it is a page hammering somebody else's servers.
 *
 * Six hours. A release that appeared this morning is worth hearing about this
 * afternoon, and nothing here is urgent enough to justify asking sooner.
 */
const RememberAVersionFor = 6 * time.Hour

var versions = struct {
	mu   sync.Mutex
	seen map[string]versionAnswer
}{seen: map[string]versionAnswer{}}

type versionAnswer struct {
	tag string
	at  time.Time
}

func rememberedTag(url string) (string, bool) {
	versions.mu.Lock()
	defer versions.mu.Unlock()

	answer, found := versions.seen[url]
	if !found || time.Since(answer.at) > RememberAVersionFor {
		return "", false
	}

	return answer.tag, true
}

/*
 * rememberTag records the answer, including when there was not one.
 *
 * A failure is cached as deliberately as a success. Without that, a machine
 * with no internet would retry every two seconds for as long as the page was
 * open, each attempt waiting the full timeout — which is how a check nobody
 * asked for turns into a page that feels broken.
 */
func rememberTag(url, tag string) {
	versions.mu.Lock()
	versions.seen[url] = versionAnswer{tag: tag, at: time.Now()}
	versions.mu.Unlock()
}

func askForTag(ctx context.Context, releasesLatestURL string) string {
	ctx, cancel := context.WithTimeout(ctx, HowLongToWaitForAVersion)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, releasesLatestURL, nil)
	if err != nil {
		return ""
	}

	// The redirect is the answer, so it must not be followed.
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	return tagFromURL(resp.Header.Get("Location"))
}

// tagFromURL reads the version out of a .../releases/tag/v1.2.3 address.
func tagFromURL(location string) string {
	const marker = "/releases/tag/"

	at := strings.Index(location, marker)
	if at < 0 {
		return ""
	}

	tag := location[at+len(marker):]

	// Anything after the tag — a query, a fragment — is not part of it.
	if cut := strings.IndexAny(tag, "?#/"); cut >= 0 {
		tag = tag[:cut]
	}

	return strings.TrimSpace(tag)
}

/*
 * IsNewer reports whether one version is later than another.
 *
 * Compared piece by piece as numbers, because "0.33.2" against "0.32.0" is the
 * case that matters and comparing those as text says 0.33.2 is older — the
 * strings differ first at the second character, where "2" sorts below "3".
 * That is the whole reason this function exists rather than a < b.
 *
 * Anything it cannot parse means no, so a version string in a shape nobody
 * anticipated produces silence rather than a wrong claim that an update is
 * waiting.
 */
func IsNewer(candidate, installed string) bool {
	a, aOK := versionNumbers(candidate)
	b, bOK := versionNumbers(installed)

	if !aOK || !bOK {
		return false
	}

	for i := 0; i < len(a) || i < len(b); i++ {
		var x, y int

		if i < len(a) {
			x = a[i]
		}

		if i < len(b) {
			y = b[i]
		}

		if x != y {
			return x > y
		}
	}

	return false
}

// versionNumbers pulls the numeric parts out of "v0.33.2" or "ollama version
// is 0.32.0".
func versionNumbers(text string) ([]int, bool) {
	// The first thing that looks like a dotted number, so a version embedded
	// in a sentence is found as readily as one on its own.
	var start = -1

	for i, r := range text {
		if r >= '0' && r <= '9' {
			start = i

			break
		}
	}

	if start < 0 {
		return nil, false
	}

	end := start

	for end < len(text) && (text[end] == '.' ||
		(text[end] >= '0' && text[end] <= '9')) {
		end++
	}

	var out []int

	for _, piece := range strings.Split(text[start:end], ".") {
		if piece == "" {
			continue
		}

		n, err := strconv.Atoi(piece)
		if err != nil {
			return nil, false
		}

		out = append(out, n)
	}

	return out, len(out) > 0
}

/*
 * withUpdateNote adds "· 0.33.2 available" to a version, when one is.
 *
 * Written onto the detail line rather than turned into a state of its own,
 * because an old Ollama still works: it is not missing, not broken, and making
 * it look like either would send somebody to fix a thing that is not wrong.
 * The word "available" is doing the work — it reports a fact and asks for
 * nothing.
 *
 * Silent whenever it cannot tell. No internet, a redirect that changed shape,
 * a version string in an unexpected form: each of those means nothing can be
 * said about whether an update exists, and saying nothing is correct. The
 * alternative — "could not check for updates" on a page about installing
 * things — is noise about a nicety.
 */
func withUpdateNote(installed, releasesLatestURL string) string {
	if installed == "" {
		return installed
	}

	latest := LatestTag(context.Background(), releasesLatestURL)

	if latest == "" || !IsNewer(latest, installed) {
		return installed
	}

	return installed + " · " + strings.TrimPrefix(latest, "v") + " available"
}
