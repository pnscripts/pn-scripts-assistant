package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"pn-scripts-assistant/internal/brain/pace"
	"strings"
	"sync"
	"time"
)

// Ollama is a model running on this machine.
//
// Timeouts here are generous on purpose. On a CPU-only machine a 7B model takes
// about a minute to load cold and then answers in seconds. A timeout tuned for
// a hosted API turns that first load into a failure, and the failure looks like
// a broken installation rather than a slow one — which is exactly the mistake
// that once got an entire extension blamed for a bug that was a 15-second
// deadline.
type Ollama struct {
	BaseURL   string
	ChatModel string
	EmbedName string

	// KeepAlive is passed to Ollama with each request; see ollamaChatRequest.
	// Empty means Ollama's own default.
	KeepAlive string

	// MostRoom caps the window asked for. See room.
	MostRoom int

	// asked is the largest window asked for so far, per model. See withRoom.
	roomMu sync.Mutex
	asked  map[string]int

	HTTPClient *http.Client
}

// NewOllama builds a client with defaults suited to local inference.
func NewOllama(baseURL, chatModel, embedModel string) *Ollama {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434"
	}

	return &Ollama{
		BaseURL:   baseURL,
		ChatModel: chatModel,
		EmbedName: embedModel,
		// Long enough to survive reading an answer and typing the next
		// question, which is the gap that actually hurts.
		KeepAlive: "30m",
		MostRoom:  MostRoom,
		// No Timeout on the client itself: the deadline belongs to the context,
		// so a caller can allow a long first load and a short health check with
		// the same client.
		HTTPClient: &http.Client{},
	}
}

func (o *Ollama) Name() string { return Local }

func (o *Ollama) EmbedModel() string { return o.EmbedName }

