package setup

import (
	"net/http"
	"strings"
	"testing"
)

/*
 * A page on another site cannot drive setup.
 *
 * Found by the 1.0 security review: setup had no Origin or Host check, so a
 * page the owner had open could post {"level":"everything"} to /freedom —
 * every approval off — or point the hosted provider at its own server, and
 * read /state by pointing its own name at 127.0.0.1.
 */
func TestAnotherSiteCannotDriveSetup(t *testing.T) {
	server, _ := startServer(t)
	base := server.URL()

	send := func(method, path, body string, set func(*http.Request)) int {
		t.Helper()

		r, err := http.NewRequest(method, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}

		if set != nil {
			set(r)
		}

		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}

		resp.Body.Close()

		return resp.StatusCode
	}

	from := func(origin string) func(*http.Request) {
		return func(r *http.Request) { r.Header.Set("Origin", origin) }
	}

	// Everything is allowed out of the box (ADR 0007); the owner chose "ask".
	postJSON(t, base+"/freedom", `{"level":"ask"}`)

	// A blind cross-site POST, the way a page sends one.
	if code := send("POST", "/freedom", `{"level":"everything"}`, from("https://evil.test")); code != http.StatusForbidden {
		t.Errorf("a POST from another site was answered %d", code)
	}

	if level := getJSON(t, base+"/state")["freedom"]; level != "ask" {
		t.Fatal("another site turned every approval off")
	}

	// A GET from an <img> on another site says so in Sec-Fetch-Site.
	if code := send("GET", "/state", "", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }); code != http.StatusForbidden {
		t.Errorf("a cross-site read was answered %d", code)
	}

	// A name pointed at this computer: DNS rebinding.
	if code := send("GET", "/state", "", func(r *http.Request) { r.Host = "evil.test" }); code != http.StatusForbidden {
		t.Errorf("a request by another name was answered %d", code)
	}

	// What changes something is never a GET, so the Origin rule always applies.
	for _, path := range []string{"/done", "/install?name=x", "/voice?kind=x", "/freedom", "/api-key"} {
		if code := send("GET", path, "", nil); code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s was answered %d", path, code)
		}
	}

	// And the page's own requests still work.
	if code := send("POST", "/freedom", `{"level":"ask"}`, func(r *http.Request) {
		r.Header.Set("Origin", base)
		r.Header.Set("Sec-Fetch-Site", "same-origin")
	}); code != http.StatusOK {
		t.Errorf("the page's own POST was answered %d", code)
	}

	// The page itself may be opened from anywhere.
	if code := send("GET", "/", "", func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }); code != http.StatusOK {
		t.Errorf("opening the page from a link was answered %d", code)
	}
}
