package llm

import (
	"errors"
	"strings"
)

/*
 * Every company this can send a request to, in one list.
 *
 * There used to be three, written out separately in four places: the setup
 * picker, the settings file, the key validator and the router. Adding a fourth
 * company meant finding all four and getting them consistent, which is why
 * there were three — and why the key validator still checked every key against
 * Anthropic's prefix long after two more had been added beside it.
 *
 * Almost all of them speak the OpenAI protocol, which is the only reason this
 * list can be long: a company here is a base URL and a name, not a client to
 * write. Anthropic is the exception and has its own, which is what Native
 * marks. Anything not on this list that speaks the same protocol — LM Studio,
 * vLLM, llama.cpp's server, a company added next year — is reachable through
 * the last entry, which asks for the address instead of assuming it.
 */

type Service struct {
	ID   string
	Name string

	// BaseURL is the OpenAI-compatible endpoint. Empty for Anthropic, which
	// has its own client, and for the custom entry, which is asked for it.
	BaseURL string

	// Native names a provider with its own protocol rather than OpenAI's.
	Native bool

	// Prefix is what its keys start with, where the company commits to one.
	// Empty means no rule worth enforcing — several publish keys with no
	// stable prefix, and rejecting a working key to enforce a guess is worse
	// than accepting one that turns out to be wrong.
	Prefix string

	// Where to get a key, and Note is one line on what the company is for.
	Where string
	Note  string

	// Model is what it asks for when nobody has said otherwise.
	Model string

	// NeedsKey is false only for a self-hosted server on the same machine.
	NeedsKey bool
}

func Services() []Service {
	return []Service{
		{
			ID: "anthropic", Name: "Anthropic", Native: true,
			Prefix: "sk-ant-", Where: "console.anthropic.com",
			Note:  "Claude. Strong at long reasoning and at using tools.",
			Model: DefaultAnthropicModel, NeedsKey: true,
		},
		{
			ID: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com/v1",
			Prefix: "sk-", Where: "platform.openai.com/api-keys",
			Note:  "GPT. The protocol every other company on this list copied.",
			Model: DefaultOpenAIModel, NeedsKey: true,
		},
		{
			ID: "openrouter", Name: "OpenRouter",
			BaseURL: "https://openrouter.ai/api/v1",
			Prefix:  "sk-or-", Where: "openrouter.ai/keys",
			Note:  "One key, models from every major company behind it.",
			Model: DefaultOpenRouterModel, NeedsKey: true,
		},
		{
			ID: "google", Name: "Google Gemini",
			BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai",
			Where:   "aistudio.google.com/apikey",
			Note:    "Gemini. Very large context and a free tier to try it on.",
			Model:   "gemini-2.0-flash", NeedsKey: true,
		},
		{
			ID: "groq", Name: "Groq", BaseURL: "https://api.groq.com/openai/v1",
			Prefix: "gsk_", Where: "console.groq.com/keys",
			Note:  "Open models, answered faster than anything else here.",
			Model: "llama-3.3-70b-versatile", NeedsKey: true,
		},
		{
			ID: "deepseek", Name: "DeepSeek",
			BaseURL: "https://api.deepseek.com/v1",
			Prefix:  "sk-", Where: "platform.deepseek.com",
			Note:  "Strong at code and reasoning, and much cheaper than most.",
			Model: "deepseek-chat", NeedsKey: true,
		},
		{
			ID: "mistral", Name: "Mistral",
			BaseURL: "https://api.mistral.ai/v1",
			Where:   "console.mistral.ai/api-keys",
			Note:    "European, and open about what it trains on.",
			Model:   "mistral-large-latest", NeedsKey: true,
		},
		{
			ID: "xai", Name: "xAI", BaseURL: "https://api.x.ai/v1",
			Prefix: "xai-", Where: "console.x.ai",
			Note:  "Grok.",
			Model: "grok-2-latest", NeedsKey: true,
		},
		{
			ID: "together", Name: "Together AI",
			BaseURL: "https://api.together.xyz/v1",
			Where:   "api.together.xyz/settings/api-keys",
			Note:    "A wide catalogue of open models, hosted.",
			Model:   "meta-llama/Llama-3.3-70B-Instruct-Turbo", NeedsKey: true,
		},
		{
			ID: "fireworks", Name: "Fireworks",
			BaseURL: "https://api.fireworks.ai/inference/v1",
			Where:   "fireworks.ai/account/api-keys",
			Note:    "Open models, tuned for speed.",
			Model:   "accounts/fireworks/models/llama-v3p3-70b-instruct", NeedsKey: true,
		},
		{
			ID: "cerebras", Name: "Cerebras",
			BaseURL: "https://api.cerebras.ai/v1",
			Prefix:  "csk-", Where: "cloud.cerebras.ai",
			Note:  "Open models on their own silicon. Very fast.",
			Model: "llama-3.3-70b", NeedsKey: true,
		},
		{
			ID: "perplexity", Name: "Perplexity",
			BaseURL: "https://api.perplexity.ai",
			Prefix:  "pplx-", Where: "perplexity.ai/settings/api",
			Note:  "Answers with the web searched and cited.",
			Model: "sonar", NeedsKey: true,
		},
		{
			/*
			 * Anything else that speaks the same protocol.
			 *
			 * LM Studio, vLLM, llama.cpp's server, LocalAI, a company that
			 * does not exist yet, or another machine in the house running
			 * this. It asks for the address because that is the only thing
			 * this program cannot guess — and it needs no key, because a
			 * server on your own network usually has none.
			 */
			ID: "custom", Name: "Somewhere else",
			Where:    "the address of any OpenAI-compatible server",
			Note:     "LM Studio, vLLM, llama.cpp, LocalAI, or another machine.",
			NeedsKey: false,
		},
	}
}

