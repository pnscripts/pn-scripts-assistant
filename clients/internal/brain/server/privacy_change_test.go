package server

import (
	"strings"
	"testing"

	"pn-brain/internal/brain/llm"
)

/*
 * Changing privacy has to take effect, not merely be recorded.
 *
 * It was read once when the brain was built, so the panel wrote the file, said
 * "Saved", and the brain went on behaving and reporting as it had before. The
 * file on this machine said research while the program said private, which is
 * the worst possible version of that bug: somebody changes their privacy,
 * sees no change, and concludes the program is lying to them about it.
 */
func TestChangingPrivacyTakesEffectAtOnce(t *testing.T) {
	_, _, b := newServer(t)

	if b.Mode != llm.ModePrivate {
		t.Fatalf("a fresh brain should be private, is %q", b.Mode)
	}

	got, err := b.UsePrivacy("research")
	if err != nil {
		t.Fatal(err)
	}

	if got != llm.ModeResearch || b.Mode != llm.ModeResearch {
		t.Fatalf("the brain still reports %q", b.Mode)
	}

	// The router is what actually enforces it, and it is a separate copy.
	if b.Router.Mode() != llm.ModeResearch {
		t.Fatalf("the router still enforces %q", b.Router.Mode())
	}

	if b.Cfg.Privacy != "research" {
		t.Fatalf("the setting was not recorded: %q", b.Cfg.Privacy)
	}
}

/*
 * Tightening is the direction that has to be immediate.
 *
 * Anything still running under the looser rule after somebody has asked for a
 * stricter one is a disclosure they have just said they did not want.
 */
func TestTighteningPrivacyStopsTheWebAtOnce(t *testing.T) {
	_, _, b := newServer(t)

	b.UsePrivacy("research")

	if !b.Mode.AllowsWeb() {
		t.Fatal("research should allow the web")
	}

	b.UsePrivacy("private")

	if b.Mode.AllowsWeb() || b.Router.Mode().AllowsWeb() {
		t.Fatal("the web is still allowed after going back to private")
	}
}

/*
 * A value nobody recognises is refused out loud.
 *
 * Configuration reads a typo as the strictest setting, which is right for a
 * file. Here it would mean somebody asking for "reserch" and silently being
 * given privacy they did not ask for — the right answer, arrived at in a way
 * that tells them nothing.
 */
func TestAnUnknownPrivacySettingIsRefused(t *testing.T) {
	_, _, b := newServer(t)

	_, err := b.UsePrivacy("reserch")

	if err == nil {
		t.Fatal("a misspelt setting was accepted")
	}

	if !strings.Contains(err.Error(), "reserch") {
		t.Fatalf("the refusal does not say what was asked for: %v", err)
	}

	if b.Mode != llm.ModePrivate {
		t.Fatalf("a refused change still moved the setting to %q", b.Mode)
	}
}
