package preflight

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

/*
 * The three things a new machine has no package for.
 *
 * Ollama, a neural voice and a recogniser. Each is a download from its own
 * project into the user's own home directory: no password, nothing outside
 * ~/.local, and no remote script run unread.
 *
 * They exist because of who this is for. Somebody who has never heard of a
 * language model, on a machine they bought last week, cannot be told to build
 * whisper.cpp from source — and an assistant that requires that of them is not
 * an assistant, it is a project.
 */

// localBin is where a program installed for one person goes.
func localBin() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp"
	}

	return filepath.Join(home, ".local", "bin")
}

func localShare(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", name)
	}

	return filepath.Join(home, ".local", "share", name)
}

/*
 * installOllama fetches the official build and unpacks it into the home
 * directory.
 *
 * The published instruction is to pipe a shell script from the internet into
 * root, which is the pattern this program should least of all teach. The same
 * release is available as an archive; this takes that, and puts it somewhere
 * that needs no password.
 */
func installOllama(w io.Writer) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("this installs Ollama only on 64-bit Linux; " +
			"get it from ollama.com/download")
	}

	/*
	 * A tar compressed with zstd, which is what the project publishes now.
	 *
	 * Go has no zstd in its standard library and this module has one
	 * dependency, so the decompression is handed to the zstd program and the
	 * unpacking is not: the tar that comes out still goes through the
	 * extractor here, which checks every entry's path. Letting tar do both
	 * would give up that check to save a line.
	 */
	tmp := filepath.Join(os.TempDir(), "pn-scripts-assistant-ollama.tar.zst")
	plain := strings.TrimSuffix(tmp, ".zst")

	defer os.Remove(tmp)
	defer os.Remove(plain)

	const release = "https://github.com/ollama/ollama/releases/latest/download/" +
		"ollama-linux-amd64.tar.zst"

	if err := download(release, tmp, w); err != nil {
		return err
	}

	if _, err := exec.LookPath("unzstd"); err != nil {
		fmt.Fprintln(w, "Installing zstd, which is needed to unpack it.")

		if err := run(w, escalate(aptInstall("zstd"), true)); err != nil {
			return fmt.Errorf("could not install zstd: %w", err)
		}
	}

	fmt.Fprintln(w, "Unpacking.")

	if err := run(w, []string{"unzstd", "-f", "-o", plain, tmp}); err != nil {
		return fmt.Errorf("could not decompress the download: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	into := filepath.Join(home, ".local")

	// The archive holds bin/ and lib/ already, so nothing is stripped.
	if err := unpackTar(plain, into, 0, w); err != nil {
		return err
	}

	binary := filepath.Join(into, "bin", "ollama")

	if err := os.Chmod(binary, 0o755); err != nil {
		return fmt.Errorf("installed Ollama but could not make it runnable: %w", err)
	}

	fmt.Fprintf(w, "Installed %s\n", binary)

	return startOllama(binary, w)
}

/*
 * startOllama leaves it running, and running after a reboot.
 *
 * Installing something that has to be started by hand before the program works
 * is not installing it. A user service needs no password and starts with the
 * desktop session.
 */
func startOllama(binary string, w io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	unit := filepath.Join(home, ".config", "systemd", "user", "ollama.service")

	if err := os.MkdirAll(filepath.Dir(unit), 0o755); err != nil {
		return err
	}

	body := "[Unit]\nDescription=Ollama\nAfter=network-online.target\n\n" +
		"[Service]\nExecStart=" + binary + " serve\nRestart=always\n" +
		"Environment=\"OLLAMA_MAX_LOADED_MODELS=3\"\n\n" +
		"[Install]\nWantedBy=default.target\n"

	if err := os.WriteFile(unit, []byte(body), 0o644); err != nil {
		return err
	}

	// Failures here are reported and not fatal: on a machine without systemd
	// the binary is still installed and can be started by hand.
	for _, argv := range [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "--now", "ollama.service"},
	} {
		out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
		if err != nil {
			fmt.Fprintf(w, "Installed, but could not start it automatically: %s\n",
				strings.TrimSpace(string(out)))

			return nil
		}
	}

	fmt.Fprintln(w, "Ollama is running, and will start with your session.")

	// It takes a moment to open its port, and whatever asked for this will
	// check immediately.
	time.Sleep(2 * time.Second)

	return nil
}

