/*
 * Package latest is the installed program knowing a newer version is out,
 * and getting it the way it was installed.
 *
 * selfupdate answers the question for a copy built from a checkout, in
 * commits. This answers it for the copies people actually have — the .deb,
 * the Windows installer, the portable zip — in released versions, because
 * "three commits behind main" means nothing to somebody who installed 0.12.0
 * and everything to somebody who builds from source.
 *
 * What it promises, because an updater is the one part of a program that
 * replaces the program:
 *
 *   - One request to see what is newest: the public releases API of this
 *     repository, carrying nothing about the person asking — no identifier,
 *     no version in the user agent, no cookie. At most once a day unless they
 *     press "check".
 *   - Nothing is installed that was not checked. A download is accepted only
 *     from this repository's own release files (and the content host GitHub
 *     hands them to), and only when its SHA-256 matches the release's own
 *     SHA256SUMS.
 *   - Where a package manager installed it, the package manager updates it.
 *     A program installed by apt that overwrote itself would leave dpkg
 *     believing a version is there that is not.
 *   - Offline is an answer, not an error page: "could not reach GitHub".
 */
package latest

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Where releases are published.
const (
	Owner = "pnscripts"
	Repo  = "pn-scripts-assistant"

	// Page is where a person reads about a release and downloads it by hand.
	Page = "https://github.com/" + Owner + "/" + Repo + "/releases/latest"

	api = "https://api.github.com/repos/" + Owner + "/" + Repo + "/releases/latest"
)

/*
 * TestEnv points the check at another place, for the tests that prove an
 * older build really sees and installs a newer one (ci.yml) without a real
 * release existing.
 *
 * Honoured only for a loopback address. A variable that could send the
 * updater to any server would be a variable that turns it into a way to run
 * somebody else's program.
 */
const TestEnv = "PN_SCRIPTS_ASSISTANT_RELEASES_URL"

// Release is the newest published version.
type Release struct {
	Version string  `json:"version"`
	Page    string  `json:"page"`
	Assets  []Asset `json:"assets"`
}

// Asset is one downloadable file of a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

// Find returns the asset with this exact name.
func (r Release) Find(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}

	return Asset{}, false
}

// ErrOffline is a check that could not reach GitHub at all, which is the
// normal state of a machine without a network rather than a fault.
var ErrOffline = errors.New("could not reach GitHub to see what is newest")

// source is where the newest release is asked for, and whether that is the
// test override.
func source() (string, bool) {
	if raw := os.Getenv(TestEnv); raw != "" {
		if u, err := url.Parse(raw); err == nil && isLoopback(u.Hostname()) {
			return raw, true
		}
	}

	return api, false
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

// Client is the HTTP client every request here uses: a timeout, no cookies,
// and a redirect rule that refuses to leave the places a release lives.
func Client() *http.Client {
	_, testing := source()

	return &http.Client{
		Timeout: 10 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 5 {
				return errors.New("too many redirects")
			}

			return allowed(req.URL, testing)
		},
	}
}

/*
 * allowed is where a release file may come from: this repository's release
 * downloads on github.com, and the content hosts github.com hands them to
 * (a name under githubusercontent.com, matched with its leading dot so a host
 * merely ending in those letters does not qualify). HTTPS only.
 */
func allowed(u *url.URL, testing bool) error {
	if testing && u.Scheme == "http" && isLoopback(u.Hostname()) {
		return nil
	}

	if u.Scheme != "https" {
		return fmt.Errorf("refusing %s: not https", u.Host)
	}

	host := strings.ToLower(u.Hostname())

	switch {
	case host == "github.com":
		if !strings.HasPrefix(u.Path, "/"+Owner+"/"+Repo+"/releases/download/") {
			return fmt.Errorf("refusing %s: not a release file of this program", u.Path)
		}

		return nil

	case host == "api.github.com":
		return nil

	case strings.HasSuffix(host, ".githubusercontent.com"):
		return nil
	}

	return fmt.Errorf("refusing %s: not where this program's releases are", host)
}

/*
 * Newest asks for the newest published release.
 *
 * The request says who is asking only as far as GitHub requires a user agent
 * at all: the program's name, no version, nothing about the machine.
 */
