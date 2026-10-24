package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

/*
 * Acquiring an integration a goal needs (owner 2026-10-23: tools are handled
 * exactly like models — found, checked and added on their own, least
 * privilege, fallback on failure).
 *
 * Where from: the catalogue this program ships first, then the official MCP
 * registry. The registry is open — anybody with a GitHub account or a domain
 * can publish there, and most of what is in it is a stranger's — so being in
 * it is not being trusted. A registry server is used on its own only when its
 * publisher is on the trusted list (verified namespaces: the protocol's own,
 * and well-known publishers; the user can add their own in
 * integrations/trusted.json).
 *
 * What is checked before anything is downloaded, every check said in words
 * for the report:
 *
 *   - the source is trusted and the entry is active;
 *   - the version is pinned exactly, from npm or PyPI, which verify the
 *     package's checksum as it is fetched (and the registry has verified that
 *     the package belongs to the publisher);
 *   - it runs on this machine over stdio, never as a server elsewhere that
 *     what it is given would be sent to;
 *   - it needs no secret, account or setting only its owner can give;
 *   - privacy lets it run: a registry program may reach the internet itself,
 *     so it is treated as doing so and is not added while privacy keeps
 *     everything on this machine.
 *
 * And least privilege once it runs: it is granted to the one specialist that
 * needed it, with no settings and no secrets, in a folder of its own; tools
 * the server itself says destroy are not offered at all; every other tool is
 * treated as changing something, whatever the server claims.
 *
 * The search goes out with a short word for the kind of tool ("sqlite",
 * "git"), checked to be nothing more, never with the user's own words.
 */

// OfficialRegistryURL is the MCP project's own registry.
const OfficialRegistryURL = "https://registry.modelcontextprotocol.io"

// TrustedPublishers are namespaces whose servers may be added without asking:
// the protocol's own and well-known publishers. Reverse-DNS namespaces are
// verified by domain, io.github.* by GitHub account.
var TrustedPublishers = []string{
	"io.modelcontextprotocol",
	"io.github.modelcontextprotocol",
	"com.microsoft",
	"io.github.microsoft",
	"io.github.github",
	"com.github",
	"io.github.upstash",
}

// Candidate is one server a source offers, as it would be added here.
type Candidate struct {
	Server

	// From is where it was found, in words; Publisher its namespace.
	From      string
	Publisher string

	// Status is the registry's: active, deprecated or deleted.
	Status string

	// Needs are settings it requires that only its owner could give; Remote
	// is a server elsewhere rather than a program here.
	Needs  []string
	Remote bool

	// Package is npm or pypi; Pinned is whether its version is exact.
	Package string
	Pinned  bool

	trusted bool
}

// Source is somewhere integrations are found.
type Source interface {
	Name() string
	Find(ctx context.Context, word string) ([]Candidate, error)
}

var plainWord = regexp.MustCompile(`^[a-z][a-z0-9-]{1,23}( [a-z][a-z0-9-]{1,23}){0,2}$`)

/*
 * SearchWord is what a registry may be asked: one to three plain words for a
 * kind of tool, at most 40 characters. Anything else — a sentence, a name, a
 * path, a letter outside a–z — is refused rather than sent, so the user's own
 * words never leave the machine through a search.
 */
func SearchWord(kind string) (string, error) {
	word := strings.Join(strings.Fields(strings.ToLower(kind)), " ")

	if len(word) > 40 || !plainWord.MatchString(word) {
		return "", fmt.Errorf("%q is not a short word for a kind of tool, so it is not sent to a registry", kind)
	}

	return word, nil
}

// CatalogueSource is the servers this program ships, pinned.
type CatalogueSource struct{}

func (CatalogueSource) Name() string { return "the built-in catalogue" }

func (CatalogueSource) Find(_ context.Context, word string) ([]Candidate, error) {
	var out []Candidate

	for _, s := range Catalogue() {
		text := strings.ToLower(s.ID + " " + s.Title + " " + s.Why)

		if !strings.Contains(text, word) {
			continue
		}

		pkg := "npm"
		if s.Recipe == "uv" {
			pkg = "pypi"
		}

		out = append(out, Candidate{Server: s, From: "the built-in catalogue", Publisher: "this program",
			Status: "active", Package: pkg, Pinned: true, trusted: true})
	}

	return out, nil
}

// OfficialRegistry is the MCP registry, read over its public API.
type OfficialRegistry struct {
	URL     string
	HTTP    *http.Client
	Trusted []string
}

func (OfficialRegistry) Name() string { return "the official MCP registry" }

