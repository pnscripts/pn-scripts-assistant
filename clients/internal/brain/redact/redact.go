/*
 * Package redact takes secrets out of text that is about to be kept.
 *
 * Evidence is exact on purpose — the command as it ran, the end of what it
 * printed — and exact is how a key typed into a command line, or printed by a
 * command, ends up in the brain's database, a task's account and the next
 * model call. So everything recorded passes through here first: the values
 * this program knows are secret (its keys, the integrations' credentials) are
 * replaced wherever they appear, and so is anything shaped like a token.
 */
package redact

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Mark is what a secret is replaced with.
const Mark = "[a secret]"

var (
	mu    sync.RWMutex
	known = map[string]bool{}
)

// Add is secret values to take out wherever they appear. Very short values
// are ignored: replacing every "abc" would make text unreadable and protect
// nothing.
func Add(values ...string) {
	mu.Lock()
	defer mu.Unlock()

	for _, v := range values {
		if v = strings.TrimSpace(v); len(v) >= 8 {
			known[v] = true
		}
	}
}

// shapes are tokens by their published forms, and credentials by the words
// people put before them.
var shapes = []*regexp.Regexp{
	regexp.MustCompile(`sk-(?:ant-|proj-)?[A-Za-z0-9_\-]{16,}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`xox[abprs]-[A-Za-z0-9\-]{10,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`AIza[0-9A-Za-z_\-]{30,}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`),
	regexp.MustCompile(`(?i)\b(bearer)\s+[A-Za-z0-9_\-\.~+/=]{12,}`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|access[_-]?token|auth[_-]?token|secret|password|passwd|token)["']?\s*[:=]\s*["']?)[^\s"',;]{8,}`),
	regexp.MustCompile(`(?i)(-{1,2}(?:access-?token|api-?key|token|password|secret)[\s=]+)[^\s"']{8,}`),
}

// Text is text with every secret taken out.
func Text(text string) string {
	if text == "" {
		return text
	}

	mu.RLock()

	values := make([]string, 0, len(known))
	for v := range known {
		values = append(values, v)
	}

	mu.RUnlock()

	// Longest first, so a secret containing another is taken out whole.
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })

	for _, v := range values {
		text = strings.ReplaceAll(text, v, Mark)
	}

	for _, shape := range shapes {
		if shape.NumSubexp() > 0 {
			text = shape.ReplaceAllString(text, "${1}"+Mark)

			continue
		}

		text = shape.ReplaceAllString(text, Mark)
	}

	return text
}
