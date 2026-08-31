package preflight

import (
	"testing"
	"time"
)

/*
 * Versions compare as numbers, which is the whole reason this exists.
 *
 * The case that prompted it: 0.33.2 is newer than 0.32.0, and comparing those
 * as text says the opposite — the strings differ first at the second
 * character, where "2" sorts below "3". A wrong answer there would either hide
 * an update that exists or claim one that does not, and the second is worse:
 * somebody re-downloads Ollama and finds nothing changed.
 */
func TestANewerVersionIsRecognised(t *testing.T) {
	for _, c := range []struct {
		candidate, installed string
		newer                bool
	}{
		// The real one, from this machine.
		{"v0.33.2", "ollama version is 0.32.0", true},

		// Text comparison gets every one of these backwards.
		{"v0.10.0", "v0.9.0", true},
		{"v1.2.10", "v1.2.9", true},
		{"v0.9.0", "v0.10.0", false},

		{"v0.32.0", "0.32.0", false},
		{"v0.32.1", "0.32.0", true},
		{"v1.0.0", "0.99.99", true},

		// Fewer parts on one side is not a difference until a number differs.
		{"v1.2", "1.2.0", false},
		{"v1.2.1", "1.2", true},

		// Nothing parseable means no claim either way.
		{"", "0.32.0", false},
		{"nightly", "0.32.0", false},
		{"v0.33.2", "", false},
	} {
		if got := IsNewer(c.candidate, c.installed); got != c.newer {
			t.Errorf("IsNewer(%q, %q) = %v, want %v",
				c.candidate, c.installed, got, c.newer)
		}
	}
}

/*
 * The tag is read out of the redirect GitHub answers with.
 *
 * Asked this way rather than through api.github.com, which allows sixty
 * unauthenticated calls an hour and then refuses — so the check would work on
 * a quiet machine and fail on a busy one for reasons nobody could see.
 */
func TestTheTagIsReadFromTheRedirect(t *testing.T) {
	for _, c := range []struct{ location, want string }{
		{"https://github.com/ollama/ollama/releases/tag/v0.33.2", "v0.33.2"},
		{"https://github.com/ggml-org/whisper.cpp/releases/tag/v1.7.4", "v1.7.4"},

		// Shapes that must not produce a confident wrong answer.
		{"https://github.com/ollama/ollama/releases", ""},
		{"", ""},
		{"https://example.com/nothing/here", ""},
	} {
		if got := tagFromURL(c.location); got != c.want {
			t.Errorf("tagFromURL(%q) = %q, want %q", c.location, got, c.want)
		}
	}
}

/*
 * The answer is remembered, including when there was not one.
 *
 * The setup page asks for its state every two seconds. Without a cache the
 * check would put a request to GitHub on the wire that often — each with a six
 * second timeout that could stack behind it — to answer a question whose
 * answer changes a few times a year.
 *
 * Failures are cached as deliberately as successes: a machine with no internet
 * would otherwise retry every two seconds for as long as the page was open,
 * each attempt waiting the full timeout, which is how a check nobody asked for
 * turns into a page that feels broken.
 */
func TestAVersionIsAskedForOnceAndRemembered(t *testing.T) {
	const url = "https://example.invalid/releases/latest"

	versions.mu.Lock()
	delete(versions.seen, url)
	versions.mu.Unlock()

	if _, known := rememberedTag(url); known {
		t.Fatal("an answer was known before anything asked")
	}

	// A failure — no such host — must still be remembered.
	rememberTag(url, "")

	tag, known := rememberedTag(url)
	if !known {
		t.Error("a failed check was not remembered, so it will be retried " +
			"every two seconds for as long as the page is open")
	}

	if tag != "" {
		t.Errorf("a failed check was remembered as %q", tag)
	}

	// And a success comes back as itself.
	rememberTag(url, "v0.33.2")

	if tag, _ := rememberedTag(url); tag != "v0.33.2" {
		t.Errorf("remembered %q, want v0.33.2", tag)
	}

	// Stale answers are asked again rather than believed for ever.
	versions.mu.Lock()
	versions.seen[url] = versionAnswer{tag: "v0.1.0", at: time.Now().Add(-2 * RememberAVersionFor)}
	versions.mu.Unlock()

	if _, known := rememberedTag(url); known {
		t.Error("an answer older than the cache lifetime was still believed")
	}
}