// Available reports whether the daemon is reachable, with a short deadline of
// its own so a health check never inherits a caller's long one.
func (o *Ollama) Available(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.BaseURL+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := o.HTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

/*
 * quietThinking asks a reasoning model not to deliberate out loud.
 *
 * Only for the models that have such a mode. Sending the field to one that
 * does not is rejected outright, so a blanket default would break every other
 * model to help this one.
 */
func quietThinking(model string) *bool {
	name := strings.ToLower(model)

	for _, thinker := range []string{"qwen3", "deepseek-r1", "qwq"} {
		if strings.Contains(name, thinker) {
			no := false

			return &no
		}
	}

	return nil
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Tools    []ollamaTool    `json:"tools,omitempty"`

	/*
	 * Think turns off a reasoning model's deliberation.
	 *
	 * A pointer so it is sent only when it means something: models that have
	 * no thinking mode reject the field, and defaulting it to false would send
	 * it to every one of them.
	 *
	 * This is what makes qwen3 usable here at all. It asks for tools properly
	 * — the thing this program most needs from a model, and the thing
	 * qwen2.5-coder does worst, writing its calls out as prose to be recovered
	 * — but it wraps its working in think tags and, on four cores with no
	 * graphics card, did not finish a single question in twenty-five minutes.
	 * The deliberation was the whole of the cost, and it can simply be
	 * switched off.
	 */
	Think *bool `json:"think,omitempty"`

	// Options carries generation settings, chiefly num_predict.
	Options map[string]any `json:"options,omitempty"`

	// KeepAlive is how long Ollama holds the model in memory after answering.
	//
	// The default is five minutes, which is shorter than the gaps between
	// messages in a real conversation. On a machine without a GPU, reloading a
	// 7B model costs about a minute, so a user who pauses to read an answer
	// pays that minute again on their next question — and it presents as the
	// assistant being slow rather than as a cache that expired.
	KeepAlive string `json:"keep_alive,omitempty"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaTool struct {
	Type     string         `json:"type"`
	Function ollamaFunction `json:"function"`
}

type ollamaFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ollamaToolCall struct {
	Function struct {
		Name string `json:"name"`
		// Ollama returns arguments as an object, not a JSON string.
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type ollamaChatResponse struct {
	Model   string        `json:"model"`
	Message ollamaMessage `json:"message"`
	Error   string        `json:"error"`
}

/*
 * ollamaMessagesOf carries an assistant's own calls back with it.
 *
 * The field has been on ollamaMessage since it was written and was never
 * filled, which mattered less here than anywhere else: Ollama accepts a tool
 * result that answers nothing and renders it as text, so the local path worked
 * while every hosted one refused the same conversation. Sending the calls is
 * the shape Ollama documents, and it stops the local path being the odd one
 * out — a turn now reads the same way whoever is asked.
 *
 * Unlike the hosted clients this does not go through Replayable. There is
 * nothing here to repair, and rewriting the one conversation that has always
 * been accepted would be a risk with nothing to buy.
 */
func ollamaMessagesOf(messages []Message) []ollamaMessage {
	out := make([]ollamaMessage, 0, len(messages))

	for _, m := range messages {
		om := ollamaMessage{Role: m.Role, Content: m.Content}

		for _, c := range m.ToolCalls {
			var call ollamaToolCall

			call.Function.Name = c.Name
			call.Function.Arguments = AsObject(c.Arguments)

			om.ToolCalls = append(om.ToolCalls, call)
		}

		out = append(out, om)
	}

	return out
}

/*
 * ChatStream answers a turn a piece at a time.
 *
 * The whole point is what can be started before the answer is finished. This
 * machine produces about ten tokens a second, so a three-sentence reply is most
 * of a minute of silence followed by all of it at once — which is not a
 * conversation, it is a form submission. Handed the pieces as they arrive, the
 * voice can begin on the first sentence while the rest is still being written,
 * and the wait before somebody hears anything falls from most of a minute to a
 * few seconds.
 *
 * Streaming is only used where there is nothing to decide: a turn that might
 * call a tool has to be read whole before anything can happen, because a tool
 * call is not speakable and half of one is not anything.
 */
func (o *Ollama) ChatStream(ctx context.Context, req Request, onText func(string)) (Response, error) {
	model := req.Model
	if model == "" {
		model = o.ChatModel
	}

	body := ollamaChatRequest{
		Model: model, Stream: true, KeepAlive: o.KeepAlive,
		Think: quietThinking(model),
	}

	if req.MaxTokens > 0 {
		body.Options = map[string]any{"num_predict": req.MaxTokens}
	}

	body.Messages = ollamaMessagesOf(req.Messages)

	/*
	 * The tools travel with a streamed turn too.
	 *
	 * Leaving them out was the first version of this and it would have been a
	 * quiet disaster: every turn that meant doing something would have been
	 * streamed beautifully and been unable to do any of it, which is precisely
	 * the failure this program has already been through twice.
	 */
	for _, t := range req.Tools {
		body.Tools = append(body.Tools, ollamaTool{
			Type: "function",
			Function: ollamaFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	// Last, because it is sized against the finished request.
	o.withRoom(&body)

	raw, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		o.BaseURL+"/api/chat", bytes.NewReader(raw))
	if err != nil {
		return Response{}, err
	}

	request.Header.Set("Content-Type", "application/json")

	res, err := o.HTTPClient.Do(request)
	if err != nil {
		return Response{}, notAnswering(o.BaseURL, err)
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("ollama answered %s", res.Status)
	}

	var (
		whole strings.Builder
		calls []ToolCall
	)

	// One JSON object per line, each carrying the next few characters.
	decoder := json.NewDecoder(res.Body)

	for {
		var chunk struct {
			Message struct {
				Content   string           `json:"content"`
				ToolCalls []ollamaToolCall `json:"tool_calls"`
			} `json:"message"`
			Done  bool   `json:"done"`
			Error string `json:"error"`
		}

		if err := decoder.Decode(&chunk); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return Response{}, fmt.Errorf("reading the answer: %w", err)
		}

		if chunk.Error != "" {
			return Response{}, fmt.Errorf("%s", chunk.Error)
		}

		if chunk.Message.Content != "" {
			// The first word back, which on a machine without a card is very
			// nearly the whole of the wait: everything before it is the
			// prompt being read, and the prompt is thousands of tokens of
			// tool descriptions.
			pace.FirstToken()

			whole.WriteString(chunk.Message.Content)

			if onText != nil {
				onText(chunk.Message.Content)
			}
		}

		// A call arrives whole rather than in pieces, and it means this turn is
		// an action rather than an answer.
		for _, c := range chunk.Message.ToolCalls {
			calls = append(calls, ToolCall{
				Name:      c.Function.Name,
				Arguments: c.Function.Arguments,
			})
		}

		if chunk.Done {
			break
		}
	}

	return Response{
		Content:   whole.String(),
		ToolCalls: calls,
		Provider:  "ollama",
		Model:     model,
	}, nil
}

/*
 * How much the model is given to read, and why this is set at all.
 *
 * Ollama's window is 4,096 tokens unless a request says otherwise, and a turn
 * here is bigger than that. Measured on this machine: the persona and the
 * thirty-nine tool descriptions came to 6,338 tokens, of which the model was
 * given the last 2,050 — everything it had been told about who it is, when to
 * use a tool and when to answer in words was thrown away before it read a
 * word. What came back was a model calling the same tool eight times and then
 * giving up, which read like a bad model and was a truncated prompt.
 *
 * Nothing says so. Ollama truncates silently, the request succeeds, and the
 * answer is merely wrong — the worst shape a fault can have.
 *
 * Sizes are quantised because changing the window makes Ollama load the model
 * again, and a window that followed the prompt token by token would reload it
 * on nearly every turn. Bytes per token is deliberately pessimistic: asking
 * for more room than the turn needs costs some memory, and asking for less
 * costs the instructions.
 */
const (
	// LeastRoom is Ollama's own default, and the floor.
	LeastRoom = 4096

	// MostRoom is as much as this asks for. A window is memory that has to be
	// found before the first token is read, and a machine with no graphics
	// card is the machine this runs on.
	MostRoom = 16384

	// RoomStep is what the estimate is rounded up to.
	RoomStep = 4096

	// BytesPerToken is the conversion, on the low side on purpose: JSON tool
	// schemas tokenise worse than prose, and under-estimating truncates.
	BytesPerToken = 3.5

	// RoomForAnAnswer is what is left for the reply when the caller did not
	// say. A window that fits the question exactly leaves nowhere to answer.
	RoomForAnAnswer = 1024
)

/*
 * RoomFor is the largest window worth asking for on a machine this size.
 *
 * A window is memory, taken before the first token is read: a seven-billion
 * model keeps roughly 56 KB per token of it, so sixteen thousand tokens is
 * near a gigabyte that the model itself does not get. On a machine with
 * plenty that is nothing; on a small one it is the difference between running
 * and swapping, and swapping a model is not slow, it is stopped.
 */
func RoomFor(ram uint64) int {
	switch {
	case ram == 0:
		// Nothing measured. The middle size rather than the largest: a guess
		// that costs memory is worse than a guess that costs a little room.
		return 8192
	case ram >= 16<<30:
		return MostRoom
	case ram >= 8<<30:
		return 8192
	default:
		return LeastRoom
	}
}

// room is the window this turn needs, in tokens.
func room(body ollamaChatRequest, most int) int {
	if most <= 0 {
		most = MostRoom
	}

	bytes := 0

	for _, m := range body.Messages {
		// The overhead of the template around each message: the role, the
		// markers, the newlines. Small and not nothing across a long turn.
		bytes += len(m.Content) + 8

		for _, c := range m.ToolCalls {
			bytes += len(c.Function.Name) + len(c.Function.Arguments) + 16
		}
	}

	if len(body.Tools) > 0 {
		if described, err := json.Marshal(body.Tools); err == nil {
			bytes += len(described)
		}
	}

	answer := RoomForAnAnswer

	if n, ok := body.Options["num_predict"].(int); ok && n > 0 {
		answer = n
	}

	want := int(float64(bytes)/BytesPerToken) + answer

	// Round up to the next step, then hold it between the floor and the cap.
	want = (want + RoomStep - 1) / RoomStep * RoomStep

	switch {
	case want < LeastRoom:
		return LeastRoom
	case want > most:
		// Bigger than anything that will be asked for. The model will see the
		// end of the turn and not its beginning; that is Ollama's doing, and
		// the caller is told through the usual channel rather than silently.
		return most
	default:
		return want
	}
}

/*
 * withRoom sets the window on a request that is otherwise finished.
 *
 * The window only ever grows while this program is running, and that is the
 * whole point of keeping it here. Ollama loads the model again whenever the
 * window changes, and it keeps what it has already read only for as long as
 * the window stays the same — so a conversation turn asking for twelve
 * thousand tokens, followed by a two-hundred-token housekeeping call asking
 * for four, makes it reload twice and read the next turn from the beginning.
 * Measured here: the turn after a small call in between started again from
 * nothing, 6,343 tokens at eight a second.
 *
 * Asking for the larger window on the small call costs memory and no time:
 * what is processed is what the request contains.
 */
func (o *Ollama) withRoom(body *ollamaChatRequest) {
	want := room(*body, o.MostRoom)

	o.roomMu.Lock()

	if o.asked == nil {
		o.asked = map[string]int{}
	}

	if was := o.asked[body.Model]; was > want {
		want = was
	}

	o.asked[body.Model] = want

	o.roomMu.Unlock()

	if body.Options == nil {
		body.Options = map[string]any{}
	}

	body.Options["num_ctx"] = want
}

func (o *Ollama) Chat(ctx context.Context, req Request) (Response, error) {
	model := req.Model
	if model == "" {
		model = o.ChatModel
	}

	body := ollamaChatRequest{
		Model: model, Stream: false, KeepAlive: o.KeepAlive,
		Think: quietThinking(model),
	}

	if req.MaxTokens > 0 {
		body.Options = map[string]any{"num_predict": req.MaxTokens}
	}

	body.Messages = ollamaMessagesOf(req.Messages)

	for _, t := range req.Tools {
		body.Tools = append(body.Tools, ollamaTool{
			Type: "function",
			Function: ollamaFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	// Last, because it is sized against the finished request.
	o.withRoom(&body)

	started := time.Now()

	var out ollamaChatResponse

	if err := o.post(ctx, "/api/chat", body, &out); err != nil {
		return Response{}, err
	}

	if out.Error != "" {
		return Response{}, fmt.Errorf("ollama: %s", out.Error)
	}

	resp := Response{
		Content:  out.Message.Content,
		Model:    out.Model,
		Provider: Local,
		Elapsed:  time.Since(started),
	}

	for i, c := range out.Message.ToolCalls {
		resp.ToolCalls = append(resp.ToolCalls, ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      c.Function.Name,
			Arguments: c.Function.Arguments,
		})
	}

	return resp, nil
}

type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`

	// KeepAlive matters more here than it looks.
	//
	// Recall embeds the question before every single reply, and without this
	// the embedding model is loaded with Ollama's default lifetime while the
	// chat model is evicted to make room. The next step then reloads 4.7GB
	// from disk to answer. Measured on this machine: the same question takes
	// 14 seconds when the chat model is already resident and 49 through the
	// brain, and the difference is that reload — paid on every turn.
	KeepAlive string `json:"keep_alive,omitempty"`
}

type ollamaEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
	Error     string    `json:"error"`
}

