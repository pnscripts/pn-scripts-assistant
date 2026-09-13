package pictures

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A one-pixel PNG, so the tests move real bytes rather than a placeholder.
const onePixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func TestALocalServerMakesAPicture(t *testing.T) {
	var asked map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sdapi/v1/txt2img" {
			t.Errorf("it asked for %s", r.URL.Path)
		}

		json.NewDecoder(r.Body).Decode(&asked)

		fmt.Fprintf(w, `{"images":["%s"]}`, onePixel)
	}))

	defer server.Close()

	data, err := Local{BaseURL: server.URL, Client: server.Client()}.
		Paint(context.Background(), "a quiet room", true)
	if err != nil {
		t.Fatal(err)
	}

	if len(data) == 0 {
		t.Fatal("no picture came back")
	}

	if asked["prompt"] != "a quiet room" {
		t.Errorf("it asked for %v", asked["prompt"])
	}

	// Wide means wide, or the flag is decoration.
	if asked["width"].(float64) <= asked["height"].(float64) {
		t.Errorf("a wide picture came out %vx%v", asked["width"], asked["height"])
	}
}

// Some builds answer with a data: URL header in front of the picture.
func TestADataURLHeaderIsNotPartOfThePicture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"images":["data:image/png;base64,%s"]}`, onePixel)
	}))

	defer server.Close()

	data, err := Local{BaseURL: server.URL, Client: server.Client()}.
		Paint(context.Background(), "anything", false)
	if err != nil {
		t.Fatalf("a picture with a data URL header was not read: %v", err)
	}

	if !strings.HasPrefix(string(data), "\x89PNG") {
		t.Errorf("what came back is not a PNG: %q", string(data[:8]))
	}
}

/*
 * A server that is not running says what to do about it.
 *
 * This is the ordinary case rather than a rare one: an image server that is
 * installed but not started is where most people will be, and "connection
 * refused" tells them nothing they can act on.
 */
func TestAServerThatIsNotRunningSaysWhatToDo(t *testing.T) {
	_, err := Local{BaseURL: "http://127.0.0.1:1"}.Paint(context.Background(), "x", false)
	if err == nil {
		t.Fatal("it claimed to make a picture with nothing running")
	}

	if !strings.Contains(err.Error(), "Privacy") {
		t.Errorf("it does not say what to do: %v", err)
	}
}

// A paid service answering with a link rather than the picture still ends in a
// file: a link that expires in an hour is not a picture somebody has.
func TestALinkIsFollowedToThePicture(t *testing.T) {
	var pictures *httptest.Server

	pictures = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/the-picture.png" {
			raw, _ := base64.StdEncoding.DecodeString(onePixel)
			w.Write(raw)

			return
		}

		fmt.Fprintf(w, `{"data":[{"url":"%s/the-picture.png"}]}`, pictures.URL)
	}))

	defer pictures.Close()

	data, err := Paid{
		ProviderName: "openai", BaseURL: pictures.URL, APIKey: "sk-test",
		Client: pictures.Client(),
	}.Paint(context.Background(), "a quiet room", false)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(string(data), "\x89PNG") {
		t.Errorf("the link was not followed to a picture: %q", string(data))
	}
}

/*
 * A refusal in a 200 is still a refusal.
 *
 * These services answer a rejected request with a 200 and an error object
 * about as often as with a 4xx, and the message inside is the one worth
 * showing — a content refusal says what to change, where "status 200" does not.
 */
func TestARefusalDressedAsSuccessIsStillARefusal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"error":{"message":"your request was rejected by the safety system"}}`)
	}))

	defer server.Close()

	_, err := Paid{
		ProviderName: "openai", BaseURL: server.URL, APIKey: "sk-test", Client: server.Client(),
	}.Paint(context.Background(), "x", false)

	if err == nil {
		t.Fatal("a refusal was read as a picture")
	}

	if !strings.Contains(err.Error(), "safety system") {
		t.Errorf("the reason was lost: %v", err)
	}
}

/*
 * Pictures are named from what they are of.
 *
 * A folder of picture-1.png through picture-40.png is a folder nobody can use,
 * and the date first means they sort into the order they were made.
 */
func TestPicturesAreNamedFromWhatTheyAreOf(t *testing.T) {
	folder := t.TempDir()

	path, err := Save(folder, "A quiet room, morning light, дървен под", []byte("not really a png"))
	if err != nil {
		t.Fatal(err)
	}

	name := filepath.Base(path)

	if !strings.Contains(name, "a-quiet-room") {
		t.Errorf("it was saved as %q", name)
	}

	if !strings.HasSuffix(name, ".png") {
		t.Errorf("it was saved as %q", name)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("nothing was written: %v", err)
	}

	// A description with nothing usable in it still produces a filename.
	empty, err := Save(folder, "。。。", []byte("x"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(filepath.Base(empty), "picture") {
		t.Errorf("a description with no letters gave %q", filepath.Base(empty))
	}
}
