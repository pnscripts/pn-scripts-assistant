package config

import (
	"os"
	"path/filepath"
	"testing"
)

/*
 * Setup runs once, and after that only when it is asked for.
 *
 * "After the setup is finished it must be run only for reinstall or upgrade
 * which must be forced from the program." A program that opens its installer
 * on every launch has not finished installing.
 *
 * Recorded as a setting rather than as the existence of a settings file,
 * because that file gets written by the naming card, by the privacy dropdown,
 * by anything at all — and "has a config" is not the same claim as "somebody
 * has seen the choices".
 */
func TestSetupIsRememberedAsDone(t *testing.T) {
	root := t.TempDir()

	fresh := Default()

	if fresh.SetupDone {
		t.Error("a brand new brain claims setup is already finished")
	}

	fresh.SetupDone = true
	fresh.Owner = "Petar"

	if err := fresh.Save(root); err != nil {
		t.Fatal(err)
	}

	again, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if !again.SetupDone {
		t.Error("setup would open again on the next launch")
	}

	if again.New {
		t.Error("a brain with settings is reported as new")
	}
}

// And the other way: a settings file that has never been through setup still
// says so, which is what makes an upgrade of an older brain offer it once.
func TestASettingsFileAloneIsNotSetup(t *testing.T) {
	root := t.TempDir()

	older := Default()
	older.Owner = "Petar"
	older.SetupDone = false

	if err := older.Save(root); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.SetupDone {
		t.Error("a settings file was taken as proof somebody saw setup")
	}
}

/*
 * What somebody switched off survives a restart.
 *
 * It was declared and never written to the file — so switching Docker off
 * lasted until the program closed, which is the kind of setting that makes
 * people stop trusting settings. Empty by default, and empty is the point:
 * what exists is available, and this is how one thing is taken away.
 */
func TestWhatIsSwitchedOffIsRemembered(t *testing.T) {
	root := t.TempDir()

	cfg := Default()

	if len(cfg.TurnedOff) != 0 {
		t.Errorf("a fresh brain has %v switched off", cfg.TurnedOff)
	}

	cfg.TurnedOff = []string{"docker", "service:openai"}

	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}

	back, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(back.TurnedOff) != 2 || back.TurnedOff[0] != "docker" || back.TurnedOff[1] != "service:openai" {
		t.Errorf("it came back as %v", back.TurnedOff)
	}

	// And switching everything back on empties it rather than keeping a
	// ghost of the old list.
	back.TurnedOff = nil

	if err := back.Save(root); err != nil {
		t.Fatal(err)
	}

	again, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(again.TurnedOff) != 0 {
		t.Errorf("switching everything back on left %v", again.TurnedOff)
	}
}

/*
 * What a brain starts as, which its owner chose.
 *
 * Two settings that used to be one, and the whole reason they are two: he
 * asked for permissions to be allowed by default, and with one switch that
 * would also have opened privacy — the assistant would have begun sending
 * conversations to other people's computers because it had been allowed to
 * write a file.
 */
func TestWhatAFreshBrainStartsAs(t *testing.T) {
	cfg := Default()

	if cfg.Freedom != "everything" {
		t.Errorf("permissions start at %q, want everything", cfg.Freedom)
	}

	if cfg.Privacy != "private" {
		t.Errorf("privacy starts at %q — allowing it to act must not let anything leave",
			cfg.Privacy)
	}

	if cfg.Asking() != "everything" {
		t.Errorf("what it asks is %q, and privacy must not decide it", cfg.Asking())
	}

	// The machine voice: the clearest neural voice on the machine, spoken
	// flat. Not a file path — which voice it is built from is decided by what
	// is installed at the time.
	if cfg.Voice != "robot" {
		t.Errorf("the voice starts as %q, want robot", cfg.Voice)
	}

	if !cfg.AlwaysSpeak {
		t.Error("it starts silent, and a voice nobody hears is not a voice")
	}
}

/*
 * The two settings do not move each other.
 *
 * Privacy that is open does not decide how much may be done here, and
 * permission to act here does not decide what leaves.
 */
func TestPrivacyAndPermissionAreSeparate(t *testing.T) {
	for _, c := range []struct{ freedom, privacy string }{
		{"ask", "open"},
		{"everything", "private"},
		{"granted", "private"},
		{"ask", "private"},
	} {
		cfg := Default()
		cfg.Freedom, cfg.Privacy = c.freedom, c.privacy

		if got := cfg.Asking(); got != c.freedom {
			t.Errorf("freedom %q with privacy %q asks as %q", c.freedom, c.privacy, got)
		}
	}
}

/*
 * A chosen permission level survives a restart, whatever privacy says.
 *
 * Loading used to raise it to match privacy — at least "granted" in research,
 * "everything" in open — so somebody who chose "ask every time" with privacy
 * open was acting freely again the next time the program started. The test
 * above set the fields directly and never went through the file, which is
 * where it happened.
 */
func TestAChosenPermissionSurvivesARestartInEveryPrivacyMode(t *testing.T) {
	t.Setenv("BRAIN_FREEDOM", "")
	t.Setenv("BRAIN_PRIVACY", "")

	for _, privacy := range []string{"private", "research", "open"} {
		for _, freedom := range []string{"ask", "granted", "everything"} {
			root := t.TempDir()

			cfg := Default()
			cfg.Freedom, cfg.Privacy = freedom, privacy

			if err := cfg.Save(root); err != nil {
				t.Fatal(err)
			}

			// Twice: a restart loads the file, and anything that saves
			// settings afterwards writes back what was loaded.
			for restart := 1; restart <= 2; restart++ {
				got, err := Load(root)
				if err != nil {
					t.Fatal(err)
				}

				if got.Freedom != freedom || got.Asking() != freedom {
					t.Errorf("privacy %s: chose %q, restart %d asks as %q",
						privacy, freedom, restart, got.Asking())
				}

				if got.Privacy != privacy {
					t.Errorf("permission %s: privacy %q came back as %q",
						freedom, privacy, got.Privacy)
				}

				if err := got.Save(root); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

/*
 * Files written before the two were separated still read as they were.
 *
 * One with no permission line at all takes the default; one written while
 * they were coupled already has two values that agreed, and keeps them.
 */
func TestOlderSettingsFilesStillRead(t *testing.T) {
	t.Setenv("BRAIN_FREEDOM", "")
	t.Setenv("BRAIN_PRIVACY", "")

	for file, want := range map[string]struct{ freedom, privacy string }{
		"BRAIN_PRIVACY=open\n":                          {"everything", "open"},
		"BRAIN_PRIVACY=research\n":                      {"everything", "research"},
		"BRAIN_PRIVACY=private\nBRAIN_FREEDOM=ask\n":    {"ask", "private"},
		"BRAIN_PRIVACY=research\nBRAIN_FREEDOM=granted": {"granted", "research"},
	} {
		root := t.TempDir()

		if err := os.WriteFile(filepath.Join(root, FileName), []byte(file), 0o600); err != nil {
			t.Fatal(err)
		}

		got, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}

		if got.Freedom != want.freedom || got.Privacy != want.privacy {
			t.Errorf("%q read as freedom %q, privacy %q; want %q, %q",
				file, got.Freedom, got.Privacy, want.freedom, want.privacy)
		}
	}
}
