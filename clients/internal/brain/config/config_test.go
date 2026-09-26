package config

import "testing"

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
