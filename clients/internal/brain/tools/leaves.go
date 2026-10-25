package tools

import (
	"encoding/json"
	"net/url"
	"strings"
)

/*
 * Where a call sends something, said for the record and for the Control page.
 *
 * Privacy decides whether anything may leave at all; this answers the next
 * question, which is the one a person actually asks when they look at what
 * the assistant did: "did that go anywhere, and where?". Read from the call's
 * own arguments — the address it opened, the person a message was sent to —
 * never from what the model said it did.
 */

// Reaching is a tool that can say where one call of it sends something.
type Reaching interface {
	Reaches(args json.RawMessage) string
}

// Reaches is where a call sends something off this machine, or empty when
// it stays here.
func Reaches(t Tool, args json.RawMessage) string {
	if t == nil {
		return ""
	}

	if r, ok := t.(Reaching); ok {
		return r.Reaches(args)
	}

	var a struct {
		URL   string `json:"url"`
		To    string `json:"to"`
		Query string `json:"query"`
		Name  string `json:"name"`
	}

	_ = json.Unmarshal(args, &a)

	switch t.Name() {
	case "fetch_url", "read_a_page":
		return hostOf(a.URL)

	case "drive_browser":
		// Driving a browser that is already on a page sends nothing new;
		// opening an address does.
		return hostOf(a.URL)

	case "send_email":
		if domain := domainOf(a.To); domain != "" {
			return "mail to " + domain
		}

		return "mail"

	case "web_search", "check_name", "closest_titles":
		return "web search"

	case "read_email", "read_mailbox", "mail_digest":
		return "your mail server"

	case "send_draft":
		return "mail"
	}

	if leaves, ok := t.(interface{ LeavesTheMachine() bool }); ok && leaves.LeavesTheMachine() {
		return "an integration off this machine"
	}

	return ""
}

// Leaves is whether a tool can send anything off this machine at all, for a
// list of capabilities where there is no call yet to read.
func Leaves(t Tool) (bool, string) {
	switch t.Name() {
	case "fetch_url", "read_a_page":
		return true, "the address it is asked to read"
	case "web_search", "check_name", "closest_titles":
		return true, "the search words, to a search engine"
	case "send_email", "send_draft":
		return true, "the message, to the people it is addressed to"
	case "drive_browser":
		return true, "what the page you open is sent"
	case "read_email", "read_mailbox", "mail_digest":
		return true, "your login, to your own mail server"
	case "install_a_model", "install_a_part", "install_requirement", "check_for_updates":
		return true, "a request for the download — nothing about you"
	}

	if leaves, ok := t.(interface{ LeavesTheMachine() bool }); ok && leaves.LeavesTheMachine() {
		return true, "what the integration is given"
	}

	return false, ""
}

func hostOf(address string) string {
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || u.Host == "" {
		return ""
	}

	return strings.TrimPrefix(u.Hostname(), "www.")
}

func domainOf(to string) string {
	first := strings.TrimSpace(strings.Split(to, ",")[0])

	if i := strings.LastIndex(first, "@"); i >= 0 {
		return strings.Trim(first[i+1:], "> ")
	}

	return ""
}
