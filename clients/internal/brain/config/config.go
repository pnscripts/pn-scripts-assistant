// Package config holds the brain's settings.
//
// Settings come from a plain key=value file in the data root, overridden by the
// environment. The file lives with the data rather than with the program
// because it describes this brain — its name, its owner, its privacy setting —
// and that belongs to the disk that can be carried to another machine, not to
// the binary that happens to be running today.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is everything the brain needs to know about itself.
type Config struct {
	// Identity.
	Name  string
	Owner string

	// Privacy, as the raw configured string; parsing belongs to the llm package
	// so the strict-fallback rule lives in one place.
	Privacy string

	// Models.
	DefaultProvider string
	OllamaURL       string
	// New is true when there was no settings file to read: nobody has set this
	// brain up yet. The interface uses it to ask for a name once, rather than
	// silently calling itself whatever the default happens to be.
	New bool

	// WakeWord is what has to be said before the brain answers, or empty for
	// it to answer anything it hears.
	//
	// A comma-separated list, because a microphone writes a name down
	// differently from one sentence to the next and the fix is to add what it
	// actually wrote.
	WakeWord string

	/*
	 * CancelRoom routes everything this machine plays through the echo
	 * canceller, so the microphone stops hearing it.
	 *
	 * The canceller subtracts what was played into its own sink, which is the
	 * brain's own voice and nothing else — a browser playing music goes
	 * straight to the speakers, and the microphone hears every note of it
	 * mixed with whoever is talking. What comes back from the recogniser is a
	 * confident blend of the two, and telling one voice from another cannot
	 * help, because the recording genuinely contains both.
	 *
	 * Off unless asked for: it changes where every program on this machine
	 * sends its sound, which is not a thing to do to somebody quietly.
	 */
	CancelRoom bool

	/*
	 * OnlyMe makes the brain answer one voice and ignore every other.
	 *
	 * The name tells being spoken to from being in a room; it cannot tell who
	 * is speaking. A television says the name as readily as a person does, and
	 * so does somebody else in the house. This is the setting that asks the
	 * other question — is this the person I work for — and it needs a
	 * voiceprint to have been taught before it means anything.
	 */
	OnlyMe bool

	/*
	 * VoiceMatch is how alike a voice has to be to count as its owner's.
	 *
	 * A setting rather than a constant because it depends on the room and the
	 * microphone. Measured on the machine this was built for, two recordings
	 * of one voice sit between 0.68 and 0.91 and two different voices between
	 * -0.06 and 0.36, so halfway leaves a wide margin either side — but a
	 * noisier room narrows both.
	 */
	VoiceMatch float64

	// AlwaysName requires the name on every sentence, rather than staying in
	// the conversation for a while after being addressed.
	//
	// On by default, because the room this was written for has a television in
	// it and other people. Staying engaged means the next forty-five seconds of
	// whatever anybody says is the brain's business, and in a room like that it
	// answers the film.
	AlwaysName bool

	OllamaModel string

	/*
	 * ModelChosen records that a person picked the working model themselves.
	 *
	 * Without it there is no way to tell a deliberate choice from the value
	 * this program shipped with, and the two have to be treated differently.
	 * The shipped value was picked against four processor cores and no
	 * graphics card, and it was being used unchanged on every machine — so a
	 * computer with a card ran the model chosen for one without, and the whole
	 * apparatus that works out what the hardware can manage computed an answer
	 * that nothing ever read.
	 *
	 * Set once somebody chooses from the Models page, and never unset. From
	 * that point their choice wins on every machine, which is the point of
	 * having chosen.
	 */
	ModelChosen bool

	/*
	 * AutoModel lets the brain answer small talk with a smaller, quicker model.
	 *
	 * Every answer is computed on this processor, so the size of the model is
	 * the whole of the wait, and a greeting does not need the model that can
	 * edit files. Only recognisably conversational turns are sent to it —
	 * anything that means doing something goes to the usual one.
	 */
	AutoModel bool

	/*
	 * AlwaysSpeak reads every answer aloud, however the question arrived.
	 *
	 * Answers were spoken only for turns that came in by voice, so a question
	 * typed into the box was answered in silence — which is reasonable for a
	 * chat window and wrong for this, where the point of the thing is that you
	 * can be doing something else while it works. On a machine where an answer
	 * takes a minute, having to come back and look is most of the cost.
	 *
	 * The answer is not reshaped for it. A spoken turn asks the model for a
	 * couple of sentences because nobody wants a page read at them; a typed
	 * one that is also read aloud should still be the full written answer,
	 * because it is on the screen as well.
	 */
	AlwaysSpeak bool

	// FastModel is the small one, or empty to pick whichever is installed.
	FastModel      string
	EmbedModel     string
	AnthropicKey   string
	AnthropicModel string

	/*
	 * The other paid providers, which all speak the OpenAI chat API.
	 *
	 * Kept as separate keys rather than one "API key" because they are
	 * separate decisions: privacy is judged by which company a request goes
	 * to, and somebody who agreed to send conversation to OpenAI has not
	 * thereby agreed to OpenRouter — where the request may be served by any of
	 * the companies behind it.
	 */
	OpenAIKey   string
	OpenAIModel string

	OpenRouterKey   string
	OpenRouterModel string

	// Voice is which installed voice reads answers aloud, by id. Empty means
	// whichever the machine offers first.
	Voice string

	// Language is the spoken language, as an ISO code such as bg or en.
	//
	// Worth setting rather than left to detection: told nothing, whisper may
	// translate rather than transcribe, so Bulgarian speech comes back as
	// English prose — right words, wrong language, and no error to explain it.
	Language string

	/*
	 * The owner's mailbox.
	 *
	 * Filled in by the person who owns it, in Settings, and never by the
	 * assistant. MailPassword should be an app password rather than the
	 * account's own — every provider worth using issues them, they can be
	 * revoked on their own, and they cannot be used to take the account over.
	 */
	MailHost     string
	MailPort     int
	MailUser     string
	MailPassword string
	MailFrom     string
	SMTPHost     string
	SMTPPort     int

	// Web search. Without a key the brain falls back to scraping DuckDuckGo,
	// which needs no account.
	BraveKey string

	// Smart home. Empty means no smart-home tools are offered at all.
	HomeAssistantURL   string
	HomeAssistantToken string

	// Serving.
	Addr string

	// Recall tuning. The defaults match the system this replaced, so answers do
	// not change character as a result of the port.
	RecallLimit int
	RecallFloor float64
}

