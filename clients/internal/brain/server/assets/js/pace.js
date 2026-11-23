/*
 * pace.js — the page costs nothing while nobody is using it.
 *
 * Measured on 08.11.2026 on a four-core machine without a graphics card, the
 * window sitting idle on the command centre cost more than half a core in the
 * page and another sixth in the program behind it, and the display server
 * about as much again to put the frames on the screen: thirty-four pollers,
 * one of them asking for the microphone level eleven times a second, three
 * animation loops and a dozen endless CSS animations, all running for a page
 * nobody was looking at. Slowing each of them one by one had been tried and
 * had not moved the number, because there are too many and each new page adds
 * another.
 *
 * So the pacing is done once, here, for every timer and animation on the
 * page, and loaded before anything else so that everything after it is paced
 * without knowing it:
 *
 *   - awake: as before. Someone touched the page in the last ten seconds,
 *     or something has just happened (the server says so through /api/pulse).
 *     A loop that draws less often than the screen refreshes asks for its
 *     next frame with pnPace.frameIn, and is woken when it has one to draw.
 *   - calm: looked at, not touched, for ten seconds. What the page says stays
 *     current and its decoration waits: the endless animations stand still
 *     (only those: one that plays once still brings in what it brings),
 *     animation frames come every two seconds, and no poller asks more
 *     often than every ten seconds. While the assistant works the page stays calm
 *     instead of going on to rest, at a quicker pace: four frames a second
 *     and the pollers every second and a half.
 *   - resting: nothing for a minute. Every interval and animation frame is
 *     held, CSS animation is paused, and one server-sent stream is the only
 *     thing open. While it learns in the background, the stream says once a
 *     minute that it knows more, and the numbers are refreshed once without
 *     waking anything else. Anything that happens — a key, the mouse, a turn starting, a
 *     voice in the room, something waiting for a decision — wakes it, and the
 *     held timers run at once so nothing on the screen is stale.
 *   - hidden: minimised or behind another window. As resting, and the stream
 *     is closed too, so not a single wakeup is left.
 *
 * With reduced motion asked for in the system, it rests after ten seconds
 * instead of a minute, and the endless CSS animations are already off.
 *
 * Resting timers are held, not slowed: a slowed poller still wakes the
 * processor, the page, the program and the display, and it is the waking that
 * costs.
 *
 * Calm was added on 21.11.2026, when the minute before resting was measured
 * on the real window (WebKitGTK 2.52.6, a private Xvfb, four cores): 62 % of
 * one core, and with every poller taken out still 68 %. It was the frames:
 * without the endless CSS animations 33 %, without those and the animation
 * frames 8 %, without all three 2 %. Each frame is drawn, composited, handed
 * to the program and put on the screen by the display server, whatever its
 * size; a 26 pixel ring that spins costs a whole frame sixty times a second.
 * A working assistant kept the page awake for as long as it worked, so the
 * page took that from the model that was doing the work. Calm, the same
 * window measured 6 % (a frame a second and pollers every five seconds: 11 %).
 */
