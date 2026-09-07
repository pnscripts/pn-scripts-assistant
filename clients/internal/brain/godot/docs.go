package godot

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

/*
 * The class reference, looked up rather than remembered.
 *
 * A model asked to write GDScript will write GDScript whether or not it knows
 * the engine. It produces method names that read exactly like real ones —
 * get_node_or_null, is_on_floor, connect — and some of them are, and some are
 * not, and the difference only appears when the game is run. That is the worst
 * shape of wrong answer: fluent, plausible, and expensive to check.
 *
 * Learning the whole reference would not fix it. There are over a thousand
 * classes, it changes every release, and a fact recalled by similarity is
 * exactly as confident when it is stale. Looking one up is better on every
 * count: it is authoritative, it costs a second, and it is checkable — the
 * answer either has the method in it or it does not.
 *
 * The reference is one XML file per class in the engine's own repository,
 * which is the source the website is built from.
 */

// Where the class reference lives. The branch matters: a project on 4.3 wants
// 4.3's reference, and master documents things that are not released.
const referenceBase = "https://raw.githubusercontent.com/godotengine/godot"

// Class is what the reference says about one class.
type Class struct {
	Name     string   `xml:"name,attr"`
	Inherits string   `xml:"inherits,attr"`
	Brief    string   `xml:"brief_description"`
	Describe string   `xml:"description"`
	Methods  []Method `xml:"methods>method"`
	Members  []Member `xml:"members>member"`
	Signals  []Signal `xml:"signals>signal"`
}

type Method struct {
	Name       string  `xml:"name,attr"`
	Qualifiers string  `xml:"qualifiers,attr"`
	Return     Typed   `xml:"return"`
	Params     []Param `xml:"param"`
	Describe   string  `xml:"description"`
}

type Typed struct {
	Type string `xml:"type,attr"`
}

type Param struct {
	Name    string `xml:"name,attr"`
	Type    string `xml:"type,attr"`
	Default string `xml:"default,attr"`
}

type Member struct {
	Name     string `xml:"name,attr"`
	Type     string `xml:"type,attr"`
	Default  string `xml:"default,attr"`
	Describe string `xml:",chardata"`
}

type Signal struct {
	Name     string  `xml:"name,attr"`
	Params   []Param `xml:"param"`
	Describe string  `xml:"description"`
}

/*
 * Lookup fetches one class's reference.
 *
 * The branch is taken from the installed engine's version when there is one,
 * so the answer matches the engine that will run the code rather than whatever
 * is being written now. A project on 4.2 given master's reference is being
 * told about methods it does not have, which is the failure this exists to
 * prevent, arriving by a different door.
 */
