package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

/*
 * Looking at the screen.
 *
 * Taking the picture is the easy half and on its own it is worth nothing: the
 * model that holds the conversation reads text and cannot see an image, so a
 * screenshot handed to it is a file path and a shrug.
 *
 * The other half is that this machine already has a model that can see —
 * gemma3, which ollama serves with images alongside the prompt. So the picture
 * goes to that model with the question, and what comes back is a description
 * the conversation model can actually use. Two models, one answer, and nothing
 * leaves the machine.
 */

// LookAtScreen takes a picture of the screen and describes it.
type LookAtScreen struct {
	// OllamaURL is where the vision model is served.
	OllamaURL string

	// Model is the vision model to ask. Empty means whichever of the known
	// ones is installed.
	Model string
}

func (LookAtScreen) Name() string { return "look_at_screen" }

func (LookAtScreen) Description() string {
	return "Take a picture of the screen and describe what is on it. Use when " +
		"asked what is on screen, what an error says, what a window is showing, " +
		"or to read something the owner is looking at."
}

func (LookAtScreen) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"question": {
				"type": "string",
				"description": "What to look for, such as \"what does the error say\". Leave out to describe everything."
			}
		}
	}`)
}

// Safe: it looks and changes nothing. It is also the most privacy-carrying
// thing here, which is why the picture is deleted as soon as it has been read
// and never goes anywhere but the model on this machine.
func (LookAtScreen) Risk() Risk { return Safe }

func (LookAtScreen) Summarize(raw json.RawMessage) string {
	var a struct {
		Question string `json:"question"`
	}

	_ = json.Unmarshal(raw, &a)

	if a.Question != "" {
		return "Look at your screen: " + a.Question
	}

	return "Look at your screen"
}

/*
 * visionModels are the ones known to take images, smallest useful first.
 *
 * Smallest, not best. On a machine with no graphics card every resident model
 * competes for the same four cores, and loading a twelve-billion-parameter
 * model to read a screenshot evicts the one holding the conversation and then
 * sits on eight gigabytes for half an hour afterwards. Measured here: two
 * language servers at 223% and 75% of a four-core processor, and every reply
 * after it crawling.
 *
 * The small one describes a screen perfectly well.
 */
var visionModels = []string{
	"gemma3:4b", "moondream", "llava:7b", "llava", "minicpm-v",
	"qwen2.5vl", "gemma3:12b", "llava:13b",
}

func (t LookAtScreen) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Question string `json:"question"`
	}

	_ = json.Unmarshal(raw, &a)

	shot, err := captureScreen(ctx)
	if err != nil {
		return "", err
	}

	// Gone as soon as it has been looked at. A picture of somebody's screen is
	// the most revealing thing this program ever holds, and it has no business
	// outliving the question that produced it.
	defer os.Remove(shot)

	model, err := t.visionModel(ctx)
	if err != nil {
		return "", err
	}

	image, err := os.ReadFile(shot)
	if err != nil {
		return "", err
	}

	question := strings.TrimSpace(a.Question)
	if question == "" {
		question = "Describe what is on this screen. Be specific about any text, " +
			"error messages, window titles and what the person appears to be doing."
	}

	return t.ask(ctx, model, question, image)
}

// captureScreen writes a picture of the whole screen and returns its path.
func captureScreen(ctx context.Context) (string, error) {
	file, err := os.CreateTemp("", "pn-brain-screen-*.png")
	if err != nil {
		return "", err
	}

	path := file.Name()

	file.Close()

	// In the order they are likely to work on a desktop: the Wayland one, the
	// GNOME one, then the X11 tools.
	attempts := [][]string{
		{"grim", path},
		{"gnome-screenshot", "-f", path},
		{"spectacle", "-b", "-n", "-o", path},
		{"import", "-window", "root", path},
		{"scrot", "-o", path},
	}

	var tried []string

	for _, attempt := range attempts {
		tool, err := exec.LookPath(attempt[0])
		if err != nil {
			continue
		}

		tried = append(tried, attempt[0])

		cmd := exec.CommandContext(ctx, tool, attempt[1:]...)

		// Without this, an X11 tool started from a service has no screen to
		// photograph and fails with something unhelpful about a display.
		if os.Getenv("DISPLAY") == "" {
			cmd.Env = append(os.Environ(), "DISPLAY=:0")
		}

		if err := cmd.Run(); err != nil {
			continue
		}

		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			return path, nil
		}
	}

	os.Remove(path)

	if len(tried) == 0 {
		return "", fmt.Errorf("nothing on this machine can take a screenshot; " +
			"install gnome-screenshot, grim, scrot or imagemagick")
	}

	return "", fmt.Errorf("tried %s and none of them could take a picture of the screen",
		strings.Join(tried, ", "))
}

// visionModel picks an installed model that can see.
func (t LookAtScreen) visionModel(ctx context.Context) (string, error) {
	if t.Model != "" {
		return t.Model, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.OllamaURL+"/api/tags", nil)
	if err != nil {
		return "", err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("reaching ollama at %s: %w", t.OllamaURL, err)
	}

	defer res.Body.Close()

	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}

	for _, want := range visionModels {
		for _, have := range out.Models {
			if strings.HasPrefix(have.Name, want) {
				return have.Name, nil
			}
		}
	}

	return "", fmt.Errorf("no model on this machine can see images; " +
		"install one with: ollama pull gemma3:4b")
}

// ask sends the picture and the question to the model that can see.
func (t LookAtScreen) ask(ctx context.Context, model, question string, image []byte) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":  model,
		"prompt": question,
		"images": []string{base64.StdEncoding.EncodeToString(image)},
		"stream": false,

		// Let go of it straight away. Looking at the screen is occasional, and
		// a vision model left resident for half an hour is memory and processor
		// taken from every reply after it.
		"keep_alive": "30s",
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		t.OllamaURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	// Looking at a picture on a processor takes far longer than answering from
	// text, and the default would give up before it finished.
	client := &http.Client{Timeout: 10 * time.Minute}

	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("asking %s to look: %w", model, err)
	}

	defer res.Body.Close()

	var out struct {
		Response string `json:"response"`
		Error    string `json:"error"`
	}

	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}

	if out.Error != "" {
		return "", fmt.Errorf("%s could not look at it: %s", model, out.Error)
	}

	answer := strings.TrimSpace(out.Response)
	if answer == "" {
		return "", fmt.Errorf("%s returned nothing about the picture", model)
	}

	return answer, nil
}
