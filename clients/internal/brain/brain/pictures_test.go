package brain

import (
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/pictures"
)

/*
 * A description of a picture leaves this machine only when privacy is open.
 *
 * Worth being strict about: what somebody wants a picture of is often the most
 * revealing sentence they will write all week — more revealing than most of
 * what they type into a chat — and there is no version of "just this once"
 * that a program should decide on its own.
 */
func TestAPictureIsMadeHereUnlessPrivacyIsOpen(t *testing.T) {
	b := testBrain(t)
	b.Cfg.ProviderKeys = map[string]string{"openai": "sk-test"}

	studio := studioOf{b}

	for _, mode := range []llm.Mode{llm.ModePrivate, llm.ModeResearch} {
		b.Mode = mode

		painter, err := studio.Painter()
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}

		if _, local := painter.(pictures.Local); !local {
			t.Errorf("in %s mode a picture would be made by %s", mode, painter.Name())
		}
	}
}

/*
 * Opened up, a paid service is used only when nothing here can do it.
 *
 * An image server that is installed but not started is the ordinary case, and
 * quietly billing somebody elsewhere because their own was not running is the
 * wrong kind of surprise.
 */
func TestAPaidServiceIsTheFallbackNotTheDefault(t *testing.T) {
	b := testBrain(t)
	b.Mode = llm.ModeOpen
	b.Cfg.PicturesURL = "http://127.0.0.1:1" // nothing is listening there
	b.Cfg.ProviderKeys = map[string]string{"openai": "sk-test"}

	studio := studioOf{b}

	painter, err := studio.Painter()
	if err != nil {
		t.Fatal(err)
	}

	paid, isPaid := painter.(pictures.Paid)

	if !isPaid {
		t.Fatalf("with nothing running here it chose %s", painter.Name())
	}

	if paid.ProviderName != "openai" {
		t.Errorf("it chose %s", paid.ProviderName)
	}
}

/*
 * With nothing running and no key, it says what to do rather than failing
 * later.
 *
 * The alternative is somebody being told a picture is on its way and finding
 * out four minutes afterwards that nothing could ever have made it.
 */
func TestWithNothingAvailableItSaysWhatToDo(t *testing.T) {
	b := testBrain(t)
	b.Mode = llm.ModeOpen
	b.Cfg.PicturesURL = "http://127.0.0.1:1"
	b.Cfg.ProviderKeys = map[string]string{}

	studio := studioOf{b}

	_, err := studio.Painter()
	if err == nil {
		t.Fatal("it offered to make a picture with nothing that could")
	}

	for _, want := range []string{"not running", "Providers"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("it does not mention %q: %v", want, err)
		}
	}
}

// A key for a service with no picture-making endpoint is not offered one.
// Otherwise somebody with a Groq key is told a picture is coming and gets a
// 404 four minutes later.
func TestOnlyServicesThatCanDrawAreUsed(t *testing.T) {
	b := testBrain(t)
	b.Mode = llm.ModeOpen
	b.Cfg.PicturesURL = "http://127.0.0.1:1"
	b.Cfg.ProviderKeys = map[string]string{"groq": "gsk-test", "deepseek": "sk-test"}

	studio := studioOf{b}

	if _, err := studio.Painter(); err == nil {
		t.Error("a chat-only service was asked for a picture")
	}
}

// They go inside the brain's own folder, so they travel with it rather than
// with the machine.
func TestPicturesTravelWithTheBrain(t *testing.T) {
	b := testBrain(t)

	studio := studioOf{b}

	if got := studio.PicturesFolder(); !strings.HasPrefix(got, b.Root) {
		t.Errorf("pictures would be saved to %q, outside the brain at %q", got, b.Root)
	}
}
