// Package mcpimport finds the MCP servers a person has already set up in
// other programs (Claude Desktop, Claude Code, Cursor, VS Code), so the
// assistant can use them too, and follows those entries afterwards.
//
// Pure: it reads bytes and returns what it found; the brain reads the files,
// watches them and keeps the integrations (package mcp).
//
// Nothing found is used by itself. A found server is listed; its owner
// imports it at the desk, and it is then an integration like any other:
// not approved until they say so. A server that cannot be run as written
// here (a shell, a variable only the other program knows, an environment
// file, the old SSE transport, several headers) is listed with the reason
// and cannot be imported.
//
// Formats (checked 14.11.2026):
//   - Claude Desktop: claude_desktop_config.json, "mcpServers"
//     (modelcontextprotocol.io/docs/develop/connect-local-servers).
//   - Claude Code: ~/.claude.json, "mcpServers" (user scope) and
//     "projects"."<folder>"."mcpServers" (local scope)
//     (code.claude.com/docs/en/mcp).
//   - Cursor: ~/.cursor/mcp.json, "mcpServers" (cursor.com/docs/context/mcp).
//   - VS Code: the profile's mcp.json, "servers" and "inputs"
//     (code.visualstudio.com/docs/copilot/reference/mcp-configuration).
//
// Project files (.mcp.json, .cursor/mcp.json, .vscode/mcp.json) are not
// read: they come with whatever folder was cloned, and the programs that
// own them ask before trusting them.
package mcpimport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"pn-scripts-assistant/internal/brain/mcp"
)

// Client is a program whose MCP settings are read.
type Client string

const (
	ClaudeDesktop Client = "claude-desktop"
	ClaudeCode    Client = "claude-code"
	Cursor        Client = "cursor"
	VSCode        Client = "vscode"
)

// Clients is every client, in the order the page lists them.
var Clients = []Client{ClaudeDesktop, ClaudeCode, Cursor, VSCode}

// Title is a client's name as people know it.
func (c Client) Title() string { return mcp.ClientTitle(string(c)) }

// MostBytes is the largest settings file read: Claude Code's grows with
// every project it has seen, but never near this.
const MostBytes = 32 << 20

// Place is one settings file of one client.
type Place struct {
	Client Client `json:"client"`
	File   string `json:"file"`
}

/*
 * Env is where a computer keeps things: the user's home, the system
 * ("linux", "windows", "darwin"), on Linux XDG_CONFIG_HOME, on Windows
 * APPDATA, and CLAUDE_CONFIG_DIR when Claude Code was told to keep its
 * settings elsewhere.
 */
type Env struct {
	GOOS            string
	Home            string
	ConfigHome      string
	AppData         string
	ClaudeConfigDir string
}

// Places is every settings file read on this kind of computer.
func Places(e Env) []Place {
	if e.Home == "" {
		return nil
	}

	var appConfig string // where desktop programs keep their settings

	switch e.GOOS {
	case "windows":
		appConfig = e.AppData
		if appConfig == "" {
			appConfig = filepath.Join(e.Home, "AppData", "Roaming")
		}
	case "darwin":
		appConfig = filepath.Join(e.Home, "Library", "Application Support")
	default:
		appConfig = e.ConfigHome
		if appConfig == "" {
			appConfig = filepath.Join(e.Home, ".config")
		}
	}

	claudeCode := filepath.Join(e.Home, ".claude.json")
	if e.ClaudeConfigDir != "" {
		claudeCode = filepath.Join(e.ClaudeConfigDir, ".claude.json")
	}

	return []Place{
		{ClaudeDesktop, filepath.Join(appConfig, "Claude", "claude_desktop_config.json")},
		{ClaudeCode, claudeCode},
		{Cursor, filepath.Join(e.Home, ".cursor", "mcp.json")},
		{VSCode, filepath.Join(appConfig, "Code", "User", "mcp.json")},
	}
}