// ServiceByID finds one, or reports that there is no such company.
func ServiceByID(id string) (Service, bool) {
	for _, s := range Services() {
		if s.ID == strings.ToLower(strings.TrimSpace(id)) {
			return s, true
		}
	}

	return Service{}, false
}

/*
 * Elsewhere reports whether a provider name is one of the companies, as
 * against something answering on this machine.
 *
 * Asked because model names do not travel: the size roles — work, quick, best
 * — resolve to what is installed here, so they are Ollama tags, and one of
 * those sent to Anthropic names a model that does not exist there. A provider
 * this program does not recognise as a company is treated as local, which is
 * the safe way round: the worst case is offering it a model name it does not
 * have, rather than silently sending work somewhere it was not meant to go.
 */
func Elsewhere(provider string) bool {
	provider = strings.ToLower(strings.TrimSpace(provider))

	if provider == "" || provider == Local {
		return false
	}

	_, known := ServiceByID(provider)

	return known
}

/*
 * KeySetting is what a company's key is called in the settings file.
 *
 * Derived rather than listed, so adding a company to Services is the whole of
 * adding a company. The three that existed before this keep the names they
 * already had, because settings files in the wild contain them.
 */
func KeySetting(id string) string {
	return strings.ToUpper(strings.ReplaceAll(id, "-", "_")) + "_API_KEY"
}

// ModelSetting is what a company's chosen model is called in the settings file.
func ModelSetting(id string) string {
	return strings.ToUpper(strings.ReplaceAll(id, "-", "_")) + "_MODEL"
}

// BaseURLSetting is where a custom server's address is kept.
func BaseURLSetting(id string) string {
	return strings.ToUpper(strings.ReplaceAll(id, "-", "_")) + "_BASE_URL"
}

/*
 * CheckKey catches a key pasted into the wrong company's box.
 *
 * Loose on purpose: it only tests the part a publisher actually commits to,
 * and says nothing where there is no such part. The mix-up it exists to catch
 * is a key saved under the wrong company, which otherwise fails much later
 * with an authentication error naming neither.
 */
func CheckKey(id, key string) error {
	svc, ok := ServiceByID(id)
	if !ok {
		return nil
	}

	if key == "" {
		return errNoKey
	}

	if svc.Prefix != "" && !strings.HasPrefix(key, svc.Prefix) {
		return &wrongKeyError{want: svc, key: key}
	}

	/*
	 * And the reverse: a key that carries another company's prefix, in a box
	 * whose own prefix is loose enough to accept it. "sk-" matches an
	 * Anthropic key and an OpenRouter one, so OpenAI's box would take either
	 * and fail on the first request.
	 */
	for _, other := range Services() {
		if other.ID == svc.ID || other.Prefix == "" {
			continue
		}

		if strings.HasPrefix(key, other.Prefix) && !strings.HasPrefix(svc.Prefix, other.Prefix) {
			return &wrongBoxError{is: other, chosen: svc}
		}
	}

	return nil
}

// errNoKey is a company that needs a key being saved without one.
var errNoKey = errors.New("no key given")

/*
 * wrongKeyError and wrongBoxError are the two ways a key lands in the wrong
 * place, and they need different sentences.
 *
 * The first is "this does not look like one of theirs" and the second is "this
 * is plainly somebody else's" — and the second can name who, which turns an
 * error into an instruction.
 */
type wrongKeyError struct {
	want Service
	key  string
}

func (e *wrongKeyError) Error() string {
	return "a " + e.want.Name + " key starts with " + e.want.Prefix
}

type wrongBoxError struct {
	is     Service
	chosen Service
}

func (e *wrongBoxError) Error() string {
	return "that is a " + e.is.Name + " key — choose " + e.is.Name + " above"
}
