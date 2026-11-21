package server

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

/*
 * A window somebody looks at without touching it (ASSISTANT-IDLE-LOAD-2604-1118).
 *
 * Measured on 21.11.2026 on this tree's real window (WebKitGTK 2.52.6, a
 * private Xvfb, four cores): for the first minute after the last touch the
 * page cost 62 % of one core, and it was not the asking. With every poller
 * taken out it still cost 68 %; with the endless CSS animations taken out,
 * 33 %; with those and the animation frames taken out, 8 %; with all three,
 * 2 %. Each frame is drawn by the page, composited, handed to the program and
 * put on the screen by the display server, sixty times a second, for a 26
 * pixel ring that spins.
 *
 * So pace.js has a state between awake and resting: calm. Ten seconds after
 * the last touch the endless animations stand still, animation frames come
 * every two seconds and no poller asks more often than every ten; what
 * is on the page stays current, the decoration waits. While the assistant
 * works the frames come four times a second and the pollers every second and
 * a half, and it does not go on to rest. A touch, or anything happening,
 * makes it move again at once.
 *
 * pace.js runs here in node on a clock the test moves, with the browser's
 * timers, frames, animations and the pulse stream stood in for.
 */
func TestAPageNobodyTouchesGoesCalmAndStaysCurrent(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "pace.js"), []byte(readAsset(t, filepath.Join("js", "pace.js"))), 0o644)

	os.WriteFile(filepath.Join(dir, "run.js"), []byte(`
const fs = require('fs');
const path = require('path');
const dir = process.argv[2];

// A clock the test moves, and the browser's timers on it.
let now = 0, nextTimer = 1;
const timers = new Map();
const frames = new Map();
const FRAME = 16;

globalThis.window = globalThis;
window.setInterval = (fn, ms) => { const id = nextTimer++; timers.set(id, { fn, ms: Math.max(1, ms), at: now + Math.max(1, ms), every: true }); return id; };
window.setTimeout = (fn, ms) => { const id = nextTimer++; timers.set(id, { fn, ms: Math.max(0, ms || 0), at: now + Math.max(0, ms || 0), every: false }); return id; };
window.clearInterval = (id) => { timers.delete(id); };
window.clearTimeout = (id) => { timers.delete(id); };
window.requestAnimationFrame = (fn) => { const id = nextTimer++; frames.set(id, fn); return id; };
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

// Two animations the page has: one that never ends, one that plays once.
function animation(iterations) {
    return { playState: 'running', paused: 0, played: 0,
        effect: { getComputedTiming: () => ({ iterations }) },
        pause() { this.playState = 'paused'; this.paused++; },
        play() { this.playState = 'running'; this.played++; } };
}
const endless = animation(Infinity), once = animation(1);

globalThis.document = {
    hidden: false,
    documentElement: { dataset: {} },
    addEventListener() {},
    getAnimations: () => [endless, once],
};

// The pulse stream, said by the test.
let stream = null;
globalThis.EventSource = class {
    constructor() { this.handlers = {}; stream = this; }
    addEventListener(kind, fn) { this.handlers[kind] = fn; }
    close() { stream = null; }
};
const pulse = (said) => stream && stream.handlers.pulse({ data: JSON.stringify(said) });

new Function(fs.readFileSync(path.join(dir, 'pace.js'), 'utf8'))();

// What a page does: a fast poller, a slow one, and a picture drawn every frame.
const ran = { fast: 0, slow: 0, drawn: 0 };
setInterval(() => { ran.fast++; }, 100);
setInterval(() => { ran.slow++; }, 20000);
(function draw() { ran.drawn++; requestAnimationFrame(draw); })();

const out = {};
const state = () => document.documentElement.dataset.pace;
const during = (ms) => { const a = { ...ran }; advance(ms); return { fast: ran.fast - a.fast, slow: ran.slow - a.slow, drawn: ran.drawn - a.drawn }; };

pulse({ busy: false, waiting: 0 });

out.touched = { ...during(9000), state: state(), endless: endless.playState };

advance(1500);
out.calmAt = state();
out.calm = { ...during(20000), state: state(), endless: endless.playState, once: once.playState, oncePaused: once.paused };

// A touch: everything moves at once.
const before = { ...ran };
for (const fn of listeners.pointermove || []) fn();
out.woke = { state: state(), fastAtOnce: ran.fast - before.fast, endless: endless.playState };
out.awakeAgain = during(2000);

// Left alone for a minute and a half: calm, then resting, where nothing runs.
advance(90000);
out.restingAt = state();
out.resting = during(30000);

// Something happens (a turn starts): awake, then calm while it works, never resting.
pulse({ busy: true, waiting: 0 });
out.busyWoke = state();
advance(12000);
out.busyCalmAt = state();
out.busyCalm = during(20000);
advance(120000);
out.busyLater = state();

process.stdout.write(JSON.stringify(out));
`), 0o644)

	raw, err := exec.Command("node", filepath.Join(dir, "run.js"), dir).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, raw)
	}

	got := string(raw)

	var out struct {
		Touched struct {
			Fast, Slow, Drawn int
			State, Endless    string
		}
		CalmAt string
		Calm   struct {
			Fast, Slow, Drawn    int
			State, Endless, Once string
			OncePaused           int
		}
		Woke struct {
			State      string
			FastAtOnce int
			Endless    string
		}
		AwakeAgain struct{ Fast, Drawn int }
		RestingAt  string
		Resting    struct{ Fast, Slow, Drawn int }
		BusyWoke   string
		BusyCalmAt string
		BusyCalm   struct{ Fast, Drawn int }
		BusyLater  string
	}

	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("what the page script said could not be read: %v\n%s", err, got)
	}

	// Touched a moment ago: as it always was.
	if out.Touched.State != "awake" || out.Touched.Fast < 80 || out.Touched.Drawn < 500 || out.Touched.Endless != "running" {
		t.Errorf("a page touched a moment ago is not fully awake: %+v", out.Touched)
	}

	// Ten seconds after the last touch: calm.
	if out.CalmAt != "calm" || out.Calm.State != "calm" {
		t.Fatalf("ten seconds after the last touch the page is %q, then %q; it should be calm\n%s", out.CalmAt, out.Calm.State, got)
	}

	// In twenty calm seconds: a frame about every two seconds, the fast
	// poller every ten seconds and still asking, the slow one as it was.
	if out.Calm.Drawn > 12 || out.Calm.Drawn < 6 {
		t.Errorf("a calm page drew %d frames in 20 s; one every two seconds is meant", out.Calm.Drawn)
	}

	if out.Calm.Fast > 3 || out.Calm.Fast < 1 {
		t.Errorf("a calm page ran its fast poller %d times in 20 s; every ten seconds is meant, and it must still run", out.Calm.Fast)
	}

	if out.Calm.Slow != 1 {
		t.Errorf("a poller slower than the calm pace ran %d times in 20 s, not once as it asked", out.Calm.Slow)
	}

	// The endless animation stands still; the one that plays once is left
	// alone, or what it brings in would stay invisible until a touch.
	if out.Calm.Endless != "paused" {
		t.Error("the endless animation still runs on a calm page")
	}

	if out.Calm.Once != "running" || out.Calm.OncePaused != 0 {
		t.Error("an animation that plays once was stopped on a calm page")
	}

	// A touch.
	if out.Woke.State != "awake" || out.Woke.FastAtOnce < 1 || out.Woke.Endless != "running" {
		t.Errorf("a touch did not bring everything back at once: %+v", out.Woke)
	}

	if out.AwakeAgain.Fast < 15 || out.AwakeAgain.Drawn < 100 {
		t.Errorf("after a touch the page is not back to its own pace: %+v", out.AwakeAgain)
	}

	// A minute untouched: resting, as before.
	if out.RestingAt != "resting" || out.Resting.Fast != 0 || out.Resting.Slow != 0 || out.Resting.Drawn != 0 {
		t.Errorf("a page left for a minute is %q and ran %+v; resting, it runs nothing", out.RestingAt, out.Resting)
	}

	// Working: awake when it starts, calm at the working pace, never resting.
	if out.BusyWoke != "awake" || out.BusyCalmAt != "calm" || out.BusyLater != "calm" {
		t.Errorf("while the assistant works the page went %q, %q, %q; awake, calm, calm is meant", out.BusyWoke, out.BusyCalmAt, out.BusyLater)
	}

	if out.BusyCalm.Drawn < 60 || out.BusyCalm.Drawn > 90 || out.BusyCalm.Fast < 10 || out.BusyCalm.Fast > 15 {
		t.Errorf("a calm page of a working assistant drew %d frames and asked %d times in 20 s; four frames a second and every second and a half are meant",
			out.BusyCalm.Drawn, out.BusyCalm.Fast)
	}
}

// The stylesheet no longer needs a rule for calm: the animations are held
// one by one (only the endless ones), which a rule for everything cannot do.
func TestTheCalmPageStopsOnlyEndlessAnimations(t *testing.T) {
	js := readAsset(t, filepath.Join("js", "pace.js"))

	for _, needs := range []string{"getAnimations", "Infinity"} {
		if !strings.Contains(js, needs) {
			t.Errorf("pace.js does not name %s: a calm page holds the endless animations and only those", needs)
		}
	}

	css := readAsset(t, filepath.Join("css", "console.css"))

	if strings.Contains(css, `html[data-pace="calm"] *`) {
		t.Error("the stylesheet pauses every animation on a calm page; one that plays once would never show what it brings in")
	}
}
