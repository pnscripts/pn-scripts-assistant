package protect

import (
	"encoding/json"
	"fmt"
	"strings"
)

/*
 * Finding the protected thing in a tool call.
 *
 * Every tool names its argument differently — path, file, directory, folder —
 * and a list of which tool uses which name would be a list to keep up to date
 * and to get wrong. So the arguments are searched rather than interpreted:
 * every string in them is checked, whatever it is called.
 *
 * The bias is deliberate. A string that merely looks like a protected path
 * produces a question, and a question is answered in two seconds and then
 * remembered; the other kind of mistake is a credentials file read silently.
 */

// InArguments finds the first protected path a tool call names.
func InArguments(raw json.RawMessage) (path string, rule Rule, ask bool) {
	var anything any

	if err := json.Unmarshal(raw, &anything); err != nil {
		return "", Rule{}, false
	}

	return walk(anything)
}

func walk(value any) (string, Rule, bool) {
	switch v := value.(type) {
	case string:
		if r, ask := Ask(v); ask {
			return v, r, true
		}

	case []any:
		for _, item := range v {
			if path, r, ask := walk(item); ask {
				return path, r, true
			}
		}

	case map[string]any:
		/*
		 * In a settled order, so the same call asks the same question twice.
		 *
		 * Go randomises map iteration, so a call naming two protected files
		 * would name one of them in the prompt on Tuesday and the other on
		 * Wednesday — and a prompt that says something different each time
		 * about the same request is one nobody can learn to trust.
		 */
		for _, key := range sortedKeys(v) {
			if path, r, ask := walk(v[key]); ask {
				return path, r, true
			}
		}
	}

	return "", Rule{}, false
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))

	for k := range m {
		out = append(out, k)
	}

	// Small maps; a plain insertion sort keeps this file free of imports it
	// does not otherwise need.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}

	return out
}

/*
 * Explain is the question, written for the person who has to answer it.
 *
 * Three things, because with fewer than three there is no way to decide: what
 * the brain wants to do, which file, and why that file is one it stops at. A
 * prompt that says only "allow access?" teaches people to say yes.
 */
func Explain(what, path string, r Rule) string {
	where := path

	if home := strings.TrimSpace(r.What); home == "" {
		return fmt.Sprintf("%s — %s", what, where)
	}

	return fmt.Sprintf("%s — %s (%s). %s", what, where, strings.ToLower(r.What), r.Why)
}
