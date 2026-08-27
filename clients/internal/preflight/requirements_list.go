package preflight

import (
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

				return OK, versionOf("ollama", "--version")
			},
			// Piping a remote script into a shell is exactly the pattern this
			// tool should not normalise, so Ollama is left as a manual step
			// with a link rather than an automated curl-into-sh.
			InstallCmd: nil,
			ManualHint: "Install from ollama.com/download",
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
				InstallCmd: func() []string { return aptInstall("libwebkit2gtk-4.1-dev", "libgtk-3-dev") },
				NeedsRoot:  true,
			},
			Requirement{
				Name:        "Voice (speaking)",
				Why:         "lets the brain read its answers aloud",
				Consequence: "the brain will listen and answer, but stay silent",
				Optional:    true,
				Check: func() (State, string) {
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
				InstallCmd: func() []string { return aptInstall("espeak-ng") },
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
				// whisper.cpp is built from source and its model is a separate
				// download, so there is no package to install. Putting a clone
				// and a compile behind a button would be a larger promise than
				// this tool should make.
				InstallCmd: nil,
				ManualHint: "Build whisper.cpp, put whisper-cli on your PATH, then: " +
					"bash models/download-ggml-model.sh base.en",
			},
		)
	}

	return list
}