// Why a found server cannot be imported as written, as a code the page says
// in its own language (Args carry the detail).
const (
	ProblemShell     = "shell"     // it runs a shell
	ProblemVariable  = "variable"  // it needs a value only the other program fills in
	ProblemInput     = "input"     // VS Code asks the person for a value when it starts
	ProblemEnvFile   = "env-file"  // its settings come from an environment file
	ProblemFolder    = "folder"    // it must run in a particular folder
	ProblemSSE       = "sse"       // the old HTTP+SSE transport
	ProblemTransport = "transport" // a transport this program does not speak
	ProblemHeaders   = "headers"   // more than one header
	ProblemNothing   = "nothing"   // neither a program nor an address
	ProblemInvalid   = "invalid"   // the program or the address cannot be used safely
)

// Problems is every code a Found's Problem can be, sorted.
func Problems() []string {
	out := []string{ProblemShell, ProblemVariable, ProblemInput, ProblemEnvFile, ProblemFolder,
		ProblemSSE, ProblemTransport, ProblemHeaders, ProblemNothing, ProblemInvalid}

	sort.Strings(out)

	return out
}

/*
 * Found is one server in one client's settings. Server is it as an
 * integration would be (ID left empty, the brain gives one), with its
 * secrets named in Server.Secrets and their values in Secrets, never in
 * anything shown. Problem is set when it cannot be imported.
 */
type Found struct {
	Client  Client            `json:"client"`
	File    string            `json:"file"`
	Scope   string            `json:"scope,omitempty"`
	Name    string            `json:"name"`
	Server  mcp.Server        `json:"server"`
	Secrets map[string]string `json:"-"`
	Problem string            `json:"problem,omitempty"`
	Detail  string            `json:"detail,omitempty"`
	Print   string            `json:"print"`
}

// Key is what identifies an entry across looks.
func (f Found) Key() string { return Key(string(f.Client), f.File, f.Scope, f.Name) }

// Key is an entry's key from its parts.
func Key(client, file, scope, name string) string {
	return client + "\x00" + file + "\x00" + scope + "\x00" + name
}

// OriginKey is the key of the entry an integration was imported from.
func OriginKey(o *mcp.Origin) string { return Key(o.Client, o.File, o.Scope, o.Name) }

