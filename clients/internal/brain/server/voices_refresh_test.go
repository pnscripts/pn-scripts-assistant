package server

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/speech"
)

/*
 * The voices 1.0 installs arrive when an updated copy first starts.
 *
 * 0.13 installed three voices fine-tuned from recordings licensed for research
 * only; 1.0 no longer speaks with them. Somebody who installed the voice agreed
 * to its download once, so the new voices are fetched at start (as the voice
 * recognition runtime is) while looking things up is on, and spoken with
 * straight away, not after a restart.
 */

// oldVoiceHome is a home with a piper install holding only 0.13's voices.
func oldVoiceHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())

	dir := filepath.Join(home, ".local", "share", "piper")
	voices := filepath.Join(dir, "voices")

	if err := os.MkdirAll(voices, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "piper"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"en_GB-alba-medium", "en_GB-jenny_dioco-medium", "en_GB-northern_english_male-medium"} {
		for _, ext := range []string{".onnx", ".onnx.json"} {
			if err := os.WriteFile(filepath.Join(voices, name+ext), []byte(name), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	speech.ForgetPiper()
	t.Cleanup(speech.ForgetPiper)

	return voices
}

func voicesServer(t *testing.T, online bool) *Server {
	t.Helper()

	cfg := config.Default()
	cfg.LookOnline = online

	return &Server{
		brain: &brain.Brain{Cfg: cfg, Root: t.TempDir()},
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func swapRefreshVoices(t *testing.T, fake func(io.Writer) (bool, error)) {
	t.Helper()

	old := refreshTheVoices
	refreshTheVoices = fake

	t.Cleanup(func() { refreshTheVoices = old })
}

func TestTheNewVoicesAreFetchedAtStartAndSpokenWithAtOnce(t *testing.T) {
	voices := oldVoiceHome(t)

	// Before: 0.13's voices only, so no neural voice at all.
	if p := speech.FindPiper(); p != nil {
		t.Fatalf("piper was found with only withdrawn voices: %+v", p)
	}

	swapRefreshVoices(t, func(w io.Writer) (bool, error) {
		io.WriteString(w, "Fetching the new voices\n")

		for _, name := range []string{"en_GB-cori-medium", "en_US-ljspeech-medium", "en_US-norman-medium"} {
			for _, ext := range []string{".onnx", ".onnx.json"} {
				if err := os.WriteFile(filepath.Join(voices, name+ext), []byte(name), 0o644); err != nil {
					return false, err
				}
			}
		}

		return true, nil
	})

	// An answer on screen meanwhile keeps its step.
	progress.Begin()
	progress.Set("thinking", "Thinking")
	t.Cleanup(progress.Done)

	voicesServer(t, true).refreshVoices(context.Background())

	if step := progress.Now(); !step.Busy || step.Kind != "thinking" || step.Note != "Thinking" {
		t.Errorf("fetching the voices wrote over or cleared the answer's step: %+v", step)
	}

	p := speech.FindPiper()
	if p == nil {
		t.Fatal("the new voices were fetched and the program still has no neural voice")
	}

	speech.SetVoice("")
	t.Cleanup(func() { speech.SetVoice("") })

	if got := speech.CurrentVoice(); got.ID != speech.RobotID || got.Path != filepath.Join(voices, "en_GB-cori-medium.onnx") {
		t.Errorf("after the update it speaks as %+v, want the robot made of Cori", got)
	}
}

// With looking things up switched off nothing is fetched: the Voice part says
// it is out of date and offers Update, which asks first.
func TestTheVoicesAreNotFetchedWithLookingOnlineOff(t *testing.T) {
	oldVoiceHome(t)

	swapRefreshVoices(t, func(io.Writer) (bool, error) {
		t.Error("the voices were fetched with looking online switched off")

		return false, nil
	})

	voicesServer(t, false).refreshVoices(context.Background())
}

// A failed fetch is logged and changes nothing; the part stays out of date.
func TestAFailedVoiceFetchLeavesTheProgramAsItWas(t *testing.T) {
	oldVoiceHome(t)

	swapRefreshVoices(t, func(io.Writer) (bool, error) {
		return false, os.ErrDeadlineExceeded
	})

	voicesServer(t, true).refreshVoices(context.Background())

	if p := speech.FindPiper(); p != nil {
		t.Errorf("a failed fetch left a neural voice in use: %+v", p)
	}
}
