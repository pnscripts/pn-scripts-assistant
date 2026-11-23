package server

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

/*
 * A window somebody is using (ASSISTANT-AWAKE-COST-1121).
 *
 * Measured on 23.11.2026 on the real window (WebKitGTK 2.52.6, a private
 * Xvfb, four cores, the pointer moving over the command centre once a
 * second): 77 % of one core on a quiet machine. On a busy one, where frames
 * are dropped, 40 to 57 %; there, without the endless CSS animations 22 %
 * and without the animation frames 30 %. Nothing was happening on the page: the
 * rings of the orb turned (which cannot be seen, a ring looks the same at
 * every angle), its dot faded smoothly, and three loops asked for every
 * frame of the screen to turn most of them down.
 *
 * So the endless animations move in steps, the rings stand still, and a loop
 * that draws less often than the screen refreshes asks pace.js for a frame
 * when it has one to draw (pnPace.frameIn).
 */

// The browser's clock, timers and frames, moved by the test; shared by the
// page scripts run here in node.
const awakeClock = `
const fs = require('fs');
const path = require('path');
const dir = process.argv[2];

let now = 0, nextTimer = 1, realFrames = 0;
const timers = new Map();
const frames = new Map();
const FRAME = 16;

globalThis.window = globalThis;
window.setInterval = (fn, ms) => { const id = nextTimer++; timers.set(id, { fn, ms: Math.max(1, ms), at: now + Math.max(1, ms), every: true }); return id; };
window.setTimeout = (fn, ms) => { const id = nextTimer++; timers.set(id, { fn, ms: Math.max(0, ms || 0), at: now + Math.max(0, ms || 0), every: false }); return id; };
window.clearInterval = (id) => { timers.delete(id); };
window.clearTimeout = (id) => { timers.delete(id); };
window.requestAnimationFrame = (fn) => { const id = nextTimer++; realFrames++; frames.set(id, fn); return id; };
window.cancelAnimationFrame = (id) => { frames.delete(id); };

function advance(ms) {
    const until = now + ms;

    while (now < until) {
        now += 1;

        for (const [id, t] of [...timers]) {
            if (!timers.has(id) || t.at > now) continue;
            if (t.every) t.at = now + t.ms; else timers.delete(id);
            t.fn();
        }

        if (now % FRAME === 0 && frames.size) {
            const due = [...frames.values()];
            frames.clear();
            for (const fn of due) fn(now);
        }
    }
}

const listeners = {};
window.addEventListener = (kind, fn) => { (listeners[kind] = listeners[kind] || []).push(fn); };
window.dispatchEvent = () => true;
window.matchMedia = () => ({ matches: false, addEventListener() {} });
globalThis.CustomEvent = class { constructor(type, init) { this.type = type; this.detail = init && init.detail; } };
`

