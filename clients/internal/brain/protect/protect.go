/*
 * Package protect is what the brain stops and asks about before it reads.
 *
 * It used to be a refusal list, and that was the wrong shape. This is the
 * owner's own machine and his own files; a program that answers "no" to its
 * owner has decided something that was not its to decide. So nothing here
 * forbids anything. Everything here asks.
 *
 * What it is defending against is narrow and real. Reading a file and fetching
 * a web page are both harmless alone and both run without asking, and together
 * they are a way for a secret to leave: a page the brain reads can carry text
 * telling it to open a credentials file and fetch a URL with the contents
 * attached. The whole attack depends on nobody seeing it happen — which is
 * exactly what an approval prompt breaks. Asking is therefore not a weaker
 * version of refusing; it is the thing that actually addresses the problem,
 * and it leaves every capability intact.
 *
 * Two rules hold it up.
 *
 * The model cannot change this list. There is no tool for it and no route from
 * a conversation to it, because a page that can talk the brain into
 * unprotecting the keys is a page that can read them. Every change comes from
 * a person, in setup or in the privacy panel.
 *
 * And it learns. Saying yes to a file teaches it that this file is fine, and
 * it stops asking — the answer is kept where the person can see it and take it
 * back. Being asked twice about the same thing is how a prompt stops being
 * read and starts being clicked through, which is the failure mode that makes
 * the whole mechanism worthless.
 */
package protect

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// FileName is where the choices are kept, in the brain's own folder so they
// travel with the brain rather than with the machine.
const FileName = "protected.json"

/*
 * Rule is one thing kept out of reach, and why.
 *
 * The reason is stored beside the rule rather than written into the interface,
 * because this is a list somebody is being asked to make decisions about and
 * "/.aws/" on its own is not a decision anybody can make.
 */
type Rule struct {
	ID   string `json:"id"`
	What string `json:"what"`
	Why  string `json:"why"`

	// Paths are matched anywhere in a path, Names against the file name, and
	// Extensions against the extension.
	Paths      []string `json:"paths,omitempty"`
	Names      []string `json:"names,omitempty"`
	Extensions []string `json:"extensions,omitempty"`

	/*
	 * Fixed marks a rule that cannot be switched off.
	 *
	 * Kept to the one case where switching it off is not a preference but a
	 * mistake with no upside: the system's own password file, which the brain
	 * has no use for and which nothing legitimate would ask it to read.
	 */
	Fixed bool `json:"fixed,omitempty"`
}

// BuiltIn is the list the brain ships with. Somebody can switch any of them
// off except the fixed one, and add their own.
var BuiltIn = []Rule{
	{
		ID: "ssh", What: "SSH keys",
		Why:   "Private keys for every machine you log into. A copy of one is a copy of your access to it.",
		Paths: []string{"/.ssh/"},
		Names: []string{"id_rsa", "id_ed25519", "id_ecdsa", "id_dsa"},
	},
	{
		ID: "env", What: "Environment files",
		Why:   "A .env holds the passwords and API keys of whatever project it belongs to, in plain text.",
		Names: []string{".env"},
	},
	{
		ID: "cloud", What: "Cloud accounts",
		Why:   "Credentials for Amazon, Azure, Google and Kubernetes — enough to spend money in your name.",
		Paths: []string{"/.aws/", "/.azure/", "/.kube/", "/.config/gcloud/"},
	},
	{
		ID: "registries", What: "Registry and package logins",
		Why:   "Docker, npm and git store their logins unencrypted. Deleting the tool does not make the folder safe — it makes it empty, and empty is temporary.",
		Paths: []string{"/.docker/"},
		Names: []string{".npmrc", ".netrc", ".git-credentials", "auth.json", "credentials"},
	},
	{
		ID: "keys", What: "Certificates and key files",
		Why:        "Anything ending .pem, .key, .p12 or .pfx is a key by definition.",
		Extensions: []string{"pem", "key", "p12", "pfx", "keystore", "jks"},
	},
	{
		ID: "passwords", What: "Password managers and keyrings",
		Why:   "GnuPG's keyring and pass's store are the master copies, not a cache of them.",
		Paths: []string{"/.gnupg/", "/.password-store/"},
	},
	{
		ID: "databases", What: "Database passwords",
		Why:   "The files Postgres and MySQL clients keep passwords in so you do not have to type them.",
		Names: []string{".pgpass", ".my.cnf"},
	},
	{
		ID: "browsers", What: "Browser profiles",
		Why:   "Saved passwords, cookies and session tokens for every site you are signed into.",
		Paths: []string{"/.mozilla/", "/.thunderbird/"},
	},
	{
		ID: "brain", What: "The brain's own folder",
		Why: "Its settings file holds the mail password and every API key it has been given, " +
			"in plain text — and its memory is everything it has ever been told. The brain " +
			"reads its own memory through its own tools, which is a different thing from " +
			"opening the file and putting the contents in a conversation.",
		Names: []string{"brain.conf", "brain.sqlite"},
	},
	{
		ID: "system", What: "The system's own password file",
		Why:   "/etc/shadow holds every account's password hash. Nothing legitimate asks the assistant for it.",
		Paths: []string{"/etc/shadow"}, Names: []string{"shadow", "passwd-"},
		Fixed: true,
	},
}

