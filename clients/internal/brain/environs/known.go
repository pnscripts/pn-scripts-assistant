package environs

import "pn-scripts-assistant/internal/brain/godot"

/*
 * The programs worth knowing about.
 *
 * Written down rather than discovered, and the reason is the model. A PATH
 * holds two thousand programs; a list of eighty is something an assistant can
 * be told about in a sentence, and a list of two thousand is a prompt nobody
 * can afford. Anything not here can still be run — run_command takes any
 * program — it simply is not announced.
 *
 * Chosen by what people actually have in front of them when they ask this
 * program for something: the languages they write in, the tools they build
 * with, the engines they make games in, the editors they keep open, and the
 * AI they already pay for. Nothing here is installed by this program; it is
 * looked for.
 */
func Known() []Program {
	return []Program{
		// What thinks, and what writes code on somebody's subscription.
		{ID: "ollama", Title: "Ollama", Kind: Model, Local: true,
			Names: []string{"ollama"}, Places: []string{"~/.local/bin", "/usr/local/bin"},
			Version: []string{"--version"},
			Needs:   "install it from ollama.com, or let setup do it"},
		{ID: "claude-code", Title: "Claude Code", Kind: Coder,
			Names: []string{"claude"}, Places: []string{"~/.local/bin"},
			Version: []string{"--version"},
			Needs:   "install Claude Code and sign in"},
		{ID: "codex", Title: "Codex", Kind: Coder,
			Names: []string{"codex"}, Places: []string{"~/.local/bin", "/usr/lib/chatgpt/resources"},
			Version: []string{"--version"},
			Needs:   "install the ChatGPT desktop app or the codex command, and sign in"},
		{ID: "cursor-agent", Title: "Cursor's agent", Kind: Coder,
			Names: []string{"cursor-agent"}, Places: []string{"~/.local/bin"},
			Needs: "install Cursor and its command-line agent"},
		{ID: "gh-copilot", Title: "GitHub Copilot CLI", Kind: Coder,
			Names: []string{"copilot"}, Needs: "install the Copilot CLI and sign in"},

		// Languages and platforms.
		{ID: "go", Title: "Go", Kind: Runtime, Local: true,
			Names: []string{"go"}, Places: []string{"/usr/local/go/bin"},
			Version: []string{"version"}, Needs: "sudo apt install golang-go, or from go.dev"},
		{ID: "node", Title: "Node.js", Kind: Runtime, Local: true,
			Names: []string{"node"}, Version: []string{"--version"},
			Needs: "sudo apt install nodejs, or from nodejs.org"},
		{ID: "npm", Title: "npm", Kind: Tool, Local: true,
			Names: []string{"npm"}, Version: []string{"--version"},
			Needs: "it comes with Node.js"},
		{ID: "pnpm", Title: "pnpm", Kind: Tool, Local: true,
			Names: []string{"pnpm"}, Version: []string{"--version"}, Needs: "npm install -g pnpm"},
		{ID: "python", Title: "Python", Kind: Runtime, Local: true,
			Names: []string{"python3", "python"}, Version: []string{"--version"},
			Needs: "sudo apt install python3"},
		{ID: "uv", Title: "uv", Kind: Tool, Local: true,
			Names: []string{"uv"}, Places: []string{"~/.local/bin"}, Version: []string{"--version"},
			Needs: "from astral.sh/uv"},
		{ID: "php", Title: "PHP", Kind: Runtime, Local: true,
			Names: []string{"php"}, Version: []string{"--version"}, Needs: "sudo apt install php-cli"},
		{ID: "composer", Title: "Composer", Kind: Tool, Local: true,
			Names: []string{"composer"}, Version: []string{"--version"},
			Needs: "from getcomposer.org — Laravel needs it"},
		{ID: "ruby", Title: "Ruby", Kind: Runtime, Local: true,
			Names: []string{"ruby"}, Version: []string{"--version"}, Needs: "sudo apt install ruby"},
		{ID: "rust", Title: "Rust", Kind: Runtime, Local: true,
			Names: []string{"cargo"}, Places: []string{"~/.cargo/bin"}, Version: []string{"--version"},
			Needs: "from rustup.rs"},
		{ID: "java", Title: "Java", Kind: Runtime, Local: true,
			Names: []string{"java"}, Version: []string{"-version"},
			Needs: "sudo apt install default-jdk"},
		{ID: "dotnet", Title: ".NET", Kind: Runtime, Local: true,
			Names: []string{"dotnet"}, Version: []string{"--version"},
			Needs: "from dotnet.microsoft.com"},

		// Building, versioning, shipping.
		{ID: "git", Title: "Git", Kind: Tool, Local: true,
			Names: []string{"git"}, Version: []string{"--version"}, Needs: "sudo apt install git"},
		{ID: "gh", Title: "GitHub CLI", Kind: Tool,
			Names: []string{"gh"}, Version: []string{"--version"},
			Needs: "sudo apt install gh, then gh auth login"},
		{ID: "docker", Title: "Docker", Kind: Tool, Local: true,
			Names: []string{"docker"}, Version: []string{"--version"},
			Needs: "sudo apt install docker.io"},
		{ID: "podman", Title: "Podman", Kind: Tool, Local: true,
			Names: []string{"podman"}, Version: []string{"--version"}, Needs: "sudo apt install podman"},
		{ID: "make", Title: "make", Kind: Tool, Local: true,
			Names: []string{"make"}, Version: []string{"--version"}, Needs: "sudo apt install make"},
		{ID: "cmake", Title: "CMake", Kind: Tool, Local: true,
			Names: []string{"cmake"}, Version: []string{"--version"}, Needs: "sudo apt install cmake"},
		{ID: "gcc", Title: "GCC", Kind: Tool, Local: true,
			Names: []string{"gcc"}, Version: []string{"--version"}, Needs: "sudo apt install build-essential"},

		// Game engines.
		{ID: "godot", Title: "Godot", Kind: Engine, Local: true,
			Names:  []string{"godot", "godot4", "Godot", "Godot_v4"},
			Places: []string{"~/.local/bin", "~/Applications", "/opt/godot", "/usr/local/bin"},
			// The engine package already knows where a downloaded Godot
			// lives, named for its own version. Asked first; the names above
			// are the fallback.
			Find:  godotHere,
			Needs: "from godotengine.org, or let the assistant install it"},
		{ID: "unity", Title: "Unity", Kind: Engine, Local: true,
			Names:  []string{"unityhub"},
			Places: []string{"/opt/unityhub", "~/Applications"},
			Needs:  "install Unity Hub and an editor, and sign in to license it"},
		{ID: "unreal", Title: "Unreal Engine", Kind: Engine, Local: true,
			Names:  []string{"UnrealEditor"},
			Places: []string{"~/UnrealEngine/Engine/Binaries/Linux", "/opt/UnrealEngine/Engine/Binaries/Linux"},
			Needs:  "from unrealengine.com"},
		{ID: "blender", Title: "Blender", Kind: Tool, Local: true,
			Names: []string{"blender"}, Version: []string{"--version"},
			Needs: "sudo apt install blender"},

		// Where people write.
		{ID: "vscode", Title: "Visual Studio Code", Kind: Editor,
			Names: []string{"code"}, Places: []string{"/snap/bin"}, Needs: "from code.visualstudio.com"},
		{ID: "cursor", Title: "Cursor", Kind: Editor,
			Names: []string{"cursor"}, Needs: "from cursor.com"},
		{ID: "zed", Title: "Zed", Kind: Editor,
			Names: []string{"zed"}, Places: []string{"~/.local/bin"}, Needs: "from zed.dev"},
		{ID: "intellij", Title: "IntelliJ IDEA", Kind: Editor,
			Names:  []string{"idea", "intellij-idea-community", "intellij-idea-ultimate"},
			Places: []string{"/snap/bin"}, Needs: "from jetbrains.com"},
		{ID: "neovim", Title: "Neovim", Kind: Editor, Local: true,
			Names: []string{"nvim"}, Version: []string{"--version"}, Needs: "sudo apt install neovim"},

		// Browsers, which are how a page gets looked at and how a web game
		// gets photographed.
		{ID: "chrome", Title: "Chrome or Chromium", Kind: Browser,
			Names:   []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"},
			Places:  []string{"/snap/bin"},
			Version: []string{"--version"}, Needs: "sudo apt install chromium-browser"},
		{ID: "firefox", Title: "Firefox", Kind: Browser,
			Names: []string{"firefox"}, Places: []string{"/snap/bin"},
			Version: []string{"--version"}, Needs: "sudo apt install firefox"},

		// The rest of what a machine is asked to do.
		{ID: "ffmpeg", Title: "FFmpeg", Kind: Tool, Local: true,
			Names: []string{"ffmpeg"}, Version: []string{"-version"}, Needs: "sudo apt install ffmpeg"},
		{ID: "imagemagick", Title: "ImageMagick", Kind: Tool, Local: true,
			Names: []string{"magick", "convert"}, Version: []string{"--version"},
			Needs: "sudo apt install imagemagick"},
		{ID: "whisper", Title: "whisper.cpp", Kind: Tool, Local: true,
			Names: []string{"whisper-cli", "whisper-cpp"}, Places: []string{"~/.local/bin"},
			Needs: "let setup install it, for listening"},
		{ID: "espeak", Title: "espeak-ng", Kind: Tool, Local: true,
			Names: []string{"espeak-ng", "espeak"}, Needs: "sudo apt install espeak-ng, for speaking"},
		{ID: "xdotool", Title: "xdotool", Kind: Tool, Local: true,
			Names: []string{"xdotool"}, Version: []string{"--version"},
			Needs: "sudo apt install xdotool, for typing and clicking"},
		{ID: "rsync", Title: "rsync", Kind: Tool, Local: true,
			Names: []string{"rsync"}, Version: []string{"--version"}, Needs: "sudo apt install rsync"},
		{ID: "jq", Title: "jq", Kind: Tool, Local: true,
			Names: []string{"jq"}, Version: []string{"--version"}, Needs: "sudo apt install jq"},
	}
}

/*
 * godotHere asks the package that drives Godot where it is.
 *
 * Newest first, which is FindAll's order: somebody with two of them meant to
 * use the newer one, and a project opened with the older is a project that
 * quietly will not open with the newer.
 */
func godotHere() (path, version string, ok bool) {
	engine, found := godot.Find()
	if !found {
		return "", "", false
	}

	return engine.Path, engine.Version, true
}