func Newest(ctx context.Context, client *http.Client) (Release, error) {
	if client == nil {
		client = Client()
	}

	from, _ := source()

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, from, nil)
	if err != nil {
		return Release{}, err
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "pn-scripts-assistant")

	resp, err := client.Do(req)
	if err != nil {
		return Release{}, ErrOffline
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub answered %d when asked for the newest version", resp.StatusCode)
	}

	var body struct {
		Tag    string `json:"tag_name"`
		Page   string `json:"html_url"`
		Draft  bool   `json:"draft"`
		Pre    bool   `json:"prerelease"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int64  `json:"size"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Release{}, fmt.Errorf("could not read GitHub's answer: %w", err)
	}

	version := strings.TrimPrefix(strings.TrimSpace(body.Tag), "v")

	if _, ok := parse(version); !ok || body.Draft || body.Pre {
		return Release{}, fmt.Errorf("the newest release (%q) is not a version I can compare", body.Tag)
	}

	rel := Release{Version: version, Page: Page}

	if strings.HasPrefix(body.Page, "https://github.com/"+Owner+"/"+Repo+"/releases/") {
		rel.Page = body.Page
	}

	for _, a := range body.Assets {
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, URL: a.URL, Size: a.Size})
	}

	return rel, nil
}

/*
 * Newer says whether latest is a later release than have.
 *
 * Plain numbers, major.minor.patch. A build carrying anything after the
 * numbers — "0.12.0~git2026…" from a commit after a release, a pre-release —
 * counts as the version it is built on top of, so a development build is
 * never told to "update" to the release it already contains.
 */
func Newer(have, latest string) bool {
	h, okH := parse(have)
	l, okL := parse(latest)

	if !okH || !okL {
		return false
	}

	for i := range h {
		if l[i] != h[i] {
			return l[i] > h[i]
		}
	}

	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int

	v = strings.TrimPrefix(strings.TrimSpace(v), "v")

	// What comes after the numbers is a label on top of them.
	if i := strings.IndexAny(v, "~-+ "); i >= 0 {
		v = v[:i]
	}

	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}

	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}

		out[i] = n
	}

	return out, true
}

/*
 * Fetch downloads one release file into dir and checks it against the
 * release's SHA256SUMS before returning its path.
 *
 * Written to a temporary name and renamed only once the sum matches, so a
 * half-finished or tampered download never sits in dir under the name of the
 * real thing. A file that does not match is deleted.
 */
func Fetch(ctx context.Context, client *http.Client, rel Release, name, dir string) (string, error) {
	if client == nil {
		client = Client()
	}

	_, testing := source()

	asset, ok := rel.Find(name)
	if !ok {
		return "", fmt.Errorf("release %s has no %s", rel.Version, name)
	}

	sumsAsset, ok := rel.Find("SHA256SUMS")
	if !ok {
		return "", fmt.Errorf("release %s publishes no SHA256SUMS, so %s cannot be checked; not installing it", rel.Version, name)
	}

	for _, a := range []Asset{asset, sumsAsset} {
		u, err := url.Parse(a.URL)
		if err != nil {
			return "", fmt.Errorf("%s has an address I cannot read", a.Name)
		}

		if err := allowed(u, testing); err != nil {
			return "", err
		}
	}

	sums, err := get(ctx, client, sumsAsset.URL, 1<<16)
	if err != nil {
		return "", fmt.Errorf("could not download SHA256SUMS: %w", err)
	}

	want, err := SumFor(string(sums), name)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}

	final := filepath.Join(dir, name)
	part := final + ".part"

	os.Remove(part)

	out, err := os.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}

	got, err := stream(ctx, client, asset.URL, out)

	if closeErr := out.Close(); err == nil {
		err = closeErr
	}

	if err != nil {
		os.Remove(part)

		return "", fmt.Errorf("could not download %s: %w", name, err)
	}

	if got != want {
		os.Remove(part)

		return "", fmt.Errorf("%s does not match the release's SHA256SUMS (got %s, want %s); deleted it", name, got[:12], want[:12])
	}

	os.Remove(final)

	if err := os.Rename(part, final); err != nil {
		os.Remove(part)

		return "", err
	}

	return final, nil
}

// SumFor reads one file's SHA-256 out of a SHA256SUMS listing.
func SumFor(sums, name string) (string, error) {
	scan := bufio.NewScanner(strings.NewReader(sums))

	for scan.Scan() {
		fields := strings.Fields(scan.Text())
		if len(fields) != 2 {
			continue
		}

		if strings.TrimPrefix(fields[1], "*") != name {
			continue
		}

		sum := strings.ToLower(fields[0])

		if len(sum) != 64 {
			break
		}

		if _, err := hex.DecodeString(sum); err != nil {
			break
		}

		return sum, nil
	}

	return "", fmt.Errorf("SHA256SUMS has no usable line for %s; not installing it", name)
}

func get(ctx context.Context, client *http.Client, from string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, from, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "pn-scripts-assistant")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("answered %d", resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

func stream(ctx context.Context, client *http.Client, from string, to io.Writer) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, from, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "pn-scripts-assistant")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("answered %d", resp.StatusCode)
	}

	h := sha256.New()

	// A release file is tens of megabytes; anything past half a gigabyte is
	// not one of ours.
	if _, err := io.Copy(io.MultiWriter(to, h), io.LimitReader(resp.Body, 512<<20)); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
