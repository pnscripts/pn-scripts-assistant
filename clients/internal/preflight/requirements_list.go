package preflight

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// requirements is the full list of what PN Brain needs, in the order a person
// should care about them.
//
// Deliberately one readable list: "what does this app expect of my machine?"
// should be answerable by reading a single file, the same way
// ToolServiceProvider answers "what can it do to my machine?".
func Requirements() []Requirement {
	list := []Requirement{
		{
			Name:        "Ollama",
			Why:         "runs language models locally, for free and privately",
			Consequence: "only the paid API provider will work",
			Size:        "1.5GB",
			Check: func() (State, string) {
				if !commandExists("ollama") {
					return Missing, ""
				}

				return OK, withUpdateNote(versionOf("ollama", "--version"),
					"https://github.com/ollama/ollama/releases/latest")
			},
			/*
			 * Fetched as an archive, not as a script piped into a shell.
			 *
			 * The published instruction is curl-into-root, which is the one
			 * pattern this program should least of all teach its owner to
			 * accept. The same release is downloadable; it goes into the home
			 * directory, needs no password, and is started as a user service
			 * so that installing it means it is actually running.
			 */
			Where: func() string {
				if at := whereIs("ollama"); at != "" {
					return at
				}

				if home, err := os.UserHomeDir(); err == nil {
					return filepath.Join(home, ".local", "bin", "ollama")
				}

				return "~/.local/bin/ollama"
			},
			InstallFunc: installOllama,
			ManualHint:  "Install from ollama.com/download",
		},
		{
			Name:        "Chat model",
			Why:         "the model PN Brain talks with",
			Consequence: "local conversations will fail",
			Size:        "4.7GB",
			Check: func() (State, string) {
				if !commandExists("ollama") {
					return Unknown, "needs Ollama first"
				}

				if !ollamaHasModel("qwen2.5-coder") {
					return Missing, ""
				}

				return OK, "qwen2.5-coder"
			},
			Where:      func() string { return ollamaModelDir() },
			InstallCmd: func() []string { return []string{"ollama", "pull", "qwen2.5-coder:7b"} },
		},
		{
			Name:        "Embedding model",
			Why:         "turns memories into vectors so they can be recalled by meaning",
			Consequence: "the brain cannot learn or recall anything",
			Size:        "274MB",
			Check: func() (State, string) {
				if !commandExists("ollama") {
					return Unknown, "needs Ollama first"
				}

				if !ollamaHasModel("nomic-embed-text") {
					return Missing, ""
				}

				return OK, "nomic-embed-text"
			},
			Where:      func() string { return ollamaModelDir() },
			InstallCmd: func() []string { return []string{"ollama", "pull", "nomic-embed-text"} },
		},
	}

	if runtime.GOOS == "linux" {
		list = append(list,
			Requirement{
				Name:        "Web engine (runtime)",
				Why:         "draws the native desktop window",
				Consequence: "the desktop app will not open; the browser still works",
				Optional:    true,
				Check: func() (State, string) {
					if pkgConfigExists("webkit2gtk-4.1") || fileGlobExists("/usr/lib/*/libwebkit2gtk-4.1.so*") {
						return OK, "webkit2gtk-4.1"
					}

					return Missing, ""
				},
				Where:      func() string { return "/usr/lib — system libraries, installed by apt" },
				InstallCmd: func() []string { return aptInstall("libwebkit2gtk-4.1-0", "libgtk-3-0t64") },
				NeedsRoot:  true,
			},
			Requirement{
				Name:        "Web engine (headers)",
				Why:         "only needed to build the desktop app from source",
				Consequence: "you can still run a prebuilt desktop app",
				Optional:    true,
				Check: func() (State, string) {
					if pkgConfigExists("webkit2gtk-4.1") && pkgConfigExists("gtk+-3.0") {
						return OK, "installed"
					}

					return Missing, ""
				},
				Where:      func() string { return "/usr/include — system headers, installed by apt" },
				InstallCmd: func() []string { return aptInstall("libwebkit2gtk-4.1-dev", "libgtk-3-dev") },
				NeedsRoot:  true,
			},
			Requirement{
				Name:        "Voice (speaking)",
				Why:         "lets the brain read its answers aloud",
				Consequence: "the brain will listen and answer, but stay silent",
				Size:        "63MB",
				Optional:    true,
				Check: func() (State, string) {
					// piper first: it is a neural voice and sounds like one,
					// where espeak-ng is formant synthesis and sounds like
					// that. Reported by name so it is obvious which is in use.
					if home, err := os.UserHomeDir(); err == nil {
						for _, dir := range []string{
							filepath.Join(home, ".local", "src", "piper"),
							filepath.Join(home, ".local", "share", "piper"),
						} {
							if voices, _ := filepath.Glob(filepath.Join(dir, "voices", "*.onnx")); len(voices) > 0 {
								return OK, "piper (neural)"
							}
						}
					}

					// speech-dispatcher is not evidence of a voice. spd-say
					// hands text to it and exits 0 whether or not anything can
					// say the words; with no engine behind it, speech-dispatcher
					// falls back to sd_dummy, whose entire purpose is to accept
					// speech and make no sound. The engine is what matters.
					for _, engine := range []string{"espeak-ng", "espeak", "pico2wave", "flite"} {
						if commandExists(engine) {
							return OK, engine
						}
					}

					return Missing, ""
				},
				/*
				 * The neural voice, not the package.
				 *
				 * espeak-ng is one apt away and sounds like a machine from
				 * 1985. The difference between the two is the difference
				 * between somebody using the voice and somebody turning it
				 * off, and this is meant for people who will never go looking
				 * for a better one.
				 */
				Where: func() string {
					if home, err := os.UserHomeDir(); err == nil {
						for _, dir := range []string{
							filepath.Join(home, ".local", "src", "piper"),
							filepath.Join(home, ".local", "share", "piper"),
						} {
							if voices, _ := filepath.Glob(filepath.Join(dir, "voices", "*.onnx")); len(voices) > 0 {
								return dir
							}
						}
					}

					for _, engine := range []string{"espeak-ng", "espeak", "pico2wave", "flite"} {
						if at := whereIs(engine); at != "" {
							return at
						}
					}

					if home, err := os.UserHomeDir(); err == nil {
						return filepath.Join(home, ".local", "src", "piper")
					}

					return "~/.local/src/piper"
				},
				InstallFunc: installPiper,
				ManualHint:  "Install piper, or: sudo apt install espeak-ng",
			},
			Requirement{
				Name:        "Using the computer",
				Why:         "lets the brain open windows, click, type and scroll for you",
				Consequence: "it can read the screen but not act on it",
				Optional:    true,
				Check: func() (State, string) {
					if commandExists("xdotool") {
						return OK, "xdotool"
					}

					return Missing, ""
				},
				Where: func() string {
					if at := whereIs("xdotool"); at != "" {
						return at
					}

					return "/usr/bin/xdotool — installed by apt"
				},
				InstallCmd: func() []string { return aptInstall("xdotool") },
				NeedsRoot:  true,
			},
			/*
			 * Four capabilities the brain has that setup never mentioned.
			 *
			 * Every one of them is used by code in this program and none of
			 * them was ever checked, so each failed the same quiet way: the
			 * feature exists, it is offered, and it does nothing. A PDF that
			 * cannot be opened is recorded as a document with nothing in it. A
			 * film with no subtitles is offered subtitles that never appear.
			 *
			 * "The setup must be able to install everything that is not on the
			 * machine" — and the harder half of that is knowing what the
			 * machine needs, which is a list that only grows as features are
			 * added and nobody remembers to come back here.
			 */
			Requirement{
				Name:        "Reading PDFs",
				Why:         "lets the brain read what is inside your PDFs, not just their names",
				Consequence: "every PDF is recorded as a document it could not open",
				Optional:    true,
				Check: func() (State, string) {
					if commandExists("pdftotext") {
						return OK, "pdftotext"
					}

					return Missing, ""
				},
				Where: func() string {
					if at := whereIs("pdftotext"); at != "" {
						return at
					}

					return "/usr/bin/pdftotext — installed by apt"
				},
				InstallCmd: func() []string { return aptInstall("poppler-utils") },
				NeedsRoot:  true,
				ManualHint: "sudo apt install poppler-utils",
			},
			Requirement{
				Name:        "Sound from films",
				Why:         "lets the brain listen to a film so it can write subtitles for it",
				Consequence: "it can find films with no subtitles but cannot make any",
				Optional:    true,
				Check: func() (State, string) {
					if commandExists("ffmpeg") {
						return OK, "ffmpeg"
					}

					return Missing, ""
				},
				Where: func() string {
					if at := whereIs("ffmpeg"); at != "" {
						return at
					}

					return "/usr/bin/ffmpeg — installed by apt"
				},
				InstallCmd: func() []string { return aptInstall("ffmpeg") },
				NeedsRoot:  true,
				ManualHint: "sudo apt install ffmpeg",
			},
			Requirement{
				Name: "Turning the music down",
				Why: "lets the brain lower whatever is playing while it speaks, " +
					"and put it back afterwards",
				Consequence: "it talks over your music instead of under it",
				Optional:    true,
				Check: func() (State, string) {
					// wpctl comes with WirePlumber and pw-play with PipeWire;
					// both are needed and they ship separately.
					switch {
					case commandExists("wpctl") && commandExists("pw-play"):
						return OK, "wpctl, pw-play"

					case commandExists("wpctl") || commandExists("pw-play"):
						return Missing, "only half of it is here"
					}

					return Missing, ""
				},
				Where: func() string {
					if at := whereIs("wpctl"); at != "" {
						return at
					}

					return "/usr/bin/wpctl — installed by apt"
				},
				InstallCmd: func() []string {
					return aptInstall("wireplumber", "pipewire-audio-client-libraries", "pipewire-bin")
				},
				NeedsRoot:  true,
				ManualHint: "sudo apt install wireplumber pipewire-bin",
			},

			/*
			 * The game engine, for somebody who asked to make games.
			 *
			 * Optional and last, because most people will never want it and a
			 * setup screen that demands an answer about a game engine has
			 * misjudged who is reading it. But it is one 78MB file with no
			 * dependencies and no compiler, which makes it the cheapest thing
			 * on this list to offer and the most annoying to find by hand.
			 */
			Requirement{
				Name:        "Godot (making games)",
				Why:         "lets the brain write, run and export Godot games",
				Consequence: "it can talk about game code but not run any of it",
				Size:        "78MB",
				Optional:    true,
				Check: func() (State, string) {
					for _, name := range []string{"godot4", "godot-4", "godot"} {
						if commandExists(name) {
							return OK, name
						}
					}

					return Missing, ""
				},
				Where: func() string {
					for _, name := range []string{"godot4", "godot-4", "godot"} {
						if at := whereIs(name); at != "" {
							return at
						}
					}

					return filepath.Join(localBin(), "godot4")
				},
				InstallFunc: installGodot,
				ManualHint: "Download the Linux build from godotengine.org, " +
					"make it executable, and put it in ~/.local/bin",
			},
			Requirement{
				Name:        "Voice (listening)",
				Why:         "lets you talk to the brain instead of typing",
				Consequence: "the Talk button will not appear",
				// Named from the installer's own constant, so the screen cannot
				// promise one model and fetch another.
				Size:     SpeechModelSize + ", plus a few minutes to build the recogniser",
				Optional: true,
				/*
				 * The recogniser and a model, because one without the other
				 * cannot hear anything.
				 *
				 * This asked only whether whisper-cli existed. Delete the
				 * speech model and it still answered "installed" — so the
				 * setup screen said listening was fine, the Talk button
				 * appeared, and pressing it failed with an error about a file
				 * format. A check that reports a capability the machine does
				 * not have is worse than no check: it sends somebody looking
				 * for the fault everywhere except where it is.
				 */
				Check: func() (State, string) {
					var found string

					for _, r := range []string{"whisper-cli", "whisper-cpp", "whisper"} {
						if commandExists(r) {
							found = r

							break
						}
					}

					if found == "" {
						return Missing, ""
					}

					if model := speechModelHere(); model != "" {
						return OK, found + " · " + model
					}

					// Half of it, and said as half rather than as absent:
					// what is left to do is a download, not a build.
					return Missing, found + " is here but has no speech model"
				},
				/*
				 * Built here, because there is no Linux binary to download.
				 *
				 * This was left as a manual step on the grounds that a clone
				 * and a compile is a larger promise than the tool should make.
				 * That is true of the promise and false of the alternative:
				 * for somebody who has never opened a terminal, "build
				 * whisper.cpp and put it on your PATH" is not an instruction,
				 * it is a wall. The build tools come from the system's package
				 * manager — the one step that asks for a password — and
				 * everything after it happens in the home directory.
				 */
				Where: func() string {
					for _, r := range []string{"whisper-cli", "whisper-cpp", "whisper"} {
						if at := whereIs(r); at != "" {
							return at
						}
					}

					if home, err := os.UserHomeDir(); err == nil {
						return filepath.Join(home, ".local", "src", "whisper.cpp")
					}

					return "~/.local/src/whisper.cpp"
				},
				InstallFunc: installWhisper,
				NeedsRoot:   true,
				// small, not base.en: the English-only model cannot understand
				// anybody who is not speaking English, and does not say so.
				ManualHint: "Build whisper.cpp, put whisper-cli on your PATH, then: " +
					"bash models/download-ggml-model.sh small",
			},
		)
	}

	return list
}

/*
 * speechModelHere reports which speech model is usable, if any.
 *
 * By size as well as by name: whisper.cpp ships test models of about a
 * megabyte that would transcribe silence, and an interrupted download used to
 * leave a truncated file wearing the real name. Anything under fifty megabytes
 * is one of those rather than a model.
 */
func speechModelHere() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	dirs := []string{
		filepath.Join(home, ".local", "src", "whisper.cpp", "models"),
		filepath.Join(home, ".local", "share", "whisper"),
		"/usr/share/whisper.cpp/models",
	}

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasPrefix(e.Name(), "ggml-") ||
				!strings.HasSuffix(e.Name(), ".bin") {
				continue
			}

			info, err := e.Info()
			if err != nil || info.Size() < 50<<20 {
				continue
			}

			return e.Name()
		}
	}

	return ""
}