// Embed turns text into a vector using the local embedding model.
//
// Embedding stays local in every privacy mode. It is applied to the contents of
// a person's disk, so sending it out would leak precisely the material the
// privacy rules exist to keep here — and unlike a chat message, nobody typed it
// with the intention of sending it anywhere.
func (o *Ollama) Embed(ctx context.Context, text string) ([]float32, error) {
	var out ollamaEmbedResponse

	err := o.post(ctx, "/api/embeddings", ollamaEmbedRequest{
		Model:     o.EmbedName,
		Prompt:    text,
		KeepAlive: o.KeepAlive,
	}, &out)
	if err != nil {
		return nil, err
	}

	if out.Error != "" {
		return nil, fmt.Errorf("ollama embeddings: %s", out.Error)
	}

	if len(out.Embedding) == 0 {
		return nil, fmt.Errorf("ollama returned an empty embedding for model %q", o.EmbedName)
	}

	return out.Embedding, nil
}

func (o *Ollama) post(ctx context.Context, path string, body, into any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}

	// Built per attempt: a request body is read as it is sent, so the same
	// request cannot be sent twice. See again.go.
	resp, err := sendWithOneMoreTry(ctx, o.HTTPClient, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+path, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}

		req.Header.Set("Content-Type", "application/json")

		return req, nil
	})
	if err != nil {
		return notAnswering(o.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)

		return fmt.Errorf("ollama returned %d: %s", resp.StatusCode, truncate(buf.String(), 300))
	}

	return json.NewDecoder(resp.Body).Decode(into)
}

