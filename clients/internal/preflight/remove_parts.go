package preflight

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

/*
 * Taking back off the machine what this program put on it.
 *
 * Installing was reversible only by knowing where nine directories are and
 * deleting them by hand in a terminal — which is the wall the whole setup
 * screen exists to remove, standing again at the other end. Somebody who is
 * allowed to install five gigabytes with one press should be allowed to take
 * it off the same way.
 *
 * Each of these removes exactly what its installer created and nothing near
 * it. That is why they are written out rather than driven from a list of
 * paths: "delete ~/.local/bin/ollama" is a fact about installOllama, and the
 * place to keep it is beside installOllama, where the next person changing
 * one will see the other.
 *
 * Models are not touched here. They are gigabytes, they are shared by
 * everything that uses ollama, and somebody removing "the voice" has not asked
 * to lose the model they spent an hour downloading. Removing those is its own
 * deliberate act.
 */

// removeAll deletes a path and says so, treating "already gone" as success —
// which it is: the point is that the thing is not there afterwards.
func removeAll(w io.Writer, path string) error {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	}

	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("could not remove %s: %w", path, err)
	}

	fmt.Fprintf(w, "Removed %s\n", path)

	return nil
}

/*
 * What each piece occupies, named once.
 *
 * The remover and the "how much would this free" measurement have to agree
 * about what a piece consists of, and the only way to guarantee that is for
 * them to read the same list. A second list written beside the first is a
 * second list that goes stale — the failure this program has already had
 * twice, in the requirement checks and in the provider table.
 */
func ollamaPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	return []string{
		filepath.Join(home, ".local", "bin", "ollama"),
		filepath.Join(home, ".local", "lib", "ollama"),
		filepath.Join(home, ".config", "systemd", "user", "ollama.service"),
	}
}

func voicePaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	return []string{
		filepath.Join(home, ".local", "share", "piper"),
		filepath.Join(home, ".local", "src", "piper"),
		filepath.Join(home, ".local", "bin", "piper"),
	}
}

func listeningPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	return []string{
		filepath.Join(home, ".local", "src", "whisper.cpp"),
		filepath.Join(home, ".local", "bin", "whisper-cli"),
		filepath.Join(home, ".local", "bin", "whisper-server"),
	}
}

func godotPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	return []string{
		filepath.Join(home, ".local", "bin", "godot4"),
		filepath.Join(home, ".local", "share", "godot"),
		filepath.Join(home, ".local", "src", "godot"),
	}
}

func removeOllama(w io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	/*
	 * The service first, and stopped before it is deleted.
	 *
	 * Deleting the unit under a running service leaves ollama running with no
	 * unit describing it — a process holding port 11434 that nothing on the
	 * machine claims, which then makes the next install look broken.
	 */
	unit := filepath.Join(home, ".config", "systemd", "user", "ollama.service")

	if _, err := os.Stat(unit); err == nil {
		for _, args := range [][]string{
			{"--user", "stop", "ollama"},
			{"--user", "disable", "ollama"},
		} {
			// Best effort: a machine with no systemd session still gets the
			// files removed, which is the part that matters.
			_ = exec.Command("systemctl", args...).Run()
		}

		if err := removeAll(w, unit); err != nil {
			return err
		}

		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	}

	for _, path := range ollamaPaths() {
		if err := removeAll(w, path); err != nil {
			return err
		}
	}

	fmt.Fprintln(w, "Ollama is removed. The models it downloaded are still in "+
		filepath.Join(home, ".ollama")+" — remove those separately if you want the space back.")

	return nil
}

func removeVoice(w io.Writer) error {
	for _, path := range voicePaths() {
		if err := removeAll(w, path); err != nil {
			return err
		}
	}

	fmt.Fprintln(w, "The neural voice is removed. It will fall back to the "+
		"system robot voice, which came with the system and is still there.")

	return nil
}

func removeListening(w io.Writer) error {
	// The speech model lives inside the source tree, so this takes it too —
	// which is what somebody removing "the recogniser" means.
	for _, path := range listeningPaths() {
		if err := removeAll(w, path); err != nil {
			return err
		}
	}

	fmt.Fprintln(w, "Listening is removed. The Talk button will not appear.")

	return nil
}

func removeGodot(w io.Writer) error {
	/*
	 * The program, not the projects.
	 *
	 * ~/.local/share/godot holds Godot's own export templates, which this
	 * installed and which are useless without it. Anybody's actual games live
	 * wherever they made them and are not touched.
	 */
	for _, path := range godotPaths() {
		if err := removeAll(w, path); err != nil {
			return err
		}
	}

	fmt.Fprintln(w, "Godot is removed. Any games you made are untouched.")

	return nil
}

/*
 * removeOllamaModel takes one model back off, by name.
 *
 * The models arrive as requirements — "Embedding model" is a requirement whose
 * installer is an ollama pull — and requirements are undone by a RemoveFunc.
 * Without one, closing setup half way removed the Ollama it had installed and
 * left the model it had downloaded sitting in a store nothing could read: the
 * rollback said "the machine is as it was" while leaving 274MB of orphan
 * behind it.
 *
 * Order saves this: the rollback undoes newest first, models were installed
 * after Ollama, so ollama is still there to run when this is called.
 */
func removeOllamaModel(name string) func(io.Writer) error {
	return func(w io.Writer) error {
		if _, err := exec.LookPath("ollama"); err != nil {
			// Nothing to do, and not a failure: with ollama gone the store is
			// unreadable anyway, and removing Ollama says where it is.
			fmt.Fprintf(w, "Ollama is not here, so %s cannot be removed through it.\n", name)

			return nil
		}

		out, err := exec.Command("ollama", "rm", name).CombinedOutput()
		if err != nil {
			// Already gone is success: the point is that it is not there.
			if strings.Contains(strings.ToLower(string(out)), "not found") {
				return nil
			}

			return fmt.Errorf("removing %s: %w %s", name, err,
				strings.TrimSpace(string(out)))
		}

		fmt.Fprintf(w, "Removed the model %s\n", name)

		return nil
	}
}