/*
 * installPiper fetches the neural voice and one voice to speak with.
 *
 * espeak-ng is a package and is already offered, but it is formant synthesis
 * and sounds like a machine from 1985. The difference between the two is the
 * difference between a person using the voice and a person turning it off.
 */
func installPiper(w io.Writer) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("this installs the neural voice only on 64-bit Linux")
	}

	into := localShare("piper")
	tmp := filepath.Join(os.TempDir(), "pn-scripts-assistant-piper.tgz")

	defer os.Remove(tmp)

	const release = "https://github.com/rhasspy/piper/releases/download/" +
		"2023.11.14-2/piper_linux_x86_64.tar.gz"

	if err := download(release, tmp, w); err != nil {
		return err
	}

	if err := unpackTarGz(tmp, into, 1, w); err != nil {
		return err
	}

	if err := os.Chmod(filepath.Join(into, "piper"), 0o755); err != nil {
		return err
	}

	/*
	 * A voice of each kind, without which the program is installed and silent.
	 *
	 * Two, because the choice offered is a robot, a woman or a man, and one
	 * downloaded voice makes two of those three do nothing. The robot needs no
	 * download — it is the system engine — so what has to arrive here is one
	 * woman, one man, and the one the robot is made of. The two human voices
	 * are British to match each other rather than to have the assistant change
	 * accent when somebody changes its sex.
	 *
	 * About 60MB each. Left out for a long time on the grounds that one
	 * voice is enough to prove speech works, which is true and is not what the
	 * setting in front of somebody promises.
	 */
	voices := filepath.Join(into, "voices")

	const voiceBase = "https://huggingface.co/rhasspy/piper-voices/resolve/main/en/"

	for _, voice := range []struct{ name, path string }{
		{"en_GB-alba-medium", "en_GB/alba/medium/en_GB-alba-medium"},
		{"en_GB-northern_english_male-medium",
			"en_GB/northern_english_male/medium/en_GB-northern_english_male-medium"},

		/*
		 * And the one the robot is built out of.
		 *
		 * The robot used to be espeak, which is genuinely a machine talking
		 * and is also the reason nobody could make out what it said. It is a
		 * neural voice with the machine timbre put on afterwards now, so the
		 * words are as clear as the voice and the character is still a
		 * machine — which means the robot needs a model like the other two.
		 *
		 * Medium rather than high: this is synthesised on the same processor
		 * the language model runs on, and high costs about twice as long to
		 * speak for a difference nobody has asked for.
		 *
		 * A woman's voice, and one of its own rather than the one above. The
		 * model the robot is made of is hidden from the list of voices, so
		 * building it out of alba would have quietly taken the woman's voice
		 * off the menu to put it behind an effect.
		 */
		{"en_GB-jenny_dioco-medium",
			"en_GB/jenny_dioco/medium/en_GB-jenny_dioco-medium"},
	} {
		for _, part := range []string{".onnx", ".onnx.json"} {
			if err := download(voiceBase+voice.path+part,
				filepath.Join(voices, voice.name+part), w); err != nil {
				return err
			}
		}
	}

	if err := link(filepath.Join(into, "piper"), filepath.Join(localBin(), "piper")); err != nil {
		return err
	}

	fmt.Fprintln(w, "The voice is installed.")

	return nil
}

/*
 * installWhisper builds the recogniser, because there is no binary release for
 * Linux to download.
 *
 * This is the largest thing behind a button here, and it is behind one anyway.
 * The alternative for somebody who has never used a terminal is "clone this
 * repository and run cmake", which is not an instruction, it is a wall.
 *
 * The build tools come from the system's own package manager, which is the one
 * step that needs a password; everything after it happens in the home
 * directory.
 */
