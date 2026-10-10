package brain

import (
	"context"
	"strings"
	"testing"
	"time"

	"pn-scripts-assistant/internal/brain/config"
)

// What a sentence gives away, and what it does not.
func TestWhoIsWhoIsReadFromWhatIsSaid(t *testing.T) {
	for said, want := range map[string]Learned{
		"Hi, I'm Petar. Let's get the itch page done.":      {Owner: "Petar"},
		"my name is ada and I build games":                  {Owner: "Ada"},
		"Call me Pete, everybody does":                      {Owner: "Pete"},
		"Казвам се Петър и правя игри":                      {Owner: "Петър", Language: "bg"},
		"Здрасти, аз съм Петър.":                            {Owner: "Петър", Language: "bg"},
		"Ще се казваш Ариел.":                               {Name: "Ариел", Language: "bg"},
		"Казвам се Петър, ще те наричам Искра.":             {Owner: "Петър", Name: "Искра", Language: "bg"},
		"Your name is Ariel.":                               {Name: "Ariel"},
		"I'm tired, can you just check the build?":          {},
		"I am going to publish the game tomorrow":           {},
		"what is on the itch page?":                         {},
		"Направи ми списък на файловете в папката с игрите": {Language: "bg"},
	} {
		if got := LearnFromWords(said); got != want {
			t.Errorf("%q read as %+v, want %+v", said, got, want)
		}
	}
}

/*
 * The first conversation fills in what setup needs, turn by turn, and writes
 * it down: no form, no questionnaire. A name already given is not replaced by
 * a later sentence that looks like one.
 */
func TestTheFirstConversationFillsInTheSettings(t *testing.T) {
	b := quietBrain(t)
	b.Cfg.Owner, b.Cfg.Language, b.Cfg.New = "", "", true

	if !b.FirstMeeting() {
		t.Fatal("a brain with no owner and no conversation is not a first meeting")
	}

	if !strings.Contains(b.SystemPrompt(), "You have only just met") {
		t.Error("the model is not told it has just met them")
	}

	b.learnFromFirstMeeting("Здравей. Казвам се Петър, ще се казваш Ариел.")

	// A later sentence that looks like a name does not replace the one given.
	b.learnFromFirstMeeting("Call me Bob")

	saved, err := config.Load(b.Root)
	if err != nil {
		t.Fatal(err)
	}

	if saved.Owner != "Петър" || saved.Name != "Ариел" || saved.Language != "bg" {
		t.Errorf("saved owner %q, name %q, language %q", saved.Owner, saved.Name, saved.Language)
	}

	if b.Cfg.New {
		t.Error("still a first run after the settings were written")
	}

	if strings.Contains(b.SystemPrompt(), "You have only just met") {
		t.Error("still told it has just met them after learning their name")
	}
}

/*
 * No composed greeting on a first meeting: no "Good evening", no list of what
 * it can do, no count of facts — and with no model to write an opener,
 * nothing at all rather than a canned line.
 */
func TestAFirstMeetingHasNoScriptedGreeting(t *testing.T) {
	b := quietBrain(t)
	b.Cfg.Owner = ""

	g := b.Greet()

	for _, part := range g.Facts {
		for _, stock := range []string{"morning", "afternoon", "evening", "Still up", "Late one",
			"How can I", "Ask me anything", "!"} {
			if strings.Contains(part, stock) {
				t.Errorf("the first meeting opens with a stock line: %q", part)
			}
		}
	}

	if g.Shown != "" {
		t.Errorf("a list of what it can do was shown: %q", g.Shown)
	}

	ctx, stop := context.WithTimeout(context.Background(), 20*time.Second)
	defer stop()

	// A real fact (a drive filling up) may still be said; a greeting may not.
	said := b.GreetInWords(ctx).Text

	for _, stock := range []string{"morning", "afternoon", "evening", "Hello", "Hi ", "How can I", "!"} {
		if strings.Contains(said, stock) {
			t.Errorf("with no model, the first meeting still opened with %q", said)
		}
	}
}
