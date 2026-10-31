package window

import (
	"net/url"
	"strings"
)

/*
 * sameOrigin says whether two addresses are the same scheme, host and port.
 *
 * The window shows the assistant's own pages and nothing else: it has no
 * address bar, so a page that moved it to another site — a link without a
 * handler, a dropped file, "location = …" written by something it displayed —
 * would look exactly like the assistant. Navigation to anything but the
 * address the program itself opened is cancelled (Windows: NavigationStarting;
 * found by the 1.0 security review), and web addresses go to the browser.
 */
func sameOrigin(a, b string) bool {
	ua, err := url.Parse(a)
	if err != nil || ua.Host == "" {
		return false
	}

	ub, err := url.Parse(b)
	if err != nil || ub.Host == "" {
		return false
	}

	return strings.EqualFold(ua.Scheme, ub.Scheme) && strings.EqualFold(ua.Host, ub.Host)
}
