/*
 * The instrument panel around the conversation.
 *
 * Separate from console.js, which owns the chat, and from brainmap.js, which
 * owns the reactor. This file owns everything that reports state: the clock,
 * the navigation between views, the core strip, the ring gauges, and the audio
 * traces either side of the message box.
 *
 * One rule runs through all of it. Every value displayed here is measured — the
 * processor and memory from the kernel, the disk from the filesystem, the audio
 * from the samples, the counts from the database. Where a figure cannot be
 * obtained the panel says so instead of showing a number. The genre this is
 * drawn from is full of readouts attached to nothing, and they are the one
 * thing worth refusing: a display with invented figures on it teaches the
 * person reading it to ignore all of them, including the true ones.
 */

(function () {
    const el = (id) => document.getElementById(id);

    const get = async (path) => {
        const res = await fetch(path, { headers: { Accept: 'application/json' } });

        if (!res.ok) throw new Error(`${path} -> ${res.status}`);

        return res.json();
    };

    /* ---------- the clock ---------- */

    /*
     * Local time, from this machine.
     *
     * Not fetched from anywhere: a clock is the one readout that must never
     * depend on a network, and this program's whole point is that it does not
     * need one.
     */
    function tickClock() {
        const now = new Date();

        el('clock-time').textContent = now.toLocaleTimeString(undefined, {
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
        });

        el('clock-date').textContent = now.toLocaleDateString(undefined, {
            weekday: 'short',
            day: 'numeric',
            month: 'short',
        });
    }

    setInterval(tickClock, 1000);
    tickClock();

    /* ---------- navigation ---------- */

    const views = Array.from(document.querySelectorAll('.view'));
    const navItems = Array.from(document.querySelectorAll('.nav-item'));

    /*
     * The memory map is not a view of its own.
     *
     * It has a panel on the command centre, and asking for it enlarges that
     * panel in place rather than moving to a different screen. Moving it would
     * mean either a second canvas drawing the same graph twice, or picking the
     * canvas up and putting it down somewhere else — and a canvas that has been
     * re-parented has to be measured and redrawn from nothing. Growing the card
     * it already lives in avoids both.
     */
    const dash = document.querySelector('.dash');

    function show(name) {
        const focusMap = name === 'memory';
        const view = focusMap ? 'console' : name;

        if (dash) {
            dash.classList.toggle('focus-map', focusMap);

            // Going anywhere leaves focus mode, or the destination would be
            // hidden behind it.
            if (!focusMap) dash.classList.remove('focus-talk');
        }

        for (const item of views) item.hidden = item.dataset.view !== view;

        for (const item of navItems) {
            item.classList.toggle('is-active', item.dataset.view === name);
        }

        // A canvas in a hidden card has no size, and gets none back on its own
        // when the card returns. Measuring it again here is what stops the
        // reactor coming back blank.
        if (view === 'console' && window.brainMapResize) {
            requestAnimationFrame(() => window.brainMapResize());
        }
    }

    // Attaching these is what makes the navigation work, and it went missing
    // for a while: an edit that rewrote show() above swallowed the loop that
    // used to follow it, so every entry in the column became inert while
    // continuing to look exactly as before.
    for (const item of navItems) {
        item.addEventListener('click', () => show(item.dataset.view));
    }

    /* ---------- quick commands ---------- */

    /*
     * Every one of these does something. A dashboard full of buttons that only
     * look like buttons is the same failure as a dashboard full of numbers that
     * only look like numbers.
     */
    const commands = {
        new: () => window.brainCommands && window.brainCommands.newConversation(),
        talk: () => window.brainCommands && window.brainCommands.talk(),
        memory: () => show('memory'),
        system: () => show('system'),
    };

    for (const button of document.querySelectorAll('[data-command]')) {
        button.addEventListener('click', () => {
            const run = commands[button.dataset.command];

            if (run) run();
        });
    }

    const newButton = el('new-conversation');

    if (newButton) newButton.addEventListener('click', commands.new);

    /* ---------- search ---------- */

    /*
     * Searches what the brain knows, by meaning.
     *
     * The query goes through the same embedding and comparison a reply uses to
     * remember things, so asking for "the hosting box" finds a memory that says
     * "server". A box that matched letters would look identical and be a much
     * poorer thing.
     *
     * Debounced, because every keystroke would otherwise embed a word — and
     * embedding runs on the same processor as the model.
     */
    const searchBox = el('search');
    const searchForm = el('search-form');

    let searchTimer = null;

    async function runSearch() {
        const q = (searchBox.value || '').trim();

        if (q.length < 2) {
            if (window.brainMapRecall) window.brainMapRecall([]);

            return;
        }

        try {
            const found = await get('/api/search?q=' + encodeURIComponent(q));
            const ids = (found.results || []).map((r) => r.id);

            // The answer is shown on the map: the memories that match light up.
            // That is more use than a list, because it also shows where they
            // sit in relation to everything else.
            if (window.brainMapRecall) window.brainMapRecall(ids);

            if (ids.length) show('memory');
        } catch {
            /* A failed search is not worth interrupting anything for. */
        }
    }

    if (searchBox) {
        searchBox.addEventListener('input', () => {
            clearTimeout(searchTimer);
            searchTimer = setTimeout(runSearch, 400);
        });
    }

    if (searchForm) {
        searchForm.addEventListener('submit', (e) => {
            e.preventDefault();
            clearTimeout(searchTimer);
            runSearch();
        });
    }

    /* ---------- the buttons in the status bar ---------- */

    for (const button of document.querySelectorAll('.icon-button[data-view]')) {
        button.addEventListener('click', () => show(button.dataset.view));
    }

    const bell = el('bell');

    if (bell) {
        bell.addEventListener('click', () => {
            const card = document.querySelector('[style*="waiting"]');

            if (!card) return;

            show('console');
            card.classList.add('flash');
            setTimeout(() => card.classList.remove('flash'), 1400);
            card.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
        });
    }

    const focusButton = el('focus-button');

    if (focusButton) {
        focusButton.addEventListener('click', () => {
            if (!dash) return;

            const on = !dash.classList.contains('focus-talk');

            dash.classList.remove('focus-map');
            dash.classList.toggle('focus-talk', on);
            focusButton.querySelector('span').textContent = on
                ? 'Back to the command centre'
                : 'Focus on the conversation';

            show('console');
        });
    }

    /* ---------- what the brain is doing ---------- */

    /*
     * The voice state text, taken from the same signal the reactor uses.
     *
     * brainMapState is wrapped rather than duplicated so there is exactly one
     * place the conversation reports its state from. Two independent copies of
     * "what is happening now" is how they end up disagreeing.
     */
    const stateWords = {
        listening: 'Listening',
        thinking: 'Thinking',
        speaking: 'Speaking',
        idle: 'Idle',
    };

    let talkState = 'idle';

    const passThrough = window.brainMapState;

    window.brainMapState = function (state) {
        talkState = state || 'idle';

        if (passThrough) passThrough(state);

        const label = el('voice-state');

        if (label) {
            label.textContent = stateWords[talkState] || 'Idle';
            label.className = `voice-state ${talkState}`;
        }
    };

    /* ---------- audio traces ---------- */

    /*
     * Bars showing the sound that is actually present.
     *
     * The history comes from brainmap.js, which is already polling the level;
     * a second poller would double the requests to show the same number twice.
     *
     * Flat means silence. That is worth stating because the easy version of
     * this — bars driven by a random walk while the state is "listening" —
     * looks better and tells you nothing, and you would never know which one
     * you had.
     */
    /*
     * Sizes are measured on resize, not on every frame.
     *
     * getBoundingClientRect forces the browser to settle pending layout before
     * it can answer. Calling it on three canvases fourteen times a second, in
     * the middle of drawing, made the page recompute its layout twenty-eight
     * times a second to find out three numbers that only change when the window
     * does.
     */
    const traceSize = new WeakMap();

    function measureTrace(canvas) {
        const rect = canvas.getBoundingClientRect();

        if (rect.width < 2 || rect.height < 2) return null;

        const dpr = Math.min(window.devicePixelRatio || 1, 2);
        const size = { w: rect.width, h: rect.height };

        canvas.width = Math.floor(rect.width * dpr);
        canvas.height = Math.floor(rect.height * dpr);
        canvas.getContext('2d').setTransform(dpr, 0, 0, dpr, 0, 0);

        traceSize.set(canvas, size);

        return size;
    }

    function drawTrace(canvas, reading) {
        let rect = traceSize.get(canvas);

        // Re-measure when the element is no longer the size it was cached at.
        //
        // The cache exists so layout is not forced on every frame, but a size
        // taken once is wrong the moment the stylesheet moves the element —
        // which is how these ended up drawing into a box of the wrong shape
        // after the bar was rebuilt, and looking simply absent.
        if (!rect || Math.abs(canvas.clientWidth - rect.w) > 1) {
            rect = measureTrace(canvas);
        }

        if (!rect) return;

        const ctx = canvas.getContext('2d');

        ctx.clearRect(0, 0, rect.w, rect.h);

        // A quiet room is a flat line, and a flat line has to be visible or the
        // instrument reads as broken rather than as still. The colour says
        // which of you is making the sound; the baseline says it is listening.
        const colour = reading.source === 'voice' ? '#7bffa8'
            : reading.source === 'mic' ? '#5fe3f5'
                : '#3d6b80';

        const bars = Math.max(8, Math.floor(rect.w / 4));
        const gap = rect.w / bars;
        const middle = rect.h / 2;

        ctx.fillStyle = colour;

        // The pair either side of the message box are mirrored, so the newest
        // sound is nearest the box on both sides and they read as one
        // instrument rather than as the same picture printed twice.
        const mirrored = canvas.id === 'wave-left';

        for (let i = 0; i < bars; i++) {
            const age = mirrored ? i : bars - 1 - i;
            const index = (reading.pos - 1 - age + reading.points * 2) % reading.points;
            const sample = reading.wave[index] || 0;

            // A floor of two pixels, so a silent room is a line rather than an
            // empty box that looks broken.
            const h = Math.max(2, sample * (rect.h - 4));

            ctx.fillRect(i * gap, middle - h / 2, Math.max(1, gap - 1.5), h);
        }
    }

    const traces = [el('voicewave'), el('wave-left'), el('wave-right')].filter(Boolean);

    const pill = el('talk-pill');

    /*
     * Redrawn at the rate the readings arrive, not at the rate the screen
     * refreshes.
     *
     * The level is polled every ninety milliseconds, so drawing three canvases
     * sixty times a second redrew the same numbers five times over. It also
     * carried a canvas shadow on every bar, which is the most expensive thing
     * a canvas can be asked for and was being spent on a strip forty pixels
     * high. Both are gone.
     */
    let lastTrace = 0;

    function drawTraces(now) {
        if (now - lastTrace < 70) {
            window.addEventListener('resize', () => {
        for (const canvas of traces) measureTrace(canvas);
    });

    requestAnimationFrame(drawTraces);

            return;
        }

        lastTrace = now;

        const reading = window.brainLevel ? window.brainLevel() : null;

        if (reading) {
            for (const canvas of traces) drawTrace(canvas, reading);

            // Lit by sound that is really there, and in the colour of whoever
            // is making it.
            if (pill) {
                const want = 'talk-pill'
                    + (reading.source === 'voice' ? ' voice'
                        : reading.source && reading.level > 0.02 ? ' live' : '');

                // Only when it changes. Assigning the same string still costs a
                // style recalculation, and it restarts the glow transition, so
                // the pill was permanently mid-fade.
                if (pill.className !== want) pill.className = want;
            }
        }

        requestAnimationFrame(drawTraces);
    }

    requestAnimationFrame(drawTraces);

    /* ---------- the core rows ---------- */

    /*
     * Six subsystems, each showing what it is actually doing.
     *
     * Built from the capability list the running program reports rather than
     * from configuration, so a voice that is configured but not installed reads
     * as unavailable instead of as ready.
     */
    function renderRows(host, rows) {
        if (!host) return;

        host.innerHTML = '';

        for (const r of rows) {
            const row = document.createElement('div');
            row.className = `row ${r.tone || ''}`;

            const icon = document.createElement('span');
            icon.className = 'row-icon';
            icon.textContent = r.mark;

            const text = document.createElement('span');
            text.className = 'row-text';

            const name = document.createElement('span');
            name.className = 'row-name';
            name.textContent = r.name;

            const state = document.createElement('span');
            state.className = 'row-state';
            state.textContent = r.state;

            text.append(name, state);
            row.append(icon, text);
            host.append(row);
        }
    }

    const COLOURS = {
        project: '#4dd0e1',
        document: '#7bc47f',
        conversation: '#f0b26b',
        website: '#b39ddb',
        unknown: '#7d8b9c',
    };

    function gigabytes(bytes) {
        return `${(bytes / 1e9).toFixed(1)}GB`;
    }

    function renderStatus(status) {
        const can = (name) => (status.capabilities || []).includes(name);
        const memory = status.memory || {};
        const storage = status.storage || {};

        const text = (id, value) => {
            const node = el(id);

            if (node) node.textContent = value;
        };

        const waiting = memory.pending_lessons ?? 0;

        renderRows(el('core-rows'), [
            {
                mark: 'AI', name: 'Model',
                state: status.model || 'none loaded',
                tone: status.model ? 'ok' : 'off',
            },
            {
                mark: 'M', name: 'Memory',
                state: `${memory.facts ?? 0} remembered`,
                tone: 'ok',
            },
            {
                mark: 'R', name: 'Recall',
                state: can('recall') ? 'ready' : 'unavailable',
                tone: can('recall') ? 'ok' : 'off',
            },
            {
                mark: 'V', name: 'Voice',
                state: can('speech') ? 'ready' : 'unavailable',
                tone: can('speech') ? 'ok' : 'off',
            },
            {
                mark: 'L', name: 'Listening',
                state: can('listening') ? 'ready' : 'unavailable',
                tone: can('listening') ? 'ok' : 'off',
            },
            {
                mark: 'Q', name: 'Review queue',
                state: waiting ? `${waiting} waiting for you` : 'clear',
                tone: waiting ? 'warn' : 'ok',
            },
        ]);

        // Providers, with whether they can actually be reached.
        renderRows(el('provider-rows'), (status.providers || []).map((p) => ({
            mark: (p.name || '?').slice(0, 2).toUpperCase(),
            name: p.name,
            state: !p.permitted ? 'not permitted in this mode'
                : p.reachable ? (p.default ? 'reachable · in use' : 'reachable')
                    : 'not reachable',
            tone: !p.permitted ? 'off' : p.reachable ? 'ok' : 'warn',
        })));

        text('stat-facts', memory.facts ?? '—');
        text('stat-convos', memory.conversations ?? '—');
        text('stat-pending', memory.pending_lessons ?? '—');
        text('fig-facts', memory.facts ?? '—');
        text('fig-convos', memory.conversations ?? '—');
        text('fig-pending', memory.pending_lessons ?? '—');

        text('core-title', status.name || 'PN Brain');
        text('core-sub', status.model ? `${status.provider} · ${status.model}` : 'no model loaded');

        // The bar along the bottom.
        text('tile-privacy', status.privacy?.summary || status.privacy?.mode || '—');
        text('tile-model', status.model || 'none');
        text('tile-storage', storage.free_bytes
            ? `${gigabytes(storage.free_bytes)} free`
            : 'unknown');

        text('owner-name', status.owner || status.name || '—');

        const waitingCount = el('waiting-count');
        const waitingEmpty = el('waiting-empty');

        if (waitingCount) {
            const approvals = el('approvals');
            const lessons = el('lessons');
            const total = (approvals ? approvals.children.length : 0)
                + (lessons ? lessons.children.length : 0);

            waitingCount.textContent = total;
            waitingCount.hidden = total === 0;

            const badge = el('bell-count');

            if (badge) {
                badge.textContent = total;
                badge.hidden = total === 0;
            }

            if (waitingEmpty) waitingEmpty.hidden = total > 0;
        }

        // Counts beside the places they belong to, as a nav should have.
        const badge = (view, count) => {
            const item = document.querySelector(`.nav-item[data-view="${view}"]`);

            if (!item) return;

            let tag = item.querySelector('.nav-count');

            if (!count) {
                if (tag) tag.remove();

                return;
            }

            if (!tag) {
                tag = document.createElement('span');
                tag.className = 'nav-count';
                item.append(tag);
            }

            tag.textContent = count;
        };

        badge('memory', memory.facts || 0);

        renderPrivacyFacts(status);
        renderCategories(memory.categories || {});
        renderCapabilities(status.capabilities || []);
        renderLegend(memory.categories || {});
        renderDiskGauge(storage);
    }

    function renderPrivacyFacts(status) {
        const facts = el('privacy-facts');

        if (!facts) return;

        const rows = [
            ['Mode', status.privacy?.mode || 'unknown'],
            ['Provider', status.provider || 'none'],
            ['Model', status.model || 'none'],
            ['Data folder', status.storage?.path || 'unknown'],
        ];

        facts.innerHTML = '';

        for (const [k, v] of rows) {
            const key = document.createElement('div');
            key.className = 'k';
            key.textContent = k;

            const val = document.createElement('div');
            val.className = 'v';
            val.textContent = v;

            facts.append(key, val);
        }
    }

    function renderCategories(categories) {
        const host = el('category-breakdown');

        if (!host) return;

        const entries = Object.entries(categories).sort((a, b) => b[1] - a[1]);
        const most = entries.length ? entries[0][1] : 1;

        host.innerHTML = '';

        for (const [name, count] of entries) {
            const row = document.createElement('div');
            row.className = 'bar-row';

            const head = document.createElement('div');
            head.className = 'bar-head';
            head.innerHTML = `<span></span><b></b>`;
            head.firstChild.textContent = name;
            head.lastChild.textContent = count;

            const track = document.createElement('div');
            track.className = 'bar-track';

            const fill = document.createElement('div');
            fill.className = 'bar-fill';
            fill.style.width = `${(count / most) * 100}%`;
            fill.style.background = COLOURS[name] || COLOURS.unknown;

            track.append(fill);
            row.append(head, track);
            host.append(row);
        }
    }

    /*
     * What the brain can actually do, one tile each.
     *
     * The names come from the running program, which reports a capability only
     * when the thing behind it is present and working — so a voice that is
     * configured but not installed does not appear here at all.
     */
    const capabilityNames = {
        memory: ['Memory', 'storing what it learns'],
        recall: ['Recall', 'finding it again'],
        'memory-map': ['Memory map', 'showing what it holds'],
        speech: ['Voice', 'speaking replies aloud'],
        listening: ['Listening', 'hearing you speak'],
    };

    function renderCapabilities(capabilities) {
        const host = el('capabilities');

        if (!host) return;

        host.innerHTML = '';

        for (const name of capabilities) {
            const known = capabilityNames[name]
                || (name.startsWith('model:')
                    ? ['Model', `answering through ${name.slice(6)}`]
                    : [name, 'available']);

            const tile = document.createElement('div');
            tile.className = 'tile-card ok';

            const icon = document.createElement('span');
            icon.className = 'row-icon';
            icon.textContent = known[0].slice(0, 1).toUpperCase();

            const text = document.createElement('span');
            text.className = 'row-text';

            const title = document.createElement('span');
            title.className = 'row-name';
            title.textContent = known[0];

            const state = document.createElement('span');
            state.className = 'row-state';
            state.textContent = known[1];

            text.append(title, state);
            tile.append(icon, text);
            host.append(tile);
        }
    }

    function renderLegend(categories) {
        const host = el('legend');

        if (!host) return;

        host.innerHTML = '';

        for (const name of Object.keys(categories)) {
            const item = document.createElement('div');
            item.className = 'legend-item';

            const swatch = document.createElement('span');
            swatch.className = 'legend-swatch';
            swatch.style.background = COLOURS[name] || COLOURS.unknown;

            const label = document.createElement('span');
            label.textContent = `${name} · ${categories[name]}`;

            item.append(swatch, label);
            host.append(item);
        }
    }

    /* ---------- ring gauges ---------- */

    // The circumference of the arc in the markup, 2 * pi * 18.
    const RING = 113.1;

    function setGauge(id, valueId, percent, text) {
        const ring = el(id);
        const label = el(valueId);

        if (!ring || !label) return;

        if (percent === null || percent === undefined || percent < 0) {
            // Not measurable on this machine. The ring is left empty and the
            // reading says so, rather than resting at zero — which would be a
            // figure, and a wrong one.
            ring.style.strokeDashoffset = RING;
            ring.className.baseVal = 'gauge-fill unknown';
            label.textContent = '—';

            return;
        }

        const clamped = Math.max(0, Math.min(100, percent));

        ring.style.strokeDashoffset = RING * (1 - clamped / 100);
        ring.className.baseVal = `gauge-fill${clamped >= 90 ? ' hot' : clamped >= 70 ? ' warm' : ''}`;
        label.textContent = text ?? `${Math.round(clamped)}%`;
    }

    function renderDiskGauge(storage) {
        if (!storage || !storage.total_bytes) {
            setGauge('gauge-disk', 'disk-value', -1);

            return;
        }

        setGauge('gauge-disk', 'disk-value',
            (storage.used_bytes / storage.total_bytes) * 100);
    }

    async function pollMachine() {
        try {
            const load = await get('/api/machine');

            setGauge('gauge-cpu', 'cpu-value', load.cpu_percent);
            setGauge('gauge-ram', 'ram-value', load.memory_percent);

            // Negative means the card could not be asked, which the gauge shows
            // as unknown rather than as nought — a card reporting zero and a
            // card that cannot be read look identical on a dial and are not.
            setGauge('gauge-gpu', 'gpu-value', load.gpu_known ? load.gpu_percent : -1);

            renderDrives(load.drives || []);

            const detail = el('machine-detail');

            if (detail) {
                const rows = [
                    ['Processors', load.cores ? `${load.cores} cores` : 'unknown'],
                    ['Processor load', load.cpu_percent >= 0
                        ? `${load.cpu_percent.toFixed(0)}%` : 'not measurable'],
                    ['Memory in use', load.memory_total_bytes
                        ? `${gigabytes(load.memory_used_bytes)} of ${gigabytes(load.memory_total_bytes)}`
                        : 'not measurable'],
                    ['Graphics', load.gpu_known
                        ? `${load.gpu_percent.toFixed(0)}% of its top clock`
                        : 'not readable on this card'],
                ];

                detail.innerHTML = '';

                for (const [k, v] of rows) {
                    const key = document.createElement('div');
                    key.className = 'k';
                    key.textContent = k;

                    const val = document.createElement('div');
                    val.className = 'v';
                    val.textContent = v;

                    detail.append(key, val);
                }
            }

            const note = el('machine-note');

            if (note) {
                note.textContent = load.available
                    ? 'The model runs on this processor, which is why an answer takes as long as it does.'
                    : 'This platform does not report its load, so no figure is shown rather than a guessed one.';
            }
        } catch {
            setGauge('gauge-cpu', 'cpu-value', -1);
            setGauge('gauge-ram', 'ram-value', -1);
        }
    }

    /*
     * Every drive, not only the one the brain lives on.
     *
     * An external disk filling up is the same kind of fact as memory filling
     * up, and on this machine the brain's own data is on one — so leaving them
     * out of the monitor meant the most likely thing to run out was the one
     * thing not being watched.
     */
    function renderDrives(drives) {
        const host = el('monitor-drives');

        if (!host) return;

        host.innerHTML = '';

        for (const drive of drives) {
            const row = document.createElement('div');

            row.className = 'drive' + (drive.current ? ' current' : '');

            const path = document.createElement('span');
            path.className = 'drive-path';
            path.textContent = drive.mount_point + (drive.removable ? '  ·  removable' : '');

            const free = document.createElement('span');
            free.className = 'drive-free';

            const used = drive.total_bytes
                ? Math.round(((drive.total_bytes - drive.free_bytes) / drive.total_bytes) * 100)
                : 0;

            free.textContent = `${gigabytes(drive.free_bytes)} free · ${used}% used`;

            row.append(path, free);
            host.append(row);
        }
    }

    async function pollStatus() {
        try {
            renderStatus(await get('/api/status'));
        } catch {
            /* console.js reports the connection; this panel just goes stale. */
        }
    }

    // The load moves second by second; everything else does not.
    setInterval(pollMachine, 2000);
    setInterval(pollStatus, 5000);

    pollMachine();
    pollStatus();
})();
