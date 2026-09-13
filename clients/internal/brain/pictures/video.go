package pictures

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

/*
 * Video, and what that honestly means here.
 *
 * Not a model that generates moving pictures. Running one of those locally
 * needs hardware this program does not assume and downloads measured in tens
 * of gigabytes, and the paid ones cost real money per second of output — so a
 * make_a_video that quietly meant either of those would be a tool that either
 * never works or bills somebody by surprise.
 *
 * What it does mean is assembling pictures into a film, which is a real thing
 * people want and which this machine can genuinely do: a sequence of images,
 * a few seconds each, optionally with a gentle move so it does not look like a
 * slideshow that has stopped. ffmpeg is already a prerequisite here, for
 * subtitles, so nothing new has to be installed.
 *
 * Said plainly in the tool's description, because a name that promises more
 * than the thing does is worse than a longer name.
 */

// Film assembles pictures into a video file.
type Film struct {
	// Pictures are the image files, in the order they should appear.
	Pictures []string

	// Seconds is how long each one is held.
	Seconds float64

	// Move adds a slow drift across each picture, so a still image does not
	// read as a frozen player.
	Move bool

	// Sound is an audio file to lay underneath, or empty.
	Sound string
}

// MakeIt writes the video and returns where it went.
func MakeIt(ctx context.Context, folder string, film Film) (string, error) {
	if len(film.Pictures) == 0 {
		return "", fmt.Errorf("a video needs at least one picture")
	}

	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf(
			"ffmpeg is not installed, and it is what puts the pictures together — " +
				"it is in the list of parts this machine needs")
	}

	for _, picture := range film.Pictures {
		if _, err := os.Stat(picture); err != nil {
			return "", fmt.Errorf("there is no picture at %s", picture)
		}
	}

	seconds := film.Seconds
	if seconds <= 0 {
		seconds = 3
	}

	if err := os.MkdirAll(folder, 0o700); err != nil {
		return "", fmt.Errorf("making the folder: %w", err)
	}

	path := filepath.Join(folder, time.Now().Format("2006-01-02-150405")+"-film.mp4")

	args := buildArgs(film, seconds, path)

	/*
	 * Long, because this is minutes of work on a processor.
	 *
	 * Encoding a two-minute film from stills without a graphics card is not
	 * quick, and a timeout that fires halfway leaves a file that looks like a
	 * video and will not play.
	 */
	run, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(run, ffmpeg, args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(path)

		return "", fmt.Errorf("putting the pictures together failed: %s", lastLines(string(output)))
	}

	return path, nil
}

/*
 * buildArgs writes the ffmpeg call.
 *
 * One input per picture rather than a concat file, because the pictures are
 * wherever somebody left them and a concat list is a second temporary file
 * with its own escaping rules. The scale-and-pad keeps mixed sizes from
 * stretching: a portrait photo in a wide film gets bars, not a squeeze.
 */
func buildArgs(film Film, seconds float64, path string) []string {
	var (
		args   []string
		filter strings.Builder
	)

	for _, picture := range film.Pictures {
		args = append(args,
			"-loop", "1",
			"-t", strconv.FormatFloat(seconds, 'f', 2, 64),
			"-i", picture)
	}

	if film.Sound != "" {
		args = append(args, "-i", film.Sound)
	}

	const size = "1280:720"

	for i := range film.Pictures {
		fmt.Fprintf(&filter,
			"[%d:v]scale=%s:force_original_aspect_ratio=decrease,"+
				"pad=%s:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=30", i, size, size)

		if film.Move {
			// A slow drift, so a still picture does not read as a player that
			// has frozen. Kept gentle: anything faster looks like a mistake.
			frames := int(seconds * 30)
			fmt.Fprintf(&filter, ",zoompan=z='min(zoom+0.0008,1.12)':d=%d:s=%s:fps=30",
				frames, strings.ReplaceAll(size, ":", "x"))
		}

		fmt.Fprintf(&filter, "[v%d];", i)
	}

	for i := range film.Pictures {
		fmt.Fprintf(&filter, "[v%d]", i)
	}

	fmt.Fprintf(&filter, "concat=n=%d:v=1:a=0[out]", len(film.Pictures))

	args = append(args, "-filter_complex", filter.String(), "-map", "[out]")

	if film.Sound != "" {
		args = append(args,
			"-map", strconv.Itoa(len(film.Pictures))+":a",
			// The film is as long as the pictures. Music that runs on past the
			// end is a black screen with sound, which reads as a broken file.
			"-shortest",
			"-c:a", "aac", "-b:a", "160k")
	}

	return append(args,
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-preset", "medium", "-crf", "20",
		"-y", path)
}

func lastLines(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) > 4 {
		lines = lines[len(lines)-4:]
	}

	return strings.Join(lines, "; ")
}
