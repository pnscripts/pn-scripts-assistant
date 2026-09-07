package tools

import (
	"fmt"
	"strings"

	"pn-scripts-assistant/internal/brain/protect"
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

/*
 * The list itself now lives in the protect package, with the reasons beside it
 * and a person's choices on top.
 *
 * It moved because it stopped being a constant. Somebody setting the brain up
 * is shown what will be off limits and can add their own; the privacy panel
 * lets them change it later. What has not changed is who can: there is no tool
 * for it and no path from a conversation to it, because a page that can talk
 * the model into unprotecting the keys is a page that can read them.
 */

// IsSensitive reports whether a path looks like it holds credentials.
func IsSensitive(path string) bool { return protect.IsSensitive(path) }

/*
 * GuardSensitive is the last line, not the first.
 *
 * The question is asked before the tool runs — the agent holds the call and
 * puts it to the owner — so by the time a tool executes, a protected path has
 * already been approved and recorded as allowed. This exists for the case
 * where that did not happen: a tool called from somewhere that does not go
 * through the agent.
 *
 * It refuses there rather than asking, because a caller outside the agent has
 * nobody to ask. In the ordinary path it never fires, and that is the point of
 * it: the thing that protects the credentials is the question, and this is
 * what makes the question impossible to bypass.
 */
func GuardSensitive(path string) error {
	rule, ask := protect.Ask(path)

	if !ask {
		return nil
	}

	return fmt.Errorf(
		"%s is protected (%s) and this was not asked about first — "+
			"open it from a conversation, where the request comes to you for a decision",
		path, strings.ToLower(rule.What))
}