func truncate(s string, n int) string {
	r := []rune(s)

	if len(r) <= n {
		return s
	}

	return string(r[:n]) + "…"
}

// Warm asks Ollama to load the chat model without generating anything.
//
// Worth doing because the two model loads in a turn were happening one after
// the other. Recall embeds the question first, which on a cold start means
// loading the embedding model and running it — measured at about twenty
// seconds — and only then does the chat model begin loading, for another
// thirty. Nothing about that is parallel, and both are waiting on disk rather
// than on each other.
//
// Starting this the moment a message arrives overlaps the two, so the chat
// model is ready at roughly the moment the context is. It is a request with no
// messages, which Ollama treats as "load and hold" rather than as a question.
func (o *Ollama) Warm(ctx context.Context) error {
	return o.WarmModel(ctx, o.ChatModel)
}

// WarmModel loads one model and holds it, without asking it anything.
func (o *Ollama) WarmModel(ctx context.Context, model string) error {
	if model == "" {
		return nil
	}

	body := ollamaChatRequest{
		Model:     model,
		Stream:    false,
		KeepAlive: o.KeepAlive,
	}

	var out ollamaChatResponse

	return o.post(ctx, "/api/chat", body, &out)
}

// Loaded is a model Ollama currently holds in memory.
type Loaded struct {
	Name string
	Size int64
}

// Resident reports which models are loaded right now.
func (o *Ollama) Resident(ctx context.Context) ([]Loaded, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.BaseURL+"/api/ps", nil)
	if err != nil {
		return nil, err
	}

	res, err := o.HTTPClient.Do(req)
	if err != nil {
		return nil, notAnswering(o.BaseURL, err)
	}

	defer res.Body.Close()

	var out struct {
		Models []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"models"`
	}

	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}

	loaded := make([]Loaded, 0, len(out.Models))

	for _, m := range out.Models {
		loaded = append(loaded, Loaded{Name: m.Name, Size: m.Size})
	}

	return loaded, nil
}

/*
 * notAnswering is what to do about Ollama not being there.
 *
 * It is the one dependency this program cannot work without on a machine with
 * no keys, and "connection refused" tells somebody nothing they can act on.
 * The address is in the message because it is configurable, and half of these
 * are a brain pointed at the wrong one.
 */
func notAnswering(baseURL string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("the model at %s did not answer in time: %w", baseURL, err)
	}

	return fmt.Errorf("Ollama is not answering at %s — start it (ollama serve), or point this at "+
		"another one with OLLAMA_BASE_URL: %w", baseURL, err)
}
