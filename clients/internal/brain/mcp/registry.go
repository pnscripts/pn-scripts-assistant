package mcp

import (
	"pn-scripts-assistant/internal/brain/redact"

	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

/*
 * Which servers there are, which its owner has approved, and what each may do.
 *
 * Two files in the brain's folder. servers.json is the list, readable and
 * editable: what each server runs or where it is, who may use it, which of
 * its tools only read. secrets.json is the credentials, one file, readable by
 * its owner alone, and read by nothing but the code that starts a server or
 * sends it a request — never shown in the interface, never logged, never in
 * anything a model sees.
 *
 * A server comes from the catalogue this program ships, pinned to a version,
 * or is added by its owner. Either way it does nothing until approved, and a
 * model can only ask for it to be approved — it can never add one.
 */

// FolderName is where the integration files live in the brain's folder.
const FolderName = "integrations"

// Server is one integration.
type Server struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Why   string `json:"why,omitempty"`

	// Transport is stdio or http. Command is the argv for stdio, fixed;
	// URL the address for http.
	Transport string   `json:"transport"`
	Command   []string `json:"command,omitempty"`
	URL       string   `json:"url,omitempty"`

	// Env is what the program is given beyond a path and a home. Secrets
	// are the names of the secrets it is given as well: as environment
	// variables for a program, as the Header for a server elsewhere.
	Env     map[string]string `json:"env,omitempty"`
	Secrets []string          `json:"secrets,omitempty"`
	Header  string            `json:"header,omitempty"`

	// Recipe is what has to be installed for it to run — node for npx.
	Recipe string `json:"recipe,omitempty"`

	// Version is the pinned version of what it runs, for showing.
	Version string `json:"version,omitempty"`

	// Network is a program on this machine that reaches the internet
	// itself, which privacy treats as leaving the machine.
	Network bool `json:"network,omitempty"`

	// Licence and Source are what somebody approving it should know.
	Licence string `json:"licence,omitempty"`
	Source  string `json:"source,omitempty"`

	// Approved is its owner having said yes; Active is whether it should be
	// running. Neither is ever set by a model.
	Approved   bool   `json:"approved"`
	ApprovedAt string `json:"approved_at,omitempty"`
	Active     bool   `json:"active"`

	// Agents may use it; "assistant" is its owner's own. Nobody else until
	// named here.
	Agents []string `json:"agents,omitempty"`

	// ReadOnly are its tools its owner has said only read. Everything else
	// is treated as changing something, whatever the server says about it.
	ReadOnly []string `json:"read_only,omitempty"`

	// Hidden are its tools its owner does not want offered at all.
	Hidden []string `json:"hidden,omitempty"`

	// FromCatalogue marks one that shipped with this program.
	FromCatalogue bool `json:"from_catalogue,omitempty"`
}

var validID = regexp.MustCompile(`^[a-z][a-z0-9]{1,20}$`)

/*
 * Remote is a server elsewhere, or a program here that reaches out: either
 * way, what it is sent leaves the machine.
 */
func (s Server) Remote() bool {
	if s.Network {
		return true
	}

	if s.Transport != "http" {
		return false
	}

	u, err := url.Parse(s.URL)
	if err != nil {
		return true
	}

	host := u.Hostname()

	if host == "localhost" {
		return false
	}

	ip := net.ParseIP(host)

	return ip == nil || !ip.IsLoopback()
}