func TestALoopIsWokenWhenItHasAFrameToDraw(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "pace.js"), []byte(readAsset(t, filepath.Join("js", "pace.js"))), 0o644)

	os.WriteFile(filepath.Join(dir, "run.js"), []byte(awakeClock+`
const onDocument = {};
globalThis.document = { hidden: false, documentElement: { dataset: {} }, getAnimations: () => [],
    addEventListener: (kind, fn) => { (onDocument[kind] = onDocument[kind] || []).push(fn); } };

// The pulse stream, said by the test.
let stream = null;
globalThis.EventSource = class {
    constructor() { this.handlers = {}; stream = this; }
    addEventListener(kind, fn) { this.handlers[kind] = fn; }
    close() { stream = null; }
};
const pulse = (said) => stream && stream.handlers.pulse({ data: JSON.stringify(said) });
const show = (shown) => { document.hidden = !shown; for (const fn of onDocument.visibilitychange || []) fn(); };

new Function(fs.readFileSync(path.join(dir, 'pace.js'), 'utf8'))();

const out = {};
const state = () => document.documentElement.dataset.pace;
const touch = () => { for (const fn of listeners.pointermove || []) fn(); };

if (!window.pnPace || typeof window.pnPace.frameIn !== 'function' || typeof window.pnPace.dropFrame !== 'function') {
    process.stdout.write(JSON.stringify({ missing: true }));
    process.exit(0);
}

// A picture drawn eight times a second, as the core is while nothing happens.
let drawn = 0, times = [];
(function draw(time) { drawn++; times.push(time); window.pnPace.frameIn(125, draw); })(0);

const during = (ms, touched) => {
    const a = drawn, f = realFrames;
    for (let done = 0; done < ms; done += 1000) { if (touched) touch(); advance(Math.min(1000, ms - done)); }
    return { drawn: drawn - a, frames: realFrames - f, state: state() };
};

// Somebody is using the page for five seconds.
out.used = during(5000, true);
out.gaps = times.slice(2).map((time, i) => time - times[i + 1]);

// A wait shorter than two frames is a plain frame.
let quick = 0;
(function draw() { quick++; if (quick < 1000) window.pnPace.frameIn(10, draw); })();
touch(); advance(1000);
out.quick = quick;

// Dropped while it waits, and dropped in the moment between the wait and
// the frame: neither is drawn. One beside them is.
let droppedEarly = 0, droppedLate = 0, kept = 0;
touch();
// Begun just after a frame of the screen, so the wait ends between two.
while (now % FRAME !== 1) advance(1);
const early = window.pnPace.frameIn(200, () => { droppedEarly++; });
const late = window.pnPace.frameIn(200, () => { droppedLate++; });
window.pnPace.frameIn(200, () => { kept++; });
advance(100);
window.pnPace.dropFrame(early);
advance(101);
window.pnPace.dropFrame(late);
advance(300);
out.dropped = { droppedEarly, droppedLate, kept };

// Left alone: calm after ten seconds, drawn at the calm pace.
advance(11000);
out.calm = during(20000, false);

// (Found by a review.) On a calm page a frame dropped by another frame of
// the same held batch is not drawn either.
let droppedInBatch = 0, dropper = 0;
let victim = 0;
window.pnPace.frameIn(40, () => { dropper++; window.pnPace.dropFrame(victim); });
victim = window.pnPace.frameIn(40, () => { droppedInBatch++; });
advance(4500);
out.batch = { dropper, droppedInBatch, state: state() };

// A minute and a half: resting, nothing drawn, and nothing asked for.
advance(90000);
out.resting = during(30000, false);

// A touch: drawn again at once.
const before = drawn;
touch(); advance(40);
out.wokeDrawn = drawn - before;
out.after = during(5000, true);

// Minimised: nothing drawn and nothing asked for; shown again, drawn at once.
show(false);
out.hidden = during(10000, false);
const beforeShown = drawn;
show(true); advance(40);
out.shownDrawn = drawn - beforeShown;

// (Found by a review.) While the assistant works the page is calm at four
// frames a second, and the loop draws at that pace: its own wait is not
// added on top of the calm one.
pulse({ busy: false, waiting: 0 });
pulse({ busy: true, waiting: 0 });
advance(12000);
out.working = during(20000, false);
pulse({ busy: false, waiting: 0 });
touch(); advance(1000);
out.waits = window.pnPace.held().waits;

process.stdout.write(JSON.stringify(out));
`), 0o644)

	raw, err := exec.Command("node", filepath.Join(dir, "run.js"), dir).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, raw)
	}

	type span struct {
		Drawn, Frames int
		State         string
	}

	var out struct {
		Missing bool
		Used    span
		Gaps    []float64
		Quick   int
		Dropped struct{ DroppedEarly, DroppedLate, Kept int }
		Calm    span
		Batch   struct {
			Dropper, DroppedInBatch int
			State                   string
		}
		Resting span
		Hidden  span
		Working span

		ShownDrawn int

		WokeDrawn int
		After     span
		Waits     int
	}

	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("what the page script said could not be read: %v\n%s", err, raw)
	}

	if out.Missing {
		t.Fatal("pace.js has no pnPace.frameIn: a loop that draws eight times a second is woken sixty times a second to turn fifty-two frames down")
	}

	// Eight times a second for five seconds, and the page woken for those
	// frames only: before, 300 frames were asked for to draw 40.
	if out.Used.State != "awake" || out.Used.Drawn < 33 || out.Used.Drawn > 41 {
		t.Errorf("a loop asking for a frame every 125 ms drew %d times in 5 s on a page that is %s; about 37 is meant", out.Used.Drawn, out.Used.State)
	}

	if out.Used.Frames > out.Used.Drawn+1 {
		t.Errorf("the page asked the browser for %d frames to draw %d", out.Used.Frames, out.Used.Drawn)
	}

	for _, gap := range out.Gaps {
		if gap < 125 || gap > 125+17 {
			t.Errorf("two frames of the loop came %v ms apart; no sooner than 125 ms and no later than the next frame after it is meant", gap)

			break
		}
	}

	if out.Quick < 55 {
		t.Errorf("a loop asking for a frame every 10 ms drew %d times in a second; every frame of the screen is meant", out.Quick)
	}

	if out.Dropped.DroppedEarly != 0 || out.Dropped.DroppedLate != 0 || out.Dropped.Kept != 1 {
		t.Errorf("dropped frames were drawn (%d while waiting, %d between the wait and the frame), the one kept %d times", out.Dropped.DroppedEarly, out.Dropped.DroppedLate, out.Dropped.Kept)
	}

	// Calm: with every other frame, one every two seconds.
	if out.Calm.State != "calm" || out.Calm.Drawn < 9 || out.Calm.Drawn > 11 {
		t.Errorf("on a %s page the loop drew %d times in 20 s; calm and one every two seconds is meant", out.Calm.State, out.Calm.Drawn)
	}

	if out.Batch.State != "calm" || out.Batch.Dropper != 1 || out.Batch.DroppedInBatch != 0 {
		t.Errorf("on a %s page a frame dropped by the frame before it in the same batch was drawn %d times (the one that dropped it %d)", out.Batch.State, out.Batch.DroppedInBatch, out.Batch.Dropper)
	}

	if out.Hidden.State != "hidden" || out.Hidden.Drawn != 0 || out.Hidden.Frames != 0 {
		t.Errorf("on a %s page the loop drew %d times and %d frames were asked for in 10 s; hidden, nothing", out.Hidden.State, out.Hidden.Drawn, out.Hidden.Frames)
	}

	if out.ShownDrawn < 1 {
		t.Error("shown again, the picture did not come back at once")
	}

	// Four frames a second for twenty seconds.
	if out.Working.State != "calm" || out.Working.Drawn < 72 || out.Working.Drawn > 84 {
		t.Errorf("while the assistant works the loop drew %d times in 20 s on a %s page; calm and four a second is meant", out.Working.Drawn, out.Working.State)
	}

	if out.Resting.State != "resting" || out.Resting.Drawn != 0 || out.Resting.Frames != 0 {
		t.Errorf("on a %s page the loop drew %d times and %d frames were asked for in 30 s; resting, nothing", out.Resting.State, out.Resting.Drawn, out.Resting.Frames)
	}

	if out.WokeDrawn < 1 {
		t.Error("a touch did not bring the picture back at once")
	}

	if out.After.Drawn < 33 || out.After.Drawn > 41 {
		t.Errorf("after resting and a touch the loop drew %d times in 5 s; about 37 is meant", out.After.Drawn)
	}

	if out.Waits > 2 {
		t.Errorf("%d waits are still kept after everything was drawn or dropped", out.Waits)
	}
}