// entry is one server as the clients write it; each reads some of it.
type entry struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	EnvFile string            `json:"envFile"`
	Cwd     string            `json:"cwd"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// ErrUnreadable is a settings file that is there and is not JSON: nothing
// is concluded from it, so a half-written file removes nothing.
var ErrUnreadable = errors.New("the settings are not readable JSON")

/*
 * Read is every server in one client's settings file. Comments and
 * trailing commas are allowed (VS Code writes them). An empty file is no
 * servers.
 */
func Read(p Place, raw []byte, home string) ([]Found, error) {
	raw = bytes.TrimSpace(Strip(raw))
	if len(raw) == 0 {
		return nil, nil
	}

	var lists []struct {
		scope string
		list  map[string]json.RawMessage
	}

	switch p.Client {
	case VSCode:
		var doc struct {
			Servers map[string]json.RawMessage `json:"servers"`
		}

		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrUnreadable, p.File)
		}

		lists = append(lists, struct {
			scope string
			list  map[string]json.RawMessage
		}{"", doc.Servers})
	case ClaudeCode:
		var doc struct {
			Servers  map[string]json.RawMessage `json:"mcpServers"`
			Projects map[string]struct {
				Servers map[string]json.RawMessage `json:"mcpServers"`
			} `json:"projects"`
		}

		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrUnreadable, p.File)
		}

		lists = append(lists, struct {
			scope string
			list  map[string]json.RawMessage
		}{"", doc.Servers})

		for folder, project := range doc.Projects {
			lists = append(lists, struct {
				scope string
				list  map[string]json.RawMessage
			}{folder, project.Servers})
		}
	default:
		var doc struct {
			Servers map[string]json.RawMessage `json:"mcpServers"`
		}

		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrUnreadable, p.File)
		}

		lists = append(lists, struct {
			scope string
			list  map[string]json.RawMessage
		}{"", doc.Servers})
	}

	var out []Found

	for _, l := range lists {
		for name, rawEntry := range l.list {
			var e entry

			// One entry that is not an object is skipped, not the file.
			if err := json.Unmarshal(rawEntry, &e); err != nil {
				continue
			}

			out = append(out, convert(p, l.scope, name, e, home))
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })

	return out, nil
}

var shells = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "fish": true, "ksh": true,
	"cmd": true, "powershell": true, "pwsh": true}

// convert is one entry as an integration, or the reason it cannot be one.
func convert(p Place, scope, name string, e entry, home string) Found {
	f := Found{Client: p.Client, File: p.File, Scope: scope, Name: name, Secrets: map[string]string{}}

	s := mcp.Server{Title: name}

	kind := strings.ToLower(strings.TrimSpace(e.Type))

	switch {
	case kind == "sse":
		f.Problem = ProblemSSE
	case kind == "http" || kind == "streamable-http" || kind == "streamablehttp" || (kind == "" && e.URL != "" && e.Command == ""):
		s.Transport = "http"
		s.URL = e.URL
	case kind == "stdio" || (kind == "" && e.Command != ""):
		s.Transport = "stdio"
		s.Command = append([]string{e.Command}, e.Args...)
	case kind != "":
		f.Problem, f.Detail = ProblemTransport, kind
	default:
		f.Problem = ProblemNothing
	}

	if f.Problem == "" && s.Transport == "stdio" {
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(strings.ReplaceAll(e.Command, `\`, "/")), ".exe"))
		if shells[base] {
			f.Problem, f.Detail = ProblemShell, base
		}
	}

	if f.Problem == "" && e.EnvFile != "" {
		f.Problem = ProblemEnvFile
	}

	if f.Problem == "" && e.Cwd != "" {
		f.Problem = ProblemFolder
	}

	if f.Problem == "" && s.Transport == "http" && len(e.Headers) > 1 {
		f.Problem = ProblemHeaders
	}

	// The settings, split into what may be shown and what is a secret.
	if s.Transport == "stdio" {
		for k, v := range e.Env {
			if Secret(k, v) {
				s.Secrets = append(s.Secrets, k)
				f.Secrets[k] = v
			} else {
				if s.Env == nil {
					s.Env = map[string]string{}
				}

				s.Env[k] = v
			}
		}

		sort.Strings(s.Secrets)
	}

	if s.Transport == "http" && len(e.Headers) == 1 {
		for k, v := range e.Headers {
			s.Header = k
			name := headerSecret(k)
			s.Secrets = []string{name}
			f.Secrets[name] = v
		}
	}

	// Variables: the home folder is known; anything else is a value only
	// the other program fills in, and a guess here would run something else.
	if f.Problem == "" {
		values := append(append([]string{}, s.Command...), s.URL)
		for _, v := range s.Env {
			values = append(values, v)
		}

		for _, v := range f.Secrets {
			values = append(values, v)
		}

		for _, v := range values {
			if what, input := unresolved(expandHome(v, home)); what != "" {
				f.Problem, f.Detail = ProblemVariable, what
				if input {
					f.Problem = ProblemInput
				}

				break
			}
		}
	}

	for i := range s.Command {
		s.Command[i] = expandHome(s.Command[i], home)
	}

	for k, v := range s.Env {
		s.Env[k] = expandHome(v, home)
	}

	s.URL = expandHome(s.URL, home)

	if f.Problem == "" {
		check := s
		check.ID = "imported"

		if err := check.Check(); err != nil {
			f.Problem, f.Detail = ProblemInvalid, err.Error()
		}
	}

	f.Server = s
	f.Print = fingerprint(s)

	return f
}

// headerSecret is the secret a header's value is kept under.
func headerSecret(header string) string {
	return "HEADER_" + strings.ToUpper(notWord.ReplaceAllString(header, "_"))
}

var (
	notWord     = regexp.MustCompile(`[^A-Za-z0-9]+`)
	secretName  = regexp.MustCompile(`(?i)(key|token|secret|passw|pwd|auth|credential|cookie|bearer|(^|_)pat(_|$)|(^|_)pass(_|$))`)
	secretValue = regexp.MustCompile(`^(sk-|ghp_|gho_|ghu_|ghs_|github_pat_|xox[abposr]-|glpat-|AKIA|AIza|ya29\.|Bearer )`)
	userInURL   = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://[^/@\s]+:[^/@\s]+@`)
)

