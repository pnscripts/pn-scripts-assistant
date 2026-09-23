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
	"pn-scripts-assistant/internal/brain/llm"
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

	/*
	 * Freedom is how much the brain may do on this machine without asking.
	 *
	 * Deliberately not part of Privacy, which is a different question with a
	 * different answer. Privacy is what may leave the machine; this is what
	 * may be done on it. Tangling them meant the only way to let the brain do
	 * more was to let more leave, which is a trade nobody would choose if it
	 * were written down that plainly.
	 *
	 * "ask" | "granted" | "everything" — see the permits package.
	 */
	Freedom string

	/*
	 * SetupDone records that somebody has been through setup.
	 *
	 * Separate from New, which only means a settings file existed. Setup is
	 * for starting and for repairing, and it must not reappear on an ordinary
	 * launch — a program that opens its installer every day has not finished
	 * installing. Once this is set, setup happens only when it is asked for:
	 * the button under System, or "brain setup".
	 *
	 * Kept as a setting rather than as the mere existence of a file, because a
	 * settings file gets written by the naming card, by the privacy dropdown,
	 * by anything at all — and "has a config" is not the same claim as
	 * "somebody has seen the choices".
	 */
	SetupDone bool

	/*
	 * LookOnline is whether the program may look things up about itself.
	 *
	 * Separate from Privacy, and the separation is the whole point. Privacy is
	 * about where *your* words go — whether a conversation reaches somebody
	 * else's model. This is about whether the program may ask a public server
	 * a question that contains nothing of yours: is there a newer version,
	 * what models exist, what does this Godot class do.
	 *
	 * They were one setting, so keeping your conversation on this machine also
	 * meant never being told an update existed and never seeing the list of
	 * models you could install. Those are not the same decision and nobody
	 * would make the second one on purpose.
	 *
	 * On by default, because it sends nothing about anybody. Off is for a
	 * machine that must not talk to the internet at all, which is a real
	 * requirement and a different one from privacy.
	 */
	LookOnline bool

	/*
	 * ProfileToHosted is whether what you have written about yourself may go
	 * to a paid service.
	 *
	 * Off. The profile is the most personal thing in the program — what you
	 * do, who you work for, how you want things handled — and unlike a typed
	 * message it would be sent again with every single turn, whether or not
	 * the question had anything to do with it.
	 *
	 * It is a separate decision from Privacy rather than a consequence of it.
	 * Opening privacy is agreeing that this conversation may be answered
	 * elsewhere; it is not agreeing that a standing description of your
	 * business is attached to all of them. The local model is given it in
	 * every mode, because nothing leaves the machine.
	 */
	ProfileToHosted bool

	/*
	 * PicturesURL is an image server running on this machine.
	 *
	 * The shape Automatic1111 made and everything since has copied — SD.Next,
	 * Forge, and most of what people actually install. Not ComfyUI's graph
	 * API, which wants a whole workflow document per request: that is a thing
	 * somebody builds, not a thing a program can reasonably compose.
	 */
	PicturesURL string

	/*
	 * Reach is how far this assistant answers: "here" or "network".
	 *
	 * Here is the default and was the only possibility until now: the listener
	 * refuses to bind anywhere but loopback, and a connection is therefore a
	 * proof that it came from this computer. Network binds the local network
	 * and requires every request from elsewhere to carry a token given in
	 * person, over TLS.
	 *
	 * Its own setting rather than a consequence of anything else, because it
	 * is the single change in this program that cannot be undone by changing
	 * it back: anything that reached the machine while it was open stays
	 * reached. It should take a deliberate act, and be visible afterwards.
	 */
	Reach string

	// PictureModel is which model a paid service should use. Empty means its
	// default, which is the right answer until somebody has a reason.
	PictureModel string

	// Models.
	DefaultProvider string

	/*
	 * A key, a model and an address for every company there is.
	 *
	 * Three of them used to have a named field each, which is why there were
	 * three: adding a fourth meant a field, a line in Load, a line in the
	 * text written back out, and a branch in the router. Keyed by the id in
	 * llm.Services instead, so a company added to that list is a company this
	 * can hold a key for.
	 *
	 * The three original fields stay beside this and are filled from the same
	 * settings, because plenty of code reads them by name and a settings file
	 * written last month still spells them that way.
	 */
	ProviderKeys   map[string]string
	ProviderModels map[string]string
	ProviderURLs   map[string]string
	OllamaURL      string
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
	 * KeepQuiet turns the rest of this machine's sound down while it talks.
	 *
	 * The other half of the same complaint, and a different problem from
	 * CancelRoom despite sounding like the same one. The canceller is about
	 * what the microphone hears; this is about what a person in the room
	 * hears. With the canceller on, music no longer confuses the recogniser
	 * and is still exactly as loud over the answer.
	 *
	 * Lowered, never paused: something that quietly stops somebody's film
	 * every time it says a word is worse than something that talks over it.
	 */
	KeepQuiet bool

	/*
	 * VoiceLoudness is how loud its own voice is, and only its own.
	 *
	 * Every other program on this machine has a volume and this one did not,
	 * so the only way to make the assistant quieter was to turn the speakers
	 * down and lose the music with it.
	 *
	 * Between 0.2 and 1.0. Below a fifth it can be heard talking and not
	 * understood, which is worse than silence.
	 */
	VoiceLoudness float64

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

	/*
	 * AlwaysListen opens the microphone whenever the window is open.
	 *
	 * On, because somebody who has a microphone and a voice installed usually
	 * wants to talk to it, and switching that on every time is a tax on the
	 * thing they came for. But it was on with no way to say otherwise, which
	 * is a different thing from a default: the Stop button worked until the
	 * page was reloaded and then the microphone was open again.
	 *
	 * It costs more than it looks. Listening is not idle — it is voice
	 * detection and then transcription of whatever the room said, on the same
	 * processor the answers are worked out on, and it holds the microphone so
	 * that nothing else on the machine can have it.
	 */
	AlwaysListen bool

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
const DefaultWakeWord = "Assistant"

