package brain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"pn-scripts-assistant/internal/brain/glossary"
	"pn-scripts-assistant/internal/brain/llm"
	"pn-scripts-assistant/internal/brain/proof"
	"pn-scripts-assistant/internal/brain/tools"
	"pn-scripts-assistant/internal/brain/translate"
)

/*
 * Rewriting a text in a tone, from the Writing page.
 *
 * The page's proofreading is rules and needs no model. A rewrite does, and
 * goes the way every model call goes: through the router, which is where
 * privacy is enforced, to the model this machine keeps for work that
 * deserves care. The rewrite is then proofread and compared with what was
 * given, so the page shows what changed and anything still wrong — the
 * model's version is a draft until its owner keeps it.
 */

// Rewritten is a rewrite, checked.
type Rewritten struct {
	Text   string       `json:"text"`
	Model  string       `json:"model"`
	Report tools.Report `json:"report"`
	Diff   proof.Diff   `json:"diff"`
}

// ErrNoModel is a rewrite asked for with no model to write it.
var ErrNoModel = errors.New("no language model is available to rewrite with")

// Rewrite rewrites text in tone, in the text's own language, in at most
// most words when most is above zero.
func (b *Brain) Rewrite(ctx context.Context, text, tone, lang string, most int) (Rewritten, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Rewritten{}, errors.New("there is no text to rewrite")
	}

	preset, ok := proof.ToneNamed(tone)
	if !ok {
		return Rewritten{}, fmt.Errorf("%q is not a tone; the tones are formal, friendly, concise, persuasive, simple and polite", tone)
	}

	if lang != "en" && lang != "bg" {
		lang = proof.Language(text)
	}

	language := map[string]string{"en": "English", "bg": "Bulgarian"}[lang]

	system := preset.BriefIn(lang) + "\n" +
		"Write in " + language + ", the language of the text. Keep every fact, name, number, date and link exactly. " +
		"Add nothing that is not in the text. Keep paragraphs and lists as they are."

	if lang == "bg" {
		system += " Follow Bulgarian spelling and punctuation rules: в/във, с/със, full and short article, commas before че, който, защото, но; quotation marks „ “."
	}

	if most > 0 {
		system += fmt.Sprintf(" Use at most %d words.", most)
	}

	system += "\nReply with the rewritten text only — no title, no comment, no quotation marks round it."

	said, model, err := b.writeWithModel(ctx, system, text, 5*time.Minute)
	if err != nil {
		return Rewritten{}, err
	}

	return Rewritten{
		Text:   said,
		Model:  model,
		Report: tools.ProofReport(said, lang, preset.Name, most),
		Diff:   proof.Compare(text, said),
	}, nil
}

/*
 * writeWithModel asks the model kept for careful work to write something,
 * through the router, and gives back its answer tidied of the wrappers small
 * models put round one. Shared by the rewrite and by translation.
 */
func (b *Brain) writeWithModel(ctx context.Context, system, user string, within time.Duration) (string, string, error) {
	if b.Router == nil {
		return "", "", ErrNoModel
	}

	provider, err := b.Router.Provider(b.Cfg.DefaultProvider)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrNoModel, err)
	}

	roles := b.modelRoles()

	model := roles.Best
	if model == "" {
		model = roles.Work
	}

	ctx, stop := context.WithTimeout(ctx, within)
	defer stop()

	reply, err := provider.Chat(ctx, llm.Request{
		Model: model,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: system},
			{Role: llm.RoleUser, Content: user},
		},
	})
	if err != nil {
		return "", model, err
	}

	said := strings.TrimSpace(unwrap(reply.Content))
	if said == "" {
		return "", model, errors.New("the model gave back nothing")
	}

	return said, model, nil
}

// unwrap takes off a fenced block or a leading "Here is…" line a small model
// puts round the text it was asked for.
func unwrap(s string) string {
	s = strings.TrimSpace(s)

	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		if nl := strings.Index(s, "\n"); nl >= 0 {
			s = s[nl+1:]
		}

		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}

	if first, rest, found := strings.Cut(s, "\n"); found {
		lower := strings.ToLower(strings.TrimSpace(first))
		if (strings.HasPrefix(lower, "here is") || strings.HasPrefix(lower, "here's") ||
			strings.HasPrefix(lower, "ето ")) && strings.HasSuffix(lower, ":") {
			s = strings.TrimSpace(rest)
		}
	}

	return s
}

/*
 * translator is a translator as things stand now: the model through the
 * router, and the glossary read fresh, so a term added a moment ago is used
 * by the next sentence translated.
 */
func (b *Brain) translator() translate.Translator {
	terms, err := glossary.Load(b.Root)
	if err != nil {
		b.Log.Warn("the glossary could not be read, so translation goes without it", "error", err)
	}

	return translate.Translator{
		Glossary: terms,
		Engine: func(ctx context.Context, system, user string) (string, error) {
			said, _, err := b.writeWithModel(ctx, system, user, 10*time.Minute)

			return said, err
		},
	}
}

// Translator is the translator the pages use.
func (b *Brain) Translator() translate.Translator { return b.translator() }