/*
 * Secret is whether a setting is a credential: by its name (…_KEY, TOKEN,
 * PASSWORD…), by a value that looks like a known kind of key, or by an
 * address with a password in it. Credentials go to the sealed secrets
 * file; everything else is shown as a setting.
 */
func Secret(name, value string) bool {
	return secretName.MatchString(name) || secretValue.MatchString(value) || userInURL.MatchString(value)
}

var variable = regexp.MustCompile(`\$\{([^}]*)\}`)

// expandHome fills in the home folder, the one variable known here for sure.
func expandHome(v, home string) string {
	if home == "" {
		return v
	}

	return strings.NewReplacer("${userHome}", home, "${env:HOME}", home, "${HOME}", home,
		"${env:USERPROFILE}", home, "${USERPROFILE}", home).Replace(v)
}

// unresolved is the first variable left in a value, and whether it is a
// question VS Code asks when the server starts.
func unresolved(v string) (string, bool) {
	m := variable.FindStringSubmatch(v)
	if m == nil {
		return "", false
	}

	return "${" + m[1] + "}", strings.HasPrefix(m[1], "input:")
}

// fingerprint is a server's definition without its secret values: a
// changed command, address, setting or secret name changes it, a new key
// does not.
func fingerprint(s mcp.Server) string {
	keys := make([]string, 0, len(s.Env))
	for k := range s.Env {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	var b strings.Builder

	b.WriteString(s.Transport + "\n" + s.URL + "\n" + s.Header + "\n")

	for _, c := range s.Command {
		b.WriteString("arg " + c + "\n")
	}

	for _, k := range keys {
		b.WriteString("env " + k + "=" + s.Env[k] + "\n")
	}

	for _, k := range s.Secrets {
		b.WriteString("secret " + k + "\n")
	}

	sum := sha256.Sum256([]byte(b.String()))

	return hex.EncodeToString(sum[:12])
}

// Strip takes JSON with comments (line and block) and trailing commas, as
// VS Code writes it, and returns plain JSON. Strings are left alone.
func Strip(raw []byte) []byte {
	out := make([]byte, 0, len(raw))

	inString, escaped := false, false

	for i := 0; i < len(raw); i++ {
		c := raw[i]

		if inString {
			out = append(out, c)

			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}

			continue
		}

		switch {
		case c == '"':
			inString = true
			out = append(out, c)
		case c == '/' && i+1 < len(raw) && raw[i+1] == '/':
			for i < len(raw) && raw[i] != '\n' {
				i++
			}

			if i < len(raw) {
				out = append(out, '\n')
			}
		case c == '/' && i+1 < len(raw) && raw[i+1] == '*':
			end := bytes.Index(raw[i+2:], []byte("*/"))
			if end < 0 {
				return out // an unclosed comment ends the document
			}

			i += end + 3
			out = append(out, ' ')
		case c == ',':
			// A comma followed only by space and comments before } or ]
			// is a trailing one.
			j := i + 1
		ahead:
			for j < len(raw) {
				switch {
				case raw[j] == ' ' || raw[j] == '\t' || raw[j] == '\n' || raw[j] == '\r':
					j++

					continue
				case raw[j] == '/' && j+1 < len(raw) && raw[j+1] == '/':
					for j < len(raw) && raw[j] != '\n' {
						j++
					}

					continue
				case raw[j] == '/' && j+1 < len(raw) && raw[j+1] == '*':
					end := bytes.Index(raw[j+2:], []byte("*/"))
					if end < 0 {
						j = len(raw)
					} else {
						j += end + 4
					}

					continue
				}

				break ahead
			}

			if j < len(raw) && (raw[j] == '}' || raw[j] == ']') {
				continue
			}

			out = append(out, c)
		default:
			out = append(out, c)
		}
	}

	return out
}

