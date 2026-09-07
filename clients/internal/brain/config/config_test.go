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