type registryAnswer struct {
	Servers []struct {
		Server struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Version     string `json:"version"`
			Repository  struct {
				URL string `json:"url"`
			} `json:"repository"`
			Packages []struct {
				RegistryType string `json:"registryType"`
				Identifier   string `json:"identifier"`
				Version      string `json:"version"`
				Transport    struct {
					Type string `json:"type"`
				} `json:"transport"`
				Environment []struct {
					Name     string `json:"name"`
					Required bool   `json:"isRequired"`
					Secret   bool   `json:"isSecret"`
					Default  string `json:"default"`
				} `json:"environmentVariables"`
				Arguments []struct {
					Name     string `json:"name"`
					Value    string `json:"value"`
					Required bool   `json:"isRequired"`
					Type     string `json:"type"`
				} `json:"packageArguments"`
			} `json:"packages"`
			Remotes []struct {
				Type string `json:"type"`
				URL  string `json:"url"`
			} `json:"remotes"`
		} `json:"server"`
		Meta map[string]struct {
			Status string `json:"status"`
		} `json:"_meta"`
	} `json:"servers"`
}

func (r OfficialRegistry) Find(ctx context.Context, word string) ([]Candidate, error) {
	base := r.URL
	if base == "" {
		base = OfficialRegistryURL
	}

	client := r.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	trusted := r.Trusted
	if trusted == nil {
		trusted = TrustedPublishers
	}

	q := url.Values{"search": {word}, "version": {"latest"}, "limit": {"30"}}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+"/v0.1/servers?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the MCP registry could not be reached: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the MCP registry answered %s", resp.Status)
	}

	var answer registryAnswer

	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&answer); err != nil {
		return nil, fmt.Errorf("the MCP registry's answer could not be read: %w", err)
	}

	var out []Candidate

	for _, entry := range answer.Servers {
		v := entry.Server
		publisher, _, _ := strings.Cut(v.Name, "/")

		c := Candidate{From: "the official MCP registry", Publisher: publisher, Status: "active",
			trusted: publishedBy(publisher, trusted)}

		for _, m := range entry.Meta {
			if m.Status != "" {
				c.Status = m.Status
			}
		}

		c.Server = Server{ID: idFor(v.Name), Title: titleFor(v.Name), Why: oneLine(v.Description, 200),
			Version: v.Version, Source: "MCP registry, " + v.Name}

		if v.Repository.URL != "" {
			c.Source += " (" + v.Repository.URL + ")"
		}

		// A package on this machine is preferred to a server elsewhere.
		usable := false

		for _, p := range v.Packages {
			if p.Transport.Type != "" && p.Transport.Type != "stdio" {
				continue
			}

			c.Package, c.Pinned = p.RegistryType, exactVersion(p.Version)
			c.Transport, c.Network = "stdio", true

			switch p.RegistryType {
			case "npm":
				c.Command, c.Recipe = []string{"npx", "-y", p.Identifier + "@" + p.Version}, "node"
			case "pypi":
				c.Command, c.Recipe = []string{"uvx", p.Identifier + "==" + p.Version}, "uv"
			default:
				continue
			}

			c.Version = p.Version

			for _, e := range p.Environment {
				switch {
				case e.Secret && e.Required:
					c.Needs = append(c.Needs, e.Name+" (a secret)")
				case e.Required && e.Default == "":
					c.Needs = append(c.Needs, e.Name)
				}
			}

			for _, a := range p.Arguments {
				if a.Required && a.Value == "" {
					c.Needs = append(c.Needs, "the argument "+orElse(a.Name, "it needs"))
				}
			}

			usable = true

			break
		}

		if !usable && len(v.Remotes) > 0 {
			c.Remote, c.Transport, c.URL = true, "http", v.Remotes[0].URL
		}

		if !usable && !c.Remote {
			c.Package = "unsupported"
		}

		out = append(out, c)
	}

	return out, nil
}

// publishedBy is whether a namespace is one of the trusted, or under one of
// them by domain (com.microsoft.azure under com.microsoft).
func publishedBy(namespace string, trusted []string) bool {
	for _, t := range trusted {
		if namespace == t || strings.HasPrefix(namespace, t+".") && !strings.HasPrefix(t, "io.github.") {
			return true
		}
	}

	return false
}

var exactly = regexp.MustCompile(`^\d+\.\d+\.\d+([-+][0-9A-Za-z.-]+)?$`)

func exactVersion(v string) bool { return exactly.MatchString(v) }

