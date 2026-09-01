package preflight

import (
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
	tmp := filepath.Join(os.TempDir(), "pn-brain-ollama.tar.zst")
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
	tmp := filepath.Join(os.TempDir(), "pn-brain-piper.tgz")

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
	 * woman and one man. Both British, to match each other rather than to have
	 * the assistant change accent when somebody changes its sex.
	 *
	 * About 60MB the pair. Left out for a long time on the grounds that one
	 * voice is enough to prove speech works, which is true and is not what the
	 * setting in front of somebody promises.
	 */
	voices := filepath.Join(into, "voices")

	const voiceBase = "https://huggingface.co/rhasspy/piper-voices/resolve/main/en/en_GB/"

	for _, voice := range []struct{ name, path string }{
		{"en_GB-alba-medium", "alba/medium/en_GB-alba-medium"},
		{"en_GB-northern_english_male-medium",
			"northern_english_male/medium/en_GB-northern_english_male-medium"},
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

	// And a model, without which it starts, prints an error and exits.
	model := filepath.Join(src, "models", "ggml-base.en.bin")

	if info, err := os.Stat(model); err != nil || info.Size() < 50<<20 {
		const from = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin"

		if err := download(from, model, w); err != nil {
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
