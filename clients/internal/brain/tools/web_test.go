package tools

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The attack this closes: a public hostname resolving to a private address
// turns "fetch a web page" into "read whatever is listening on this machine",
// including the brain's own unauthenticated API.
func TestFetchRefusesInternalTargets(t *testing.T) {
	// A name that resolves wherever an attacker wants it to.
	f := FetchURL{Resolver: func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}}

	if err := f.GuardTarget("http://totally-public-site.example.com/"); err == nil {
		t.Fatal("a public name resolving to loopback was allowed")
	}
}

func TestFetchRefusesEveryInternalRange(t *testing.T) {
	refuse := []string{
		"http://127.0.0.1/",
		"http://127.0.0.1:8790/api/knowledge",
		"http://localhost/",
		"http://0.0.0.0/",
		"http://10.0.0.5/",
		"http://172.16.0.1/",
		"http://192.168.1.1/",
		// Cloud metadata: the classic SSRF prize.
		"http://169.254.169.254/latest/meta-data/",
		"http://100.64.0.1/",
		"http://[::1]/",
		"http://[fc00::1]/",
		"http://[fe80::1]/",
	}

	var f FetchURL

	for _, u := range refuse {
		if err := f.GuardTarget(u); err == nil {
			t.Errorf("allowed an internal target: %s", u)
		}
	}
}

func TestFetchRefusesNonHTTPSchemes(t *testing.T) {
	var f FetchURL

	for _, u := range []string{
		"file:///etc/passwd",
		"gopher://example.com/",
		"ftp://example.com/",
		"data:text/html,hello",
	} {
		if err := f.GuardTarget(u); err == nil {
			t.Errorf("allowed scheme: %s", u)
		}
	}
}

func TestFetchAllowsPublicAddresses(t *testing.T) {
	f := FetchURL{Resolver: func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}}

	if err := f.GuardTarget("https://example.com/page"); err != nil {
		t.Errorf("refused a public address: %v", err)
	}
}

// A target that passes the check and then redirects to a private address would
// defeat the whole guard, so every hop is checked again.
func TestFetchGuardsRedirects(t *testing.T) {
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("secrets"))
	}))
	defer internal.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL, http.StatusFound)
	}))
	defer redirector.Close()

	// Allow the first hop by treating the test server as public, but use the
	// real guard for redirects.
	f := FetchURL{}
	f.Client = &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return f.GuardTarget(req.URL.String())
		},
	}

	out, err := f.Execute(context.Background(),
		json.RawMessage(`{"url":`+quote(redirector.URL)+`}`))

	if err == nil && strings.Contains(out, "secrets") {
		t.Fatal("a redirect reached an internal address")
	}
}

func TestFetchIsSafeAndFetchSummaryNamesTheURL(t *testing.T) {
	var f FetchURL

	if f.Risk() != Safe {
		t.Error("fetching a page should not need approval")
	}

	if s := f.Summarize(json.RawMessage(`{"url":"https://example.com"}`)); !strings.Contains(s, "example.com") {
		t.Errorf("summary is %q", s)
	}
}

func TestReadableTextStripsMarkup(t *testing.T) {
	html := `<html><head><title>x</title><style>body{color:red}</style></head>
	<body><script>alert('hi')</script><h1>Heading</h1>
	<p>Some&nbsp;text &amp; more.</p></body></html>`

	got := ReadableText(html)

	for _, want := range []string{"Heading", "Some text & more."} {
		if !strings.Contains(got, want) {
			t.Errorf("readable text is missing %q: %q", want, got)
		}
	}

	// Script and style contents are not prose and must not reach the model.
	for _, unwanted := range []string{"alert(", "color:red", "<h1>"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("readable text still contains %q: %q", unwanted, got)
		}
	}
}

func TestReadableTextIsBounded(t *testing.T) {
	got := ReadableText("<p>" + strings.Repeat("word ", MaxFetchChars) + "</p>")

	if len([]rune(got)) > MaxFetchChars+40 {
		t.Errorf("readable text is %d runes, want about %d", len([]rune(got)), MaxFetchChars)
	}
}