var notLetters = regexp.MustCompile(`[^a-z0-9]+`)

// idFor is a registry name as an integration's id: lower-case letters and
// digits, starting with a letter, at most 21.
func idFor(name string) string {
	_, last, found := strings.Cut(name, "/")
	if !found {
		last = name
	}

	id := notLetters.ReplaceAllString(strings.ToLower(last), "")

	for _, noise := range []string{"mcpserver", "servermcp", "mcp", "server"} {
		if trimmed := strings.ReplaceAll(id, noise, ""); len(trimmed) >= 2 {
			id = trimmed
		}
	}

	id = strings.TrimLeft(id, "0123456789")
	if len(id) < 2 {
		id = "integration"
	}

	if len(id) > 21 {
		id = id[:21]
	}

	return id
}

func titleFor(name string) string {
	_, last, found := strings.Cut(name, "/")
	if !found {
		last = name
	}

	return strings.TrimSpace(strings.ReplaceAll(last, "-", " "))
}

// Review is what was checked about one candidate, in words.
type Review struct {
	OK     bool
	Passed []string
	Failed []string
}

// Summary is the review as one line for a report.
func (r Review) Summary() string {
	if r.OK {
		return "passed: " + strings.Join(r.Passed, "; ")
	}

	return "refused: " + strings.Join(r.Failed, "; ")
}

/*
 * Inspect reviews a candidate before anything of it is downloaded. online is
 * whether privacy lets anything leave this machine.
 */
func Inspect(c Candidate, online bool) Review {
	var r Review

	check := func(ok bool, passed, failed string) {
		if ok {
			r.Passed = append(r.Passed, passed)
		} else {
			r.Failed = append(r.Failed, failed)
		}
	}

	check(c.trusted, "from "+c.From+", published by "+c.Publisher+", a trusted publisher",
		"published by "+orElse(c.Publisher, "an unknown publisher")+", who is not on the trusted list")
	check(c.Status == "active", "active in its registry", "marked "+c.Status+" in its registry")
	check(!c.Remote, "runs on this machine", "is a server elsewhere: what it is given would leave this machine")

	if !c.Remote {
		check(c.Package == "npm" || c.Package == "pypi",
			"comes from "+c.Package+", which verifies the package's checksum as it downloads",
			"comes as "+orElse(c.Package, "nothing")+", which cannot be fetched and checked from here")
		check(c.Pinned, "pinned to version "+c.Version, "not pinned to one exact version ("+orElse(c.Version, "none")+")")
	}

	check(len(c.Needs) == 0, "needs no account, secret or setting",
		"needs what only you can give: "+strings.Join(c.Needs, ", "))
	check(online || !c.Network, "privacy lets it run",
		"may reach the internet itself, and privacy keeps everything on this machine")

	if err := c.Server.Check(); err != nil {
		r.Failed = append(r.Failed, err.Error())
	}

	r.OK = len(r.Failed) == 0

	return r
}

// Acquired is an integration added for a specialist, and what was checked.
type Acquired struct {
	View   View
	Review Review
	For    string
	Hidden []string
}

// ErrNoneFit is no source offering an integration that passed the review.
var ErrNoneFit = errors.New("no integration that passed the review was found")

/*
 * Acquire finds an integration of a kind for one specialist, reviews it, adds
 * it with the least it needs and starts it.
 *
 * Provide installs what it runs on (node, uv) when that is missing; nil means
 * it must already be here. One already added is reused and granted to the
 * specialist as well. Candidates are tried in order — the catalogue first —
 * and a failure to start moves on to the next.
 */
func (g *Gateway) Acquire(ctx context.Context, sources []Source, kind, forWhom string, online bool,
	provide func(recipe, forWhom string) error) (Acquired, error) {
	word, err := SearchWord(kind)
	if err != nil {
		return Acquired{}, err
	}

	if forWhom == "" {
		forWhom = "assistant"
	}

	var refused []string

	for _, source := range sources {
		found, err := source.Find(ctx, word)
		if err != nil {
			refused = append(refused, source.Name()+": "+err.Error())

			continue
		}

		for _, c := range found {
			review := Inspect(c, online)
			if !review.OK {
				refused = append(refused, c.Title+" ("+c.Source+"): "+review.Summary())

				continue
			}

			got, err := g.take(ctx, c, forWhom, provide)
			if err != nil {
				refused = append(refused, c.Title+": "+err.Error())

				continue
			}

			got.Review = review

			if g.Acquired != nil {
				g.Acquired(got.View.Server, forWhom)
			}

			return got, nil
		}
	}

	if len(refused) == 0 {
		return Acquired{}, fmt.Errorf("%w: nothing for %q", ErrNoneFit, word)
	}

	return Acquired{}, fmt.Errorf("%w for %q — %s", ErrNoneFit, word, strings.Join(refused, " | "))
}

