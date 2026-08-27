/*
 * Where the accelerated core goes.
 *
 * The core can be drawn by a small Godot program on the graphics card instead
 * of by hand on the processor. That program has a window of its own, made a
 * child of the brain's window and moved to sit exactly over the panel reserved
 * for it here — a web page has no element that owns a piece of the graphics
 * card, so the surface has to be a real window placed on top.
 *
 * Which means the page has to say where the panel is. Nothing outside the page
 * can work that out: the position comes from a grid layout that depends on the
 * window size, the font metrics and which view is showing.
 *
 * When there is no accelerated core — another platform, a missing runtime,
 * anything gone wrong — none of this does anything and the page goes on drawing
 * the core itself, which is what it did before any of this existed.
 */

(function () {
    const panel = document.querySelector('.card-core');

    if (!panel) return;

    let running = false;
    let last = '';

    /*
     * Report the panel's position, and only when it has moved.
     *
     * Every report crosses a process boundary and moves a window on the X
     * server. Sending the same rectangle sixty times a second would be a great
     * deal of work to keep something exactly where it already is.
     */
    async function publish(force) {
        const box = panel.getBoundingClientRect();
        const ratio = window.devicePixelRatio || 1;

        // Hidden when the panel is not on screen — a different view, or the map
        // enlarged over it. The window knows nothing about the page's layout,
        // so left mapped it would sit over whatever replaced the panel.
        const visible = !panel.closest('[hidden]')
            && box.width > 1
            && box.height > 1
            && getComputedStyle(panel).display !== 'none';

        const at = {
            x: Math.round(box.left * ratio),
            y: Math.round(box.top * ratio),
            width: Math.round(box.width * ratio),
            height: Math.round(box.height * ratio),
            visible: visible,
        };

        const key = JSON.stringify(at);

        if (!force && key === last) return;

        last = key;

        try {
            const reply = await fetch('/api/surface', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: key,
            }).then((r) => r.json());

            setRunning(!!reply.running);
        } catch {
            setRunning(false);
        }
    }

    /*
     * When the surface is up, the page stops drawing the core.
     *
     * Both at once would be two pictures of the same thing, one of them behind
     * an opaque window and costing a processor the machine is using to think.
     */
    function setRunning(now) {
        if (now === running) return;

        running = now;
        panel.classList.toggle('has-surface', running);

        if (window.brainMapReactor) window.brainMapReactor(!running);
    }

    // The panel moves when the window resizes, when a view changes, and when
    // the map is enlarged. A ResizeObserver catches the first two directly;
    // the interval is the backstop for anything that moves it without changing
    // its size, which a grid layout can do.
    new ResizeObserver(() => publish(false)).observe(panel);

    window.addEventListener('resize', () => publish(true));
    document.addEventListener('visibilitychange', () => publish(true));

    setInterval(() => publish(false), 500);
    publish(true);
})();