func installWhisper(w io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	if _, err := exec.LookPath("cmake"); err != nil {
		fmt.Fprintln(w, "Installing the build tools first.")

		if err := run(w, escalate(aptInstall("build-essential", "cmake", "git"), true)); err != nil {
			return fmt.Errorf("could not install the build tools: %w", err)
		}
	}

	src := filepath.Join(home, ".local", "src", "whisper.cpp")

	if _, err := os.Stat(filepath.Join(src, "CMakeLists.txt")); err != nil {
		fmt.Fprintln(w, "Fetching whisper.cpp.")

		if err := run(w, []string{
			"git", "clone", "--depth", "1",
			"https://github.com/ggerganov/whisper.cpp", src,
		}); err != nil {
			return err
		}
	}

	fmt.Fprintln(w, "Building it. This takes a few minutes.")

	build := filepath.Join(src, "build")

	if err := runIn(w, src, []string{"cmake", "-B", "build", "-DCMAKE_BUILD_TYPE=Release"}); err != nil {
		return err
	}

	if err := runIn(w, src, []string{"cmake", "--build", "build", "--config", "Release", "-j"}); err != nil {
		return err
	}

	built := filepath.Join(build, "bin", "whisper-cli")

	if _, err := os.Stat(built); err != nil {
		return fmt.Errorf("the build finished but whisper-cli is not where it was expected")
	}

	if err := link(built, filepath.Join(localBin(), "whisper-cli")); err != nil {
		return err
	}

	/*
	 * And a model, without which it starts, prints an error and exits.
	 *
	 * The multilingual one, and this is the whole of the decision.
	 *
	 * It used to install ggml-base.en.bin: a third the size, faster, and
	 * unable to understand any language but English. Given Bulgarian it does
	 * not fail — it invents English, confidently, and the words come back
	 * fluent and wrong. Petar has been talking to a brain that could not
	 * understand him and neither of us knew, because nothing anywhere said so.
	 *
	 * An English-only default is defensible only if everybody who will ever
	 * run this speaks English. They do not. So the one that works for
	 * everybody is the one that is installed, and English speakers pay for it
	 * in seconds rather than everybody else paying for it in being
	 * misunderstood.
	 */
	model := filepath.Join(src, "models", SpeechModel)

	if info, err := os.Stat(model); err != nil || info.Size() < 50<<20 {
		fmt.Fprintln(w, "Downloading the speech model — it understands 99 languages, "+
			"which is why it is 488MB rather than 148.")

		if err := download(SpeechModelFrom, model, w); err != nil {
			return err
		}
	}

	fmt.Fprintln(w, "Listening is installed.")

	return nil
}

// link points a name in ~/.local/bin at something, replacing what was there.
func link(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}

	_ = os.Remove(to)

	return os.Symlink(from, to)
}

func run(w io.Writer, argv []string) error {
	return runIn(w, "", argv)
}

func runIn(w io.Writer, dir string, argv []string) error {
	fmt.Fprintf(w, "$ %s\n", strings.Join(argv, " "))

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdout = w
	cmd.Stderr = w

	return cmd.Run()
}

/*
 * Which speech model is installed, and where it comes from.
 *
 * Named here rather than written into the installer so that the interface can
 * say what it is about to download before it starts, and so the two cannot
 * drift apart — a setup screen promising one model and fetching another is a
 * small lie that costs somebody 488MB of surprise.
 */
const (
	// SpeechModel understands ninety-nine languages. See installWhisper for
	// why that rather than the English-only one, which is a third the size.
	SpeechModel = "ggml-small.bin"

	SpeechModelFrom = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin"

	// SpeechModelSize is what it costs, for saying so before asking.
	SpeechModelSize = "488MB"
)

/*
 * installGodot fetches the engine, which is one file and nothing else.
 *
 * No compiler, no dependencies, no package manager and no password: the Linux
 * build is a single executable in a zip. That is what makes it worth offering
 * here — everything else optional on this list either needs apt or needs
 * building, and this needs neither.
 *
 * The release is pinned, and checked against the hash the project published
 * with it. It used to ask which release was current and fetch whatever came
 * back, which meant trusting an answer from the network and a file from the
 * network at once. A newer release is still reported — see godot.Latest — but
 * moving to it is an edit to pinned.go, not something a download decides.
 */
