package tools

/*
 * Tools that are not everybody's.
 *
 * Everything built in is offered to whoever may use it, narrowed by their
 * tool list. A tool from an integration is the opposite way round: it is
 * offered to nobody until its owner grants it, agent by agent, and on a
 * project only if the project allows that integration. An agent with no limit
 * at all — the generalist — still does not get it by default: "no limit"
 * means every tool this program ships, not every tool somebody's server
 * happens to describe.
 */

// Granted is a tool offered only to those it has been granted to.
type Granted interface {
	// GrantedTo reports whether an agent may use it. The owner's own
	// assistant is asked about as "assistant".
	GrantedTo(agent string) bool
}

// FromServer is a tool that belongs to an integration.
type FromServer interface {
	Server() string
}

/*
 * MayUse is whether an agent may use a tool at all, and why not.
 *
 * servers is the integrations the work in hand allows — a project's — and
 * nil when the work is not confined to one.
 */
func MayUse(t Tool, agent string, servers *[]string) (bool, string) {
	if agent == "" {
		agent = "assistant"
	}

	if g, ok := t.(Granted); ok && !g.GrantedTo(agent) {
		return false, "that integration has not been granted to " + agent
	}

	if f, ok := t.(FromServer); ok && servers != nil && !Listed(*servers, f.Server()) {
		return false, "this project does not allow the integration " + f.Server()
	}

	return true, ""
}