/*
 * Product is what this software is called; DefaultName is what an assistant is
 * called before its owner has named it.
 *
 * Two different things and they were the same string, which is why "PN Brain"
 * appeared both on the window and as the name somebody was expected to talk
 * to. The product has a name because it is a product; the assistant in
 * somebody's house should be called whatever they call it, and until they say,
 * the honest default is what it is rather than a brand.
 *
 * Named here so there is one place to change them and nothing can drift.
 */
const (
	Product     = "PN Scripts Assistant"
	DefaultName = "Assistant"
)

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

/*
 * reconcile carries an old privacy setting onto the one switch that replaced
 * it.
 *
 * Privacy and "how much it asks" used to be two settings. They are one now,
 * and the mapping runs one way — the switch decides what may leave. Which
 * means a brain whose owner had opened privacy, and left the asking alone,
 * would have been quietly tightened by an upgrade: the program would have
 * started refusing things it had allowed the day before, from a setting the
 * owner never touched.
 *
 * So a privacy value that is more open than the switch raises the switch to
 * match. Once, at load, and then the file is written back with both in
 * agreement. It never tightens: an old privacy setting cannot take away
 * something the switch already allows.
 */
// Asking is how much this configuration says the program should ask, with an
// old privacy setting carried onto it. The one place that question is
// answered, so that a Config built in a test and one read from a file behave
// the same way.
func (c Config) Asking() string { return reconcile(c.Freedom, c.Privacy) }

func reconcile(freedom, privacy string) string {
	openness := map[string]int{"private": 0, "research": 1, "open": 2}

	wanted := map[string]string{"research": "granted", "open": "everything"}

	by := map[string]int{"ask": 0, "granted": 1, "everything": 2}

	was, known := openness[strings.ToLower(strings.TrimSpace(privacy))]
	if !known {
		return freedom
	}

	if by[strings.ToLower(strings.TrimSpace(freedom))] >= was {
		return freedom
	}

	if raised, ok := wanted[strings.ToLower(strings.TrimSpace(privacy))]; ok {
		return raised
	}

	return freedom
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
		Name:    DefaultName,
		Owner:   "",
		Privacy: "private",

		// Asks about everything that changes something, which is where this
		// program has always started. A permission is something somebody
		// gives, not something they find already given.
		Freedom: "ask",

		// Looking up its own updates and the list of models sends nothing
		// about anybody, so it is on. See LookOnline.
		LookOnline: true,

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
		OnlyMe:     false,
		CancelRoom: false,

		// On by default, unlike the canceller above. This one is gentle and
		// undoes itself; that one moves where every program on the machine
		// sends its sound.
		KeepQuiet:   true,
		VoiceMatch:  0.5,
		OllamaModel: "qwen2.5-coder:7b",
		ModelChosen: false,
		AutoModel:   true,
		AlwaysSpeak: true,
		// Off until somebody turns it on. A program that opens the
		// microphone the first time it is started, on a machine it was
		// installed on minutes ago, has helped itself to something nobody
		// offered it. Setup asks, and the switch is in the panel.
		AlwaysListen:    false,
		EmbedModel:      "nomic-embed-text",
		AnthropicModel:  "",
		OpenAIKey:       "",
		OpenAIModel:     "",
		OpenRouterKey:   "",
		OpenRouterModel: "",
		// Loopback only. Binding to every interface once exposed this brain's
		// knowledge endpoints to the local network, which is a mistake worth
		// making impossible rather than remembering not to make.
		Addr: "127.0.0.1:8790",

		// Where an image server listens by default, which is where every one
		// of them listens unless somebody moved it.
		PicturesURL: "http://127.0.0.1:7860",

		// Only this computer, until somebody deliberately says otherwise.
		Reach: ReachHere,

		RecallLimit: 12,
		RecallFloor: 0.5,
	}
}

