package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pn-scripts-assistant/internal/brain/askclaude"
	"pn-scripts-assistant/internal/brain/protect"
)

/*
 * Asking the user's own Claude one question (the Developer role's "ask
 * Claude"): their Claude Code, installed and signed in by them, or the
 * Anthropic key they typed into this assistant; never a key of ours, never
 * one that is only in the environment.
 *
 * Every hand-off is put to the user, whatever else they allowed, and shows
 * which Claude, the question word for word and every file with its size.
 * The files are read by this program after the yes, compared with what was
 * shown, and given to Claude whole; Claude has no tools and reaches nothing
 * else. Its answer comes back as information, not instructions.
 *
 * Asked to save the answer, the same question names the new file it will be
 * saved as, and the answer is written there only if nothing is there yet:
 * a person approving "ask Claude to summarise notes.md and save it" sees
 * the whole of it on one card, and nothing is ever overwritten.
 */

// The ways to the user's own Claude.
const (
	ClaudeCodeWay = "claude-code"
	ClaudeKeyWay  = "anthropic-key"
)

// ClaudeWay is one way to reach the user's own Claude, as the brain finds it.
type ClaudeWay struct {
	ID    string `json:"id"`
	Title string `json:"title"`

	// TitleBG is the same in Bulgarian, for the approval card.
	TitleBG string `json:"title_bg,omitempty"`

	Ready bool   `json:"ready"`
	Why   string `json:"why,omitempty"`
}

// ClaudeAnswer is what Claude said.
type ClaudeAnswer struct {
	Who   string
	Model string
	Text  string
	Cut   bool
}

// ClaudeAsking is what the tool needs from the brain.
type ClaudeAsking interface {
	ClaudeWays() []ClaudeWay
	AskClaude(ctx context.Context, via, question string, docs []askclaude.Doc) (ClaudeAnswer, error)
}

/*
 * Unready is a tool that can tell, from a call alone, that it cannot run as
 * asked: the loop then says why at once instead of putting a question to
 * its user that could only fail after the yes.
 */
type Unready interface {
	Unready(args json.RawMessage) string
}

// AskClaude hands one question, and the files the user names, to their own
// Claude.
type AskClaude struct {
	Claude ClaudeAsking

	// Home is where a file named without a full path is looked for; empty
	// is the user's home folder.
	Home string

	// Protected is a file never given to Claude; nil is protect.IsSensitive
	// (keys, passwords, the assistant's own knowledge, anything its user
	// added).
	Protected func(string) bool
}

func (AskClaude) Name() string { return "ask_claude" }

func (AskClaude) Description() string {
	return "Ask the user's own Claude one question, giving it only the files the user names: their " +
		"Claude Code signed in on this computer, or the Anthropic key they typed into this assistant. " +
		"Claude gets no tools and sees nothing but the question and those files (text, whole). The user " +
		"approves every hand-off. Use it when the user asks Claude something, e.g. \"ask Claude to " +
		"summarise notes.md\"; when they also want the answer saved, give save_as (a new file). For work in a folder " +
		"(changing code, running things) use ask_coding_agent instead."
}

