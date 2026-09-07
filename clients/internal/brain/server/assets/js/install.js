/*
 * Installing things from inside the program.
 *
 * Two lists that work the same way: the pieces the brain runs on, and the
 * models it can think with. Both were reachable only from setup — a screen for
 * starting, not for changing your mind six weeks later — so anything not
 * chosen on the first day was effectively unavailable.
 *
 * Neither list is an opinion this file holds. The pieces come from the same
 * list setup reads, and what to say about a model is worked out on the server
 * from what this machine measured, because the same model is a good idea on
 * one computer and an afternoon of waiting on another.
 */
(function () {
    const el = (id) => document.getElementById(id);

    async function call(path, payload) {
        const res = await fetch(path, {
            method: payload ? 'POST' : 'GET',
            headers: payload
                ? { 'Content-Type': 'application/json', Accept: 'application/json' }
                : { Accept: 'application/json' },
            body: payload ? JSON.stringify(payload) : undefined,
        });

        const body = await res.json().catch(() => ({}));

        if (!res.ok) throw new Error(body.error || `that did not work (${res.status})`);

        return body.data !== undefined ? body.data : body;
    }

    /*
     * A button that says what became of it.
     *
     * An install is minutes and happens in the background, so the press has to
     * leave a mark — otherwise the only evidence is a job appearing in a panel
     * on another screen, and the honest reading of a button that does nothing
     * visible is that it did nothing.
     */
    async function press(button, work, done) {
        const was = button.textContent;

        button.disabled = true;
        button.textContent = 'starting…';

        try {
            await work();
            button.textContent = done || 'started';
        } catch (err) {
            button.textContent = String(err.message || err).slice(0, 48);
            button.disabled = false;

            setTimeout(() => { button.textContent = was; }, 4000);

            return;
        }

        // Left saying "started", because it has: the job is running and the
        // row will say so on the next look.
        setTimeout(() => {
            button.textContent = was;
            button.disabled = false;
        }, 6000);
    }

    /* ---------- the pieces ---------- */

    async function loadParts() {
        const host = el('parts-list');

        if (!host) return;

        let data;

        try {
            data = await call('/api/parts');
        } catch (err) {
            return;
        }

        if (host.contains(document.activeElement)) return;

        host.textContent = '';

        let missing = 0;

        for (const p of data.parts || []) {
            if (p.state !== 'ok') missing += 1;

            host.appendChild(partRow(p));
        }

        const badge = el('parts-missing');

        if (badge) {
            badge.textContent = missing;
            badge.hidden = missing === 0;
        }
    }

    function partRow(p) {
        const row = document.createElement('div');
        row.className = 'permit';
        row.dataset.state = p.state;

        const text = document.createElement('span');
        text.className = 'permit-text';

        const name = document.createElement('span');
        name.className = 'permit-name';
        name.textContent = p.name + (p.optional ? '' : ' · needed');

        const what = document.createElement('span');
        what.className = 'permit-what';
        // Why it exists when it is here; what is lost when it is not. Those
        // are different questions and only one of them is useful at a time.
        what.textContent = p.state === 'ok'
            ? `${p.why}${p.detail ? ' — ' + p.detail : ''}`
            : `Without this: ${p.consequence}`;

        text.append(name, what);

        const note = document.createElement('span');
        note.className = 'permit-note';

        const bits = [];

        if (p.state !== 'ok' && p.size) bits.push(p.size);
        if (p.state !== 'ok' && p.needs_password) bits.push('asks for your password');
        if (p.where) bits.push((p.state === 'ok' ? 'at ' : 'goes to ') + p.where);

        note.textContent = bits.join(' · ');

        if (note.textContent) text.appendChild(note);

        row.appendChild(text);

        if (p.state === 'ok') {
            const here = document.createElement('span');
            here.className = 'permit-reading';
            here.textContent = 'installed';
            row.appendChild(here);

            return row;
        }

        if (!p.installable) {
            const by = document.createElement('span');
            by.className = 'permit-reading';
            by.textContent = 'by hand';
            by.title = p.hint || '';
            row.appendChild(by);

            return row;
        }

        const button = document.createElement('button');
        button.className = 'model-action';
        button.textContent = 'Install';
        button.onclick = () => press(button, async () => {
            await call('/api/parts/install', { name: p.name });
        }, 'installing…');

        row.appendChild(button);

        return row;
    }

    /* ---------- the models ---------- */

    async function loadCatalogue() {
        const host = el('catalogue-list');

        if (!host) return;

        let data;

        try {
            data = await call('/api/models/available');
        } catch (err) {
            return;
        }

        if (host.contains(document.activeElement)) return;

        const machine = el('catalogue-machine');

        if (machine) {
            /*
             * The machine, named, above the list.
             *
             * Every verdict below is about this computer and nothing else, and
             * saying which computer is what makes them read as measurements
             * rather than as opinions somebody typed once.
             */
            machine.textContent = `This machine: ${data.machine}. `
                + 'What each one would be like here is worked out from that.';
        }

        host.textContent = '';

        for (const m of data.models || []) host.appendChild(modelRow(m));
    }

    function modelRow(m) {
        const row = document.createElement('div');
        row.className = 'permit';
        row.dataset.state = m.installed ? 'ok' : (m.fits ? '' : 'missing');

        const text = document.createElement('span');
        text.className = 'permit-text';

        const name = document.createElement('span');
        name.className = 'permit-name';
        name.textContent = `${m.name} · ${m.job}`;

        const what = document.createElement('span');
        what.className = 'permit-what';
        what.textContent = m.what;

        const note = document.createElement('span');
        note.className = 'permit-note';
        note.textContent = `${m.size} to download · ${m.verdict}`;

        text.append(name, what, note);
        row.appendChild(text);

        if (m.installed) {
            const here = document.createElement('span');
            here.className = 'permit-reading';
            here.textContent = 'installed';
            row.appendChild(here);

            return row;
        }

        const button = document.createElement('button');
        button.className = 'model-action';
        button.textContent = 'Install';

        // Offered even when it does not fit, and said so plainly beside it.
        // Refusing outright would be deciding for somebody about their own
        // machine, and they may be about to add memory.
        if (!m.fits) button.classList.add('danger');

        button.onclick = () => press(button, async () => {
            await call('/api/models/pull', { name: m.name });
        }, 'downloading…');

        row.appendChild(button);

        return row;
    }

    function load() {
        loadParts();
        loadCatalogue();
    }

    for (const item of document.querySelectorAll('[data-view="updates"], [data-view="models"]')) {
        item.addEventListener('click', () => load());
    }

    load();
}());