// StepKind is what following an imported entry calls for.
type StepKind string

const (
	StepGone    StepKind = "gone"    // the entry is no longer there, or no longer usable
	StepBack    StepKind = "back"    // it is there again, unchanged
	StepChanged StepKind = "changed" // it is there, defined differently
	StepSecrets StepKind = "secrets" // only a secret's value changed
)

// Step is one thing to do for one imported integration.
type Step struct {
	Kind    StepKind
	ID      string
	Next    mcp.Server        // StepChanged: the new definition, with its id and origin
	Secrets map[string]string // StepChanged, StepSecrets: the values the entry carries now
}

/*
 * Follow compares the integrations imported earlier with what the settings
 * say now. read says which files were read to the end (a file that is not
 * there counts: its entries are gone); an entry in a file that could not
 * be read is left as it is. secretsOf is the values kept for an
 * integration, compared and never returned elsewhere.
 */
func Follow(imported []mcp.Server, found []Found, read map[string]bool, secretsOf func(id string) map[string]string) []Step {
	by := make(map[string]Found, len(found))
	for _, f := range found {
		by[f.Key()] = f
	}

	var steps []Step

	for _, s := range imported {
		if s.From == nil || !read[s.From.File] {
			continue
		}

		f, there := by[OriginKey(s.From)]

		switch {
		case !there || f.Problem != "":
			if !s.Gone() {
				steps = append(steps, Step{Kind: StepGone, ID: s.ID})
			}
		case f.Print != s.From.Print:
			next := f.Server
			next.ID = s.ID
			next.From = &mcp.Origin{Client: string(f.Client), File: f.File, Scope: f.Scope, Name: f.Name, Print: f.Print}

			steps = append(steps, Step{Kind: StepChanged, ID: s.ID, Next: next, Secrets: f.Secrets})
		default:
			if s.Gone() {
				steps = append(steps, Step{Kind: StepBack, ID: s.ID})
			}

			if !sameSecrets(secretsOf(s.ID), f.Secrets) {
				steps = append(steps, Step{Kind: StepSecrets, ID: s.ID, Secrets: f.Secrets})
			}
		}
	}

	return steps
}

func sameSecrets(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}

	for k, v := range a {
		if b[k] != v {
			return false
		}
	}

	return true
}

/*
 * Imported is the integration a found entry would become, with the id
 * given: its origin set from the entry. Refused when it cannot be
 * imported as written.
 */
func Imported(f Found, id string) (mcp.Server, error) {
	if f.Problem != "" {
		return mcp.Server{}, fmt.Errorf("%s cannot be used as %s writes it (%s)", f.Name, f.Client.Title(), f.Problem)
	}

	s := f.Server
	s.ID = id
	s.From = &mcp.Origin{Client: string(f.Client), File: f.File, Scope: f.Scope, Name: f.Name, Print: f.Print}

	return s, s.Check()
}

// FreeID is an id for a found name that taken does not hold yet.
func FreeID(name string, taken func(id string) bool) string {
	base := mcp.IDFor(name)

	if !taken(base) {
		return base
	}

	for n := 2; ; n++ {
		suffix := fmt.Sprint(n)

		id := base
		if len(id)+len(suffix) > 21 {
			id = id[:21-len(suffix)]
		}

		if !taken(id + suffix) {
			return id + suffix
		}
	}
}