// Check refuses a server that could not be run safely as written.
func (s Server) Check() error {
	if !validID.MatchString(s.ID) {
		return fmt.Errorf("an integration's name is lower-case letters and digits, like files or github")
	}

	switch s.Transport {
	case "stdio":
		if len(s.Command) == 0 {
			return fmt.Errorf("%s runs a program and names none", s.ID)
		}

		// An argv, never a shell. A shell here would be a way to run
		// anything at all under the name of an approved integration.
		switch filepath.Base(s.Command[0]) {
		case "sh", "bash", "zsh", "dash", "fish", "ksh":
			return fmt.Errorf("%s would run a shell; give the program and its arguments instead", s.ID)
		}
	case "http":
		u, err := url.Parse(s.URL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return fmt.Errorf("%s needs an http or https address", s.ID)
		}

		if u.Scheme == "http" && s.Remote() {
			return fmt.Errorf("%s is elsewhere and not https: its credentials would cross the network readable", s.ID)
		}
	default:
		return fmt.Errorf("%s is neither stdio nor http", s.ID)
	}

	for name := range s.Env {
		for _, secret := range s.Secrets {
			if name == secret {
				return fmt.Errorf("%s is both a setting and a secret", name)
			}
		}
	}

	return nil
}

// Where says in words where it runs, for somebody approving it.
func (s Server) Where() string {
	switch {
	case s.Transport == "http" && s.Remote():
		return "a server elsewhere, at " + s.URL + " — what it is sent leaves this machine"
	case s.Transport == "http":
		return "a server on this machine, at " + s.URL
	case s.Network:
		return "a program on this machine that reaches the internet itself: " + strings.Join(s.Command, " ")
	default:
		return "a program on this machine, with your access to it: " + strings.Join(s.Command, " ")
	}
}

// Book is the servers file and the secrets file.
type Book struct {
	root string
	mu   sync.Mutex
}

// Open is the brain's integrations.
func Open(root string) *Book { return &Book{root: root} }

func (b *Book) path(name string) string { return filepath.Join(b.root, FolderName, name) }

// Servers is every server its owner has added or approved.
func (b *Book) Servers() []Server {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.load()
}

func (b *Book) load() []Server {
	raw, err := os.ReadFile(b.path("servers.json"))
	if err != nil {
		return nil
	}

	var list []Server

	if json.Unmarshal(raw, &list) != nil {
		return nil
	}

	return list
}

func (b *Book) save(list []Server) error {
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })

	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(b.root, FolderName), 0o700); err != nil {
		return err
	}

	path := b.path("servers.json")

	if err := os.WriteFile(path+".new", append(raw, '\n'), 0o600); err != nil {
		return err
	}

	return os.Rename(path+".new", path)
}

// Get is one server.
func (b *Book) Get(id string) (Server, bool) {
	for _, s := range b.Servers() {
		if s.ID == id {
			return s, true
		}
	}

	return Server{}, false
}

// Put adds or replaces a server, as its owner edited it.
func (b *Book) Put(s Server) error {
	if err := s.Check(); err != nil {
		return err
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	list := b.load()

	for i := range list {
		if list[i].ID == s.ID {
			list[i] = s

			return b.save(list)
		}
	}

	return b.save(append(list, s))
}

// Change edits one server in place.
func (b *Book) Change(id string, edit func(*Server)) (Server, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	list := b.load()

	for i := range list {
		if list[i].ID == id {
			edit(&list[i])

			if err := list[i].Check(); err != nil {
				return Server{}, err
			}

			return list[i], b.save(list)
		}
	}

	return Server{}, fmt.Errorf("there is no integration called %q", id)
}

// Remove takes a server off the list, and its secrets with it.
func (b *Book) Remove(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	list := b.load()
	kept := list[:0]

	for _, s := range list {
		if s.ID != id {
			kept = append(kept, s)
		}
	}

	if err := b.save(kept); err != nil {
		return err
	}

	secrets := b.secrets()
	delete(secrets, id)

	return b.saveSecrets(secrets)
}

func (b *Book) secrets() map[string]map[string]string {
	raw, err := os.ReadFile(b.path("secrets.json"))
	if err != nil {
		return map[string]map[string]string{}
	}

	var all map[string]map[string]string

	if json.Unmarshal(raw, &all) != nil || all == nil {
		return map[string]map[string]string{}
	}

	return all
}

func (b *Book) saveSecrets(all map[string]map[string]string) error {
	raw, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(b.root, FolderName), 0o700); err != nil {
		return err
	}

	path := b.path("secrets.json")

	if err := os.WriteFile(path+".new", raw, 0o600); err != nil {
		return err
	}

	return os.Rename(path+".new", path)
}