func (g *Gateway) take(ctx context.Context, c Candidate, forWhom string, provide func(recipe, forWhom string) error) (Acquired, error) {
	s := c.Server

	// Least privilege: nobody else, no secret of the owner's, no claims
	// believed. (Settings are only ever what a source wrote itself; a
	// registry entry has none.)
	s.Secrets, s.Header, s.ReadOnly = nil, "", nil
	s.Approved, s.Active = false, false

	if have, ok := g.Book.Get(s.ID); ok {
		if have.Source != s.Source || strings.Join(have.Command, " ") != strings.Join(s.Command, " ") {
			// The same short name for something else: never replace it.
			for i := 2; ; i++ {
				id := fmt.Sprintf("%s%d", strings.TrimRight(s.ID[:min(len(s.ID), 19)], "0123456789"), i)
				if _, taken := g.Book.Get(id); !taken {
					s.ID = id

					break
				}
			}
		} else {
			s = have
		}
	}

	if s.Recipe != "" && g.Ready != nil && !g.Ready(s.Recipe) {
		if provide == nil {
			return Acquired{}, fmt.Errorf("it runs on %s, which is not installed", s.Recipe)
		}

		if err := provide(s.Recipe, forWhom); err != nil {
			return Acquired{}, fmt.Errorf("%s, which it runs on, could not be installed: %w", s.Recipe, err)
		}
	}

	agents := []string{forWhom}

	for _, a := range s.Agents {
		if a != forWhom {
			agents = append(agents, a)
		}
	}

	if _, ok := g.Book.Get(s.ID); !ok {
		if err := g.Book.Put(s); err != nil {
			return Acquired{}, err
		}
	}

	if _, err := g.Approve(s.ID, agents); err != nil {
		return Acquired{}, err
	}

	g.note(s.ID, "acquired", "", "for "+forWhom+" from "+c.From+", reviewed: "+Inspect(c, true).Summary())

	view, err := g.Activate(ctx, s.ID)
	if err != nil {
		// Nothing half-added is left behind.
		g.forgetAcquired(s.ID)

		return Acquired{}, err
	}

	// Tools the server itself says destroy are not offered at all.
	var hidden []string

	g.mu.Lock()
	if l := g.live[s.ID]; l != nil {
		for _, t := range l.tools {
			if a := t.Annotations; a != nil && a.DestructiveHint != nil && *a.DestructiveHint {
				hidden = append(hidden, t.Name)
			}
		}
	}
	g.mu.Unlock()

	if len(hidden) > 0 {
		if _, err := g.Book.Change(s.ID, func(s *Server) { s.Hidden = hidden }); err == nil {
			g.refresh(s.ID)
			g.note(s.ID, "least privilege", "", "not offered: "+strings.Join(hidden, ", "))
		}

		if v, ok := g.Book.Get(s.ID); ok {
			view = g.view(v, true)
		}
	}

	return Acquired{View: view, For: forWhom, Hidden: hidden}, nil
}

// forgetAcquired stops and removes an integration, with its working folder.
func (g *Gateway) forgetAcquired(id string) {
	g.Deactivate(id)
	_ = g.Book.Remove(id)
	_ = os.RemoveAll(filepath.Join(g.Book.root, FolderName, "run", id))
}

/*
 * Discard removes an integration goal mode added, when it has gone unused:
 * stopped, taken off the list with anything it kept, never one its owner
 * added — that is for the caller's ledger to say.
 */
func (g *Gateway) Discard(id string) error {
	if _, ok := g.Book.Get(id); !ok {
		return nil
	}

	g.forgetAcquired(id)
	g.note(id, "removed", "", "unused")

	return nil
}

// TrustedFrom is the trusted publishers with the user's own added, from
// integrations/trusted.json (a JSON list of namespaces).
func TrustedFrom(root string) []string {
	out := append([]string(nil), TrustedPublishers...)

	raw, err := os.ReadFile(filepath.Join(root, FolderName, "trusted.json"))
	if err != nil {
		return out
	}

	var own []string

	if json.Unmarshal(raw, &own) == nil {
		for _, ns := range own {
			if ns = strings.TrimSpace(ns); ns != "" {
				out = append(out, ns)
			}
		}
	}

	return out
}
