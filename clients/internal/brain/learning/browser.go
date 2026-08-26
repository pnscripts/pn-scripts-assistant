package learning

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

// Site is a domain the owner uses, with how often.
//
// Deliberately a domain and a count, never a URL. "Petar regularly uses
// tools.swytchbike.com" is a useful thing for an assistant to know; "Petar
// opened this specific page at 2am on Tuesday" is surveillance, and it would
// also be recalled into prompts, which makes it worse rather than better. The
// aggregate carries almost all of the value at a fraction of the intrusion.
type Site struct {
	Domain string
	Visits int
	Source string // firefox or chrome
}

// MinimumVisits filters out noise. A domain seen once or twice says nothing
// about how somebody works, and a store full of such entries makes recall worse
// by crowding out the facts that matter.
const MinimumVisits = 5

// TopSites caps how many are kept.
const TopSites = 40

// skipDomains are visited constantly and mean nothing about the person: search
// boxes and CDNs.
var skipDomains = map[string]bool{
	"www.google.com": true, "google.com": true, "duckduckgo.com": true,
	"www.bing.com": true, "gstatic.com": true, "googleapis.com": true,
	"cdn.jsdelivr.net": true, "localhost": true, "127.0.0.1": true,
}

// skipPrefixes catch sign-in and mail hosts by shape rather than by name.
//
// "Petar uses accounts.google.bg" is not a fact about how he works — everybody
// with a Google account visits it, and it is a login redirect rather than a
// destination. Listing them individually would need a list that grows forever;
// these are recognisable by their subdomain.
var skipPrefixes = []string{
	"accounts.", "account.", "myaccount.", "id.", "login.", "signin.",
	"auth.", "sso.", "oauth.", "mail.", "webmail.", "cloudlogin.",
}

// skipContains catch the same thing where it is not a leading subdomain.
var skipContains = []string{".cloudlogin.", "login.microsoftonline"}

// isNoise reports whether a domain says nothing worth remembering.
func isNoise(domain string) bool {
	if skipDomains[domain] {
		return true
	}

	for _, p := range skipPrefixes {
		if strings.HasPrefix(domain, p) {
			return true
		}
	}

	for _, c := range skipContains {
		if strings.Contains(domain, c) {
			return true
		}
	}

	return false
}

// BrowserProfiles finds history databases without opening them.
func BrowserProfiles(home string) map[string][]string {
	found := map[string][]string{}

	firefox := []string{
		filepath.Join(home, ".mozilla", "firefox"),
		filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox"),
	}

	for _, base := range firefox {
		matches, _ := filepath.Glob(filepath.Join(base, "*", "places.sqlite"))
		found["firefox"] = append(found["firefox"], matches...)
	}

	chrome := []string{
		filepath.Join(home, ".config", "google-chrome"),
		filepath.Join(home, ".config", "chromium"),
		filepath.Join(home, "snap", "chromium", "common", "chromium"),
	}

	for _, base := range chrome {
		matches, _ := filepath.Glob(filepath.Join(base, "*", "History"))
		found["chrome"] = append(found["chrome"], matches...)
	}

	for k, v := range found {
		if len(v) == 0 {
			delete(found, k)
		}
	}

	return found
}

// ScanBrowsers reads history and returns the domains the owner actually uses.
//
// The file is copied before being opened. A running browser holds a lock, and
// more importantly opening the live database read-write could corrupt somebody's
// history — which would be an unforgivable thing for a note-taking assistant to
// do to a browser.
func ScanBrowsers(home string) ([]Site, error) {
	profiles := BrowserProfiles(home)

	if len(profiles) == 0 {
		return nil, fmt.Errorf("no browser history found under %s", home)
	}

	totals := map[string]*Site{}

	for browser, files := range profiles {
		for _, path := range files {
			sites, err := readHistory(browser, path)
			if err != nil {
				// One unreadable profile — a guest profile, a locked file —
				// must not abandon the others.
				continue
			}

			for _, s := range sites {
				if existing, ok := totals[s.Domain]; ok {
					existing.Visits += s.Visits

					continue
				}

				copied := s
				totals[s.Domain] = &copied
			}
		}
	}

	out := make([]Site, 0, len(totals))

	for _, s := range totals {
		if s.Visits < MinimumVisits || s.Domain == "" || isNoise(s.Domain) {
			continue
		}

		out = append(out, *s)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Visits > out[j].Visits })

	if len(out) > TopSites {
		out = out[:TopSites]
	}

	return out, nil
}

