package speech

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pn-scripts-assistant/internal/preflight"
)

/*
 * "A woman's voice or a man's" is the question people actually have.
 *
 * "alba, amy, lessac, northern_english_male" is not an answer to it; it is a
 * list of names that has to be researched before it can be chosen from.
 */
func TestPickingAVoiceByKind(t *testing.T) {
	// The names carry no pattern, which is why this is a list and not a rule.
	women := []string{"en_GB-cori-medium", "en_US-ljspeech-medium", "en_US-kristin-medium",
		"en_GB-alba-medium", "en_US-amy-medium", "it_IT-paola-medium"}
	men := []string{"en_US-norman-medium", "en_US-john-medium", "en_US-lessac-medium",
		"en_GB-northern_english_male-medium", "de_DE-thorsten-medium", "bg_BG-dimitar-medium"}

	for _, id := range women {
		if voiceSex[id] != "woman" {
			t.Errorf("%s is not marked as a woman's voice", id)
		}
	}

	for _, id := range men {
		if voiceSex[id] != "man" {
			t.Errorf("%s is not marked as a man's voice", id)
		}
	}

	// Nothing is guessed at: an unknown voice is offered without a description
	// rather than assigned one.
	if voiceSex["xx_XX-unknown-medium"] != "" {
		t.Error("a voice nobody has described was given a kind anyway")
	}
}

/*
 * A piper reached through a symlink still has its voices found.
 *
 * Unpacking piper into ~/.local/share/piper and linking it from ~/.local/bin
 * is the ordinary way to install it — it is what this program's own installer
 * does — and the voices sit beside the target, not beside the link. Searching
 * only beside the link found nothing, so the neural voice was installed,
 * reported as installed, and unreachable: every answer came out in the robotic
 * fallback with nothing anywhere explaining why.
 */
func TestVoicesAreFoundThroughASymlink(t *testing.T) {
	real := t.TempDir()
	linked := t.TempDir()

	// A piper installation: the binary, with its voices in a folder beside it.
	binary := filepath.Join(real, "piper")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	voices := filepath.Join(real, "voices")
	if err := os.MkdirAll(voices, 0o755); err != nil {
		t.Fatal(err)
	}

	model := filepath.Join(voices, "en_GB-test-medium.onnx")
	for _, f := range []string{model, model + ".json"} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// And the link somebody actually runs.
	link := filepath.Join(linked, "piper")
	if err := os.Symlink(binary, link); err != nil {
		t.Skipf("cannot make symlinks here: %v", err)
	}

	found := piperVoiceFiles(&Piper{Binary: link})

	var got bool

	for _, f := range found {
		if filepath.Base(f) == "en_GB-test-medium.onnx" {
			got = true
		}
	}

	if !got {
		t.Errorf("the voice beside the real binary was not found through the link; looked at %v", found)
	}
}

/*
 * A machine voice is the honest default for a machine.
 *
 * CurrentVoice used to fall through to the first entry in the list, which is
 * sorted by name — so the default was whichever neural voice happened to sort
 * first, chosen by the alphabet rather than by anybody. It also meant the
 * assistant introduced itself in a human voice nobody had picked.
 */
func TestTheRobotIsTheDefaultVoice(t *testing.T) {
	SetVoice("")

	available := Voices()

	var hasRobot bool

	for _, v := range available {
		if v.Sex == "robot" {
			hasRobot = true
		}
	}

	if !hasRobot {
		t.Skip("nothing on this machine can be a robot")
	}

	/*
	 * A robot, rather than one particular robot.
	 *
	 * There are two now: a neural voice with the machine timbre put on it,
	 * which is clear, and espeak, which is not and is kept for the machine
	 * that has no neural voice installed. Which one is available is a fact
	 * about the machine; that the default is a machine voice is the promise.
	 */
	if got := CurrentVoice(); got.Sex != "robot" {
		t.Errorf("the default voice is %q (%s), want a robot", got.ID, got.Sex)
	}
}

// And it is offered as a kind somebody can ask for by name, not only as what
// is left when nothing else is installed.
func TestTheRobotCanBeAskedForByKind(t *testing.T) {
	asked := PickVoice("robot", "")

	if asked == "" {
		t.Skip("nothing on this machine can be a robot")
	}

	var sex string

	for _, v := range Voices() {
		if v.ID == asked {
			sex = v.Sex
		}
	}

	if sex != "robot" {
		t.Errorf("asking for a robot gave %q, which is a %q", asked, sex)
	}
}

