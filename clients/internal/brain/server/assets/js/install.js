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

        /*
         * Installed, out of date, or missing — three states and three
         * different things to offer.
         *
         * "Installed" with no button is the answer to "is it there", which is
         * the question somebody scanning this list actually has. An update
         * offered on everything would bury the two rows that need attention
         * under ten that do not.
         */
        if (p.state === 'ok') {
            const here = document.createElement('span');
            here.className = 'permit-reading';
            here.textContent = 'installed';
            row.appendChild(here);

            if (p.installable) {
                const update = document.createElement('button');
                update.className = 'model-action quiet';
                update.textContent = 'Update';
                update.title = `Fetch the current version of ${p.name}`;
                update.onclick = () => press(update, async () => {
                    await call('/api/parts/install', { name: p.name });
                }, 'updating…');

                row.appendChild(update);
            }

            /*
             * Remove, for the pieces this program put there.
             *
             * Two presses, not a dialog: the first turns the button into
             * "Sure?" and the second does it. A confirm box for a reversible
             * action somebody can simply install again is ceremony; no
             * confirmation at all for a button sitting beside "Update" is a
             * mis-click that deletes five gigabytes.
             */
            if (p.removable) {
                const remove = document.createElement('button');
                remove.className = 'model-action quiet';
                remove.textContent = 'Remove';
                remove.title = `Take ${p.name} off this machine`;

                let armed = false;

                remove.onclick = () => {
                    if (!armed) {
                        armed = true;
                        remove.textContent = 'Sure?';
                        remove.classList.add('danger');

                        // Disarms itself, so a "Sure?" left on screen from a
                        // stray click is not still waiting minutes later.
                        setTimeout(() => {
                            if (!armed) return;
                            armed = false;
                            remove.textContent = 'Remove';
                            remove.classList.remove('danger');
                        }, 4000);

                        return;
                    }

                    armed = false;
                    remove.classList.remove('danger');

                    press(remove, async () => {
                        await call('/api/parts/remove', { name: p.name });
                    }, 'removing…');
                };

                row.appendChild(remove);
            }

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
        button.textContent = p.state === 'outdated' ? 'Update' : 'Install';
        button.onclick = () => press(button, async () => {
            await call('/api/parts/install', { name: p.name });
        }, p.state === 'outdated' ? 'updating…' : 'installing…');

        row.appendChild(button);

        return row;
    }

    /* ---------- the models ---------- */

    /*
     * What can be taken off, and what the user has said should go.
     *
     * Held outside the render because the list is redrawn on a timer and the
     * ticks must survive that: a checkbox that clears itself two seconds after
     * being ticked cannot be used to choose four things.
     */
    const ticked = new Set();

    async function loadRemovable() {
        const host = el('remove-list');

        if (!host) return;

        let parts;
        let models;

        try {
            parts = await call('/api/parts');
            models = await call('/api/models');
        } catch (err) {
            return;
        }

        // Never redraw under somebody's cursor: the same rule as the parts
        // list above, and it matters more here because these are checkboxes.
        if (host.contains(document.activeElement)) return;

        host.textContent = '';

        const rows = [];

        for (const p of parts.parts || []) {
            // Only what this program installed. The system's own packages are
            // shared with the rest of the machine — poppler is what printing
            // uses — so they are not offered, and the note above says why.
            if (!p.removable || p.state !== 'ok') continue;

            rows.push({
                key: 'part:' + p.name,
                name: p.name,
                detail: p.why,
                where: p.where || '',
                // What removing it actually gives back, measured on disk.
                sizeText: p.on_disk_text || '',
                bytes: p.on_disk || 0,
                freeGB: p.free_gb || 0,
            });
        }

        for (const m of models.models || models.installed || []) {
            const name = m.name || m.model || m;

            rows.push({
                key: 'model:' + name,
                name: name,
                detail: 'a language model',
                sizeText: m.size_text || m.size || '',
                bytes: m.size_bytes || m.bytes || 0,
                model: true,
            });
        }

        if (!rows.length) {
            const none = document.createElement('p');
            none.className = 'field-note';
            none.textContent = 'There is nothing here that this program installed.';
            host.appendChild(none);
            refreshRemoveButton();

            return;
        }

        weights.clear();

        for (const r of rows) {
            weights.set(r.key, r.bytes || 0);
            host.appendChild(removeRow(r));
        }

        refreshRemoveButton();
    }

    function removeRow(r) {
        const row = document.createElement('label');
        row.className = 'remove-row';

        const tick = document.createElement('input');
        tick.type = 'checkbox';
        tick.checked = ticked.has(r.key);
        tick.onchange = () => {
            if (tick.checked) ticked.add(r.key);
            else ticked.delete(r.key);

            refreshRemoveButton();
        };

        const body = document.createElement('span');
        body.className = 'remove-body';

        const name = document.createElement('span');
        name.className = 'remove-name';
        name.textContent = r.name;
        body.appendChild(name);

        const detail = document.createElement('span');
        detail.className = 'remove-detail';
        detail.textContent = [r.detail, r.where].filter(Boolean).join(' · ');
        body.appendChild(detail);

        row.append(tick, body);

        /*
         * What it takes up, on the right where a size belongs.
         *
         * The number people are here for: this list is read by somebody who
         * wants space back, and a list of names with no sizes cannot answer
         * the question they came with.
         */
        const size = document.createElement('span');
        size.className = 'remove-size';
        size.textContent = r.sizeText || '';
        row.appendChild(size);

        return row;
    }

    // What everything on screen weighs, so the footer can total what is ticked.
    const weights = new Map();

    function inWords(bytes) {
        if (!bytes) return '';
        if (bytes < 1024 * 1024) return Math.round(bytes / 1024) + 'KB';
        if (bytes < 1024 * 1024 * 1024) return Math.round(bytes / (1024 * 1024)) + 'MB';

        return (bytes / (1024 * 1024 * 1024)).toFixed(1) + 'GB';
    }

    function refreshRemoveButton() {
        const go = el('remove-go');
        const size = el('remove-size');

        if (!go) return;

        go.disabled = ticked.size === 0;
        go.textContent = ticked.size
            ? 'Remove ' + ticked.size + (ticked.size === 1 ? ' thing' : ' things')
            : 'Remove what is ticked';

        if (size) {
            if (!ticked.size) {
                size.textContent = '';
            } else {
                /*
                 * The total is the point of ticking several.
                 *
                 * Somebody freeing space is adding these up in their head
                 * otherwise, and getting it wrong — the sizes are in three
                 * different units.
                 */
                let total = 0;

                for (const key of ticked) total += weights.get(key) || 0;

                const freed = inWords(total);

                size.textContent = (freed ? 'Frees about ' + freed + '. ' : '')
                    + 'Nothing is removed until you press the button.';
            }
        }
    }

    /*
     * Removing what was ticked, one at a time and in a safe order.
     *
     * Models before Ollama: taking away the thing that runs models first
     * leaves the model removals with no ollama to run, and they would fail
     * having reported that they were about to succeed.
     */
    async function removeTicked() {
        const go = el('remove-go');
        const chosen = [...ticked];

        const models = chosen.filter((k) => k.startsWith('model:'));
        const parts = chosen.filter((k) => k.startsWith('part:'));

        go.disabled = true;

        const failures = [];

        for (const key of [...models, ...parts]) {
            const name = key.slice(key.indexOf(':') + 1);

            try {
                if (key.startsWith('model:')) {
                    await call('/api/models/remove', { name });
                } else {
                    await call('/api/parts/remove', { name });
                }

                ticked.delete(key);
            } catch (err) {
                failures.push(name + ': ' + err.message);
            }
        }

        const size = el('remove-size');

        if (size) {
            size.textContent = failures.length
                ? failures.join(' — ')
                : 'Started. It appears under Activity while it runs.';
        }

        refreshRemoveButton();
        loadRemovable();
    }

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
             *
             * With looking things up switched off there is no list and no
             * machine reading, only the reason — which is the thing to show,
             * not "This machine: undefined".
             */
            machine.textContent = data.machine
                ? `This machine: ${data.machine}. `
                    + 'What each one would be like here is worked out from that.'
                : (data.why || '');
        }

        host.textContent = '';

        for (const m of data.models || []) host.appendChild(modelRow(m));
    }

    /*
     * Which size to show first.
     *
     * The one already here, because somebody who has a model came to look at
     * it; otherwise the largest that fits, because among the sizes this
     * machine can run the larger is usually the better answer; otherwise the
     * smallest, which is the nearest thing to fitting.
     */
    function firstChoice(variants) {
        const here = variants.find((v) => v.installed);

        if (here) return here;

        const fitting = variants.filter((v) => v.fits);

        return fitting.length ? fitting[fitting.length - 1] : variants[0];
    }

    /*
     * What is known about one size, and nothing that is not.
     *
     * The library publishes parameter counts, not download sizes, so there is
     * no download size to quote. What it wants in memory is an estimate and
     * says so; a size this cannot judge simply has no verdict. A blank is
     * honest; "undefined to download" was not.
     */
    function variantNote(v) {
        const parts = [];

        if (v.needs_gb > 0) parts.push(`wants about ${Math.ceil(v.needs_gb)}GB while running`);
        if (v.verdict) parts.push(v.verdict);

        return parts.join(' · ');
    }

    /*
     * One row per model, with its sizes in a list beside it.
     *
     * The library answers with a model and the sizes it comes in, and each
     * size is a different download with a different verdict. This used to read
     * a flat model with a size and a verdict of its own — a shape the server
     * stopped sending — so every row said "undefined".
     */
    function modelRow(m) {
        const variants = m.variants || [];

        const row = document.createElement('div');
        row.className = 'permit';

        const text = document.createElement('span');
        text.className = 'permit-text';

        const name = document.createElement('span');
        name.className = 'permit-name';
        name.textContent = m.can && m.can.length ? `${m.name} · ${m.can.join(', ')}` : m.name;
        text.appendChild(name);

        if (m.what) {
            const what = document.createElement('span');
            what.className = 'permit-what';
            what.textContent = m.what;
            what.title = m.what;
            text.appendChild(what);
        }

        const note = document.createElement('span');
        note.className = 'permit-note';
        text.appendChild(note);

        row.appendChild(text);

        if (!variants.length) return row;

        let v = firstChoice(variants);

        let size = null;

        if (variants.length > 1) {
            size = document.createElement('select');
            size.className = 'permit-choice';
            size.setAttribute('aria-label', `Size of ${m.name}`);

            for (const each of variants) {
                const option = document.createElement('option');
                option.value = each.name;
                option.textContent = (each.size || each.name) + (each.installed ? ' — installed' : '');
                option.selected = each === v;
                size.appendChild(option);
            }

            row.appendChild(size);
        }

        const button = document.createElement('button');
        row.appendChild(button);

        function show() {
            row.dataset.state = v.installed ? 'ok' : (v.fits ? '' : 'missing');

            const said = variantNote(v);

            note.textContent = said;
            note.hidden = !said;

            button.disabled = false;

            if (v.installed) {
                /*
                 * Updating a model is pulling it again.
                 *
                 * Ollama replaces it when the published one has changed and
                 * answers in a second when it has not, so the button is honest
                 * about what it does — it checks, and updates if there is
                 * anything to update. Claiming to know in advance would mean
                 * asking a registry on every page load for an answer that is
                 * almost always no.
                 */
                button.className = 'model-action quiet';
                button.textContent = 'Update';
                button.title = `Fetch ${v.name} again if it has changed`;
            } else {
                // Offered even when it does not fit, and said so plainly beside it.
                // Refusing outright would be deciding for somebody about their own
                // machine, and they may be about to add memory.
                button.className = 'model-action' + (v.fits ? '' : ' danger');
                button.textContent = 'Install';
                button.title = `Download ${v.name}`;
            }
        }

        if (size) {
            size.onchange = () => {
                v = variants.find((each) => each.name === size.value) || v;
                show();
            };
        }

        button.onclick = () => {
            const chosen = v;

            press(button, async () => {
                await call('/api/models/pull', { name: chosen.name });
            }, chosen.installed ? 'checking…' : 'downloading…');
        };

        show();

        return row;
    }

    function load() {
        loadParts();
        loadRemovable();
        loadCatalogue();
    }

    for (const item of document.querySelectorAll('[data-view="updates"], [data-view="models"]')) {
        item.addEventListener('click', () => load());
    }

    /*
     * One press to arm, a second to do it.
     *
     * The same two-step the per-row Remove uses, for the same reason and more
     * of it: this button can be carrying four things at once, and one of them
     * may be four gigabytes that took an hour to fetch.
     */
    const go = el('remove-go');

    if (go) {
        let armed = false;

        go.addEventListener('click', () => {
            if (!armed) {
                armed = true;
                go.classList.add('danger');
                go.textContent = 'Sure? This cannot be undone';

                setTimeout(() => {
                    if (!armed) return;

                    armed = false;
                    go.classList.remove('danger');
                    refreshRemoveButton();
                }, 5000);

                return;
            }

            armed = false;
            go.classList.remove('danger');
            removeTicked();
        });
    }

    load();
}());
