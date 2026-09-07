/*
 * The loading screen, driven by what has actually happened.
 *
 * The screen itself is markup and CSS so it is on the first frame, before any
 * of this runs — see index.html. What this adds is the only part that cannot
 * be done without knowing something: which of the things being waited for have
 * finished.
 *
 * Every other panel in this program refuses to show a figure it has not
 * measured. A progress bar that fills itself over three seconds is the same
 * lie in friendlier clothes, so this one moves when a step completes and stops
 * when they have, however long that takes.
 */
(function () {
    const screen = document.getElementById('boot-screen');

    if (!screen) return;

    const step = document.getElementById('boot-step');
    const fill = document.getElementById('boot-bar-fill');
    const name = document.getElementById('boot-name');

    /*
     * The things worth waiting for, in the order they finish.
     *
     * Not everything the page does — only the parts without which the console
     * would be visibly unfinished: it has to know what it is called, what it
     * is running on, and it has to have drawn the reactor at least once.
     */
    const waitingFor = ['answered', 'named', 'drawn'];
    const done = new Set();

    // What each one is, said the way the rest of the program says things.
    const words = {
        start: 'waking up',
        answered: 'asking what it knows',
        named: 'reading its own name',
        drawn: 'lighting the reactor',
        ready: 'ready',
    };

    let gone = false;

    function say(text) {
        if (!step || gone) return;

        step.textContent = text;
    }

    function finished(what) {
        if (gone || done.has(what)) return;

        done.add(what);

        if (fill) fill.style.width = `${Math.round((done.size / waitingFor.length) * 100)}%`;

        const next = waitingFor.find((w) => !done.has(w));

        say(next ? words[next] : words.ready);

        if (waitingFor.every((w) => done.has(w))) leave();
    }

    /*
     * Leaving takes a moment on purpose.
     *
     * The shape has to finish drawing before it is thrown away, or a machine
     * that loads quickly shows a triangle mid-stroke for a hundred
     * milliseconds and then nothing — which reads as a glitch rather than as
     * having been fast. The floor is the length of the drawing animation.
     */
    const started = Date.now();
    const SHOW_AT_LEAST = 2300;

    function leave() {
        if (gone) return;

        gone = true;

        const wait = Math.max(0, SHOW_AT_LEAST - (Date.now() - started));

        setTimeout(() => {
            screen.classList.add('done');

            // Taken out of the page once it has faded, so nothing underneath is
            // covered by an invisible layer that still catches clicks.
            setTimeout(() => screen.remove(), 800);
        }, wait);
    }

    say(words.start);

    /*
     * Told by the code that does the work, rather than guessing.
     *
     * dashboard.js calls this when the first status arrives and when the name
     * is on screen; the reactor calls it when it has drawn a frame. A loading
     * screen that polls for signs of life is a loading screen that is wrong
     * about them.
     */
    window.brainBooted = finished;

    /*
     * And a way out, because a loading screen that can outlive its program is
     * worse than none at all.
     *
     * If something it is waiting for never arrives — no model, a panel that
     * failed to start — the console underneath is still usable, and sitting in
     * front of a triangle is not. Twelve seconds is well past every measured
     * start on this machine.
     */
    setTimeout(() => {
        if (gone) return;

        say('taking longer than usual — carrying on');
        leave();
    }, 12000);

    // The name it is actually called, once that is known.
    window.brainBootName = (called) => {
        if (name && called) name.textContent = called;
    };
}());
