package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Files the brain must never read, whatever it is asked.
//
// The reason is a specific attack, not general caution. Reading a file and
// fetching a URL are both classed Safe and so run without asking — reasonably,
// since neither changes anything. Together they are an exfiltration route: a
// web page can contain text instructing the model to read a credentials file
// and then fetch a URL with the contents attached, and because neither step
// needs approval, no person sees it happen.
//
// FetchURL refuses private addresses, which stops the brain being pointed at
// the local network. This is the other half: stopping secrets from leaving in
// the first place.
//
// Enforced in code rather than in the prompt, deliberately. A system prompt is
// a request. This is a rule — the model cannot be talked out of it, and neither
// can a page it reads.

// sensitiveNames are matched against the file name alone.
var sensitiveNames = map[string]bool{
	".env": true, ".env.local": true, ".env.production": true, ".env.backup": true,
	".npmrc": true, ".netrc": true, ".git-credentials": true, ".pgpass": true, ".my.cnf": true,
	"id_rsa": true, "id_ed25519": true, "id_ecdsa": true, "id_dsa": true,
	"credentials": true, "auth.json": true, "shadow": true, "passwd-": true,
}

// sensitiveDirectories are matched against any part of the path.
var sensitiveDirectories = []string{
	"/.ssh/", "/.gnupg/", "/.aws/", "/.azure/", "/.kube/",
	"/.docker/", "/.config/gcloud/", "/.password-store/",
	"/etc/shadow", "/.mozilla/", "/.thunderbird/",
}

// sensitiveExtensions are matched against the extension.
var sensitiveExtensions = map[string]bool{
	"pem": true, "key": true, "p12": true, "pfx": true, "keystore": true, "jks": true,
}

// IsSensitive reports whether a path looks like it holds credentials.
func IsSensitive(path string) bool {
	normalised := strings.ReplaceAll(path, `\`, "/")
	name := strings.ToLower(filepath.Base(normalised))

	if sensitiveNames[name] {
		return true
	}

	// Catches .env.whatever without listing every variant.
	if strings.HasPrefix(name, ".env") {
		return true
	}

	if ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(normalised), ".")); sensitiveExtensions[ext] {
		return true
	}

	for _, fragment := range sensitiveDirectories {
		if strings.Contains(normalised, fragment) {
			return true
		}
	}

	return false
}

// GuardSensitive refuses a path that holds credentials.
//
// The message says what was refused and why, so a legitimate request produces
// an explanation rather than a puzzle.
func GuardSensitive(path string) error {
	if !IsSensitive(path) {
		return nil
	}

	return fmt.Errorf(
		"refusing to touch %s: it looks like it holds credentials; "+
			"secrets are off limits to the assistant even when asked directly", path)
}