// Choices is what a person decided, and what the brain has learned from them.
type Choices struct {
	// Off names built-in rules deliberately switched off — things it should
	// not bother asking about. Stored as what was turned off rather than as
	// what was left on, so a rule added in a later version arrives switched
	// on, which is the safe direction.
	Off []string `json:"off,omitempty"`

	// Extra are paths or names somebody added themselves.
	Extra []string `json:"extra,omitempty"`

	/*
	 * Allowed are the particular files it has been told are fine.
	 *
	 * This is the learning half, and it is deliberately about one file rather
	 * than a whole rule: saying yes to reading one .env in one project should
	 * not quietly open every .env on the machine. Kept where a person can see
	 * the list and take any of it back.
	 */
	Allowed []string `json:"allowed,omitempty"`
}

// Load reads the choices, or the defaults on a brain that has never been asked.
func Load(root string) Choices {
	raw, err := os.ReadFile(filepath.Join(root, FileName))

	if errors.Is(err, os.ErrNotExist) || err != nil {
		return Choices{}
	}

	var c Choices

	if err := json.Unmarshal(raw, &c); err != nil {
		// A file that cannot be read is treated as no choices at all, which
		// means everything is protected. The failure has to fall that way.
		return Choices{}
	}

	return c
}

// Save writes the choices and applies them.
func Save(root string, c Choices) error {
	c = tidy(c)

	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(root, FileName), raw, 0o644); err != nil {
		return err
	}

	Use(c)

	return nil
}

// tidy drops blanks, duplicates and anything naming a rule that does not
// exist, so a hand-edited file cannot leave the list in a state the interface
// cannot describe.
func tidy(c Choices) Choices {
	known := map[string]bool{}

	for _, r := range BuiltIn {
		if !r.Fixed {
			known[r.ID] = true
		}
	}

	c.Off = keep(c.Off, func(s string) bool { return known[s] })
	c.Extra = keep(c.Extra, func(s string) bool { return s != "" })
	c.Allowed = keep(c.Allowed, func(s string) bool { return s != "" })

	return c
}

func keep(in []string, wanted func(string) bool) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))

	for _, s := range in {
		s = strings.TrimSpace(s)

		if s == "" || seen[s] || !wanted(s) {
			continue
		}

		seen[s] = true

		out = append(out, s)
	}

	sort.Strings(out)

	return out
}

/*
 * own is the brain's own folder, which is not known until it is opened.
 *
 * Named at run time rather than listed above because the folder is wherever
 * its owner put it — a drive, a home directory, somewhere else entirely — and
 * a rule that only protects a folder called PN-BRAIN-DATA protects nobody who
 * chose a different name.
 */
var own struct {
	mu   sync.RWMutex
	root string
}

// OwnFolder tells the list where the brain keeps itself.
func OwnFolder(root string) {
	own.mu.Lock()
	own.root = strings.ToLower(strings.TrimSpace(root))
	own.mu.Unlock()
}

