package config

import (
	"os"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/brain/vault"
)

/*
 * Settings that only look sealed (found 19.11.2026 by the review of the
 * sealed-mark fix in the store).
 *
 * Only a personal setting is ever sealed in the settings file (SealSetting),
 * so only a personal setting is ever opened. Everything else is read as it
 * was written, whatever it begins with: a calendar user name or an address
 * typed with the mark used to read back as not set, and a sealed value copied
 * into a setting that is not personal was opened and shown on the page.
 */

// markedText begins with the mark and nothing else of a sealed value.
const markedText = vault.Prefix + "sam-marker"

func withKey(t *testing.T) *vault.Key {
	t.Helper()

	k, err := vault.Generate()
	if err != nil {
		t.Fatal(err)
	}

	vault.Use(k)
	t.Cleanup(func() { vault.Use(nil) })

	return k
}

func clearEnv(t *testing.T, keys ...string) {
	t.Helper()

	for _, key := range keys {
		t.Setenv(key, "")
	}
}

func TestASettingThatIsNotPersonalReadsBackAsWrittenWhateverItBeginsWith(t *testing.T) {
	clearEnv(t, "CALDAV_USER", "CALDAV_URL", "HOME_ASSISTANT_URL", "BRAIN_NAME", "BRAIN_VOICE")

	for _, keyed := range []bool{false, true} {
		name := "without a key"
		if keyed {
			name = "with a key"
		}

		t.Run(name, func(t *testing.T) {
			if keyed {
				withKey(t)
			}

			root := t.TempDir()

			cfg := Default()
			cfg.CalendarUser = markedText
			cfg.CalendarURL = vault.Prefix + "//calendar.example"
			cfg.HomeAssistantURL = vault.Prefix + "home"
			cfg.Name = vault.Prefix + "Ariel"
			cfg.Voice = vault.SealedHead() + "voice"

			if err := cfg.Save(root); err != nil {
				t.Fatal(err)
			}

			got, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}

			for setting, pair := range map[string][2]string{
				"CALDAV_USER":        {got.CalendarUser, cfg.CalendarUser},
				"CALDAV_URL":         {got.CalendarURL, cfg.CalendarURL},
				"HOME_ASSISTANT_URL": {got.HomeAssistantURL, cfg.HomeAssistantURL},
				"BRAIN_NAME":         {got.Name, cfg.Name},
				"BRAIN_VOICE":        {got.Voice, cfg.Voice},
			} {
				if pair[0] != pair[1] {
					t.Errorf("%s reads %q, want %q as it was written", setting, pair[0], pair[1])
				}
			}
		})
	}
}

// A value this brain sealed, copied into a setting that is not personal, is
// not opened: it reads as the ciphertext it is, and its secret stays sealed.
func TestASealedValueCopiedIntoASettingThatIsNotPersonalIsNotOpened(t *testing.T) {
	clearEnv(t, "CALDAV_USER", "HOME_ASSISTANT_URL", "MAIL_PASSWORD")

	k := withKey(t)

	root := t.TempDir()

	cfg := Default()
	cfg.MailPassword = "hunter2-marker"

	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}

	sealed := k.SealString("hunter2-marker")

	f, err := os.OpenFile(Path(root), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	f.WriteString("CALDAV_USER=" + sealed + "\nHOME_ASSISTANT_URL=" + sealed + "\n")
	f.Close()

	got, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if got.CalendarUser != sealed || got.HomeAssistantURL != sealed {
		t.Errorf("a sealed value in a setting that is not personal was opened: %q %q",
			got.CalendarUser, got.HomeAssistantURL)
	}

	if got.MailPassword != "hunter2-marker" {
		t.Errorf("the personal setting beside it reads %q", got.MailPassword)
	}
}

/*
 * A personal value that begins with the mark, saved while no key is in
 * force, is written as it came and reads back as it came: with no key, after
 * a key appears, and after the file is sealed with that key.
 */
func TestAPersonalValueWithTheMarkSavedWithoutAKeyReadsBack(t *testing.T) {
	clearEnv(t, "MAIL_PASSWORD", "BRAIN_OWNER", "HOME_ASSISTANT_TOKEN")

	vault.Use(nil)

	root := t.TempDir()

	cfg := Default()
	cfg.MailPassword = markedText
	cfg.Owner = vault.Prefix + "Sam"
	cfg.HomeAssistantToken = vault.Prefix

	if err := cfg.Save(root); err != nil {
		t.Fatal(err)
	}

	check := func(when string) {
		t.Helper()

		got, err := Load(root)
		if err != nil {
			t.Fatalf("%s: %v", when, err)
		}

		if got.MailPassword != cfg.MailPassword || got.Owner != cfg.Owner ||
			got.HomeAssistantToken != cfg.HomeAssistantToken {
			t.Errorf("%s: read %q %q %q, want %q %q %q", when,
				got.MailPassword, got.Owner, got.HomeAssistantToken,
				cfg.MailPassword, cfg.Owner, cfg.HomeAssistantToken)
		}
	}

	check("no key")

	withKey(t)
	check("a key in force, the file not yet sealed")

	if err := SealFile(Path(root)); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(Path(root))
	if strings.Contains(string(raw), "sam-marker") {
		t.Error("the personal value is in the clear after sealing")
	}

	check("the file sealed")
}

/*
 * The upgrade path: a file the current code sealed reads back the same, and
 * a value that begins the way every sealed value does but will not open
 * (another brain's, damaged, or no key here) still reads as not set, never
 * as ciphertext and never as somebody's text.
 */
func TestAFileSealedBeforeReadsBackAndADamagedValueStaysUnset(t *testing.T) {
	clearEnv(t, "MAIL_PASSWORD", "BRAIN_OWNER", "BRAVE_SEARCH_KEY", "OPENAI_API_KEY", "BRAIN_NAME")

	k := withKey(t)

	root := t.TempDir()

	// As 1.1.0 and the current dev write it: personal lines sealed under
	// the brain's key, everything else in the clear.
	text := "BRAIN_NAME=Ariel\n" +
		"BRAIN_OWNER=" + k.SealString("Sam Marker") + "\n" +
		"MAIL_PASSWORD=" + k.SealString("hunter2-marker") + "\n" +
		"OPENAI_API_KEY=" + k.SealString("sk-marker") + "\n"

	other, _ := vault.Generate()
	damaged := k.SealString("brave-marker")
	damaged = damaged[:len(damaged)-4] + "AAAA"

	text += "BRAVE_SEARCH_KEY=" + damaged + "\n"

	if err := os.WriteFile(Path(root), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if got.Owner != "Sam Marker" || got.MailPassword != "hunter2-marker" ||
		got.ProviderKeys["openai"] != "sk-marker" || got.Name != "Ariel" {
		t.Errorf("sealed before: %q %q %q %q", got.Owner, got.MailPassword, got.ProviderKeys["openai"], got.Name)
	}

	if got.BraveKey != "" {
		t.Errorf("a damaged sealed value reads %q, want not set", got.BraveKey)
	}

	vault.Use(other)

	if without, _ := Load(root); without.Owner != "" || without.MailPassword != "" {
		t.Errorf("under another brain's key: %q %q", without.Owner, without.MailPassword)
	} else {
		noCiphertextIn(t, "another brain's key", without)
	}

	vault.Use(nil)

	if without, _ := Load(root); without.Owner != "" || without.MailPassword != "" {
		t.Errorf("without a key: %q %q", without.Owner, without.MailPassword)
	} else {
		noCiphertextIn(t, "without a key", without)
	}

}
