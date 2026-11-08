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
 *   - awake: as before. Someone touched the page in the last minute, or the
 *     assistant is doing something (the server says so through /api/pulse).
 *   - resting: nothing for a minute. Every interval and animation frame is
 *     held, CSS animation is paused, and one server-sent stream is the only
 *     thing open. Anything that happens — a key, the mouse, a turn starting, a
 *     voice in the room, something waiting for a decision — wakes it, and the
 *     held timers run at once so nothing on the screen is stale.
 *   - hidden: minimised or behind another window. As resting, and the stream
 *     is closed too, so not a single wakeup is left.
 *
 * With reduced motion asked for in the system, it rests after ten seconds
 * instead of a minute, and the endless CSS animations are already off.
 *
 * Timers are held, not slowed: a slowed poller still wakes the processor, the
 * page, the program and the display, and it is the waking that costs.
 */
(function () {
    'use strict';

    const AWAKE_FOR = 60 * 1000;
    const REDUCED_AWAKE_FOR = 10 * 1000;

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

    // Animation frames asked for while held, run when it wakes.
    let heldFrames = new Map();
    let nextFrame = 1;

    let state = 'awake';
    let restTimer = 0;
    let busy = false;
    let pulse = null;
    let lastPulse = null;

    function holding() { return state !== 'awake'; }

    function start(entry) {
        if (entry.real === 0) entry.real = realSetInterval(entry.run, entry.ms);
    }

    function stop(entry) {
        if (entry.real !== 0) {
            realClearInterval(entry.real);
            entry.real = 0;
        }
    }

    // Ids of its own, far above the browser's, so the two never meet.
    let nextId = 1 << 30;

    window.setInterval = function (fn, ms, ...args) {
        if (typeof fn !== 'function') return realSetInterval(fn, ms, ...args);

        const id = nextId++;
        const run = () => fn(...args);
        const entry = { run, ms: Math.max(0, Number(ms) || 0), real: 0, missed: false };

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
        if (!holding()) return realFrame(fn);

        const id = -(nextFrame++);
        heldFrames.set(id, fn);

        return id;
    };

    window.cancelAnimationFrame = function (id) {
        if (id < 0) heldFrames.delete(id);
        else realCancelFrame(id);
    };

    function setState(next) {
        if (next === state) return;

        const was = state;
        state = next;

        document.documentElement.dataset.pace = next;

        if (next === 'awake') {
            for (const entry of intervals.values()) {
                start(entry);

                // A poller that missed its turns runs now, so what is on the
                // screen is current the moment someone looks.
                if (entry.missed) {
                    entry.missed = false;
                    try { entry.run(); } catch (error) { console.error(error); }
                }
            }

            const frames = heldFrames;
            heldFrames = new Map();

            for (const fn of frames.values()) realFrame(fn);
        } else if (was === 'awake') {
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

    function rest() {
        realClearTimeout(restTimer);
        restTimer = 0;

        if (document.hidden) return;

        // Working is not resting: the picture shows the work.
        if (busy) {
            restTimer = realSetTimeout(rest, awakeFor());

            return;
        }

        setState('resting');
    }

    function wake() {
        if (document.hidden) return;

        setState('awake');

        realClearTimeout(restTimer);
        restTimer = realSetTimeout(rest, awakeFor());
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

            lastPulse = event.data;

            if ((first && busy) || (!first && changed)) wake();
        });

        // The browser reconnects by itself; an error while resting is not a
        // reason to wake everything up.
        pulse.onerror = () => {};
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
            restTimer = 0;
            setState('hidden');
        } else {
            wake();
        }
    });

    reduced.addEventListener('change', wake);

    window.pnPace = {
        state: () => state,
        wake,
        // For tests: what is being held right now.
        held: () => ({ intervals: intervals.size, frames: heldFrames.size }),
    };

    document.documentElement.dataset.pace = state;

    if (document.hidden) setState('hidden');
    else {
        openPulse();
        wake();
    }
})();