func installGodot(w io.Writer) error {
	release, err := GodotEngine()
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "Fetching %s\n", release.Name)

	temp, err := os.MkdirTemp("", "pn-scripts-assistant-godot-*")
	if err != nil {
		return err
	}

	defer os.RemoveAll(temp)

	archive := filepath.Join(temp, "godot.zip")

	if err := Fetch(release, archive, w); err != nil {
		return err
	}

	engine, err := unzipGodot(archive, temp)
	if err != nil {
		return err
	}

	/*
	 * Beside what is there, never over it.
	 *
	 * Each release goes into a folder of its own, and godot4 on the PATH is
	 * a link to the one in use — so a newer release does not destroy the one
	 * projects were made with, and going back is pointing the link back. A
	 * godot4 this program did not put there is somebody's own and is left
	 * exactly as it is; the new release is still installed beside it and
	 * said so.
	 */
	folder := GodotFolder(release.Version)

	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}

	into := filepath.Join(folder, "godot")

	if err := copyExecutable(engine, into); err != nil {
		return err
	}

	link := filepath.Join(localBin(), "godot4")

	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}

	previous, err := os.Readlink(link)

	switch {
	case err == nil && !strings.HasPrefix(previous, GodotFolders()):
		fmt.Fprintf(w, "%s is a link somebody else made, and is left alone; Godot %s is at %s\n", link, release.Version, into)

		return nil
	case err != nil && !os.IsNotExist(err):
		if _, statErr := os.Lstat(link); statErr == nil {
			fmt.Fprintf(w, "%s is a Godot this program did not install, and is left alone; Godot %s is at %s\n",
				link, release.Version, into)

			return nil
		}
	}

	staged := link + ".new"
	os.Remove(staged)

	if err := os.Symlink(into, staged); err != nil {
		return err
	}

	if err := os.Rename(staged, link); err != nil {
		return err
	}

	if previous != "" && previous != into {
		fmt.Fprintf(w, "Godot %s is in use; the one before it is kept at %s, and godot4 can be pointed back at it\n",
			release.Version, previous)
	} else {
		fmt.Fprintf(w, "Godot %s is installed at %s\n", release.Version, into)
	}

	return nil
}

// GodotFolders is where this program keeps the Godot releases it installed.
func GodotFolders() string { return filepath.Join(localShare("pn-scripts-assistant"), "engines") }

// GodotFolder is one release's folder.
func GodotFolder(version string) string {
	return filepath.Join(GodotFolders(), "godot-"+strings.TrimSuffix(version, "-stable"))
}

// unzipGodot takes the engine out of the archive and returns where it landed.
func unzipGodot(archive, into string) (string, error) {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return "", fmt.Errorf("the download is not a zip: %w", err)
	}

	defer r.Close()

	for _, f := range r.File {
		name := filepath.Base(f.Name)

		if f.FileInfo().IsDir() || !strings.HasPrefix(strings.ToLower(name), "godot") {
			continue
		}

		// The console wrapper is a shell script beside the engine; the engine
		// is the one without an extension.
		if strings.Contains(strings.ToLower(name), "console") {
			continue
		}

		inside, err := f.Open()
		if err != nil {
			return "", err
		}

		out := filepath.Join(into, name)

		written, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			inside.Close()

			return "", err
		}

		_, err = io.Copy(written, inside)

		inside.Close()
		written.Close()

		if err != nil {
			return "", err
		}

		return out, nil
	}

	return "", fmt.Errorf("there is no engine in the archive")
}

// copyExecutable puts a file where it will be run from.
//
// Copied rather than linked: the source is in a temporary folder that is about
// to be deleted, and a link to a deleted file is a Godot that vanishes the
// moment setup finishes.
func copyExecutable(from, to string) error {
	source, err := os.Open(from)
	if err != nil {
		return err
	}

	defer source.Close()

	dest, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}

	defer dest.Close()

	if _, err := io.Copy(dest, source); err != nil {
		return err
	}

	return dest.Sync()
}