/*
 * The sound traces beside the message box are drawn when a reading is due
 * (every 70 ms), and the page is woken for that only.
 *
 * And a fault found on the way (23.11.2026): the frame that was turned down
 * added a listener for the window's resize, every time. Four frames in five
 * were turned down, so a window in use gained some fifty listeners a
 * second, each of which measured the three canvases again on a resize.
 */
func TestTheSoundTracesAreDrawnWhenAReadingIsDue(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "dashboard.js"), []byte(readAsset(t, filepath.Join("js", "dashboard.js"))), 0o644)

	os.WriteFile(filepath.Join(dir, "run.js"), []byte(awakeClock+`
// Each canvas counts how often it was wiped to be drawn again.
function node(id) {
    const context = new Proxy({}, { get: (target, name) => name === 'clearRect' ? () => { self.cleared++; } : () => ({ addColorStop() {} }) });
    const self = { id, cleared: 0, hidden: false, textContent: '', className: '', children: [], style: { setProperty() {} }, dataset: {}, clientWidth: 120, width: 0, height: 0,
        classList: { add() {}, remove() {}, toggle() {}, contains: () => false },
        append() {}, appendChild(c) { return c; }, prepend() {}, remove() {}, replaceChildren() {}, setAttribute() {}, getAttribute: () => null,
        addEventListener() {}, querySelector: () => null, querySelectorAll: () => [], getContext: () => context,
        getBoundingClientRect: () => ({ width: 120, height: 24 }) };

    return self;
}
const nodes = {};

// One of the three traces is in a panel that is not on the screen yet.
const later = nodes.voicewave = node('voicewave');
const sized = later.getBoundingClientRect;
later.getBoundingClientRect = () => ({ width: 0, height: 0 });
globalThis.fetch = () => new Promise(() => {});
globalThis.document = { readyState: 'complete', hidden: false, addEventListener() {}, getElementById: (id) => (nodes[id] = nodes[id] || node(id)),
    querySelector: () => null, querySelectorAll: () => [], createElement: (t) => node(t), documentElement: node('html'), body: node('body') };

// pace.js as the page has it: a frame after a wait.
let waited = 0;
window.pnPace = { frameIn: (ms, fn) => { waited++; return window.setTimeout(() => window.requestAnimationFrame(fn), ms); } };

// The reading the page gets from signals.js: somebody speaking (a new
// sample every 90 ms), or a quiet room (nothing but noughts, moving on).
let speaking = true;
const wave = new Array(72).fill(0);
window.setInterval(() => { wave.push(speaking ? 0.5 : 0); wave.shift(); }, 90);
window.brainLevel = () => ({ source: speaking ? 'mic' : '', level: speaking ? 0.5 : 0, wave, pos: Math.floor(now / 90) % 72, points: 72 });

eval(fs.readFileSync(dir + '/dashboard.js', 'utf8'));

const during = (ms) => {
    const f = realFrames, c = nodes['wave-left'].cleared, r = nodes['wave-right'].cleared, l = later.cleared;
    advance(ms);
    return { frames: realFrames - f, drawn: nodes['wave-left'].cleared - c, right: nodes['wave-right'].cleared - r, later: later.cleared - l };
};

advance(1000);
const resizeBefore = (listeners.resize || []).length;
const out = { spoken: during(5000) };
out.resizeAdded = (listeners.resize || []).length - resizeBefore;
out.resizeHeard = (listeners.resize || []).length;

// The room goes quiet: what was said runs out of the trace (72 samples of
// 90 ms), and then the picture is a flat line that stays as it is.
speaking = false;
advance(8000);
out.quiet = during(5000);

// The window changes size: a canvas measured again is an empty one.
for (const fn of listeners.resize || []) fn();
out.resized = during(2000);

// The panel of the third trace comes onto the screen: a canvas that had
// no size was never taken for drawn, so its flat line is drawn now.
later.getBoundingClientRect = sized;
out.shownLater = during(1000);

// Somebody speaks again: drawn again, soon.
speaking = true;
out.again = during(1000);
out.waited = waited;

process.stdout.write(JSON.stringify(out));
`), 0o644)

	raw, err := exec.Command("node", filepath.Join(dir, "run.js"), dir).CombinedOutput()
	if err != nil {
		t.Fatalf("the page's script failed: %v\n%s", err, raw)
	}

	type span struct {
		Frames, Drawn, Right, Later int
	}

	var out struct {
		ResizeAdded, ResizeHeard, Waited          int
		Spoken, Quiet, Resized, ShownLater, Again span
	}

	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("the page's script said %s: %v", raw, err)
	}

	if out.ResizeAdded != 0 {
		t.Errorf("in five seconds of drawing the traces the page added %d listeners for the window's resize; it needs the one it has", out.ResizeAdded)
	}

	if out.ResizeHeard < 1 {
		t.Error("the page does not measure the traces again when the window changes size")
	}

	// Somebody speaking, five seconds at a reading every 70 ms (and the
	// frame after it): some sixty pictures of each of the three traces.
	if out.Spoken.Drawn < 50 || out.Spoken.Drawn > 72 {
		t.Errorf("while somebody spoke each trace was drawn %d times in 5 s; about sixty is meant (a reading every 70 ms)", out.Spoken.Drawn)
	}

	if out.Spoken.Frames > out.Spoken.Drawn+3 {
		t.Errorf("the page asked the browser for %d frames to draw the traces %d times", out.Spoken.Frames, out.Spoken.Drawn)
	}

	if out.Spoken.Right != out.Spoken.Drawn {
		t.Errorf("the two traces beside the message box were drawn %d and %d times; they are one instrument", out.Spoken.Drawn, out.Spoken.Right)
	}

	if out.Spoken.Later != 0 || out.Quiet.Later != 0 {
		t.Errorf("a trace with no size was wiped %d times", out.Spoken.Later+out.Quiet.Later)
	}

	// A quiet room is a flat line, and a flat line is drawn once: before,
	// the same line was drawn fourteen times a second for as long as the
	// window was in use.
	if out.Quiet.Drawn != 0 {
		t.Errorf("in a quiet room each trace was drawn %d times in 5 s; the flat line that is there already is meant to stay", out.Quiet.Drawn)
	}

	if out.Quiet.Frames > 25 {
		t.Errorf("in a quiet room the page asked for %d frames in 5 s to look whether there was sound; four a second is meant", out.Quiet.Frames)
	}

	if out.Resized.Drawn != 1 {
		t.Errorf("after the window changed size each trace was drawn %d times in 2 s; once, into the canvas the measuring emptied", out.Resized.Drawn)
	}

	if out.ShownLater.Later != 1 || out.ShownLater.Drawn != 0 {
		t.Errorf("a trace that got its size later was drawn %d times, and one already drawn %d times more; once and not at all is meant", out.ShownLater.Later, out.ShownLater.Drawn)
	}

	if out.Again.Drawn < 8 {
		t.Errorf("when somebody spoke again each trace was drawn %d times in the first second", out.Again.Drawn)
	}

	if out.Waited == 0 {
		t.Error("the traces do not ask pace.js for their next frame (pnPace.frameIn)")
	}
}

