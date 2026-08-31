package preflight

import (
	"os"
	"path/filepath"
	"runtime"
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
			Requirement{
				Name:        "Voice (listening)",
				Why:         "lets you talk to the brain instead of typing",
				Consequence: "the Talk button will not appear",
				Optional:    true,
				Check: func() (State, string) {
					for _, r := range []string{"whisper-cli", "whisper-cpp", "whisper"} {
						if commandExists(r) {
							return OK, r
						}
					}

					return Missing, ""
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
				ManualHint: "Build whisper.cpp, put whisper-cli on your PATH, then: " +
					"bash models/download-ggml-model.sh base.en",
			},
		)
	}

	return list
}
