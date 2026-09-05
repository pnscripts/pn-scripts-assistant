package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

/*
 * Changing how it behaves by saying so.
 *
 * The question this answers is "how do I talk to it if I want to change
 * something", and until now the honest answer was that for most things you
 * could not — you went and found a panel. From the program whose whole
 * argument is that everything works from inside it.
 */
func settingsTool(t *testing.T) (Settings, *[]string) {
	t.Helper()

	var done []string

	return Settings{
		Read: func() Setting {
			return Setting{SpeakAloud: true, OnlyOwner: false, KeepQuiet: true, Voice: "robot"}
		},
		Set: func(_ context.Context, what string, on bool) (string, error) {
			done = append(done, what+"="+map[bool]string{true: "on", false: "off"}[on])

			return "done", nil
		},
		Voice: func(_ context.Context, which string) (string, error) {
			done = append(done, "voice="+which)

			return "This is the " + which + " voice.", nil
		},
	}, &done
}

func TestASettingIsChangedWhenAskedFor(t *testing.T) {
	tool, done := settingsTool(t)

	if _, err := tool.Execute(context.Background(),
		json.RawMessage(`{"setting":"only_me","on":true}`)); err != nil {
		t.Fatal(err)
	}

	if len(*done) != 1 || (*done)[0] != "only_me=on" {
		t.Fatalf("changed %v", *done)
	}
}

// Switching something off is as ordinary as switching it on, and "on" left out
// means off rather than a guess.
func TestLeavingOutOnMeansOff(t *testing.T) {
	tool, done := settingsTool(t)

	tool.Execute(context.Background(), json.RawMessage(`{"setting":"keep_quiet"}`))

	if (*done)[0] != "keep_quiet=off" {
		t.Fatalf("changed %v", *done)
	}
}

func TestTheVoiceCanBeChangedBySaying(t *testing.T) {
	tool, done := settingsTool(t)

	said, err := tool.Execute(context.Background(), json.RawMessage(`{"voice":"woman"}`))
	if err != nil {
		t.Fatal(err)
	}

	if (*done)[0] != "voice=woman" || !strings.Contains(said, "woman") {
		t.Fatalf("changed %v, said %q", *done, said)
	}
}

/*
 * Asked with nothing to change, it says how things stand.
 *
 * That is asked far more often than any single change, and answering it with a
 * question would be the assistant making somebody guess the name of a setting.
 */
func TestAskedWithNoChangeItReportsHowThingsStand(t *testing.T) {
	tool, done := settingsTool(t)

	said, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}

	if len(*done) != 0 {
		t.Fatalf("changed something when only asked: %v", *done)
	}

	for _, want := range []string{"speak my answers aloud: yes", "only your voice: no",
		"robot voice", "privacy panel"} {
		if !strings.Contains(said, want) {
			t.Errorf("the report is missing %q:\n%s", want, said)
		}
	}
}

/*
 * Privacy is not changeable from a conversation, and saying so is the answer.
 *
 * The protected-paths list is reachable only from setup and the privacy panel,
 * for the reason that a conversation which can talk the brain into sharing
 * more is a conversation somebody else can have with it. The same argument
 * applies here exactly.
 */
func TestPrivacyIsNotChangedByTalking(t *testing.T) {
	tool, done := settingsTool(t)

	said, err := tool.Execute(context.Background(),
		json.RawMessage(`{"setting":"privacy","on":true}`))
	if err != nil {
		t.Fatal(err)
	}

	if len(*done) != 0 {
		t.Fatalf("changed privacy from a sentence: %v", *done)
	}

	if !strings.Contains(said, "privacy panel") {
		t.Fatalf("did not say where privacy is set:\n%s", said)
	}
}

// A setting it does not have is answered with the ones it does, rather than
// with a failure somebody has to guess their way out of.
func TestAnUnknownSettingListsTheRealOnes(t *testing.T) {
	tool, _ := settingsTool(t)

	said, _ := tool.Execute(context.Background(),
		json.RawMessage(`{"setting":"turbo","on":true}`))

	for _, want := range []string{"speaking answers aloud", "answering only your voice"} {
		if !strings.Contains(said, want) {
			t.Errorf("did not offer %q:\n%s", want, said)
		}
	}
}

// It asks before changing anything, and the prompt says which way.
func TestChangingASettingWaitsToBeAgreed(t *testing.T) {
	if got := (Settings{}).Risk(); got != Mutating {
		t.Fatalf("changing how it behaves should ask first, got %v", got)
	}

	on := Settings{}.Summarize(json.RawMessage(`{"setting":"only_me","on":true}`))
	off := Settings{}.Summarize(json.RawMessage(`{"setting":"only_me","on":false}`))

	if on == off {
		t.Fatal("switching on and off are described the same way")
	}

	if !strings.Contains(on, "only your voice") {
		t.Fatalf("the prompt does not name the setting: %q", on)
	}
}

// A failure is reported rather than swallowed into a cheerful confirmation.
func TestAFailedChangeIsNotReportedAsDone(t *testing.T) {
	tool := Settings{
		Set: func(context.Context, string, bool) (string, error) {
			return "", errors.New("no echo canceller on this machine")
		},
	}

	said, err := tool.Execute(context.Background(),
		json.RawMessage(`{"setting":"cancel_room","on":true}`))

	if err == nil {
		t.Fatalf("a failed change was reported as %q", said)
	}
}

func TestWithNothingWiredUpItSaysSo(t *testing.T) {
	if _, err := (Settings{}).Execute(context.Background(),
		json.RawMessage(`{"setting":"only_me","on":true}`)); err == nil {
		t.Fatal("with nothing wired up it should say so")
	}
}
