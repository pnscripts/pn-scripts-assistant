package pictures

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * Pictures really do become a film.
 *
 * Run against ffmpeg rather than against a mock, because the whole of this is
 * getting the arguments right — a filter graph that is subtly wrong produces a
 * file that exists, has a size, and will not play.
 */
func TestPicturesBecomeAFilm(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed here, and it is what puts the pictures together")
	}

	folder := t.TempDir()

	var shots []string

	for _, colour := range []string{"red", "blue"} {
		path := filepath.Join(folder, colour+".png")

		made := exec.Command("ffmpeg", "-f", "lavfi", "-i",
			"color=c="+colour+":s=640x480:d=1", "-frames:v", "1", "-y", path)

		if out, err := made.CombinedOutput(); err != nil {
			t.Skipf("could not make a test picture: %s", out)
		}

		shots = append(shots, path)
	}

	path, err := MakeIt(context.Background(), folder, Film{
		Pictures: shots, Seconds: 1, Move: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("nothing was written: %v", err)
	}

	if info.Size() < 1000 {
		t.Errorf("the film is %d bytes, which is not a film", info.Size())
	}

	// And it plays: a file that exists and will not open is the failure this
	// test is for.
	probe := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name", "-of", "csv=p=0", path)

	out, err := probe.CombinedOutput()
	if err != nil {
		t.Fatalf("the film will not open: %s", out)
	}

	if !strings.Contains(string(out), "h264") {
		t.Errorf("the film is %q", strings.TrimSpace(string(out)))
	}
}

// A picture that is not there is said so before ffmpeg is started, rather than
// coming back as a wall of ffmpeg output.
func TestAMissingPictureIsSaidPlainly(t *testing.T) {
	_, err := MakeIt(context.Background(), t.TempDir(), Film{
		Pictures: []string{"/nowhere/at/all.png"},
	})

	if err == nil {
		t.Fatal("it made a film out of nothing")
	}

	if !strings.Contains(err.Error(), "no picture at") {
		t.Errorf("it said %v", err)
	}
}

// And a film with no pictures in it is not a film.
func TestAFilmNeedsPictures(t *testing.T) {
	if _, err := MakeIt(context.Background(), t.TempDir(), Film{}); err == nil {
		t.Error("it made a film with no pictures")
	}
}
