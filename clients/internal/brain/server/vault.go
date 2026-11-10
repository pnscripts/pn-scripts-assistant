package server

import (
	"net/http"
	"strings"

	"pn-scripts-assistant/internal/brain/vault"
)

/*
 * The brain's encryption, as its owner sees it (ADR 0008): whether it is
 * sealed, where this machine keeps the key, and — once, until he says he has
 * kept it — the recovery key that opens it anywhere else.
 */

func (s *Server) handleVault(w http.ResponseWriter, r *http.Request) {
	u := vault.Remembered()
	k := vault.Active()

	pending := false
	if k != nil {
		_, pending = vault.RecoveryToShow(s.brain.Root, k)
	}

	where, dir := keyringWords(u.Keyring)

	ok(w, map[string]any{
		"sealed":           k != nil,
		"keyring":          u.Keyring,
		"keyring_key":      where,
		"keyring_dir":      dir,
		"warning":          u.Warning,
		"warning_key":      warningWords(u.Warning),
		"recovery_pending": pending,
	})
}

/*
 * keyringWords names where the key is kept as a string of i18n.js, and the
 * folder for a key file, so the notice says it in the page's language. The
 * vault says it in English, for the log and for anything that is not a page;
 * a keyring this does not know is said in English as before (empty key).
 */
func keyringWords(name string) (key, dir string) {
	switch {
	case strings.HasPrefix(name, vault.FileKeyring{}.Name()):
		return "vault.keyring.file", strings.TrimPrefix(name, vault.FileKeyring{}.Name())
	case name == "this computer's keyring (Secret Service)":
		return "vault.keyring.secret", ""
	case name == "the macOS Keychain":
		return "vault.keyring.macos", ""
	case strings.HasPrefix(name, "Windows (DPAPI"):
		return "vault.keyring.windows", ""
	case name == (&vault.MemoryKeyring{}).Name():
		return "vault.keyring.memory", ""
	}

	return "", ""
}

// warningWords names the vault's warning as a string of i18n.js.
func warningWords(warning string) string {
	if warning != "" && warning == (vault.FileKeyring{}).Warning() {
		return "vault.warning.file"
	}

	return ""
}

// handleRecovery hands over the recovery key while it has not been kept.
func (s *Server) handleRecovery(w http.ResponseWriter, r *http.Request) {
	// Only to this computer's own screen: a paired phone has no business
	// holding the key that opens the brain anywhere.
	if !s.whoIsAsking(r).Here {
		fail(w, http.StatusForbidden, "The recovery key is shown only on the computer the brain is on.")

		return
	}

	k := vault.Active()
	if k == nil {
		fail(w, http.StatusNotFound, "This brain is not encrypted.")

		return
	}

	text, pending := vault.RecoveryToShow(s.brain.Root, k)
	if !pending {
		fail(w, http.StatusGone, "The recovery key has already been kept, and is not shown again.")

		return
	}

	if r.URL.Query().Get("as") == "file" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="pn-scripts-assistant-recovery-key.txt"`)

		w.Write([]byte("PN Scripts Assistant: recovery key\n\n" + text + "\n\n" +
			"This opens your encrypted brain on another computer, or on this one if its keyring is lost.\n" +
			"Keep it somewhere safe and away from the drive the brain is on. Anybody who has both\n" +
			"this key and the drive can read the brain.\n"))

		return
	}

	ok(w, map[string]any{"key": text})
}

// handleRecoveryKept forgets the recovery key: its owner has it.
func (s *Server) handleRecoveryKept(w http.ResponseWriter, r *http.Request) {
	if err := vault.RecoveryKept(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"kept": true})
}
