/*
 * Package skills is what its owner has taught the assistant to do.
 *
 * A skill is instructions, not capability. Asked for, it returns the way this
 * person wants a particular job done — and every actual action in it still
 * goes through the ordinary tools, each with the risk it always had. So a
 * skill can never do anything the assistant could not already do: it changes
 * what it knows, not what it may do, and teaching one cannot widen the gate.
 *
 * That property is why a skill is Safe and why teaching one is not. Writing
 * the file changes the machine and stops for approval; reading it back on a
 * later turn does not need to ask anybody anything.
 *
 * They are plain files in the brain's own folder, so they travel with the
 * brain and can be opened, edited and version-controlled by hand. Taught in
 * conversation or written in an editor, they are the same thing either way —
 * which is the whole point, because the ones worth keeping get corrected.
 */
package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"pn-scripts-assistant/internal/brain/tools"
)

// FolderName is where they live inside the brain's folder.
const FolderName = "skills"

// MostRunes caps one skill. Long enough for a real procedure, short enough
// that reading it is not the whole of the turn.
const MostRunes = 6000

/*
 * Names are narrow on purpose.
 *
 * A skill's name is what the model calls, so it lives in the same namespace as
 * read_file and send_email. Anything that could be mistaken for one of those,
 * or that needs quoting, or that differs from another only by case, is a name
 * that will eventually be typed wrongly by a model at three in the morning.
 */
var validName = regexp.MustCompile(`^[a-z][a-z0-9_]{2,40}$`)

// Skill is one thing its owner has taught it.
type Skill struct {
	// Name is what the model calls. Lower case, words joined by underscores.
	Name string `json:"name"`

	// When says what it is for, in the owner's words. It becomes the
	// description the model reads, and the cues that decide when it is
	// offered at all.
	When string `json:"when"`

	// How is the instructions themselves.
	How string `json:"how"`

	// Taught is true when it was written by the assistant rather than by hand.
	// Shown in the interface, so somebody can tell the two apart.
	Taught bool `json:"taught,omitempty"`
}

// Folder is where the skills live for a given brain.
func Folder(root string) string { return filepath.Join(root, FolderName) }

// Path is the file one skill lives in.
func Path(root, name string) string {
	return filepath.Join(Folder(root), name+".md")
}

// CheckName says why a name will not do, or nothing.
func CheckName(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf(
			"a skill's name must be three to forty lower-case letters, digits or " +
				"underscores, starting with a letter — for example invoicing or new_client")
	}

	return nil
}

/*
 * Load reads every skill in a brain.
 *
 * A file that will not parse is skipped rather than failing the lot. These are
 * hand-editable by design, so one of them being half-written is an ordinary
 * Tuesday and is not a reason for the other nine to stop working.
 */
func Load(root string) ([]Skill, error) {
	entries, err := os.ReadDir(Folder(root))

	if os.IsNotExist(err) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("reading the skills folder: %w", err)
	}

	var out []Skill

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(Folder(root), entry.Name()))
		if err != nil {
			continue
		}

		skill := parse(string(raw), strings.TrimSuffix(entry.Name(), ".md"))

		if CheckName(skill.Name) != nil || strings.TrimSpace(skill.How) == "" {
			continue
		}

		out = append(out, skill)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out, nil
}

// One reads a single skill by name.
func One(root, name string) (*Skill, error) {
	raw, err := os.ReadFile(Path(root, name))

	if os.IsNotExist(err) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	skill := parse(string(raw), name)

	return &skill, nil
}

/*
 * parse reads the small header and the body.
 *
 * Forgiving on purpose. A file with no header at all is still a skill — its
 * name comes from the filename and it is simply offered whenever anything is
 * asked — because somebody who drops a text file in this folder has clearly
 * said what they meant, and refusing it over a missing colon would be the
 * program being pedantic about its own format.
 */
func parse(raw, filename string) Skill {
	skill := Skill{Name: filename}

	header, body, found := strings.Cut(raw, "\n---")

	if !found {
		skill.How = strings.TrimSpace(raw)

		return skill
	}

	for _, line := range strings.Split(header, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		value = strings.TrimSpace(value)

		switch strings.ToLower(strings.TrimSpace(key)) {
		case "name":
			if value != "" {
				skill.Name = value
			}
		case "when":
			skill.When = value
		case "taught":
			skill.Taught = value == "yes" || value == "true"
		}
	}

	skill.How = strings.TrimSpace(strings.TrimLeft(body, "-\n"))

	return skill
}