func insideOwnFolder(path string) bool {
	own.mu.RLock()
	root := own.root
	own.mu.RUnlock()

	return root != "" && strings.HasPrefix(path, root)
}

// chosen is what is in force, which every read goes through.
var chosen struct {
	mu sync.RWMutex
	c  Choices
}

// Use applies choices for the rest of this run.
func Use(c Choices) {
	chosen.mu.Lock()
	chosen.c = tidy(c)
	chosen.mu.Unlock()
}

// InForce is what is currently applied.
func InForce() Choices {
	chosen.mu.RLock()
	defer chosen.mu.RUnlock()

	return chosen.c
}

// On reports whether a built-in rule is in force.
func On(id string) bool {
	for _, off := range InForce().Off {
		if off == id {
			return false
		}
	}

	return true
}

/*
 * Ask reports whether this path is one to stop and ask about, and which rule
 * says so.
 *
 * Case-insensitive and backslash-tolerant, because the same folder is written
 * three ways depending on where the request came from, and a question that can
 * be stepped around by typing a capital letter is not being asked.
 */
func Ask(path string) (Rule, bool) {
	normalised := strings.ToLower(strings.ReplaceAll(path, `\`, "/"))
	name := filepath.Base(normalised)
	ext := strings.TrimPrefix(filepath.Ext(normalised), ".")

	// Already answered for this file. The point of remembering is not being
	// asked again.
	for _, allowed := range InForce().Allowed {
		if strings.EqualFold(strings.TrimSpace(allowed), strings.TrimSpace(path)) {
			return Rule{}, false
		}
	}

	for _, r := range BuiltIn {
		if !r.Fixed && !On(r.ID) {
			continue
		}

		if matches(r, normalised, name, ext) {
			return r, true
		}

		// The brain's own folder, wherever its owner put it.
		if r.ID == "brain" && insideOwnFolder(normalised) {
			return r, true
		}
	}

	for _, extra := range InForce().Extra {
		if own := strings.ToLower(strings.TrimSpace(extra)); own != "" &&
			(strings.Contains(normalised, own) || name == own) {
			return Rule{
				ID:   "yours",
				What: "Something you added",
				Why:  "You asked to be told before anything here is opened: " + extra,
			}, true
		}
	}

	return Rule{}, false
}

// IsSensitive is Ask without the reason, for callers that only need to know.
func IsSensitive(path string) bool {
	_, ask := Ask(path)

	return ask
}

/*
 * Learn records that a particular file was allowed, so it is not asked about
 * again.
 *
 * The whole of the point: a question answered twice is a question that stops
 * being read. Best effort on writing — failing to remember costs one more
 * prompt, and refusing the approval over it would cost the thing that was
 * approved.
 */
func Learn(root, path string) {
	path = strings.TrimSpace(path)

	if path == "" {
		return
	}

	c := InForce()

	for _, allowed := range c.Allowed {
		if strings.EqualFold(allowed, path) {
			return
		}
	}

	c.Allowed = append(c.Allowed, path)

	Save(root, c)
}

// Forget takes back something it learned, which is what makes remembering safe
// to do automatically.
func Forget(root, path string) error {
	c := InForce()
	kept := c.Allowed[:0]

	for _, allowed := range c.Allowed {
		if !strings.EqualFold(allowed, path) {
			kept = append(kept, allowed)
		}
	}

	c.Allowed = kept

	return Save(root, c)
}

func matches(r Rule, path, name, ext string) bool {
	for _, fragment := range r.Paths {
		if strings.Contains(path, strings.ToLower(fragment)) {
			return true
		}
	}

	for _, n := range r.Names {
		n = strings.ToLower(n)

		if name == n {
			return true
		}

		// .env.local, .env.production, .env.whatever — one rule rather than a
		// list that is always one variant out of date.
		if n == ".env" && strings.HasPrefix(name, ".env") {
			return true
		}
	}

	for _, e := range r.Extensions {
		if ext == strings.ToLower(e) {
			return true
		}
	}

	return false
}
