package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// MaxFetchChars caps what a page contributes to the conversation.
//
// A long article would otherwise fill the context and push out the exchange
// that made it relevant.
const MaxFetchChars = 40000

// FetchURL reads a public web page.
//
// Safe, because reading a page changes nothing — but it is half of an
// exfiltration route, which is why the other half (SensitivePaths) exists. A
// page can contain text telling the model to read a credentials file and fetch
// a URL with the contents attached, and neither step needs approval.
//
// AllowWeb is checked by the caller from the privacy mode, not here, so there
// is one place that decides whether the brain may reach the internet at all.
type FetchURL struct {
	// Resolver is swapped in tests. Nil means the system resolver.
	Resolver func(host string) ([]net.IP, error)

	Client *http.Client
}

func (FetchURL) Name() string { return "fetch_url" }

func (FetchURL) Description() string {
	return "Fetch a public web page and return its readable text. http and https only."
}

func (FetchURL) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {"type": "string", "description": "The page to fetch"}
		},
		"required": ["url"]
	}`)
}

func (FetchURL) Risk() Risk { return Safe }

func (FetchURL) Summarize(raw json.RawMessage) string {
	var a struct {
		URL string `json:"url"`
	}
	argsOf(raw, &a)

	return "Fetch " + a.URL
}

// GuardTarget refuses anything that is not a public web address.
//
// Every resolved address is checked, not just the hostname, because a public
// name can point at a private one. That is the whole trick behind server-side
// request forgery: evil.example.com resolving to 127.0.0.1 turns "fetch a web
// page" into "read whatever is listening on this machine" — including this
// brain's own unauthenticated API.
func (f FetchURL) GuardTarget(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("that is not a URL: %w", err)
	}

	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("only http and https can be fetched, not %q", parsed.Scheme)
	}

	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("that URL has no host")
	}

	var addresses []net.IP

	if ip := net.ParseIP(host); ip != nil {
		addresses = []net.IP{ip}
	} else {
		resolve := f.Resolver
		if resolve == nil {
			resolve = net.LookupIP
		}

		addresses, err = resolve(host)
		if err != nil || len(addresses) == 0 {
			return fmt.Errorf("could not resolve host: %s", host)
		}
	}

	for _, ip := range addresses {
		if isInternal(ip) {
			return fmt.Errorf(
				"refusing to fetch %s: it resolves to the internal address %s, "+
					"which is this machine or the local network", host, ip)
		}
	}

	return nil
}

// isInternal covers every range that is not the public internet.
func isInternal(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast() ||
		// Carrier-grade NAT: not private by Go's definition, not the public
		// internet either.
		inCIDR(ip, "100.64.0.0/10") ||
		// IPv6 unique local, the equivalent of the private ranges.
		inCIDR(ip, "fc00::/7")
}

func inCIDR(ip net.IP, cidr string) bool {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}

	return network.Contains(ip)
}

func (f FetchURL) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		URL string `json:"url"`
	}

	if err := argsOf(raw, &a); err != nil {
		return "", err
	}

	if err := f.GuardTarget(a.URL); err != nil {
		return "", err
	}

	client := f.Client
	if client == nil {
		client = &http.Client{
			Timeout: 20 * time.Second,
			// A redirect can point at a private address after the original
			// target passed the check, so every hop is guarded again.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}

				return f.GuardTarget(req.URL.String())
			},
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "PN Scripts Assistant (personal assistant)")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not fetch %s: %w", a.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%s returned %d", a.URL, resp.StatusCode)
	}

	// Read a bounded amount: a Content-Length can lie, and an endless stream
	// would otherwise be read until memory ran out.
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxFetchChars*4))
	if err != nil {
		return "", err
	}

	return ReadableText(string(body)), nil
}

// Go's regexp is RE2, which has no backreferences, so each element that must
// be removed with its contents gets its own expression rather than one pattern
// matching a tag against its own closing tag.
var dropElements = func() []*regexp.Regexp {
	names := []string{"script", "style", "noscript", "svg", "head"}
	out := make([]*regexp.Regexp, 0, len(names))

	for _, n := range names {
		out = append(out, regexp.MustCompile(`(?is)<`+n+`\b[^>]*>.*?</`+n+`>`))
	}

	return out
}()

var (
	htmlTags     = regexp.MustCompile(`(?s)<[^>]+>`)
	manySpaces   = regexp.MustCompile(`[ \t]+`)
	manyNewlines = regexp.MustCompile(`\n{3,}`)
)

// ReadableText strips a page down to what a person would read.
//
// Deliberately crude. A real HTML parser would be better, but the model only
// needs the prose, and a regex that occasionally leaves an artefact is a far
// smaller dependency than a browser engine.
func ReadableText(html string) string {
	text := html

	for _, re := range dropElements {
		text = re.ReplaceAllString(text, " ")
	}

	text = htmlTags.ReplaceAllString(text, " ")

	replacer := strings.NewReplacer(
		"&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">",
		"&quot;", `"`, "&#39;", "'", "&mdash;", "—", "&ndash;", "–",
	)
	text = replacer.Replace(text)

	text = manySpaces.ReplaceAllString(text, " ")
	text = manyNewlines.ReplaceAllString(text, "\n\n")
	text = strings.TrimSpace(text)

	if r := []rune(text); len(r) > MaxFetchChars {
		return string(r[:MaxFetchChars]) + "\n\n[truncated]"
	}

	return text
}