// Load reads the settings for a data root.
func Load(root string) (Config, error) {
	return LoadFrom(filepath.Join(root, FileName))
}

/*
 * LoadFrom reads a settings file by name rather than by the folder holding it.
 *
 * Load assumes the file is called FileName inside a root, which is true of
 * every brain and not true of everything that holds settings — setup is handed
 * an explicit path and had to reconstruct a folder from it to ask a question
 * about its own file. That worked only while the two agreed, silently answered
 * "no key" when they did not, and would have failed in exactly the way that is
 * hardest to notice: a working key reported as absent.
 */
func LoadFrom(path string) (Config, error) {
	cfg := Default()

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
	assign(&cfg.Freedom, "BRAIN_FREEDOM")

	cfg.Freedom = reconcile(cfg.Freedom, cfg.Privacy)

	if v := get("BRAIN_LOOK_ONLINE"); v != "" {
		cfg.LookOnline = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}

	if v := get("BRAIN_REACH"); v != "" {
		cfg.Reach = strings.ToLower(strings.TrimSpace(v))
	}

	if v := get("PICTURES_URL"); v != "" {
		cfg.PicturesURL = v
	}

	if v := get("PICTURE_MODEL"); v != "" {
		cfg.PictureModel = v
	}

	if v := get("BRAIN_PROFILE_TO_HOSTED"); v != "" {
		cfg.ProfileToHosted = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}

	if v := get("BRAIN_SETUP_DONE"); v != "" {
		cfg.SetupDone = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
	assign(&cfg.DefaultProvider, "LLM_DEFAULT_PROVIDER")
	assign(&cfg.OllamaURL, "OLLAMA_BASE_URL")
	assign(&cfg.WakeWord, "BRAIN_WAKE_WORD")
	assign(&cfg.FastModel, "OLLAMA_FAST_MODEL")

	if v := get("BRAIN_ALWAYS_SPEAK"); v != "" {
		cfg.AlwaysSpeak = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}

	if v := get("BRAIN_ALWAYS_LISTEN"); v != "" {
		cfg.AlwaysListen = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
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
	/*
	 * And the same three, plus every other company, generically.
	 *
	 * After the named ones, so a settings file holding both spellings resolves
	 * to the same value either way — they read the same setting names.
	 */
	if cfg.ProviderKeys == nil {
		cfg.ProviderKeys = map[string]string{}
		cfg.ProviderModels = map[string]string{}
		cfg.ProviderURLs = map[string]string{}
	}

	for _, svc := range llm.Services() {
		if v := get(llm.KeySetting(svc.ID)); v != "" {
			cfg.ProviderKeys[svc.ID] = v
		}

		if v := get(llm.ModelSetting(svc.ID)); v != "" {
			cfg.ProviderModels[svc.ID] = v
		}

		if v := get(llm.BaseURLSetting(svc.ID)); v != "" {
			cfg.ProviderURLs[svc.ID] = v
		}
	}

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

	b.WriteString("# " + Product + " settings. Environment variables override these.\n\n")
	b.WriteString("BRAIN_NAME=" + c.Name + "\n")
	b.WriteString("BRAIN_OWNER=" + c.Owner + "\n\n")
	b.WriteString("# private | research | open. Worked out from BRAIN_FREEDOM below\n")
	b.WriteString("# and written here so the file says what is actually in force.\n")
	b.WriteString("# Setting it by hand raises the freedom to match; it cannot\n")
	b.WriteString("# lower it, since the switch below is the one that decides.\n")
	b.WriteString("BRAIN_PRIVACY=" + c.Privacy + "\n\n")

	b.WriteString("# The one switch: how much it asks, and how much leaves.\n")
	b.WriteString("# ask        — asks before anything that changes something,\n")
	b.WriteString("#              and nothing leaves this machine\n")
	b.WriteString("# granted    — does what has been allowed; the web is open,\n")
	b.WriteString("#              the model answering stays here\n")
	b.WriteString("# everything — never stops and never refuses. Hosted models,\n")
	b.WriteString("#              the web, and what it knows about you may be sent.\n")
	b.WriteString("BRAIN_FREEDOM=" + c.Freedom + "\n")
	b.WriteString("# Set once somebody has been through setup. Setup then only\n")
	b.WriteString("# runs when it is asked for, from System or \"brain setup\".\n")
	b.WriteString("BRAIN_SETUP_DONE=" + boolText(c.SetupDone) + "\n")

	b.WriteString("# Whether it may look things up about itself — updates, the\n")
	b.WriteString("# list of models, documentation. Sends nothing about you, and is\n")
	b.WriteString("# a different question from privacy, which is where your words go.\n")
	b.WriteString("BRAIN_LOOK_ONLINE=" + boolText(c.LookOnline) + "\n\n")

	b.WriteString("# Whether what you wrote about yourself in profile.md may be sent to a\n")
	b.WriteString("# paid service. Off: it is only ever given to the model on this machine.\n")
	b.WriteString("# Unlike a message you type, it would go with every turn.\n")
	b.WriteString("BRAIN_PROFILE_TO_HOSTED=" + boolText(c.ProfileToHosted) + "\n\n")

	b.WriteString("# How far it answers: here, or network. Network binds the local\n")
	b.WriteString("# network and requires every device to be paired in person, over TLS.\n")
	b.WriteString("BRAIN_REACH=" + c.Reach + "\n\n")

	b.WriteString("# An image server running on this machine, for making pictures without\n")
	b.WriteString("# anything leaving it. Automatic1111, SD.Next or Forge all speak this.\n")
	b.WriteString("PICTURES_URL=" + c.PicturesURL + "\n\n")
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
	b.WriteString("BRAIN_ALWAYS_LISTEN=" + boolText(c.AlwaysListen) + "\n")
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

/*
 * HasPaidProvider reports whether anything is configured that can answer
 * without a model on this machine.
 *
 * Asked because three of the requirements — Ollama, a chat model, an embedding
 * model — exist only to run a brain locally. Demanded of somebody who has paid
 * for a service, they are a gigabyte and a half of downloads with no purpose,
 * and worse than pointless: they block the program from starting at all.
 *
 * A key for any company counts, and so does an address with no key: a server
 * on your own network usually has none, which is the whole of what the
 * "Somewhere else" entry is for.
 */
func (c Config) HasPaidProvider() bool {
	for _, key := range c.ProviderKeys {
		if strings.TrimSpace(key) != "" {
			return true
		}
	}

	/*
	 * An address counts only where no key is wanted.
	 *
	 * That is the self-hosted entry — LM Studio, vLLM, a machine in the house
	 * — which needs an address and usually no key at all. For a real company
	 * the key is what makes it configured, and the address is a detail.
	 *
	 * The distinction is not academic. These are read from the environment as
	 * well as the settings file, and ANTHROPIC_BASE_URL or OPENAI_BASE_URL is
	 * an ordinary thing to have exported — a proxy, a gateway, a development
	 * shim. Counting one as a configured brain told the program a paid service
	 * was ready on a machine that had nothing: it skipped the pieces it needed,
	 * started, and could not answer a single question.
	 */
	for _, svc := range llm.Services() {
		if svc.NeedsKey {
			continue
		}

		if strings.TrimSpace(c.ProviderURLs[svc.ID]) != "" {
			return true
		}
	}

	return false
}

// How far the assistant answers. See Config.Reach.
const (
	ReachHere    = "here"
	ReachNetwork = "network"
)

// OpenToNetwork reports whether this brain may be reached from off this
// machine. Anything unrecognised means here, which is the safe way round: a
// settings file with a typo in it must not quietly open a door.
func (c Config) OpenToNetwork() bool { return c.Reach == ReachNetwork }