// SetSecret keeps one secret for one server. Empty removes it.
func (b *Book) SetSecret(id, name, value string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("which secret?")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	all := b.secrets()

	if all[id] == nil {
		all[id] = map[string]string{}
	}

	if value == "" {
		delete(all[id], name)
	} else {
		all[id][name] = value
	}

	return b.saveSecrets(all)
}

// secretsFor is one server's secrets — for starting it, never for showing.
func (b *Book) secretsFor(id string) map[string]string {
	defer func() {
		for _, v := range b.secrets()[id] {
			redact.Add(v)
		}
	}()

	b.mu.Lock()
	defer b.mu.Unlock()

	return b.secrets()[id]
}

// HasSecret says whether a secret is set, without saying what it is.
func (b *Book) HasSecret(id, name string) bool {
	_, ok := b.secretsFor(id)[name]

	return ok
}

// Catalogue is the servers this program knows, pinned, for its owner to
// choose from. None is added, let alone approved, until they say.
func Catalogue() []Server {
	npm := func(pkg, version string, args ...string) []string {
		return append([]string{"npx", "-y", pkg + "@" + version}, args...)
	}

	uvx := func(pkg, version string, args ...string) []string {
		return append([]string{"uvx", pkg + "==" + version}, args...)
	}

	return []Server{
		{ID: "everything", Title: "MCP test server", Transport: "stdio",
			Why:     "the protocol's own reference server, with sample tools, resources and prompts — for checking that integrations work",
			Command: npm("@modelcontextprotocol/server-everything", "2026.8.31"), Recipe: "node",
			Version: "2026.8.31", Licence: "MIT", Source: "npm, @modelcontextprotocol/server-everything", FromCatalogue: true},
		{ID: "memory", Title: "Knowledge graph memory", Transport: "stdio",
			Why:     "a separate store of entities and relations a project's agents can build up",
			Command: npm("@modelcontextprotocol/server-memory", "2026.8.31"), Recipe: "node",
			Version: "2026.8.31", Licence: "MIT", Source: "npm, @modelcontextprotocol/server-memory", FromCatalogue: true},
		{ID: "thinking", Title: "Sequential thinking", Transport: "stdio",
			Why:     "a structured scratchpad for working a problem through step by step",
			Command: npm("@modelcontextprotocol/server-sequential-thinking", "2026.8.31"), Recipe: "node",
			Version: "2026.8.31", Licence: "MIT", Source: "npm, @modelcontextprotocol/server-sequential-thinking", FromCatalogue: true},
		{ID: "git", Title: "Git", Transport: "stdio",
			Why:     "reads a repository's history, diffs and branches",
			Command: uvx("mcp-server-git", "2026.8.18"), Recipe: "uv",
			Version: "2026.8.18", Licence: "MIT", Source: "PyPI, mcp-server-git", FromCatalogue: true},
		{ID: "fetch", Title: "Web fetch", Transport: "stdio", Network: true,
			Why:     "fetches web pages as text",
			Command: uvx("mcp-server-fetch", "2026.8.18"), Recipe: "uv",
			Version: "2026.8.18", Licence: "MIT", Source: "PyPI, mcp-server-fetch", FromCatalogue: true},
		{ID: "time", Title: "Time and time zones", Transport: "stdio",
			Why:     "the time anywhere, and converting between zones",
			Command: uvx("mcp-server-time", "2026.8.18"), Recipe: "uv",
			Version: "2026.8.18", Licence: "MIT", Source: "PyPI, mcp-server-time", FromCatalogue: true},
	}
}

// FromCatalogue is one server the catalogue knows.
func FromCatalogue(id string) (Server, bool) {
	for _, s := range Catalogue() {
		if s.ID == id {
			return s, true
		}
	}

	return Server{}, false
}