func Lookup(ctx context.Context, client *http.Client, class, branch string) (Class, error) {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}

	class = strings.TrimSpace(class)

	if class == "" {
		return Class{}, fmt.Errorf("which class?")
	}

	// A name, not a path: this becomes part of a URL and nothing else here
	// checks it.
	if strings.ContainsAny(class, "/\\?#. ") {
		return Class{}, fmt.Errorf("%q is not a class name", class)
	}

	if branch == "" {
		branch = "master"
	}

	url := fmt.Sprintf("%s/%s/doc/classes/%s.xml", referenceBase, branch, class)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Class{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return Class{}, fmt.Errorf("could not reach the class reference: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// The likeliest reason by far, and worth saying: the class reference
		// is case-sensitive and everything in it is CamelCase.
		return Class{}, fmt.Errorf(
			"there is no class called %q in Godot %s — names are case-sensitive "+
				"and written like CharacterBody2D", class, branch)
	}

	if resp.StatusCode != http.StatusOK {
		return Class{}, fmt.Errorf("the class reference answered %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Class{}, err
	}

	var c Class

	if err := xml.Unmarshal(raw, &c); err != nil {
		return Class{}, fmt.Errorf("could not read the reference for %s: %w", class, err)
	}

	return c, nil
}

/*
 * BranchFor turns an engine version into the branch its reference is on.
 *
 * "4.3.stable.official" becomes "4.3", which is what the repository calls that
 * release's branch. Anything unrecognised is master, because a wrong branch is
 * a 404 and a missing branch is only slightly newer documentation.
 */
func BranchFor(version string) string {
	m := number.FindStringSubmatch(version)
	if m == nil {
		return "master"
	}

	return m[1] + "." + m[2]
}

/*
 * Summary is the class written out for somebody about to use it.
 *
 * Signatures first and prose second, because the question is nearly always
 * "what is it called and what does it take". The description is the part a
 * model can already guess; the exact spelling of the parameters is the part it
 * cannot.
 */
func (c Class) Summary(most int) string {
	if most <= 0 {
		most = 40
	}

	var b strings.Builder

	fmt.Fprintf(&b, "%s", c.Name)

	if c.Inherits != "" {
		fmt.Fprintf(&b, " (inherits %s)", c.Inherits)
	}

	if brief := tidy(c.Brief); brief != "" {
		fmt.Fprintf(&b, "\n%s", brief)
	}

	if len(c.Members) > 0 {
		b.WriteString("\n\nProperties:")

		for i, m := range c.Members {
			if i >= most {
				fmt.Fprintf(&b, "\n  … and %d more", len(c.Members)-i)

				break
			}

			fmt.Fprintf(&b, "\n  %s: %s", m.Name, m.Type)

			if m.Default != "" {
				fmt.Fprintf(&b, " = %s", m.Default)
			}
		}
	}

	if len(c.Methods) > 0 {
		b.WriteString("\n\nMethods:")

		for i, m := range c.Methods {
			if i >= most {
				fmt.Fprintf(&b, "\n  … and %d more", len(c.Methods)-i)

				break
			}

			fmt.Fprintf(&b, "\n  %s", m.Signature())
		}
	}

	if len(c.Signals) > 0 {
		b.WriteString("\n\nSignals:")

		for i, s := range c.Signals {
			if i >= most {
				fmt.Fprintf(&b, "\n  … and %d more", len(c.Signals)-i)

				break
			}

			fmt.Fprintf(&b, "\n  %s(%s)", s.Name, params(s.Params))
		}
	}

	return b.String()
}

// Signature is the method as it would be written in GDScript.
func (m Method) Signature() string {
	out := fmt.Sprintf("%s(%s)", m.Name, params(m.Params))

	if m.Return.Type != "" && m.Return.Type != "void" {
		out += " -> " + m.Return.Type
	}

	if m.Qualifiers != "" {
		out += " " + m.Qualifiers
	}

	return out
}

func params(list []Param) string {
	parts := make([]string, 0, len(list))

	for _, p := range list {
		one := p.Name + ": " + p.Type

		if p.Default != "" {
			one += " = " + p.Default
		}

		parts = append(parts, one)
	}

	return strings.Join(parts, ", ")
}

/*
 * Find returns the one method whose name matches, for the narrower question.
 *
 * "Does CharacterBody2D have move_and_slide" is a different question from
 * "what is CharacterBody2D", and answering the second when the first was asked
 * buries the answer in four hundred lines.
 */
func (c Class) Find(name string) (Method, bool) {
	name = strings.TrimSpace(name)

	for _, m := range c.Methods {
		if strings.EqualFold(m.Name, name) {
			return m, true
		}
	}

	return Method{}, false
}

// tidy strips the reference's own markup, which is BBCode rather than XML and
// so survives the parser: [method start], [code]true[/code].
func tidy(s string) string {
	s = strings.TrimSpace(s)

	for _, tag := range []string{
		"[code]", "[/code]", "[b]", "[/b]", "[i]", "[/i]",
		"[codeblock]", "[/codeblock]", "[gdscript]", "[/gdscript]",
		"[csharp]", "[/csharp]",
	} {
		s = strings.ReplaceAll(s, tag, "")
	}

	// [method X], [member Y], [Class] all read fine as their contents.
	for _, prefix := range []string{"[method ", "[member ", "[signal ", "[constant ", "[param ", "[enum ", "[annotation "} {
		s = strings.ReplaceAll(s, prefix, "")
	}

	s = strings.ReplaceAll(s, "[", "")
	s = strings.ReplaceAll(s, "]", "")

	return strings.Join(strings.Fields(s), " ")
}
