package server

import (
	"path/filepath"
	"regexp"
	"testing"

	"pn-scripts-assistant/internal/brain/protect"
	"pn-scripts-assistant/internal/brain/tunnel"
)

/*
 * What the server names for Reaching it and Privacy, the page says in both
 * languages.
 *
 * The tunnel says what is still missing as a name (tunnel.Missing*) and the
 * Privacy page names each built-in rule by its id. A rule or a missing step
 * added later without its words would fall back to English on a Bulgarian
 * page; this says so first.
 */
func TestReachAndPrivacyNamesAreSaidInBothLanguages(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "i18n.js"))
	cyrillic := regexp.MustCompile(`[а-яА-Я]`)

	check := func(key string) {
		t.Helper()

		if en, bg := strings2(t, js, key); en == "" || !cyrillic.MatchString(bg) {
			t.Errorf("%s is not said in both languages: %q / %q", key, en, bg)
		}
	}

	for _, missing := range []string{tunnel.MissingSetup, tunnel.MissingEndpoint, tunnel.MissingDevices} {
		check("reach.missing." + missing)
	}

	for _, rule := range protect.BuiltIn {
		check("prv.rule." + rule.ID)
		check("prv.rule." + rule.ID + ".why")
	}
}

// Every state the tunnel can be in before it is ready has a name, and the
// English it has always said is unchanged.
func TestTheTunnelNamesWhatIsMissing(t *testing.T) {
	var tun tunnel.Tunnel

	if got := tun.MissingWhat(); got != tunnel.MissingSetup {
		t.Errorf("a tunnel never set up is missing %q, want %q", got, tunnel.MissingSetup)
	}

	if got := tun.Ready(); got != "the tunnel has not been set up yet" {
		t.Errorf("Ready says %q", got)
	}
}
