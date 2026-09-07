// Package subtitles writes subtitles for a film that has none.
//
// The one part of "should the brain learn subtitles" that is unambiguously
// worth doing. It is not learning at all — nothing goes into memory — it is
// the machine doing a job with the parts it already has: ffmpeg to get the
// sound out, and the recogniser that is already resident for listening.
//
// 658 of the 866 films on Petar's drive have no subtitles beside them, which
// is the whole reason this exists. It is also why it is one film at a time and
// only when asked: a two-hour film is between one and three hours of work on a
// processor with no graphics acceleration, so 658 of them is a month. A
// feature that quietly starts a month of work is not a feature.
package subtitles

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Kinds are the films this can work on.
var Kinds = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".mov": true,
	".m4v": true, ".webm": true, ".wmv": true, ".mpg": true, ".mpeg": true,
}

// Beside are the subtitle formats that count as "this film already has some".
var Beside = map[string]bool{
	".srt": true, ".sub": true, ".vtt": true, ".ass": true, ".ssa": true,
}

// Film is one film and where it is.
type Film struct {
	Path string
	Name string
}

/*
 * Missing lists the films under a folder that have no subtitles beside them.
 *
 * Beside, rather than inside: a subtitle track muxed into an .mkv is already
 * there and this would be writing a worse copy of it. Checking for that needs
 * ffprobe on every file, which is 866 processes to answer a question the
 * filename usually answers — so the muxed case is left to the person, who can
 * see it in their player.
 *
 * A language suffix is stripped before matching, because subtitles are named
 * "Film.bg.srt" as often as "Film.srt", and treating those as different files
 * would offer to write a subtitle for a film that has one.
 */
func Missing(root string) ([]Film, error) {
	films := map[string]Film{}
	have := map[string]bool{}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		stem := strings.TrimSuffix(path, filepath.Ext(path))

		switch {
		case Kinds[ext]:
			films[stem] = Film{Path: path, Name: d.Name()}

		case Beside[ext]:
			have[stem] = true
			have[withoutLanguage(stem)] = true
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	var out []Film

	for stem, f := range films {
		if have[stem] {
			continue
		}

		out = append(out, f)
	}

	return out, nil
}

// withoutLanguage drops a trailing language tag: "Film.bg" is a subtitle for
// "Film", not for a film called "Film.bg".
func withoutLanguage(stem string) string {
	for _, tag := range []string{
		".bg", ".en", ".de", ".fr", ".es", ".it", ".ru",
		".bul", ".eng", ".ger", ".fre", ".spa", ".rus",
	} {
		if strings.HasSuffix(strings.ToLower(stem), tag) {
			return stem[:len(stem)-len(tag)]
		}
	}

	return stem
}

// Where the subtitle for a film will be written.
func Path(film string) string {
	return strings.TrimSuffix(film, filepath.Ext(film)) + ".srt"
}

/*
 * Make writes an .srt beside a film.
 *
 * Two outside programs, in the order they are needed. ffmpeg takes the sound
 * out as the 16kHz mono the recogniser wants — which is also what makes the
 * intermediate file bearable, about 100MB an hour rather than the film's own
 * gigabytes. Then whisper writes the subtitle itself: it has an SRT writer,
 * and using it means the timings come from the thing that heard the words
 * rather than from arithmetic here.
 *
 * The audio goes to a temporary file that is always removed, including when
 * the work is abandoned half way. Nothing is written next to the film until
 * whisper has finished, so an interrupted run leaves no half-subtitle for
 * somebody to load and wonder about.
 */
func Make(ctx context.Context, film, recogniser, model, language string, note func(string)) (string, error) {
	if note == nil {
		note = func(string) {}
	}

	if _, err := os.Stat(film); err != nil {
		return "", fmt.Errorf("no film at %s", film)
	}

	if recogniser == "" || model == "" {
		return "", fmt.Errorf("the speech recogniser is not set up, so there is nothing to listen with")
	}

	/*
	 * An English-only model cannot subtitle a Bulgarian film.
	 *
	 * Whisper ships two of everything: ggml-base.bin understands ninety-nine
	 * languages and ggml-base.en.bin understands one. Given Bulgarian speech
	 * the English model does not fail — it invents English, confidently, for
	 * two hours, and writes it out with plausible timings. Three hours of work
	 * to produce a file that is worse than none, and nothing in it says so.
	 *
	 * So it is refused here, by name, with what to do about it. This is also
	 * the model the brain listens to its owner with, which is worth knowing
	 * for anyone who does not speak English at home.
	 */
	if OnlyEnglish(model) && language != "" && !strings.HasPrefix(strings.ToLower(language), "en") {
		return "", fmt.Errorf(
			"the installed recogniser model is %s, which understands English and "+
				"nothing else — given %s speech it would invent English rather than "+
				"fail. A multilingual model (ggml-small.bin or ggml-medium.bin) is "+
				"what this needs",
			filepath.Base(model), language)
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", fmt.Errorf("ffmpeg is not installed, and it is what takes the sound out of a film")
	}

	sound, err := os.CreateTemp("", "pn-brain-film-*.wav")
	if err != nil {
		return "", err
	}

	wav := sound.Name()
	sound.Close()

	defer os.Remove(wav)

	note("taking the sound out of " + filepath.Base(film))

	// -vn drops the picture, -ac 1 -ar 16000 is what the recogniser wants, and
	// -y overwrites the empty file CreateTemp just made.
	extract := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-y",
		"-i", film, "-vn", "-ac", "1", "-ar", "16000", "-f", "wav", wav)

	if out, err := extract.CombinedOutput(); err != nil {
		return "", fmt.Errorf("could not take the sound out: %w: %s", err, lastLine(string(out)))
	}

	note("making out the words — this is the slow part")

	/*
	 * Written to its final name by whisper itself.
	 *
	 * -of takes the path without an extension and whisper appends .srt, so
	 * this lands beside the film with the film's name, which is where every
	 * player looks for it.
	 */
	args := []string{"-m", model, "-f", wav, "-osrt", "-of", strings.TrimSuffix(film, filepath.Ext(film))}

	if language != "" {
		args = append(args, "-l", language)
	}

	written := Path(film)

	if out, err := exec.CommandContext(ctx, recogniser, args...).CombinedOutput(); err != nil {
		// Whatever it managed to write is not a subtitle anybody wants.
		os.Remove(written)

		return "", fmt.Errorf("could not make out the words: %w: %s", err, lastLine(string(out)))
	}

	if _, err := os.Stat(written); err != nil {
		return "", fmt.Errorf("the recogniser finished but wrote nothing to %s", written)
	}

	return written, nil
}

/*
 * OnlyEnglish reports whether a whisper model understands English alone.
 *
 * The ".en" in the filename is whisper's own convention and the only thing
 * that distinguishes them: ggml-base.bin and ggml-base.en.bin are the same
 * size and behave identically until the speech is not English.
 */
func OnlyEnglish(model string) bool {
	name := strings.ToLower(filepath.Base(model))

	return strings.Contains(name, ".en.") || strings.HasSuffix(name, ".en")
}

// lastLine is the end of a program's output, which is where it says what went
// wrong. The rest is a banner.
func lastLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")

	if len(lines) == 0 {
		return ""
	}

	return strings.TrimSpace(lines[len(lines)-1])
}
