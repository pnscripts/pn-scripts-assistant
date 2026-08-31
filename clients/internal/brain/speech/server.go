package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"pn-brain/internal/brain/progress"
	"strconv"
	"strings"
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
	/*
	 * Check the microphone wiring on the way up, and fix it if it is wrong.
	 *
	 * In its own goroutine because it can restart the audio services, which
	 * takes a few seconds, and nothing about starting the recogniser depends
	 * on the answer. It usually does nothing at all: the wiring is checked
	 * first and rewritten only when the canceller is pointed at a microphone
	 * nobody is speaking into, which is what happens when the devices change
	 * underneath it — a headset plugged in, a USB microphone unplugged — and
	 * which otherwise presents as the assistant having gone deaf.
	 */
	go func() {
		if err := EnsureEchoCancellation(context.WithoutCancel(ctx)); err != nil {
			log.Printf("echo cancellation not set up: %v", err)
		}
	}()

	r, why := FindRecogniser()
	if r == nil {
		return fmt.Errorf("%s", why)
	}

	/*
	 * An existing server is stopped rather than adopted.
	 *
	 * Reusing one that is already listening looks like the thrifty choice and
	 * leaks a process forever: the instance that started it recorded the handle
	 * and every instance after it did not, so when the last one exits there is
	 * nobody left who knows how to stop the thing. Found by closing the program
	 * and seeing a whisper-server still resident, holding its model, reparented
	 * to init, started three hours and a dozen restarts earlier.
	 *
	 * Only one copy of the brain runs at a time, so the recogniser has exactly
	 * one owner. Taking the few seconds to load the model again is the price of
	 * it always being ours to stop.
	 */
	if serverReady(ctx) {
		stopStrayServers()
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

	/*
	 * The resident server gets speech detection too.
	 *
	 * Otherwise the two transcription paths disagree about what counts as
	 * speech — the fallback would refuse a recording of an empty room and the
	 * fast path would answer it — and which one runs depends on whether a
	 * server happens to be up. That is the worst kind of difference: invisible,
	 * intermittent, and impossible to reproduce on purpose.
	 */
	/*
	 * VAD at launch; the vocabulary goes with each request instead.
	 *
	 * A prompt fixed here would be whatever the brain knew at startup, and it
	 * learns names all day. Per request it is always current.
	 */
	args := withVAD([]string{
		"-m", r.Model, "--host", host, "--port", port, "-t", "4",
	})

	cmd := exec.Command(binary, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	// And it dies with whoever started it, however that ends. StopResident
	// covers the orderly exit; this covers a crash, a kill -9, a session
	// ending — the same guard the recorder has, and for the same reason.
	dieWithParent(cmd)

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

/*
 * stopStrayServers ends a recogniser left behind by an earlier run.
 *
 * By name, which is blunt, and correct here: this program allows exactly one
 * copy of itself at a time, and the recogniser it starts listens on a port only
 * it uses. Anything answering there now is a leftover of a previous run of this
 * same program.
 */
func stopStrayServers() {
	out, err := exec.Command("pgrep", "-x", "whisper-server").Output()
	if err != nil {
		return
	}

	for _, line := range strings.Fields(string(out)) {
		pid, err := strconv.Atoi(line)
		if err != nil || pid <= 1 {
			continue
		}

		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
		}
	}

	// It holds the port for a moment after it dies.
	time.Sleep(500 * time.Millisecond)
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

	/*
	 * The names this person uses, sent with every request rather than fixed
	 * when the server started.
	 *
	 * The server takes both at launch and per request, and per request is the
	 * one that stays true: the brain learns a name at eleven and would go on
	 * mishearing it until the next restart otherwise. This is what stops
	 * "pnscripts.com" arriving as "pncryptz.com".
	 */
	if prompt := Prompt(); prompt != "" {
		form.WriteField("prompt", prompt)
		form.WriteField("carry_initial_prompt", "true")
	}

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

/*
 * ListenForTurn records until somebody has finished, and returns what was said.
 *
 * "Finished" is decided from the words rather than from a stopwatch. A fixed
 * gap of quiet cannot tell a pause from an ending — set it short and it cuts
 * in while somebody is still assembling a sentence, set it long and every
 * quick question is followed by a wait — so when the transcript stops
 * mid-thought the microphone is simply opened again and the pieces joined.
 */
func ListenForTurn(ctx context.Context, device string) (Heard, error) {
	heard, err := listenOnce(ctx, device)
	if err != nil || heard.Text == "" {
		return heard, err
	}

	parts := []string{heard.Text}
	spoke := heard.SpokeForMS

	for i := 0; i < MostContinuations; i++ {
		if !NeedsMore(JoinTurns(parts), spoke) {
			break
		}

		progress.Detail("still talking — waiting for the rest")

		more, err := listenAgain(ctx, device)
		if err != nil {
			break
		}

		// Nothing further said: they had finished after all, and whisper
		// simply did not punctuate it.
		if more.Text == "" {
			break
		}

		parts = append(parts, more.Text)

		// The levels of the last piece, which is the one that just happened.
		heard.PeakRMS = more.PeakRMS
		heard.NoiseFloor = more.NoiseFloor
		heard.SpokeForMS += more.SpokeForMS
		spoke = more.SpokeForMS
	}

	heard.Text = JoinTurns(parts)

	if len(parts) > 1 {
		progress.Detail("joined " + itoa(len(parts)) + " pieces")
	}

	// Worked out on the whole thing, since a name may be in any piece.
	heard.Unfamiliar = UnfamiliarIn(heard.Text, heard.PeakRMS)

	return heard, nil
}

// itoa keeps the detail line free of a fmt import dance.
func itoa(n int) string {
	return strconv.Itoa(n)
}

// listenOnce records until the speaker falls quiet, once.
func listenOnce(ctx context.Context, device string) (Heard, error) {
	return listenWaiting(ctx, device, PatienceBeforeSpeech)
}

// listenAgain carries a turn on, waiting only briefly for it to resume.
func listenAgain(ctx context.Context, device string) (Heard, error) {
	return listenWaiting(ctx, device, PatienceForMore)
}

func listenWaiting(
	ctx context.Context, device string, patience time.Duration,
) (Heard, error) {
	f, err := os.CreateTemp("", "pn-brain-turn-*.wav")
	if err != nil {
		return Heard{}, err
	}

	path := f.Name()
	f.Close()

	defer os.Remove(path)

	/*
	 * Say that it is listening, while it listens.
	 *
	 * This was the one state the program never reported, and its absence is
	 * most of what "nothing is happening when I am talking" meant. Recording a
	 * turn and making out the words takes a second or three, and for all of it
	 * the interface held whatever it last said — usually nothing — so the only
	 * evidence that speaking had any effect arrived after the answer did. The
	 * kinds were listed in the progress package from the start, and this one
	 * was never set by anybody.
	 */
	progress.SetBackground("listening", "Listening")

	turn, err := RecordTurnWaiting(ctx, device, path, patience)
	if err != nil {
		progress.Done()

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
		/*
		 * Said plainly when it was the machine rather than silence.
		 *
		 * "Nothing crossed the threshold" and "that was the cooling fan" are
		 * different problems with different answers — the first is a
		 * microphone or a volume, the second is a fan next to a microphone —
		 * and reporting both as nothing heard sent two days of debugging
		 * after the wrong one.
		 */
		if turn.WasSteadyNoise {
			progress.SetBackground("listening", "Listening")
			progress.Detail("steady noise, not a voice — ignored")
		}

		progress.Done()

		measured.Advice = Explain(level, "")

		return measured, nil
	}

	/*
	 * The header first, because the recorder never got to finish it.
	 *
	 * A turn ends when somebody stops talking, and the recorder is killed on
	 * the spot — so the two lengths in the file still say it holds nothing.
	 * Everything in this program reads the samples directly and never noticed;
	 * whisper reads the header and refuses the file.
	 */
	if _, err := RepairWAV(path); err != nil {
		return measured, err
	}

	/*
	 * Turned up before it is transcribed.
	 *
	 * The detector and the recogniser want very different amounts of signal,
	 * and the gap between them is what "loud enough, but no words came back"
	 * was: peaks of 2012 and 6392 were understood, peaks of 215 and 282 came
	 * back empty, from the same voice in the same room.
	 */
	/*
	 * If they cut in, only what they said is transcribed.
	 *
	 * Everything before that point is the assistant still talking, and giving
	 * the recogniser both voices at once returns a blend of the two — a
	 * sentence neither of them said, delivered with no sign that a second
	 * voice was in the room.
	 */
	var cutIn bool

	if turn.CutInAt > 0 {
		cutIn = TrimTo(path, turn.CutInAt) == nil
	}

	if gain, err := Normalise(path); err == nil && gain > 1 {
		measured.Gain = gain
	}

	// Heard, but not yet understood. Worth saying, because on this machine the
	// recogniser is a second of work on its own and the two stages fail for
	// completely different reasons.
	progress.Set("transcribing", "Making out the words")

	/*
	 * This step, so its findings are reported against it.
	 *
	 * The agent and the listening loop both write to one progress, and they
	 * overlap: the brain starts listening again while the last answer is still
	 * being written. Detail attached to "whatever is current" therefore landed
	 * on the wrong step — "no words made out" appeared under Answering, which
	 * had not been listening to anything.
	 */
	transcribing := progress.Mark()

	if cutIn {
		progress.DetailOn(transcribing, "you cut in — dropped my own voice first")
	}

	/*
	 * How loud it was against the room, while the recogniser runs.
	 *
	 * These three numbers are the whole diagnosis when speech goes unheard,
	 * and they used to be reachable only by asking an endpoint nobody knows
	 * about. A peak barely above the floor explains an empty transcript
	 * completely, and it explains it at the moment somebody is wondering why
	 * nothing happened rather than the next day.
	 */
	progress.DetailOn(transcribing, fmt.Sprintf("%.1fs of speech · peak %d · room %d",
		float64(measured.SpokeForMS)/1000, turn.PeakRMS, turn.NoiseFloor))

	text, err := TranscribeFast(ctx, path)
	if err != nil {
		progress.Done()

		return measured, err
	}

	/*
	 * The recogniser narrating noise is not somebody talking.
	 *
	 * Kept even with Silero in front of it, because the two fail differently:
	 * VAD asks whether the sound was speech, and this asks whether the words
	 * that came back describe speech or merely describe the room. "(crickets
	 * chirping)" passes the first test on a recording where a chair moved.
	 */
	if looksLikeNoise(text) {
		text = ""
	}

	/*
	 * Its own voice is not a turn.
	 *
	 * Checked here rather than trusted to the canceller, because cancellation
	 * only has to fail slightly to fail completely. Observed: it asked "I
	 * heard siga.joshina.com but I do not know that name", transcribed its own
	 * question back, found another name it did not know inside it, and asked
	 * again — three rounds in a minute, each inventing the next name.
	 */
	if text != "" && SoundsLikeItself(text) {
		progress.DetailOn(transcribing, "that was my own voice — ignored")

		// Counted as well as discarded. A room that does this repeatedly is
		// one where the microphone is next to the speaker, and there the
		// answer is to stop listening while talking rather than to go on
		// catching echoes one at a time.
		HeardItself()

		text = ""
	}

	// The words themselves, which is the one detail worth more than all the
	// measurements: a transcript that is nearly right explains a wrong answer
	// in a way no level ever could.
	if text != "" {
		progress.DetailOn(transcribing, "heard: "+text)
	} else {
		progress.DetailOn(transcribing, "no words made out")

		/*
		 * And the recording is kept, because the numbers have stopped
		 * explaining this.
		 *
		 * Nineteen seconds of speech at a peak of 1358 against a floor of 4
		 * came back empty, and those exact levels reproduced from a file
		 * transcribe perfectly. Whatever is wrong is in the audio itself, and
		 * no summary of it will show what.
		 */
		if kept := KeepFailedTurn(path, measured.SpokeForMS, turn.PeakRMS); kept != "" {
			progress.DetailOn(transcribing, "kept the recording: "+filepath.Base(kept))
		}
	}

	measured.Text = text
	measured.Advice = Explain(level, text)

	/*
	 * Names nothing here has met, flagged rather than assumed.
	 *
	 * Not corrected — there is nothing to correct them against, which is the
	 * whole point. Repeating one back is what a person does taking a name over
	 * the telephone, and it costs a second against an answer that is otherwise
	 * confidently about the wrong subject.
	 */
	if unfamiliar := UnfamiliarIn(text, turn.PeakRMS); len(unfamiliar) > 0 {
		measured.Unfamiliar = unfamiliar

		progress.DetailOn(transcribing, "unfamiliar: "+strings.Join(unfamiliar, ", "))
	}

	/*
	 * Left standing when words came back, cleared when they did not.
	 *
	 * Something heard is handed straight to the agent, which sets its own step
	 * within a moment; clearing here would put an idle core on screen for that
	 * gap and make every turn flicker. Nothing heard ends here, so this is the
	 * only place that can put it back to idle.
	 */
	if text == "" {
		progress.Done()
	}

	return measured, nil
}