// Save writes a skill, replacing one of the same name.
func Save(root string, skill Skill) error {
	if err := CheckName(skill.Name); err != nil {
		return err
	}

	skill.How = strings.TrimSpace(skill.How)

	if skill.How == "" {
		return fmt.Errorf("a skill with no instructions in it would do nothing")
	}

	if utf8.RuneCountInString(skill.How) > MostRunes {
		skill.How = string([]rune(skill.How)[:MostRunes])
	}

	if err := os.MkdirAll(Folder(root), 0o700); err != nil {
		return fmt.Errorf("making the skills folder: %w", err)
	}

	var b strings.Builder

	b.WriteString("name: " + skill.Name + "\n")
	b.WriteString("when: " + strings.TrimSpace(skill.When) + "\n")

	if skill.Taught {
		b.WriteString("taught: yes\n")
	}

	b.WriteString("---\n\n")
	b.WriteString(skill.How)
	b.WriteString("\n")

	// Written whole and moved into place, like everything else in this folder.
	temp := Path(root, skill.Name) + ".new"

	if err := os.WriteFile(temp, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("writing the skill: %w", err)
	}

	return os.Rename(temp, Path(root, skill.Name))
}

// Remove deletes a skill. Removing one that is not there is not an error.
func Remove(root, name string) error {
	if err := CheckName(name); err != nil {
		return err
	}

	err := os.Remove(Path(root, name))

	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

/*
 * asTool is one skill, offered to the model.
 *
 * Safe, and that is not a shortcut. What comes back is text — the way its
 * owner wants this job done — and every action described in it is carried out
 * afterwards by the ordinary tools, each stopping for approval exactly as it
 * would have done anyway. A skill that could act would be a way of writing new
 * capability into the machine without passing the gate, which is precisely
 * what this must not be.
 */
type asTool struct{ skill Skill }

// AsTool wraps a skill so the registry can hold it.
func AsTool(skill Skill) tools.Tool { return asTool{skill: skill} }

func (t asTool) Name() string { return t.skill.Name }

func (t asTool) Description() string {
	when := strings.TrimSpace(t.skill.When)

	if when == "" {
		return "How " + t.skill.Name + " is done here. Ask for this before doing it."
	}

	return "How this is done here. Use it when: " + when
}

func (t asTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

// Safe: it returns instructions and changes nothing. See the type's comment.
func (t asTool) Risk() tools.Risk { return tools.Safe }

func (t asTool) Summarize(json.RawMessage) string {
	return "Read how " + t.skill.Name + " is done"
}

func (t asTool) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return "How " + t.skill.Name + " is done here. Follow this, using your tools " +
		"as usual — anything that changes something still needs approval.\n\n" +
		t.skill.How, nil
}

// AddedAtRunTime marks this as replaceable, so re-teaching a skill replaces it
// rather than being refused. Nothing built in says this.
func (t asTool) AddedAtRunTime() {}

/*
 * Cues are the words that mean this skill is worth offering.
 *
 * Taken from what its owner said it is for, plus its own name. Without this
 * every skill would be offered on every turn, and how well a model chooses
 * among tools falls off sharply with its size — the small model here already
 * picks wrongly among thirty-one, and a dozen skills would make it worse at
 * everything rather than better at one thing.
 */
func (t asTool) Cues() []string {
	out := []string{strings.ReplaceAll(t.skill.Name, "_", " "), t.skill.Name}

	for _, word := range strings.FieldsFunc(strings.ToLower(t.skill.When), notALetter) {
		if len(word) > 3 && !tooCommon[word] {
			out = append(out, word)
		}
	}

	return out
}

func notALetter(r rune) bool {
	return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r < 128
}

// tooCommon are words that appear in any sentence and would make a skill match
// everything, which is the same as having no cues at all.
var tooCommon = map[string]bool{
	"about": true, "after": true, "anything": true, "asks": true, "ask": true,
	"been": true, "before": true, "being": true, "does": true, "doing": true,
	"from": true, "have": true, "into": true, "just": true, "like": true,
	"make": true, "more": true, "must": true, "need": true, "over": true,
	"same": true, "should": true, "some": true, "something": true, "that": true,
	"them": true, "then": true, "they": true, "thing": true, "things": true,
	"this": true, "want": true, "wants": true, "what": true, "when": true,
	"where": true, "which": true, "with": true, "would": true, "your": true,
}