/*
 * The robot is built from a woman's voice.
 *
 * Asked for, and the only part of the robot that is a matter of taste: the
 * machine quality comes from the delivery rather than from the timbre, so the
 * voice underneath is free to be whichever one somebody wants to listen to.
 */
func TestTheRobotIsBuiltFromAWomansVoice(t *testing.T) {
	for _, id := range RobotModels[:len(RobotModels)-1] {
		if voiceSex[id] != "woman" {
			t.Errorf("%s is the %q voice, and it is not the last resort", id, voiceSex[id])
		}
	}

	// The last is a man's, deliberately: a robot that sounds like the wrong
	// person beats no robot at all on a machine that has only that model.
	last := RobotModels[len(RobotModels)-1]

	if voiceSex[last] != "man" {
		t.Errorf("the last resort is %q", voiceSex[last])
	}
}

/*
 * The robot never takes a voice that is offered in the list.
 *
 * The model the robot is made of is hidden, so a robot built from the woman
 * setup installs would silently remove the woman's voice somebody could
 * otherwise have chosen, leaving a list with a man and a robot on it and no
 * way to say what happened. Of the voices setup installs, exactly one is
 * there for the robot.
 */
func TestTheRobotDoesNotTakeAVoiceThatIsOffered(t *testing.T) {
	var forTheRobot []string

	for _, p := range preflight.PiperVoices() {
		id, model := strings.CutSuffix(p.Name, ".onnx")
		if !model {
			continue
		}

		for _, r := range RobotModels {
			if r == id {
				forTheRobot = append(forTheRobot, id)
			}
		}
	}

	if len(forTheRobot) != 1 || forTheRobot[0] != RobotModels[0] {
		t.Errorf("of the voices setup installs, the robot would be made of %v; want only %s", forTheRobot, RobotModels[0])
	}
}

// Whichever of them is installed is the one used, in order, so a machine
// missing the first still gets a neural robot rather than falling back to
// formant synthesis from 1985.
func TestTheRobotUsesWhicheverModelIsThere(t *testing.T) {
	if got := RobotModel(map[string]string{}); got != "" {
		t.Errorf("a machine with no voices gave %q", got)
	}

	only := map[string]string{"en_US-kristin-medium": "/somewhere/kristin.onnx"}

	if got := RobotModel(only); got != "en_US-kristin-medium" {
		t.Errorf("the only installed model gave %q", got)
	}

	both := map[string]string{
		"en_US-kristin-medium": "/somewhere/kristin.onnx",
		"en_GB-cori-medium":    "/somewhere/cori.onnx",
	}

	if got := RobotModel(both); got != "en_GB-cori-medium" {
		t.Errorf("with both installed it chose %q", got)
	}
}

/*
 * Nothing the robot is made of was trained from data that may only be used
 * for research.
 *
 * Every voice on the list was trained from scratch on recordings in the
 * public domain, or fine-tuned from one that was (john from kristin). The
 * ones fine-tuned from lessac, whose recordings are licensed for research
 * only, or trained on non-commercial data, were on it until 1.0.
 */
func TestTheRobotIsMadeOnlyOfVoicesFreeToUse(t *testing.T) {
	free := map[string]bool{
		"en_GB-cori-medium": true, "en_GB-cori-high": true,
		"en_US-kristin-medium": true, "en_US-john-medium": true,
	}

	for _, id := range RobotModels {
		if !free[id] {
			t.Errorf("the robot can be made of %s, whose lineage is not known to be free to use", id)
		}
	}
}

// The default the assistant answers in is a British woman's voice, Cori's,
// with the machine timbre on it.
func TestTheDefaultRobotIsMadeOfCori(t *testing.T) {
	if RobotModels[0] != "en_GB-cori-medium" {
		t.Errorf("the robot is made of %s first", RobotModels[0])
	}
}

