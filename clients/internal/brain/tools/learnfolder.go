package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"pn-brain/internal/brain/learning"
)

/*
 * Learning a folder, which is the thing this program is for and had no tool.
 *
 * Asked to "update the brain based on everything inside /media/.../DEV", the
 * assistant listed the folders and offered to list more, because listing was
 * the whole of what it could do. Reading files one at a time and holding them
 * in a conversation is not learning: it lasts until the context is full and
 * then it is gone.
 *
 * The machinery existed the whole time and only the command line could reach
 * it — brain ingest <dir> scans, embeds and stores. A capability the owner can
 * only use by finding a terminal is one this program says it does not have.
 */
type LearnFolder struct {
	/*
	 * Learn is asked for the learner when the tool runs, not when it is built.
	 *
	 * A field would be filled from a brain that has not finished assembling
	 * itself — the tools are registered forty lines before the learner is
	 * created — and a nil *Worker stored in an interface is not a nil
	 * interface. The tool would then hold something that reads as present,
	 * pass its own nil check, and do nothing: called, timed, reported as
	 * having run, and storing not one of the sixty-two things it found.
	 */
	Learn func() Ingests

	// Owner is whose work this is, which is what the observations are about.
	Owner string
}

// Ingests is the part of the learner this tool needs.
type Ingests interface {
	Ingest(ctx context.Context, obs learning.Observations,
		progress func(learning.IngestReport)) (learning.IngestReport, error)
}

func (LearnFolder) Name() string { return "learn_from_folder" }

func (LearnFolder) Description() string {
	return "Read a folder and remember what is in it — the projects or documents it " +
		"holds, what they are and what they are written in — so it can be recalled in " +
		"later conversations. This is what to use when asked to learn, study, index, " +
		"take in or update yourself from a directory. Listing and reading files does " +
		"not remember anything; this does."
}

func (LearnFolder) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "The folder to learn, as a full path"},
			"kind": {
				"type": "string",
				"enum": ["projects", "documents"],
				"description": "projects for code and repositories, documents for text and papers. Defaults to projects."
			}
		},
		"required": ["path"],
		"additionalProperties": false
	}`)
}

/*
 * Safe: it reads and remembers, and changes nothing outside the brain's own
 * memory. Somebody asking it to learn a folder has asked for exactly this, and
 * a confirmation prompt in front of it would only teach them to click through.
 */
func (LearnFolder) Risk() Risk { return Safe }

func (LearnFolder) Summarize(args json.RawMessage) string {
	var a struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	}

	json.Unmarshal(args, &a)

	what := "projects"
	if a.Kind == "documents" {
		what = "documents"
	}

	return "Learn the " + what + " in " + a.Path
}

func (t LearnFolder) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Path string `json:"path"`
		Kind string `json:"kind"`
	}

	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("could not read the arguments: %w", err)
	}

	path := strings.TrimSpace(a.Path)
	if path == "" {
		return "", fmt.Errorf("which folder should be learned")
	}

	if info, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("cannot read %s: %w", path, err)
	} else if !info.IsDir() {
		return "", fmt.Errorf("%s is a file, not a folder", path)
	}

	if t.Learn == nil {
		return "", fmt.Errorf("there is nothing here that can learn")
	}

	learner := t.Learn()
	if learner == nil {
		return "", fmt.Errorf("the part of the brain that learns is not running")
	}

	var observations learning.Observations

	if a.Kind == "documents" {
		found, err := learning.ScanDocuments(path)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", path, err)
		}

		observations = learning.FromDocuments(found, t.Owner)
	} else {
		found, err := learning.ScanProjects(path)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", path, err)
		}

		observations = learning.FromProjects(found, t.Owner)
	}

	if len(observations) == 0 {
		return "Nothing in " + path + " to learn — no projects or documents were found there.", nil
	}

	report, err := learner.Ingest(ctx, observations, nil)
	if err != nil {
		return "", fmt.Errorf("learning %s: %w", path, err)
	}

	/*
	 * Said in the terms somebody asked in: how much is now known, not how many
	 * rows were written. "Already knew" matters because learning the same
	 * folder twice is a reasonable thing to do and reporting it as nothing
	 * learned reads as a failure.
	 */
	out := fmt.Sprintf("Learned %s: %d things seen, %d newly remembered, %d already known.",
		path, report.Seen, report.Promoted, report.Duplicates)

	if report.Waiting > 0 {
		out += fmt.Sprintf(" %d are waiting for you to confirm in the app.", report.Waiting)
	}

	return out, nil
}