(function () {
    'use strict';

    const AWAKE_FOR = 60 * 1000;
    const REDUCED_AWAKE_FOR = 10 * 1000;

    // How long after the last touch the page goes calm.
    const CALM_AFTER = 10 * 1000;
    const REDUCED_CALM_AFTER = 2 * 1000;

    // The pace of a calm page: a frame every so many milliseconds, and no
    // poller more often than the floor. Quicker while the assistant works.
    const CALM = { frame: 2000, floor: 10000 };
    const CALM_WORKING = { frame: 250, floor: 1500 };

    const reduced = window.matchMedia
        ? window.matchMedia('(prefers-reduced-motion: reduce)')
        : { matches: false, addEventListener() {} };

    const realSetInterval = window.setInterval.bind(window);
    const realClearInterval = window.clearInterval.bind(window);
    const realSetTimeout = window.setTimeout.bind(window);
    const realClearTimeout = window.clearTimeout.bind(window);
    const realFrame = window.requestAnimationFrame.bind(window);
    const realCancelFrame = window.cancelAnimationFrame.bind(window);

    // Every interval the page has asked for, by the id it was given.
    const intervals = new Map();

    // Animation frames asked for while held, run when it wakes; and the
    // ones handed on to a real frame that has not come yet, which can
    // still be cancelled by the id they were given.
    let heldFrames = new Map();
    const handed = new Map();
    let nextFrame = 1;

    let state = 'awake';
    let restTimer = 0;
    let calmTimer = 0;

    // While calm: its pace, the timer for the next frame, the timer that
    // looks for endless animations begun since, and the ones stood still.
    let cadence = CALM;
    let frameTimer = 0;
    let stillTimer = 0;
    const stilled = new Set();
    let busy = false;
    let pulse = null;
    let lastPulse = null;
    let lastSaid = {};

    function holding() { return state === 'resting' || state === 'hidden'; }

    // How often a poller runs now: as it asked, or no more often than the
    // calm pace allows.
    function period(entry) {
        return state === 'calm' ? Math.max(entry.ms, cadence.floor) : entry.ms;
    }

    function start(entry) {
        if (entry.real !== 0) return;

        const every = period(entry);

        entry.every = every;
        entry.slowed = every !== entry.ms;
        entry.real = realSetInterval(entry.run, every);
    }

    function stop(entry) {
        if (entry.real !== 0) {
            realClearInterval(entry.real);
            entry.real = 0;
        }
    }

    /*
     * A poller is started again only when how often it runs changes. One
     * that asks no more often than the calm pace allows keeps its own
     * rhythm across awake and calm: started again at every change, a poller
     * of twenty seconds on a page touched every fifteen never came round
     * at all (found by the review of the calm state).
     */
    function repace(entry) {
        if (entry.real !== 0 && entry.every === period(entry)) return;

        stop(entry);
        start(entry);
    }

    // Ids of its own, far above the browser's, so the two never meet.
    let nextId = 1 << 30;

    window.setInterval = function (fn, ms, ...args) {
        if (typeof fn !== 'function') return realSetInterval(fn, ms, ...args);

        const id = nextId++;
        const run = () => fn(...args);
        const entry = { run, ms: Math.max(0, Number(ms) || 0), real: 0, every: 0, missed: false, slowed: false };

        intervals.set(id, entry);

        if (holding()) entry.missed = true;
        else start(entry);

        return id;
    };

    window.clearInterval = function (id) {
        const entry = intervals.get(id);

        if (entry) {
            stop(entry);
            intervals.delete(id);
        } else {
            realClearInterval(id);
        }
    };

    window.requestAnimationFrame = function (fn) {
        if (state === 'awake') return realFrame(fn);

        const id = -(nextFrame++);
        heldFrames.set(id, fn);

        // Calm: the frames asked for are drawn together, at the calm pace.
        if (state === 'calm' && frameTimer === 0) frameTimer = realSetTimeout(calmFrame, cadence.frame);

        return id;
    };

    function calmFrame() {
        frameTimer = 0;

        if (state !== 'calm') return;

        handOn();
    }

    /*
     * The endless animations stand still while the page is calm, one by one
     * and only those. A rule in the stylesheet for everything would also
     * hold an animation that plays once, and what such a one brings in (a
     * new line arriving from nothing) would stay invisible until a touch.
     * New ones begun while calm (a spinner of work that started) are found
     * at the calm pace.
     */
    function stillTheEndless() {
        if (typeof document.getAnimations !== 'function') return;

        for (const animation of document.getAnimations()) {
            let timing = null;

            try { timing = animation.effect && animation.effect.getComputedTiming(); } catch { /* not ours to hold */ }

            if (timing && timing.iterations === Infinity && animation.playState === 'running') {
                try {
                    animation.pause();
                    stilled.add(animation);
                } catch { /* gone in this moment */ }
            }
        }
    }

    function moveAgain() {
        for (const animation of stilled) {
            // Only one still standing where it was stood: an animation whose
            // rule went away meanwhile is over, and playing it would start
            // it again with nothing in the stylesheet to end it.
            if (animation.playState !== 'paused') continue;

            try { animation.play(); } catch { /* its element is gone */ }
        }

        stilled.clear();
    }

    window.cancelAnimationFrame = function (id) {
        if (id < 0) {
            heldFrames.delete(id);
            handed.delete(id);
        } else {
            realCancelFrame(id);
        }
    };

    /*
     * A frame no sooner than so many milliseconds from now, for a loop that
     * draws less often than the screen refreshes.
     *
     * Such a loop used to ask for every frame and turn most of them down:
     * the core draws eight times a second while nothing happens and was
     * woken sixty times to find that out, and so were the sound traces and
     * the memory map nobody had open. Asked for through here, the page is
     * woken when there is something to draw (measured 23.11.2026 on the
     * real window with the pointer moving over it).
     *
     * The wait is a timer and the frame after it goes through the page's
     * own requestAnimationFrame above, so a calm, resting or hidden page
     * holds it like any other. A wait shorter than two frames is a plain
     * frame: a timer cannot pace that finely. On a page that holds its
     * frames the wait is not added on top of the holding: a resting or
     * hidden page draws when it wakes, and a calm one at its own pace
     * unless the wait asked for is longer than that. Returns a number
     * that dropFrame takes.
     */
    const SHORTEST_WAIT = 34;
    const waits = new Map();
    let nextWait = 1;

    function frameIn(ms, fn) {
        const id = nextWait++;
        const entry = { timer: 0, frame: 0 };
        // Not drawn when it was dropped meanwhile, also by a frame drawn
        // just before it in the same batch of held frames.
        const drawn = (time) => {
            if (waits.delete(id)) fn(time);
        };

        waits.set(id, entry);

        const held = holding() || (state === 'calm' && Number(ms) <= cadence.frame);

        if (held || !(Number(ms) >= SHORTEST_WAIT)) {
            entry.frame = window.requestAnimationFrame(drawn);

            return id;
        }

        entry.timer = realSetTimeout(() => {
            entry.timer = 0;
            entry.frame = window.requestAnimationFrame(drawn);
        }, Number(ms));

        return id;
    }

    function dropFrame(id) {
        const entry = waits.get(id);

        if (!entry) return;

        waits.delete(id);

        if (entry.timer !== 0) realClearTimeout(entry.timer);
        else window.cancelAnimationFrame(entry.frame);
    }

    // Held frames are drawn in one real frame, each unless it was
    // cancelled in the meantime.
    function handOn() {
        if (heldFrames.size === 0) return;

        for (const [id, fn] of heldFrames) handed.set(id, fn);
        heldFrames = new Map();

        realFrame((time) => {
            const frames = [...handed];
            handed.clear();

            for (const [, fn] of frames) {
                try { fn(time); } catch (error) { console.error(error); }
            }
        });
    }

    function setState(next) {
        if (next === state) return;

        const was = state;
        state = next;

        document.documentElement.dataset.pace = next;

        if (was === 'calm') {
            realClearTimeout(frameTimer);
            realClearInterval(stillTimer);
            frameTimer = 0;
            stillTimer = 0;
        }

        if (next === 'awake') {
            moveAgain();

            for (const entry of intervals.values()) {
                // A poller that missed its turns, or ran slower than it
                // asked, runs now, so what is on the screen is current the
                // moment someone looks.
                const behind = entry.missed || entry.slowed;

                repace(entry);

                if (behind) {
                    entry.missed = false;
                    try { entry.run(); } catch (error) { console.error(error); }
                }
            }

            handOn();
        } else if (next === 'calm') {
            cadence = busy ? CALM_WORKING : CALM;

            for (const entry of intervals.values()) repace(entry);

            stillTheEndless();
            stillTimer = realSetInterval(stillTheEndless, cadence.floor);
        } else {
            for (const entry of intervals.values()) {
                stop(entry);
                entry.missed = true;
            }
        }

        if (next === 'hidden') closePulse();
        else openPulse();

        window.dispatchEvent(new CustomEvent('pn-pace', { detail: next }));
    }

    function awakeFor() { return reduced.matches ? REDUCED_AWAKE_FOR : AWAKE_FOR; }
    function calmAfter() { return reduced.matches ? REDUCED_CALM_AFTER : CALM_AFTER; }

    function rest() {
        realClearTimeout(restTimer);
        restTimer = 0;

        if (document.hidden) return;

        // Working is not resting: the page stays calm and shows the work.
        if (busy) {
            restTimer = realSetTimeout(rest, awakeFor());

            return;
        }

        setState('resting');
    }

    function calmDown() {
        calmTimer = 0;

        if (document.hidden || state !== 'awake') return;

        setState('calm');
    }

    function wake() {
        if (document.hidden) return;

        setState('awake');

        realClearTimeout(restTimer);
        realClearTimeout(calmTimer);
        restTimer = realSetTimeout(rest, awakeFor());
        calmTimer = realSetTimeout(calmDown, calmAfter());
    }

    // The stream says when the assistant starts or stops doing something.
    function openPulse() {
        if (pulse || typeof EventSource === 'undefined') return;

        pulse = new EventSource('/api/pulse');

        // Each message is what the assistant is doing now. The first one after
        // connecting (or reconnecting) only says where things stand; a change
        // after it is something happening, and wakes the page.
        pulse.addEventListener('pulse', (event) => {
            let said = {};

            try { said = JSON.parse(event.data); } catch { /* an empty pulse still wakes */ }

            busy = Boolean(said.busy);

            const first = lastPulse === null;
            const changed = event.data !== lastPulse;
            const before = lastSaid;

            lastPulse = event.data;
            lastSaid = said;

            if (first) {
                if (busy) wake();

                return;
            }

            if (!changed) return;

            // Only what it knows has grown (it is learning in the
            // background): the numbers on the screen are brought up to date
            // once, and the page goes on resting.
            if (state !== 'awake' && sameApartFromMemory(before, said)) {
                refreshOnce();

                return;
            }

            wake();
        });

        // The browser reconnects by itself; an error while resting is not a
        // reason to wake everything up. Whether the assistant works is not
        // known while the stream is down, and "working" left standing
        // would keep the page from ever resting: the first message after
        // it is back is read as the first, and wakes the page when the
        // assistant is working.
        pulse.onerror = () => {
            busy = false;
            lastPulse = null;
        };
    }

    function sameApartFromMemory(a, b) {
        const keys = new Set([...Object.keys(a || {}), ...Object.keys(b || {})]);

        for (const key of keys) {
            if (key !== 'memory' && (a || {})[key] !== (b || {})[key]) return false;
        }

        return true;
    }

    // Every held poller runs once; the timers stay held.
    function refreshOnce() {
        for (const entry of intervals.values()) {
            try { entry.run(); } catch (error) { console.error(error); }
        }
    }

    function closePulse() {
        if (pulse) {
            pulse.close();
            pulse = null;
        }
    }

    for (const kind of ['pointerdown', 'pointermove', 'keydown', 'wheel', 'touchstart', 'focus', 'resize']) {
        window.addEventListener(kind, wake, { passive: true, capture: true });
    }

    document.addEventListener('visibilitychange', () => {
        if (document.hidden) {
            realClearTimeout(restTimer);
            realClearTimeout(calmTimer);
            restTimer = 0;
            calmTimer = 0;
            setState('hidden');
        } else {
            wake();
        }
    });

    reduced.addEventListener('change', wake);

    window.pnPace = {
        state: () => state,
        wake,
        frameIn,
        dropFrame,
        // For tests: what is being held right now.
        held: () => ({ intervals: intervals.size, frames: heldFrames.size, waits: waits.size }),
    };

    document.documentElement.dataset.pace = state;

    if (document.hidden) setState('hidden');
    else {
        openPulse();
        wake();
    }
})();