/*
 * DefaultWakeWord is what a brain answers to before anybody has said otherwise.
 *
 * A single ordinary word, because that is the one thing transcription reliably
 * gets right. The full name of this program is written down by whisper as
 * "Piembring" one time and "Piendren" the next — never the same way twice, so a
 * list of spellings never converges on it. "Brain" comes back as "Brain".
 */
const DefaultWakeWord = "Brain"

// assignInt sets a number from a setting, leaving it alone when unset or
// unreadable — a typo should not silently become port zero.
func assignInt(target *int, text string) {
	if text == "" {
		return
	}

	if n, err := strconv.Atoi(strings.TrimSpace(text)); err == nil {
		*target = n
	}
}

// intText writes a number, or nothing at all when it has not been set.
func intText(n int) string {
	if n == 0 {
		return ""
	}

	return strconv.Itoa(n)
}

// boolText writes a setting the way the file reads it back.
func boolText(on bool) string {
	if on {
		return "1"
	}

	return "0"
}

// FileName is the settings file inside a data root.
const FileName = "brain.conf"

// Default returns settings that work on a machine with Ollama installed and
// nothing else configured.
//
// Privacy defaults to private. A default that leaks is a default that will be
// shipped by somebody who never read this file.
func Default() Config {
	return Config{
		Name:            "PN Brain",
		Owner:           "",
		Privacy:         "private",
		DefaultProvider: "ollama",
		OllamaURL:       "http://127.0.0.1:11434",
		WakeWord:        DefaultWakeWord,
		AlwaysName:      true,

		/*
		 * Off until somebody turns it on, and a line that leaves room on both
		 * sides when they do.
		 *
		 * Off, because a brain that ignores its owner is worse than one that
		 * occasionally answers the television, and until a voice has been
		 * taught this can only do the first.
		 */
		OnlyMe:          false,
		CancelRoom:      false,
		VoiceMatch:      0.5,
		OllamaModel:     "qwen2.5-coder:7b",
		ModelChosen:     false,
		AutoModel:       true,
		AlwaysSpeak:     true,
		EmbedModel:      "nomic-embed-text",
		AnthropicModel:  "",
		OpenAIKey:       "",
		OpenAIModel:     "",
		OpenRouterKey:   "",
		OpenRouterModel: "",
		// Loopback only. Binding to every interface once exposed this brain's
		// knowledge endpoints to the local network, which is a mistake worth
		// making impossible rather than remembering not to make.
		Addr:        "127.0.0.1:8790",
		RecallLimit: 12,
		RecallFloor: 0.5,
	}
}