func (AskClaude) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"question": {"type": "string", "description": "The question, in full; Claude does not see this conversation."},
			"files": {"type": "array", "items": {"type": "string"}, "description": "The files to give it, as the user named them (a full path, ~/..., or relative to their home folder). Only these."},
			"save_as": {"type": "string", "description": "Only when the user asked for the answer to be saved: the new file to save it as (it must not exist yet)."},
			"via": {"type": "string", "enum": ["claude-code", "anthropic-key"], "description": "Default: Claude Code when it is signed in, else the key typed into this assistant."}
		},
		"required": ["question"]
	}`)
}

// Mutating, and asked every time (AlwaysAsks): what is sent leaves the
// machine in its user's name.
func (AskClaude) Risk() Risk { return Mutating }

func (AskClaude) Cues() []string {
	return []string{"claude", "клод", "anthropic"}
}

// LeavesTheMachine: the question and the files go to Anthropic, under the
// user's own sign-in or key.
func (AskClaude) LeavesTheMachine() bool { return true }

// Reaches says where.
func (AskClaude) Reaches(json.RawMessage) string {
	return "Claude (Anthropic), under your own sign-in or key"
}

// AlwaysAsks: every hand-off, whatever else is allowed.
func (AskClaude) AlwaysAsks(json.RawMessage) string {
	return "giving files and a question to Claude"
}

type claudeArgs struct {
	Question string            `json:"question"`
	Files    []string          `json:"files,omitempty"`
	SaveAs   string            `json:"save_as,omitempty"`
	Via      string            `json:"via,omitempty"`
	Seen     map[string]string `json:"seen,omitempty"`
}

func readClaudeArgs(raw json.RawMessage) claudeArgs {
	var a claudeArgs

	_ = json.Unmarshal(raw, &a)

	a.Via = strings.ToLower(strings.TrimSpace(a.Via))

	return a
}

func (t AskClaude) home() string {
	if t.Home != "" {
		return t.Home
	}

	home, _ := os.UserHomeDir()

	return home
}

func (t AskClaude) protected() func(string) bool {
	if t.Protected != nil {
		return t.Protected
	}

	return protect.IsSensitive
}

// way is the way a call goes: the one it names, or Claude Code when it is
// ready, else the key.
func (t AskClaude) way(name string) (ClaudeWay, error) {
	if t.Claude == nil {
		return ClaudeWay{}, errors.New("Claude cannot be asked from here")
	}

	ways := t.Claude.ClaudeWays()

	if name != "" {
		for _, w := range ways {
			if w.ID == name {
				if !w.Ready {
					return w, fmt.Errorf("%s cannot be used now: %s", w.Title, w.Why)
				}

				return w, nil
			}
		}

		return ClaudeWay{}, fmt.Errorf("there is no way called %q; it is %s or %s", name, ClaudeCodeWay, ClaudeKeyWay)
	}

	var why []string

	for _, w := range ways {
		if w.Ready {
			return w, nil
		}

		why = append(why, w.Title+": "+w.Why)
	}

	return ClaudeWay{}, errors.New("your own Claude cannot be asked from here (" + strings.Join(why, "; ") +
		"). Install Claude Code and sign in to it yourself, or type your own Anthropic key on the Models page")
}

// prepared is a call checked and its files read, or why it cannot go.
func (t AskClaude) prepared(raw json.RawMessage) (claudeArgs, ClaudeWay, []askclaude.Doc, error) {
	a := readClaudeArgs(raw)

	if err := askclaude.CheckQuestion(a.Question); err != nil {
		return a, ClaudeWay{}, nil, err
	}

	docs, err := askclaude.Gather(a.Files, t.home(), t.protected())
	if err != nil {
		return a, ClaudeWay{}, nil, err
	}

	if strings.TrimSpace(a.SaveAs) != "" {
		path, err := t.saveTarget(a.SaveAs)
		if err != nil {
			return a, ClaudeWay{}, nil, err
		}

		a.SaveAs = path
	}

	way, err := t.way(a.Via)
	if err != nil {
		return a, way, nil, err
	}

	return a, way, docs, nil
}

// saveTarget is where the answer may be saved: a new file, in a folder that
// exists, that is not protected.
func (t AskClaude) saveTarget(named string) (string, error) {
	path, err := askclaude.Resolve(named, t.home())
	if err != nil {
		return "", err
	}

	if t.protected()(path) {
		return "", fmt.Errorf("%s is protected and is not written to", path)
	}

	if _, err := os.Lstat(path); err == nil {
		return "", fmt.Errorf("%s already exists; the answer is only saved as a new file", path)
	}

	if info, err := os.Stat(filepath.Dir(path)); err != nil || !info.IsDir() {
		return "", fmt.Errorf("there is no folder %s to save the answer in", filepath.Dir(path))
	}

	return path, nil
}

// Touches: the file the answer is saved as, when it is.
func (t AskClaude) Touches(raw json.RawMessage) (writes []string, runsIn []string) {
	a := readClaudeArgs(raw)

	if strings.TrimSpace(a.SaveAs) == "" {
		return nil, nil
	}

	path, err := askclaude.Resolve(a.SaveAs, t.home())
	if err != nil {
		return []string{a.SaveAs}, nil
	}

	return []string{path}, nil
}

// Outward: saving the answer writes a file.
func (t AskClaude) Outward(raw json.RawMessage) []string {
	if strings.TrimSpace(readClaudeArgs(raw).SaveAs) == "" {
		return nil
	}

	return []string{"files"}
}

// Unready says, before anybody is asked, why a hand-off cannot go.
func (t AskClaude) Unready(raw json.RawMessage) string {
	if _, _, _, err := t.prepared(raw); err != nil {
		return err.Error()
	}

	return ""
}

/*
 * Pin ties the yes to what the question showed: the full path of every
 * file, its fingerprint as it was read, and the way. Whatever fingerprints
 * the model wrote itself are dropped.
 */
func (t AskClaude) Pin(raw json.RawMessage) json.RawMessage {
	a, way, docs, err := t.prepared(raw)
	if err != nil {
		// Nothing to pin; Execute refuses a call that carries no pin.
		a.Seen = nil
		out, _ := json.Marshal(a)

		return out
	}

	pinned := claudeArgs{Question: strings.TrimSpace(a.Question), Via: way.ID, Seen: askclaude.Sums(docs), SaveAs: a.SaveAs}

	for _, d := range docs {
		pinned.Files = append(pinned.Files, d.Path)
	}

	out, err := json.Marshal(pinned)
	if err != nil {
		return raw
	}

	return out
}

// Summarize is the question as its person reads it.
func (t AskClaude) Summarize(raw json.RawMessage) string { return t.SummarizeIn(raw, "en") }

func (t AskClaude) SummarizeIn(raw json.RawMessage, lang string) string {
	a, way, docs, err := t.prepared(raw)
	if err != nil {
		return Say(lang, "Ask Claude: ", "Въпрос към Claude: ") + strings.TrimSpace(a.Question) +
			Say(lang, "\n(This cannot be sent: ", "\n(Това не може да бъде изпратено: ") + err.Error() + ")"
	}

	var b strings.Builder

	fmt.Fprintf(&b, Say(lang, "Ask Claude through %s (it gets no tools and nothing else on this computer):\n%s\n",
		"Въпрос към Claude чрез %s (не получава инструменти и нищо друго от този компютър):\n%s\n"),
		wayTitleIn(way, lang), strings.TrimSpace(a.Question))

	if len(docs) == 0 {
		b.WriteString(Say(lang, "\nNo files: only the question is sent. Nothing else goes with it.",
			"\nБез файлове: изпраща се само въпросът. Нищо друго не се изпраща с него."))

		return b.String() + savedAsIn(a.SaveAs, lang)
	}

	total := 0
	for _, d := range docs {
		total += d.Size
	}

	fmt.Fprintf(&b, Say(lang, "\nFiles it is given, whole (%d, %s):", "\nФайлове, които получава изцяло (%d, %s):"),
		len(docs), sizeIn(total, lang))

	for _, d := range docs {
		fmt.Fprintf(&b, "\n- %s (%s)", d.Path, sizeIn(d.Size, lang))
	}

	b.WriteString(Say(lang, "\nNothing else goes with it.", "\nНищо друго не се изпраща с тях."))

	return b.String() + savedAsIn(a.SaveAs, lang)
}

// wayTitleIn is how Claude is reached, in the card's language.
func wayTitleIn(way ClaudeWay, lang string) string {
	if lang == "bg" && way.TitleBG != "" {
		return way.TitleBG
	}

	return way.Title
}

// savedAsIn is the card's line about saving the answer, in a language.
func savedAsIn(path, lang string) string {
	if path == "" {
		return ""
	}

	return Say(lang, "\n\nThen saves Claude's answer as a new file: ", "\n\nСлед това записва отговора на Claude като нов файл: ") +
		path + Say(lang, " (nothing is there now; nothing is overwritten).", " (там сега няма нищо; нищо не се презаписва).")
}

// sizeIn is a size as the card says it: askclaude.Size in English, with a
// decimal comma in Bulgarian.
func sizeIn(n int, lang string) string {
	if lang != "bg" {
		return askclaude.Size(n)
	}

	switch {
	case n == 1:
		return Say(lang, "1 byte", "1 байт")
	case n < 1024:
		return fmt.Sprintf(Say(lang, "%d bytes", "%d байта"), n)
	case n < 1<<20:
		return strings.Replace(fmt.Sprintf("%.1f", float64(n)/1024), ".", ",", 1) + Say(lang, " KB", " КБ")
	default:
		return strings.Replace(fmt.Sprintf("%.1f", float64(n)/(1<<20)), ".", ",", 1) + Say(lang, " MB", " МБ")
	}
}

func (t AskClaude) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	shown := readClaudeArgs(raw)

	if shown.Seen == nil || shown.Via == "" {
		return "", errors.New("this hand-off was not approved as shown; ask again so you see what goes to Claude")
	}

	a, way, docs, err := t.prepared(raw)
	if err != nil {
		return "", err
	}

	if way.ID != shown.Via {
		return "", errors.New("the way to Claude is not the one you were shown; ask again")
	}

	if changed := askclaude.Changed(docs, shown.Seen); len(changed) > 0 || len(docs) != len(shown.Seen) {
		return "", fmt.Errorf("%s changed after you were asked; ask again so you see what goes to Claude",
			strings.Join(orAll(changed, docs), ", "))
	}

	answer, err := t.Claude.AskClaude(ctx, way.ID, strings.TrimSpace(a.Question), docs)
	if err != nil {
		return "", err
	}

	who := answer.Who
	if who == "" || who == way.ID {
		who = way.Title
	}

	if answer.Model != "" {
		who += ", " + answer.Model
	}

	text, cut := askclaude.Bounded(answer.Text)
	marked := askclaude.Marked(who, text, docs, cut || answer.Cut)

	if a.SaveAs == "" {
		return marked, nil
	}

	if a.SaveAs != shown.SaveAs {
		return marked + "\n\nNot saved: the file is not the one you were shown.", nil
	}

	// A new file only: O_EXCL, so something that appeared meanwhile is
	// never overwritten.
	f, err := os.OpenFile(a.SaveAs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return marked + "\n\nNot saved as " + a.SaveAs + ": " + err.Error(), nil
	}

	_, werr := f.WriteString(text + "\n")
	cerr := f.Close()

	if werr != nil || cerr != nil {
		return marked + "\n\nSaving " + a.SaveAs + " failed: " + errors.Join(werr, cerr).Error(), nil
	}

	return marked + "\n\nSaved Claude's answer as a new file: " + a.SaveAs, nil
}

func orAll(changed []string, docs []askclaude.Doc) []string {
	if len(changed) > 0 {
		return changed
	}

	var all []string
	for _, d := range docs {
		all = append(all, d.Path)
	}

	if len(all) == 0 {
		return []string{"the files"}
	}

	return all
}
