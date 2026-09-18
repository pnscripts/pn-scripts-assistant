package workspace

import (
	"fmt"
	"strings"

	"pn-scripts-assistant/internal/brain/engines"
)

/*
 * Starting points for work that is not a game engine's: a book and a plan.
 *
 * A folder layout and a few files that say what goes where, so the first
 * chapter lands in the manuscript and the outline stays beside it. An engine's
 * own new project comes from its adapter; these are for everything else.
 */

// Template is the files a kind of project starts with, or false.
func Template(name, title string) ([]engines.File, bool) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Untitled"
	}

	switch name {
	case "book":
		return []engines.File{
			{Path: "README.md", Content: []byte(fmt.Sprintf("# %s\n\nA book. The chapters are in manuscript/, one file each, "+
				"named so they sort in order; the outline and the people in it are in notes/.\n", title))},
			{Path: "notes/outline.md", Content: []byte("# Outline\n\nWhat happens, to whom, and why it matters — chapter by chapter.\n\n## Chapter 1\n\n")},
			{Path: "notes/characters.md", Content: []byte("# Characters\n\nWho they are, what they want, and what stands in the way.\n")},
			{Path: "manuscript/01.md", Content: []byte("# Chapter One\n\n")},
		}, true

	case "plan":
		return []engines.File{
			{Path: "README.md", Content: []byte(fmt.Sprintf("# %s\n\nPlans, checklists and the questions for a "+
				"qualified professional are in plans/. Nothing here is certified or approved; it is "+
				"preparation for somebody who can.\n", title))},
			{Path: "plans/questions.md", Content: []byte("# Questions for a qualified professional\n\n")},
		}, true
	}

	return nil, false
}
