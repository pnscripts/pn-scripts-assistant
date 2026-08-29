package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// ServerAddr is where a resident whisper is expected.
//
// Loopback and a fixed port: it is started by this program for this program,
// and a recogniser reachable from the network would be a microphone reachable
// from the network.
const ServerAddr = "127.0.0.1:8791"

// Resident keeps a whisper model in memory.
//
// Spawning whisper-cli per utterance reloads 141MB of weights every time, which
// costs about a second before any audio is looked at. That is tolerable for a
// single dictation and ruinous in a conversation, where it is paid on every
// turn. The server pays it once.
type Resident struct {
	cmd  *exec.Cmd
	once sync.Once
	mu   sync.Mutex
}

var resident = &Resident{}

// StartResident launches the whisper server if one is not already listening.
//
// Failure is not fatal: transcription falls back to running whisper-cli, which
// is slower but works. A conversation that is sluggish beats one that refuses
// to start.
func StartResident(ctx context.Context) error {
	r, why := FindRecogniser()
	if r == nil {
		return fmt.Errorf("%s", why)
	}

	if serverReady(ctx) {
		return nil
	}

	binary := filepath.Join(filepath.Dir(r.Command), "whisper-server")

	if _, err := os.Stat(binary); err != nil {
		// The symlinked whisper-cli may live beside its siblings elsewhere.
		if resolved, err := filepath.EvalSymlinks(r.Command); err == nil {
			binary = filepath.Join(filepath.Dir(resolved), "whisper-server")
		}
	}

	if _, err := os.Stat(binary); err != nil {
		return fmt.Errorf("whisper-server not found beside %s", r.Command)
	}

	host, port := "127.0.0.1", "8791"

	cmd := exec.Command(binary, "-m", r.Model, "--host", host, "--port", port, "-t", "4")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start whisper-server: %w", err)
	}

	resident.mu.Lock()
	resident.cmd = cmd
	resident.mu.Unlock()

	// Loading the model takes a few seconds; nothing should be sent until it
	// answers.
	deadline := time.Now().Add(30 * time.Second)

	for time.Now().Before(deadline) {
		if serverReady(ctx) {
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("whisper-server did not become ready")
}

// StopResident shuts the server down.
func StopResident() {
	resident.mu.Lock()
	defer resident.mu.Unlock()

	if resident.cmd != nil && resident.cmd.Process != nil {
		resident.cmd.Process.Kill()
		resident.cmd.Wait()
		resident.cmd = nil
	}
}

func serverReady(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+ServerAddr+"/", nil)
	if err != nil {
		return false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}

	resp.Body.Close()

	return true
}

// TranscribeFast sends audio to the resident server, falling back to the CLI.
//
// When nothing is known about the language it asks once, separately, and keeps
// the answer. The server does not report what it detected, so without this a
// conversation would re-guess on every clip — which is what turned one sentence
// into Bulgarian, Turkish and English at once.
func TranscribeFast(ctx context.Context, wav string) (string, error) {
	if Language() == "" && DetectedLanguage() == "" {
		if code := detectLanguage(ctx, wav); code != "" {
			detectedMu.Lock()
			sessionLanguage = code
			detectedMu.Unlock()
		}
	}

	if text, err := transcribeViaServer(ctx, wav); err == nil {
		return text, nil
	}

	return Transcribe(ctx, wav)
}

func transcribeViaServer(ctx context.Context, wav string) (string, error) {
	audio, err := os.ReadFile(wav)
	if err != nil {
		return "", err
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)

	part, err := form.CreateFormFile("file", filepath.Base(wav))
	if err != nil {
		return "", err
	}

	if _, err := part.Write(audio); err != nil {
		return "", err
	}

	form.WriteField("response_format", "json")

	// Same reasoning as the command line: the server also assumes English
	// unless told, so detection has to be asked for by name.
	form.WriteField("language", languageForTurn())
	form.Close()

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"http://"+ServerAddr+"/inference", &body)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", form.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("whisper-server returned %d", resp.StatusCode)
	}

	var out struct {
		Text string `json:"text"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}

	return CleanTranscript(out.Text), nil
}

// ListenForTurn records until the speaker stops and returns what was said.
func ListenForTurn(ctx context.Context, device string) (Heard, error) {
	f, err := os.CreateTemp("", "pn-brain-turn-*.wav")
	if err != nil {
		return Heard{}, err
	}

	path := f.Name()
	f.Close()

	defer os.Remove(path)

	turn, err := RecordTurn(ctx, device, path)
	if err != nil {
		return Heard{}, err
	}

	level, err := MeasureWAV(path)
	if err != nil {
		return Heard{}, err
	}

	// What the detector measured, on every path out of here, so that a turn
	// that heard nothing is as legible as one that did.
	measured := Heard{
		Level:       level,
		HeardSpeech: turn.HeardSpeech,
		PeakRMS:     turn.PeakRMS,
		NoiseFloor:  turn.NoiseFloor,
		Threshold:   turn.Threshold,
		SpokeForMS:  int(turn.SpokeFor / time.Millisecond),
	}

	if !turn.HeardSpeech {
		measured.Advice = Explain(level, "")

		return measured, nil
	}

	text, err := TranscribeFast(ctx, path)
	if err != nil {
		return measured, err
	}

	measured.Text = text
	measured.Advice = Explain(level, text)

	return measured, nil
}
