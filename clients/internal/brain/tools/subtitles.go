package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/subtitles"
)

/*
 * Writing subtitles for a film that has none.
 *
 * Petar asked whether the brain should learn subtitles, make them, and learn
 * from them. Making them is the part that is unambiguously worth having: 658
 * of the 866 films on his drive have none beside them, the machine already has
 * ffmpeg and a speech recogniser, and the answer is a file his player will
 * pick up without being told.
 *
 * Learning from what they say is the part that is not, and it is refused
 * elsewhere rather than here. See learning/subtitles.go.
 *
 * Mutating, because it writes a file into somebody's film folder. Everything
 * else in this program that touches their disk asks first and so does this,
 * even though what it writes is only ever a new file beside an existing one.
 */

// MakeSubtitles writes an .srt for a film.
type MakeSubtitles struct {
	// Recogniser is the whisper binary and the model it should use, asked for
	// when the tool runs rather than when it is built — the same reasoning as
	// LearnFolder.Learn, and the same failure if it is a field.
	Recogniser func() (command, model, language string)
}

func (MakeSubtitles) Name() string { return "make_subtitles" }

func (MakeSubtitles) Description() string {
	return "Write subtitles for a film that has none, by listening to it. " +
		"Give the path to one film. It takes one to three hours for a feature " +
		"on this machine, so it runs in the background. Use when asked to " +
		"subtitle a film, or to find which films have no subtitles."
}

func (MakeSubtitles) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"film": {
				"type": "string",
				"description": "Full path to one film file."
			},
			"language": {
				"type": "string",
				"description": "Optional ISO code of the language spoken in it (bg, en, de). Empty lets the recogniser decide."
			}
		},
		"required": ["film"],
		"additionalProperties": false
	}`)
}

// Mutating: it writes a file into somebody's folder.
func (MakeSubtitles) Risk() Risk { return Mutating }

func (MakeSubtitles) Summarize(args json.RawMessage) string {
	var a struct {
		Film string `json:"film"`
	}

	json.Unmarshal(args, &a)

	if a.Film == "" {
		return "Write subtitles for a film"
	}

	// The summary is what somebody approves, so it says exactly which file
	// appears and where, not "make subtitles".
	return fmt.Sprintf("Listen to %s and write %s beside it",
		filepath.Base(a.Film), filepath.Base(subtitles.Path(a.Film)))
}

func (t MakeSubtitles) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Film     string `json:"film"`
		Language string `json:"language"`
	}

	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("could not read the arguments: %w", err)
	}

	film := strings.TrimSpace(a.Film)
	if film == "" {
		return "", fmt.Errorf("which film?")
	}

	if !subtitles.Kinds[strings.ToLower(filepath.Ext(film))] {
		return "", fmt.Errorf("%s is not a film I know how to listen to", filepath.Base(film))
	}

	command, model, language := "", "", ""

	if t.Recogniser != nil {
		command, model, language = t.Recogniser()
	}

	if a.Language != "" {
		language = a.Language
	}

	progress.SetBackground("transcribing", "Listening to "+filepath.Base(film))
	defer progress.Done()

	written, err := subtitles.Make(ctx, film, command, model, language, func(note string) {
		progress.SetBackground("transcribing", note)
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Written %s. Your player will find it beside the film.", written), nil
}

/*
 * FilmsWithoutSubtitles answers the question that comes before the work.
 *
 * Safe, and separate from making them, because "which of my films have no
 * subtitles" is a question somebody asks without wanting to start a month of
 * transcription — and because the answer is what tells them whether to ask for
 * one film or to leave the whole thing alone.
 */
type FilmsWithoutSubtitles struct {
	// Places is where the brain is allowed to look, so this cannot be pointed
	// at a folder nobody agreed to.
	Places func() []string
}

func (FilmsWithoutSubtitles) Name() string { return "films_without_subtitles" }

func (FilmsWithoutSubtitles) Description() string {
	return "List the films that have no subtitle file beside them. " +
		"Use before offering to make any, and when asked what has no subtitles."
}

func (FilmsWithoutSubtitles) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"folder": {
				"type": "string",
				"description": "Optional folder to look in. Defaults to every place the brain watches."
			}
		},
		"additionalProperties": false
	}`)
}

func (FilmsWithoutSubtitles) Risk() Risk { return Safe }

func (FilmsWithoutSubtitles) Summarize(json.RawMessage) string {
	return "List films with no subtitles"
}

// MostFilmsToName is how many are listed by name before it stops.
//
// The count is the useful part of the answer and the names are for choosing
// one; a list of 658 filenames in a conversation is neither.
const MostFilmsToName = 15

func (t FilmsWithoutSubtitles) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Folder string `json:"folder"`
	}

	json.Unmarshal(args, &a)

	roots := []string{}

	if a.Folder != "" {
		roots = append(roots, a.Folder)
	} else if t.Places != nil {
		roots = t.Places()
	}

	if len(roots) == 0 {
		return "", fmt.Errorf("there is nowhere I am allowed to look for films")
	}

	var missing []subtitles.Film

	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		found, err := subtitles.Missing(root)
		if err != nil {
			continue
		}

		missing = append(missing, found...)
	}

	if len(missing) == 0 {
		return "Every film I can see already has subtitles beside it.", nil
	}

	var b strings.Builder

	fmt.Fprintf(&b, "%d films have no subtitles beside them.", len(missing))

	if len(missing) > MostFilmsToName {
		fmt.Fprintf(&b, " The first %d:", MostFilmsToName)
	} else {
		b.WriteString(" They are:")
	}

	for i, f := range missing {
		if i >= MostFilmsToName {
			break
		}

		fmt.Fprintf(&b, "\n  %s\n    %s", f.Name, f.Path)
	}

	// The cost, because it is the whole of the decision. Somebody who is not
	// told will ask for all of them.
	b.WriteString("\n\nEach one is one to three hours of listening on this machine, " +
		"so they are done one at a time and only when asked for by name.")

	return b.String(), nil
}

// Touches is the folder the subtitles are written to.
func (MakeSubtitles) Touches(raw json.RawMessage) ([]string, []string) {
	var a struct {
		Folder string `json:"folder"`
	}

	json.Unmarshal(raw, &a)

	if a.Folder == "" {
		return []string{"/"}, nil
	}

	return []string{a.Folder}, nil
}