/*
 * Every endless animation of the stylesheet moves in steps. One that moves
 * smoothly asks for every frame of the screen for as long as it is shown,
 * and without a graphics card each of those is drawn by the processor the
 * model needs.
 */
func TestEveryEndlessAnimationMovesInSteps(t *testing.T) {
	css := readAsset(t, filepath.Join("css", "console.css"))

	steps := regexp.MustCompile(`(?m)^\s*(--in-steps[a-z-]*):\s*([^;]+);`)
	named := map[string]bool{}

	for _, m := range steps.FindAllStringSubmatch(css, -1) {
		if !strings.HasPrefix(strings.TrimSpace(m[2]), "steps(") {
			t.Errorf("%s is %q, which is not a steps() timing", m[1], m[2])
		}

		named[m[1]] = true
	}

	if len(named) == 0 {
		t.Fatal("the stylesheet names no stepped timing (--in-steps)")
	}

	endless := regexp.MustCompile(`animation(?:-iteration-count)?:[^;{}]*infinite[^;{}]*[;}]`)
	found := endless.FindAllString(css, -1)

	if len(found) < 5 {
		t.Fatalf("only %d endless animations were found in the stylesheet; the test no longer reads it", len(found))
	}

	uses := regexp.MustCompile(`var\((--in-steps[a-z-]*)\)`)

	for _, rule := range found {
		if strings.Contains(rule, "steps(") {
			continue
		}

		m := uses.FindStringSubmatch(rule)

		if m == nil || !named[m[1]] {
			t.Errorf("an endless animation that moves at every frame: %s", rule)
		}
	}

	// The rings of the orb: a ring of one colour looks the same at every
	// angle, so turning it only costs.
	for _, m := range regexp.MustCompile(`(?s)\.orb(?:\.thinking)? span[^{]*\{([^}]*)\}`).FindAllStringSubmatch(css, -1) {
		if strings.Contains(m[1], "animation") {
			t.Errorf("the rings of the orb are animated again: %s", strings.TrimSpace(m[1]))
		}
	}

	if strings.Contains(css, "orb-spin") {
		t.Error("the stylesheet still turns the rings of the orb (orb-spin)")
	}
}

