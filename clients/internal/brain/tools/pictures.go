package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"pn-scripts-assistant/internal/brain/pictures"
)

/*
 * Making pictures, and films out of pictures.
 *
 * Both Mutating. They write files, they take minutes of this machine, and one
 * of them may spend somebody's money at a paid service — any of which is
 * enough on its own. The summary says which way it is about to go, because
 * "make a picture of the new logo" going to a company's server and going to a
 * program on this computer are not the same act, and the difference is the one
 * thing somebody approving it needs to know.
 */

// Studio is what the picture tools need from the brain. An interface rather
// than the brain itself, because a tool that imported the brain would be a
// tool the brain could not hold.
type Studio interface {
	// Painter is who may make a picture right now, given the privacy setting
	// and what is installed. The error says what to do about it.
	Painter() (pictures.Painter, error)

	// PicturesFolder is where they go.
	PicturesFolder() string
}

type MakeAPicture struct {
	Studio Studio
}

func (MakeAPicture) Name() string { return "make_a_picture" }

func (MakeAPicture) Description() string {
	return "Make a picture from a description and save it as a file. Use when asked to " +
		"draw, design, illustrate, or make an image or a logo."
}

func (MakeAPicture) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"what": {"type": "string", "description": "What the picture should be of, described in full. More detail gives a better picture."},
			"wide": {"type": "boolean", "description": "True for a wide picture rather than a square one."}
		},
		"required": ["what"]
	}`)
}

func (MakeAPicture) Risk() Risk { return Mutating }

type pictureArgs struct {
	What string `json:"what"`
	Wide bool   `json:"wide"`
}

func (t MakeAPicture) Summarize(raw json.RawMessage) string {
	var a pictureArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "Make a picture"
	}

	/*
	 * Where it is about to go, said in the summary.
	 *
	 * A description of what somebody wants a picture of is often the most
	 * revealing sentence they will write all week, and whether it stays on
	 * this machine is the one thing worth knowing before saying yes.
	 */
	where := "on this machine"

	if painter, err := t.Studio.Painter(); err == nil {
		where = painter.Name()

		if !strings.Contains(where, "this machine") {
			where = "by " + where + ", which means sending the description there"
		}
	}

	return "Make a picture " + where + ":\n  " + a.What
}

func (t MakeAPicture) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a pictureArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("could not read what to draw: %w", err)
	}

	if strings.TrimSpace(a.What) == "" {
		return "", fmt.Errorf("say what the picture should be of")
	}

	painter, err := t.Studio.Painter()
	if err != nil {
		return "", err
	}

	data, err := painter.Paint(ctx, a.What, a.Wide)
	if err != nil {
		return "", err
	}

	path, err := pictures.Save(t.Studio.PicturesFolder(), a.What, data)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Made by %s and saved as %s (%d KB).",
		painter.Name(), path, len(data)/1024), nil
}

/*
 * MakeAVideo puts pictures together into a film.
 *
 * Not a model that generates moving pictures, and the description says so.
 * Running one of those locally needs hardware this program does not assume and
 * tens of gigabytes of downloads, and the paid ones bill by the second — so a
 * tool that quietly meant either would be one that never works or one that
 * surprises somebody with an invoice.
 */
type MakeAVideo struct {
	Studio Studio
}

func (MakeAVideo) Name() string { return "make_a_video" }

func (MakeAVideo) Description() string {
	return "Put pictures together into a video file, each held for a few seconds, " +
		"optionally with music. Use when asked to make a video, a film or a slideshow " +
		"out of images. It does not generate moving footage — it assembles pictures " +
		"that already exist, so make them first if there are none."
}

func (MakeAVideo) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pictures": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Absolute paths to the image files, in the order they should appear."
			},
			"seconds": {"type": "number", "description": "How long each picture is held. Three if not given."},
			"move": {"type": "boolean", "description": "True to drift slowly across each picture rather than holding it still."},
			"sound": {"type": "string", "description": "An audio file to lay underneath, if there is one."}
		},
		"required": ["pictures"]
	}`)
}

func (MakeAVideo) Risk() Risk { return Mutating }

type videoArgs struct {
	Pictures []string `json:"pictures"`
	Seconds  float64  `json:"seconds"`
	Move     bool     `json:"move"`
	Sound    string   `json:"sound"`
}

func (MakeAVideo) Summarize(raw json.RawMessage) string {
	var a videoArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "Make a video"
	}

	names := make([]string, 0, len(a.Pictures))

	for _, p := range a.Pictures {
		names = append(names, filepath.Base(p))
	}

	out := fmt.Sprintf("Make a video from %d pictures: %s",
		len(a.Pictures), strings.Join(names, ", "))

	if a.Sound != "" {
		out += "\nwith " + filepath.Base(a.Sound) + " underneath"
	}

	return out
}

func (t MakeAVideo) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a videoArgs

	if err := json.Unmarshal(raw, &a); err != nil {
		return "", fmt.Errorf("could not read what to put together: %w", err)
	}

	path, err := pictures.MakeIt(ctx, t.Studio.PicturesFolder(), pictures.Film{
		Pictures: a.Pictures,
		Seconds:  a.Seconds,
		Move:     a.Move,
		Sound:    a.Sound,
	})
	if err != nil {
		return "", err
	}

	return "Made and saved as " + path + ".", nil
}