func readHistory(browser, path string) ([]Site, error) {
	temp, err := os.CreateTemp("", "pn-brain-history-*.sqlite")
	if err != nil {
		return nil, err
	}

	tempPath := temp.Name()
	temp.Close()

	defer os.Remove(tempPath)

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(tempPath, raw, 0o600); err != nil {
		return nil, err
	}

	// Read-only, and on a copy. Two layers, because the cost of being wrong
	// here is somebody's browser history.
	db, err := sql.Open("sqlite", tempPath+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	query := `SELECT rev_host, SUM(visit_count) FROM moz_places
	          WHERE rev_host IS NOT NULL AND visit_count > 0 GROUP BY rev_host`

	if browser == "chrome" {
		query = `SELECT url, visit_count FROM urls WHERE visit_count > 0`
	}

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Site

	for rows.Next() {
		var key string
		var visits int

		if err := rows.Scan(&key, &visits); err != nil {
			continue
		}

		domain := key

		if browser == "firefox" {
			// Firefox stores the host reversed, with a trailing dot.
			domain = reverseHost(key)
		} else {
			domain = domainOf(key)
		}

		if domain == "" {
			continue
		}

		out = append(out, Site{Domain: domain, Visits: visits, Source: browser})
	}

	return out, rows.Err()
}

// reverseHost undoes Firefox's rev_host encoding: "moc.elpmaxe.www." becomes
// "www.example.com".
func reverseHost(rev string) string {
	rev = strings.TrimSuffix(rev, ".")

	runes := []rune(rev)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}

	return string(runes)
}

// domainOf pulls the host out of a URL without parsing the rest of it, since
// the rest is exactly the part that must not be kept.
func domainOf(rawURL string) string {
	rest := rawURL

	if i := strings.Index(rest, "://"); i >= 0 {
		rest = rest[i+3:]
	}

	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}

	if i := strings.Index(rest, "@"); i >= 0 {
		rest = rest[i+1:]
	}

	if i := strings.Index(rest, ":"); i >= 0 {
		rest = rest[:i]
	}

	return strings.ToLower(rest)
}

// Sentence renders a site as the fact the brain will remember.
func (s Site) Sentence(owner string) string {
	if owner == "" {
		owner = "The owner"
	}

	return fmt.Sprintf("%s regularly uses the website %s (%d visits in browser history).",
		owner, s.Domain, s.Visits)
}

// Source marks these as browser-derived. Unlike a project or a document there is
// no path to recheck, so the Validator leaves them for a person — which is
// right: a domain someone stopped using should not silently persist as a fact
// about how they work.
func (s Site) SourceKey() string { return "browser:" + s.Domain }

// TopSitesInSummary is how many appear in the ranked summary.
const TopSitesInSummary = 10

// FromSites renders scanned sites as observations.
//
// One observation per site, plus one ranked summary of the top few — and the
// summary is the point. Every per-site fact is worded almost identically
// ("Petar regularly uses X (N visits)"), so they are all near-neighbours of each
// other in embedding space, and a question like "which sites do I use most"
// recalls an essentially arbitrary twelve of them. The model then ranks whatever
// it happened to get, which produced an answer that put a site with 26 visits
// above one with 282 and omitted the most-used site entirely.
//
// Semantic recall answers "do I use X". It cannot answer "which is most",
// because that is a property of the whole set rather than of any member. The
// summary puts the whole set into one fact, so recalling it once is enough.
func FromSites(sites []Site, owner string) Observations {
	out := make(Observations, 0, len(sites)+1)

	if owner == "" {
		owner = "The owner"
	}

	if len(sites) > 0 {
		top := sites
		if len(top) > TopSitesInSummary {
			top = top[:TopSitesInSummary]
		}

		var b strings.Builder

		fmt.Fprintf(&b, "%s's most-used websites, in order of how often they are visited: ", owner)

		for i, s := range top {
			if i > 0 {
				b.WriteString(", ")
			}

			fmt.Fprintf(&b, "%s (%d)", s.Domain, s.Visits)
		}

		b.WriteString(".")

		out = append(out, Observation{Content: b.String(), Source: "browser:__ranking__"})
	}

	for _, s := range sites {
		out = append(out, Observation{Content: s.Sentence(owner), Source: s.SourceKey()})
	}

	return out
}