/*
 * The two scenes drawn with three.js cannot run in node; what is held here
 * is how they ask for their next frame. The core draws eight or thirty times
 * a second and the memory map only while its page is open, so neither may
 * begin its frame by asking for the next one of the screen.
 */
func TestTheScenesAskForTheFramesTheyDraw(t *testing.T) {
	for _, name := range []string{"core3d.js", "memorymap3d.js"} {
		js := readAsset(t, filepath.Join("js", name))

		at := strings.Index(js, "    function frame(now) {\n")
		if at < 0 {
			t.Fatalf("%s: the frame loop was not found", name)
		}

		body := js[at:]
		first := strings.TrimSpace(strings.SplitN(body, "\n", 3)[1])

		if strings.HasPrefix(first, "requestAnimationFrame(") {
			t.Errorf("%s asks for every frame of the screen before it knows whether it will draw one", name)
		}

		if !strings.Contains(js, "window.pnPace.frameIn") || !strings.Contains(body, "frameIn(") {
			t.Errorf("%s does not ask pace.js for its frames (pnPace.frameIn)", name)
		}

		body = body[:strings.Index(body, "\n    }\n")]

		if !strings.Contains(body, "WHILE_UNSEEN") {
			t.Errorf("%s does not wait longer for its next frame while another page is open in front of it", name)
		}

		// (Found by a review.) A frame that comes a moment early is turned
		// down; what is asked for then is the rest of the wait, not the
		// whole of it again, or the picture is drawn half as often.
		if name == "core3d.js" && !regexp.MustCompile(`if \(since < gap\) \{\s+frameIn\(gap - since, frame\);\s+return;`).MatchString(body) {
			t.Errorf("%s: a frame turned down for coming early does not ask for the rest of its wait", name)
		}
	}
}