// pipersHome makes a home with a piper install holding the voices named, and
// points the search at it for the rest of the test.
func pipersHome(t *testing.T, names ...string) string {
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

	for _, name := range names {
		for _, ext := range []string{".onnx", ".onnx.json"} {
			if err := os.WriteFile(filepath.Join(voices, name+ext), []byte(name), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	ForgetPiper()
	t.Cleanup(ForgetPiper)

	SetVoice("")
	t.Cleanup(func() { SetVoice("") })

	return voices
}

/*
 * A copy updated from 0.13 has the old voices and the new ones side by side,
 * and only the new ones are offered.
 *
 * The old files are not deleted (that is the person's choice), so the list is
 * what keeps them silent: not offered, not the robot, not the voice piper
 * falls back to.
 */
func TestTheWithdrawnVoicesAreNeitherOfferedNorUsed(t *testing.T) {
	voices := pipersHome(t,
		"en_GB-alba-medium", "en_GB-jenny_dioco-medium", "en_GB-northern_english_male-medium",
		"en_GB-cori-medium", "en_US-ljspeech-medium", "en_US-norman-medium")

	if p := FindPiper(); p == nil || preflight.Withdrawn(p.Voice) {
		t.Fatalf("piper was found as %+v", p)
	}

	kinds := map[string]Voice{}

	for _, v := range Voices() {
		if preflight.Withdrawn(v.ID) || preflight.Withdrawn(v.Path) {
			t.Errorf("%s (%s) is offered", v.ID, v.Path)
		}

		if v.Engine == "piper" {
			kinds[v.Sex] = v
		}
	}

	if robot := kinds["robot"]; robot.ID != RobotID || robot.Path != filepath.Join(voices, "en_GB-cori-medium.onnx") {
		t.Errorf("the robot is %+v, want it made of Cori", robot)
	}

	if woman := kinds["woman"]; woman.ID != "en_US-ljspeech-medium" || woman.Name != "LJ Speech · American" {
		t.Errorf("the woman is %+v", woman)
	}

	if man := kinds["man"]; man.ID != "en_US-norman-medium" || man.Name != "Norman · American" {
		t.Errorf("the man is %+v", man)
	}

	// And the default, with nothing chosen, is that robot.
	if got := CurrentVoice(); got.ID != RobotID || got.Engine != "piper" {
		t.Errorf("the default is %+v", got)
	}
}

// A copy that has only the old voices (looking online off, the update not yet
// pressed) speaks in the system voice rather than in a withdrawn one.
func TestOnlyTheWithdrawnVoicesIsNoNeuralVoice(t *testing.T) {
	pipersHome(t, "en_GB-alba-medium", "en_GB-jenny_dioco-medium", "en_GB-northern_english_male-medium")

	if p := FindPiper(); p != nil {
		t.Errorf("piper was found with only withdrawn voices: %+v", p)
	}

	for _, v := range Voices() {
		if v.Engine == "piper" {
			t.Errorf("%s is offered", v.ID)
		}
	}
}

/*
 * A voice chosen in 0.13 that is withdrawn now becomes the default.
 *
 * The setting is moved when the brain opens (brain.New); this is the rule it
 * asks. Everything else somebody chose is theirs and stays.
 */
func TestAVoiceFromAnOlderReleaseIsReplacedByTheDefault(t *testing.T) {
	for _, old := range []string{
		"en_GB-alba-medium", "en_GB-jenny_dioco-medium", "en_GB-northern_english_male-medium",
		"/home/alex/.local/share/piper/voices/en_GB-alba-medium.onnx",
	} {
		if got := Replacing(old); got != RobotID {
			t.Errorf("%s is replaced by %q, want the robot", old, got)
		}
	}

	for _, kept := range []string{"", RobotID, RobotVoice, WomanVoice, ManVoice,
		"en_US-ljspeech-medium", "de_DE-thorsten-medium", "system:klatt4"} {
		if got := Replacing(kept); got != kept {
			t.Errorf("%q was changed to %q", kept, got)
		}
	}

	// And until it is moved, choosing one speaks in the default anyway.
	pipersHome(t, "en_GB-alba-medium", "en_GB-cori-medium", "en_US-ljspeech-medium", "en_US-norman-medium")

	SetVoice("en_GB-alba-medium")

	if got := CurrentVoice(); got.ID != RobotID {
		t.Errorf("a withdrawn voice still chosen speaks as %q", got.ID)
	}
}
