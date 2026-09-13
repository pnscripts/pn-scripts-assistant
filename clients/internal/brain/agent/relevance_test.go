package agent

import (
	"testing"

	"pn-scripts-assistant/internal/brain/llm"
)

func allTools() []llm.ToolSpec {
	names := []string{
		"read_file", "write_file", "list_directory", "search_files",
		"web_search", "fetch_url", "run_command", "remind_me",
		"list_models", "look_at_screen", "list_windows", "set_appearance",
		"set_device", "list_devices", "set_wake_word",
	}

	out := make([]llm.ToolSpec, 0, len(names))
	for _, n := range names {
		out = append(out, llm.ToolSpec{Name: n})
	}

	return out
}

func offered(specs []llm.ToolSpec) map[string]bool {
	got := map[string]bool{}
	for _, s := range specs {
		got[s.Name] = true
	}

	return got
}

/*
 * A question about a website is not offered the tools for reading a screen or
 * listing models.
 *
 * How well a model chooses among tools falls off sharply with its size, and
 * thirty-one is a lot to choose between. Asked what it thought of a website,
 * the small model here called list_models — not a near miss — and before that
 * look_at_screen, which spent seven minutes reading a desktop that could never
 * have held the answer.
 */
func TestAWebsiteQuestionIsNotOfferedTheScreen(t *testing.T) {
	got := offered(relevant(allTools(), "what do you think about pnscripts.com?", noCues))

	for _, wrong := range []string{"look_at_screen", "list_models", "list_windows", "set_device"} {
		if got[wrong] {
			t.Errorf("%s was offered for a question about a website", wrong)
		}
	}

	// And the ones that can actually answer it are still there.
	for _, needed := range []string{"web_search", "fetch_url"} {
		if !got[needed] {
			t.Errorf("%s was withheld, so the site cannot be looked at", needed)
		}
	}
}

// Asked for directly, a housekeeping tool is offered as normal.
func TestHousekeepingToolsAppearWhenAskedFor(t *testing.T) {
	got := offered(relevant(allTools(), "which models are installed?", noCues))

	if !got["list_models"] {
		t.Error("list_models was withheld from a question about models")
	}

	screen := offered(relevant(allTools(), "what is on my screen right now?", noCues))

	if !screen["look_at_screen"] {
		t.Error("look_at_screen was withheld from a question about the screen")
	}
}

/*
 * The tools that only look are never filtered.
 *
 * Guessing wrongly here produces the worse failure: the right tool was never
 * offered, and the brain reports it cannot do something it can do perfectly
 * well. Reading, listing and searching therefore stay on the list whatever was
 * asked — none of them changes anything, so offering one needlessly costs a
 * moment and nothing else.
 *
 * Writing a file and running a command used to be on this list, and that was
 * wrong. They change the machine, they stop the turn for approval, and a small
 * model reaches for them unprompted: "say hello in four words" produced a
 * request to create a file, and the greeting was never answered. Those are
 * offered when somebody asks for them, and tested separately.
 */
func TestTheToolsThatOnlyLookAreAlwaysOffered(t *testing.T) {
	for _, message := range []string{
		"what do you think about pnscripts.com?",
		"good morning",
		"which models are installed?",
	} {
		got := offered(relevant(allTools(), message, noCues))

		for _, always := range []string{
			"read_file", "list_directory", "search_files",
			"web_search", "fetch_url", "remind_me",
		} {
			if !got[always] {
				t.Errorf("%s was withheld for %q", always, message)
			}
		}
	}
}

/*
 * Nothing offers to drive the keyboard unless somebody asked it to.
 *
 * Asked "do you listen every time that I call you", the small model proposed
 * typing "Press Return" into whatever window happened to be focused. Nothing
 * in that question implies touching the keyboard, and the approval prompt it
 * produced is worse than no prompt at all: a stream of confirmations for
 * things nobody asked for teaches its owner to approve without reading, which
 * is the one habit these confirmations exist to prevent.
 */
func TestNothingOffersToTypeUnlessAsked(t *testing.T) {
	driving := []string{"type_text", "click", "scroll", "open_app"}

	all := append(allTools(), llm.ToolSpec{Name: "type_text"},
		llm.ToolSpec{Name: "click"}, llm.ToolSpec{Name: "scroll"})

	for _, innocent := range []string{
		"do you listen every time that I call you",
		"what do you think about that",
		"how are you today",
		"what is the weather in Sofia",
	} {
		got := offered(relevant(all, innocent, noCues))

		for _, risky := range driving {
			if got[risky] {
				t.Errorf("%s was offered for %q, which asks for nothing of the sort",
					risky, innocent)
			}
		}
	}

	// And when somebody does ask, it is there.
	if !offered(relevant(all, "type my email address into that box", noCues))["type_text"] {
		t.Error("type_text was withheld from somebody asking it to type")
	}

	if !offered(relevant(all, "click the save button", noCues))["click"] {
		t.Error("click was withheld from somebody asking it to click")
	}
}

/*
 * A greeting is not offered the tools for changing the machine.
 *
 * "Say hello in four words" made the small model propose creating
 * /home/Petar/greetings.txt, and the turn ended in an approval prompt instead
 * of an answer — so the question was never answered at all. Not a wrong answer
 * but no answer, arrived at by way of a confirmation nobody asked for.
 *
 * A model with thirty tools and a greeting in front of it will find something
 * to do with them. The prompt asks it not to; a three-billion-parameter model
 * does not reliably listen, and the list it is shown is the part that can be
 * relied on.
 */
func TestConversationIsNotOfferedTheToolsForChangingThings(t *testing.T) {
	all := append(allTools(),
		llm.ToolSpec{Name: "do_in_background"},
		llm.ToolSpec{Name: "list_background"},
		llm.ToolSpec{Name: "stop_background"},
		llm.ToolSpec{Name: "edit_file"})

	changing := []string{"write_file", "edit_file", "run_command",
		"do_in_background", "list_background", "stop_background"}

	for _, chat := range []string{
		"Say hello in four words.",
		"In two sentences, what can you help me with?",
		"good evening",
		"how are you today",
	} {
		got := offered(relevant(all, chat, noCues))

		for _, risky := range changing {
			if got[risky] {
				t.Errorf("%s was offered for %q, which asks for nothing of the sort",
					risky, chat)
			}
		}
	}

	// Asked for plainly, they are there.
	for _, c := range []struct{ asked, want string }{
		{"write that down in a file", "write_file"},
		{"run the tests for me", "run_command"},
		{"what is still running in the background?", "list_background"},
	} {
		if !offered(relevant(all, c.asked, noCues))[c.want] {
			t.Errorf("%s was withheld from %q, which plainly asks for it",
				c.want, c.asked)
		}
	}
}

// noCues is the ordinary case: nothing built in names its own cues, so every
// test here is about the housekeeping list rather than about skills.
func noCues(string) []string { return nil }