// Load reads the settings for a data root.
func Load(root string) (Config, error) {
	cfg := Default()

	path := filepath.Join(root, FileName)

	if _, err := os.Stat(path); err != nil {
		cfg.New = true
	}

	values, err := readFile(path)
	if err != nil {
		return cfg, err
	}

	// Environment wins over the file, so a one-off run can override without
	// editing anything.
	get := func(key string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}

		return values[key]
	}

	assign := func(dst *string, key string) {
		if v := get(key); v != "" {
			*dst = v
		}
	}

	assign(&cfg.Name, "BRAIN_NAME")
	assign(&cfg.Owner, "BRAIN_OWNER")
	assign(&cfg.Privacy, "BRAIN_PRIVACY")
	assign(&cfg.DefaultProvider, "LLM_DEFAULT_PROVIDER")
	assign(&cfg.OllamaURL, "OLLAMA_BASE_URL")
	assign(&cfg.WakeWord, "BRAIN_WAKE_WORD")
	assign(&cfg.FastModel, "OLLAMA_FAST_MODEL")

	if v := get("BRAIN_ALWAYS_SPEAK"); v != "" {
		cfg.AlwaysSpeak = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}

	if v := get("BRAIN_AUTO_MODEL"); v != "" {
		cfg.AutoModel = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}

	assign(&cfg.MailHost, "MAIL_HOST")
	assign(&cfg.MailUser, "MAIL_USER")
	assign(&cfg.MailPassword, "MAIL_PASSWORD")
	assign(&cfg.MailFrom, "MAIL_FROM")
	assign(&cfg.SMTPHost, "SMTP_HOST")

	assignInt(&cfg.MailPort, get("MAIL_PORT"))
	assignInt(&cfg.SMTPPort, get("SMTP_PORT"))

	if v := get("BRAIN_ONLY_ME"); v != "" {
		cfg.OnlyMe = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}

	if v := get("BRAIN_CANCEL_ROOM"); v != "" {
		cfg.CancelRoom = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}

	if v := get("BRAIN_VOICE_MATCH"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > -1 && n < 1 {
			cfg.VoiceMatch = n
		}
	}

	if v := get("BRAIN_ALWAYS_NAME"); v != "" {
		cfg.AlwaysName = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
	assign(&cfg.OllamaModel, "OLLAMA_DEFAULT_MODEL")

	if v := get("OLLAMA_MODEL_CHOSEN"); v != "" {
		cfg.ModelChosen = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}

	assign(&cfg.EmbedModel, "EMBEDDING_MODEL")
	assign(&cfg.AnthropicKey, "ANTHROPIC_API_KEY")
	assign(&cfg.AnthropicModel, "ANTHROPIC_MODEL")
	assign(&cfg.OpenAIKey, "OPENAI_API_KEY")
	assign(&cfg.OpenAIModel, "OPENAI_MODEL")
	assign(&cfg.OpenRouterKey, "OPENROUTER_API_KEY")
	assign(&cfg.OpenRouterModel, "OPENROUTER_MODEL")
	assign(&cfg.Addr, "BRAIN_ADDR")
	assign(&cfg.Voice, "BRAIN_VOICE")
	assign(&cfg.Language, "BRAIN_LANGUAGE")
	assign(&cfg.BraveKey, "BRAVE_SEARCH_KEY")
	assign(&cfg.HomeAssistantURL, "HOME_ASSISTANT_URL")
	assign(&cfg.HomeAssistantToken, "HOME_ASSISTANT_TOKEN")

	if v := get("RECALL_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.RecallLimit = n
		}
	}

	if v := get("RECALL_FLOOR"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.RecallFloor = f
		}
	}

	return cfg, nil
}

