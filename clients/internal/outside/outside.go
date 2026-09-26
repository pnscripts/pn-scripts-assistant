/*
 * Package outside is everywhere this program can connect to, and why.
 *
 * A program that reads somebody's files and calls itself local-first is
 * making a claim that cannot be checked by reading the README. This is the
 * list, it is in the source, and the test beside it fails if the source ever
 * grows a host that is not on it. So a telemetry endpoint added quietly is not
 * a thing that can happen here without somebody adding a line to this file
 * saying what it is for — which is the point.
 *
 * What this covers, precisely: every address written into the program. What it
 * does not cover, and this matters as much as what it does:
 *
 *   - Addresses its owner types. The Ollama address, a paired device, a mail
 *     server, an MCP server, a smart home. Those are theirs, and the program
 *     connecting to them is the program doing as it is told.
 *   - Web pages it is asked to read. "What does this page say" is a request to
 *     fetch a page, and which page is not knowable from here.
 *   - What Ollama, a browser, or any other program on the machine does on its
 *     own account.
 *
 * So this is not a firewall and does not pretend to be one. It is the answer
 * to "what does it talk to without being told", checked rather than asserted.
 */
package outside

// Place is somewhere the program may connect, and the reason it may.
type Place struct {
	// Host as it appears in the source.
	Host string

	// For is what it is used for, in a few words.
	For string

	// When is what has to be true first. Empty means it can happen whenever
	// the feature is used; anything else is a condition worth knowing about,
	// and "never in private mode" is the one that matters most.
	When string

	// Redirect marks a host that is never asked for by name: it is where one
	// of the others hands the download off to. A release on github.com answers
	// with a redirect to a signed URL on a content host, so the connection is
	// real and the address appears nowhere in the source.
	//
	// Those hops are checked at runtime, not merely listed here — see
	// `allowedSuffixes` in internal/preflight/fetch.go, which is applied to
	// every hop of every download rather than only to the first.
	Redirect bool
}

// Places is every host written into this program.
//
// Grouped by what they are for rather than sorted, because the grouping is the
// interesting part: nearly all of it is either fetching something the owner
// asked to install, or a model provider they configured with their own key.
var Places = []Place{
	// Things it downloads, because it was asked to install them.
	{Host: "github.com", For: "releases it installs: whisper, piper, Ollama's installer, the job catalogues", When: "setup, or when asked to install something"},
	{Host: "api.github.com", For: "which version is the latest, for the program's own updates", When: "checking for an update"},
	{Host: "raw.githubusercontent.com", For: "files inside a repository, for game templates", When: "starting a project that needs one"},
	{Host: "objects.githubusercontent.com", For: "where github.com hands a release download off to", When: "as above", Redirect: true},
	{Host: "release-assets.githubusercontent.com", For: "the same, under its newer name", When: "as above", Redirect: true},
	{Host: "huggingface.co", For: "speech and voice models", When: "setup, or when asked to install a voice"},
	{Host: "nodejs.org", For: "a pinned Node, for the web game engines", When: "starting a web project that needs it"},
	{Host: "ollama.com", For: "the model catalogue, and the installer", When: "choosing or installing a model"},
	{Host: "registry.ollama.ai", For: "what a model weighs before pulling it", When: "choosing a model"},

	// The job catalogues, which is where the thousands of occupations come
	// from. Public data, downloaded once.
	{Host: "ec.europa.eu", For: "the ESCO occupation catalogue", When: "setup, once"},
	{Host: "www.onetcenter.org", For: "the O*NET occupation catalogue", When: "setup, once"},

	// Documentation, read when the assistant is working on something and
	// needs the current answer rather than the one it was trained on.
	{Host: "docs.godotengine.org", For: "Godot documentation", When: "working on a Godot project"},
	{Host: "docs.unity3d.com", For: "Unity documentation", When: "working on a Unity project"},
	{Host: "dev.epicgames.com", For: "Unreal documentation", When: "working on an Unreal project"},
	{Host: "docs.phaser.io", For: "Phaser documentation", When: "working on a Phaser project"},
	{Host: "doc.babylonjs.com", For: "Babylon documentation", When: "working on a Babylon project"},
	{Host: "api.playcanvas.com", For: "PlayCanvas documentation", When: "working on a PlayCanvas project"},
	{Host: "manual.gamemaker.io", For: "GameMaker documentation", When: "working on a GameMaker project"},
	{Host: "defold.com", For: "Defold documentation", When: "working on a Defold project"},
	{Host: "docs.rs", For: "Rust crate documentation", When: "working on Rust"},

	// Web search, which is the whole of what `research` mode permits.
	{Host: "html.duckduckgo.com", For: "web search", When: "never in private mode"},
	{Host: "api.search.brave.com", For: "web search, when a key is configured", When: "never in private mode"},

	// Hosted models. Every one of these needs a key its owner entered, and
	// none of them is reachable at all unless privacy is set to open.
	{Host: "api.anthropic.com", For: "Anthropic models", When: "open mode, with your own key"},
	{Host: "api.openai.com", For: "OpenAI models", When: "open mode, with your own key"},
	{Host: "openrouter.ai", For: "OpenRouter models", When: "open mode, with your own key"},
	{Host: "generativelanguage.googleapis.com", For: "Google models", When: "open mode, with your own key"},
	{Host: "api.groq.com", For: "Groq models", When: "open mode, with your own key"},
	{Host: "api.deepseek.com", For: "DeepSeek models", When: "open mode, with your own key"},
	{Host: "api.mistral.ai", For: "Mistral models", When: "open mode, with your own key"},
	{Host: "api.x.ai", For: "xAI models", When: "open mode, with your own key"},
	{Host: "api.together.xyz", For: "Together models", When: "open mode, with your own key"},
	{Host: "api.fireworks.ai", For: "Fireworks models", When: "open mode, with your own key"},
	{Host: "api.cerebras.ai", For: "Cerebras models", When: "open mode, with your own key"},
	{Host: "api.perplexity.ai", For: "Perplexity models", When: "open mode, with your own key"},
}

/*
 * Names that are identifiers rather than addresses.
 *
 * An XML namespace looks exactly like a URL and is never fetched — it is a
 * string that says which vocabulary a document is written in. They are listed
 * separately so that the test can tell them apart from somewhere the program
 * actually goes, rather than either failing on them or quietly allowing a real
 * host to hide among them.
 */
var Namespaces = []string{
	"schemas.openxmlformats.org", // the .docx and .xlsx vocabularies
	"www.w3.org",                 // SVG and XML
	"www.freedesktop.org",        // the D-Bus configuration DTD
	"purl.org",                   // Dublin Core, in document metadata

	// ESCO gives every occupation and skill a URI under this domain. They are
	// identifiers in the downloaded data and are never fetched — the download
	// itself comes from ec.europa.eu, and internal/preflight/fetch.go would
	// refuse a redirect anywhere else.
	"data.europa.eu",
}

// Local is this machine, under the names it answers to.
var Local = []string{"127.0.0.1", "localhost", "[::1]", "0.0.0.0"}
