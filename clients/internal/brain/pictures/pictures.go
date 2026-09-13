/*
 * Package pictures makes images, here or elsewhere.
 *
 * Two ways, chosen by the same rule as everything else that could leave this
 * machine: a server running on this computer when privacy is closed, and a
 * paid service only when its owner has opened it and supplied a key. The fork
 * is not a preference — a description of what somebody wants a picture of is
 * often the most revealing sentence they will write all week.
 *
 * Local means an image server speaking the shape that Automatic1111 made and
 * everything since has copied — SD.Next, Forge, and most of what people
 * actually install. Not ComfyUI's graph API: that wants a whole workflow
 * document per request, which is a thing somebody builds rather than a thing a
 * program can reasonably compose.
 */
package pictures

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Painter makes one picture from a description.
type Painter interface {
	// Name is what to call it in the answer, so somebody knows where their
	// description went.
	Name() string

	Paint(ctx context.Context, what string, wide bool) ([]byte, error)
}

/*
 * Local is an image server on this machine.
 *
 * No key, no account, nothing leaves. The timeout is long because this is the
 * whole point of running one: on a processor rather than a graphics card a
 * picture is minutes, and a client that gives up at thirty seconds would make
 * the local path look broken rather than slow.
 */
type Local struct {
	BaseURL string
	Client  *http.Client
}

func (Local) Name() string { return "the image server on this machine" }

const localPatience = 20 * time.Minute

func (l Local) client() *http.Client {
	if l.Client != nil {
		return l.Client
	}

	return &http.Client{Timeout: localPatience}
}

func (l Local) Paint(ctx context.Context, what string, wide bool) ([]byte, error) {
	width, height := 768, 768

	if wide {
		width, height = 1024, 576
	}

	body, err := json.Marshal(map[string]any{
		"prompt":     what,
		"width":      width,
		"height":     height,
		"steps":      28,
		"cfg_scale":  6.5,
		"n_iter":     1,
		"batch_size": 1,
	})
	if err != nil {
		return nil, err
	}

	url := strings.TrimSuffix(l.BaseURL, "/") + "/sdapi/v1/txt2img"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"the image server on this machine is not answering at %s — start it, or "+
				"turn on a paid service in Privacy: %w", l.BaseURL, err)
	}

	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("the image server returned %d: %s",
			resp.StatusCode, shortly(string(raw)))
	}

	var parsed struct {
		Images []string `json:"images"`
		Error  string   `json:"error"`
		Detail string   `json:"detail"`
	}

	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("the image server replied with something unreadable: %w", err)
	}

	if len(parsed.Images) == 0 {
		if why := firstOf(parsed.Error, parsed.Detail); why != "" {
			return nil, fmt.Errorf("the image server refused: %s", why)
		}

		return nil, fmt.Errorf("the image server returned no picture")
	}

	// Some builds prefix the data with a data: URL header.
	encoded := parsed.Images[0]

	if _, after, found := strings.Cut(encoded, ","); found {
		encoded = after
	}

	return base64.StdEncoding.DecodeString(encoded)
}

/*
 * Paid is an image service somewhere else.
 *
 * The OpenAI shape, which is what the services this program already holds keys
 * for speak. Everything about the request leaves the machine, which is why the
 * router has to have allowed it before anything here is constructed.
 */
type Paid struct {
	ProviderName string
	BaseURL      string
	APIKey       string
	Model        string
	Client       *http.Client
}

func (p Paid) Name() string { return p.ProviderName }

func (p Paid) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}

	return &http.Client{Timeout: 4 * time.Minute}
}

func (p Paid) Paint(ctx context.Context, what string, wide bool) ([]byte, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("no %s key is configured", p.ProviderName)
	}

	size := "1024x1024"
	if wide {
		size = "1536x1024"
	}

	model := p.Model
	if model == "" {
		model = "gpt-image-1"
	}

	body, err := json.Marshal(map[string]any{
		"model":  model,
		"prompt": what,
		"size":   size,
		"n":      1,
	})
	if err != nil {
		return nil, err
	}

	url := strings.TrimSuffix(p.BaseURL, "/") + "/images/generations"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := p.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach %s: %w", p.ProviderName, err)
	}

	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Data []struct {
			B64 string `json:"b64_json"`
			URL string `json:"url"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("%s replied with something unreadable: %w", p.ProviderName, err)
	}

	// Checked before the status code: these services answer a refused request
	// with a 200 and an error object about as often as with a 4xx, and the
	// message inside is the one worth showing.
	if parsed.Error != nil {
		return nil, fmt.Errorf("%s: %s", p.ProviderName, parsed.Error.Message)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s returned %d: %s",
			p.ProviderName, resp.StatusCode, shortly(string(raw)))
	}

	if len(parsed.Data) == 0 {
		return nil, fmt.Errorf("%s returned no picture", p.ProviderName)
	}

	if parsed.Data[0].B64 != "" {
		return base64.StdEncoding.DecodeString(parsed.Data[0].B64)
	}

	/*
	 * Some models answer with a link instead of the picture.
	 *
	 * Fetched here rather than handed back as a URL, because a link that
	 * expires in an hour is not a picture somebody has — and the whole point
	 * of asking for one is to end up with a file.
	 */
	if parsed.Data[0].URL == "" {
		return nil, fmt.Errorf("%s returned neither a picture nor a link to one", p.ProviderName)
	}

	return fetch(ctx, p.client(), parsed.Data[0].URL)
}

func fetch(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("collecting the picture: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("collecting the picture returned %d", resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

/*
 * Save writes a picture where somebody will find it again.
 *
 * Named from the description rather than numbered, because a folder of
 * picture-1.png through picture-40.png is a folder nobody can use. The date
 * goes first so they sort into the order they were made.
 */
func Save(folder, what string, data []byte) (string, error) {
	if err := os.MkdirAll(folder, 0o700); err != nil {
		return "", fmt.Errorf("making the pictures folder: %w", err)
	}

	path := filepath.Join(folder, time.Now().Format("2006-01-02-150405")+"-"+slug(what)+".png")

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("writing the picture: %w", err)
	}

	return path, nil
}

var notAName = regexp.MustCompile(`[^a-z0-9]+`)

func slug(what string) string {
	out := notAName.ReplaceAllString(strings.ToLower(what), "-")
	out = strings.Trim(out, "-")

	if len([]rune(out)) > 40 {
		out = string([]rune(out)[:40])
		out = strings.Trim(out, "-")
	}

	if out == "" {
		return "picture"
	}

	return out
}

func shortly(text string) string {
	text = strings.TrimSpace(text)

	if len(text) <= 200 {
		return text
	}

	return text[:200] + "…"
}

func firstOf(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}

	return ""
}

/*
 * Reachable says whether an image server is actually running here.
 *
 * Asked before falling back to a paid service, because an image server that is
 * installed but not started is the ordinary case — and quietly billing
 * somebody elsewhere because their own was not running is the wrong surprise.
 *
 * A short timeout: this is a question about a program on the same computer,
 * and anything slower than a moment means it is not there.
 */
func Reachable(baseURL string) bool {
	if baseURL == "" {
		return false
	}

	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get(strings.TrimSuffix(baseURL, "/") + "/sdapi/v1/options")
	if err != nil {
		return false
	}

	defer resp.Body.Close()

	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	return resp.StatusCode < 400
}