// Save writes the settings a person is expected to edit.
//
// The API key is written here too, which is why the file is 0600: it is a
// credential sitting on a disk that is designed to be carried around.
func (c Config) Save(root string) error {
	path := filepath.Join(root, FileName)

	var b strings.Builder

	b.WriteString("# PN Brain settings. Environment variables override these.\n\n")
	b.WriteString("BRAIN_NAME=" + c.Name + "\n")
	b.WriteString("BRAIN_OWNER=" + c.Owner + "\n\n")
	b.WriteString("# private | research | open   (anything unrecognised is treated as private)\n")
	b.WriteString("BRAIN_PRIVACY=" + c.Privacy + "\n\n")
	b.WriteString("# A word that must be said before it answers. Empty means it answers\n")
	b.WriteString("# anything it hears, which is the default: requiring a name means\n")
	b.WriteString("# transcription has to get that name right before anything can match.\n")
	b.WriteString("BRAIN_WAKE_WORD=" + c.WakeWord + "\n")
	b.WriteString("BRAIN_ALWAYS_NAME=" + boolText(c.AlwaysName) + "\n")
	b.WriteString("# Answer one voice and ignore every other. Needs a voice to have\n")
	b.WriteString("# been taught first, in Privacy.\n")
	b.WriteString("BRAIN_ONLY_ME=" + boolText(c.OnlyMe) + "\n")
	b.WriteString("# Send everything this machine plays through the echo canceller, so\n")
	b.WriteString("# music and videos are subtracted from what the microphone hears.\n")
	b.WriteString("BRAIN_CANCEL_ROOM=" + boolText(c.CancelRoom) + "\n")
	b.WriteString("BRAIN_VOICE_MATCH=" + fmt.Sprintf("%.2f", c.VoiceMatch) + "\n\n")

	b.WriteString("BRAIN_AUTO_MODEL=" + boolText(c.AutoModel) + "\n")
	b.WriteString("BRAIN_ALWAYS_SPEAK=" + boolText(c.AlwaysSpeak) + "\n")
	b.WriteString("OLLAMA_FAST_MODEL=" + c.FastModel + "\n\n")

	b.WriteString("MAIL_HOST=" + c.MailHost + "\n")
	b.WriteString("MAIL_PORT=" + intText(c.MailPort) + "\n")
	b.WriteString("MAIL_USER=" + c.MailUser + "\n")
	b.WriteString("MAIL_PASSWORD=" + c.MailPassword + "\n")
	b.WriteString("MAIL_FROM=" + c.MailFrom + "\n")
	b.WriteString("SMTP_HOST=" + c.SMTPHost + "\n")
	b.WriteString("SMTP_PORT=" + intText(c.SMTPPort) + "\n\n")
	b.WriteString("LLM_DEFAULT_PROVIDER=" + c.DefaultProvider + "\n")
	b.WriteString("OLLAMA_BASE_URL=" + c.OllamaURL + "\n")
	b.WriteString("OLLAMA_DEFAULT_MODEL=" + c.OllamaModel + "\n")
	b.WriteString("OLLAMA_MODEL_CHOSEN=" + boolText(c.ModelChosen) + "\n")
	b.WriteString("EMBEDDING_MODEL=" + c.EmbedModel + "\n")
	b.WriteString("ANTHROPIC_API_KEY=" + c.AnthropicKey + "\n")
	b.WriteString("ANTHROPIC_MODEL=" + c.AnthropicModel + "\n")
	b.WriteString("OPENAI_API_KEY=" + c.OpenAIKey + "\n")
	b.WriteString("OPENAI_MODEL=" + c.OpenAIModel + "\n")
	b.WriteString("OPENROUTER_API_KEY=" + c.OpenRouterKey + "\n")
	b.WriteString("OPENROUTER_MODEL=" + c.OpenRouterModel + "\n\n")
	b.WriteString("BRAIN_ADDR=" + c.Addr + "\n\n")
	b.WriteString("# Which voice reads answers aloud. Empty means the first available.\n")
	b.WriteString("BRAIN_VOICE=" + c.Voice + "\n\n")
	b.WriteString("# The language you speak, as an ISO code (bg, en, de...). Empty lets\n")
	b.WriteString("# whisper guess, which sometimes translates instead of transcribing.\n")
	b.WriteString("BRAIN_LANGUAGE=" + c.Language + "\n\n")
	b.WriteString("# Optional: a Brave Search API key. Without one, DuckDuckGo is scraped.\n")
	b.WriteString("BRAVE_SEARCH_KEY=" + c.BraveKey + "\n\n")
	b.WriteString("# Smart home. A long-lived access token from your Home Assistant profile.\n")
	b.WriteString("HOME_ASSISTANT_URL=" + c.HomeAssistantURL + "\n")
	b.WriteString("HOME_ASSISTANT_TOKEN=" + c.HomeAssistantToken + "\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}

// readFile parses key=value lines, ignoring blanks and # comments. A missing
// file is not an error: it means "all defaults", which is a valid state.
func readFile(path string) (map[string]string, error) {
	out := map[string]string{}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}

		return out, fmt.Errorf("reading %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		value = strings.TrimSpace(value)

		// Tolerate quoted values, which people write out of habit.
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		out[strings.TrimSpace(key)] = value
	}

	return out, scanner.Err()
}

// Path is where the settings file lives inside a data root.
//
// Exposed so callers that write settings — first-run setup, for one — do not
// have to reconstruct the path and drift from it.
func Path(root string) string { return filepath.Join(root, FileName) }
