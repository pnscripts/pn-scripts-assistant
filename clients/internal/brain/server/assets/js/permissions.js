/*
 * What the brain may do on this machine.
 *
 * Its own page beside Privacy, not inside it, because they are two questions:
 * privacy is what may leave this machine, permission is what may happen on it.
 * While they were one setting the only way to let the brain do more was to let
 * more leave, and nobody would choose that trade written down plainly.
 *
 * Every capability is listed, including the ones nothing has been decided
 * about. A permissions page showing only what you have agreed to answers the
 * wrong question — the one somebody actually has is "what can this thing do".
 */
(function () {
    const el = (id) => document.getElementById(id);

    async function get(path) {
        const res = await fetch(path, { headers: { Accept: 'application/json' } });
        const body = await res.json().catch(() => ({}));

        if (!res.ok) throw new Error(body.error || `that did not work (${res.status})`);

        return body.data !== undefined ? body.data : body;
    }

    async function post(path, payload) {
        const res = await fetch(path, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
            body: JSON.stringify(payload),
        });
        const body = await res.json().catch(() => ({}));

        if (!res.ok) throw new Error(body.error || `that did not work (${res.status})`);

        return body.data !== undefined ? body.data : body;
    }

    /*
     * What each setting means for what happens on this machine.
     *
     * Only that. These once described what may leave the machine too, because
     * the two were one switch; privacy has its own setting now, and a line here
     * about what leaves would promise something this control does not decide.
     * Kept in step with permits.Means, which the server sends after a change.
     */
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);

    // What each setting means, in the page's language. Kept in step with
    // permits.Means, which the server sends after a change in English.
    const means = (level) => T('means.' + (['ask', 'granted', 'everything'].includes(level) ? level : 'ask'));
    const leavesOf = (level) => T('leaves.' + (['ask', 'granted', 'everything'].includes(level) ? level : 'ask'));

    let last = null;

    let loaded = false;

    async function load() {
        let data;

        try {
            data = await get('/api/permissions');
        } catch (err) {
            return;
        }

        const online = el('look-online');

    if (online) {
        online.onchange = async () => {
            try {
                await post('/api/permissions/look-online', { on: online.checked });
            } catch (err) {
                // Put it back rather than showing a switch that was not thrown.
                online.checked = !online.checked;
            }
        };
    }

    const freedom = el('freedom');

        /*
         * The dropdown is not refilled while somebody is using it.
         *
         * The same fault the settings form had: a poll that rewrites a select
         * every few seconds takes the choice out from under the person making
         * it, and it looks like the program arguing back.
         */
        if (freedom && document.activeElement !== freedom) {
            freedom.value = data.freedom || 'ask';
        }

        const summary = el('freedom-summary');

        if (summary) summary.textContent = means(data.freedom);

        const leaves = el('freedom-means');

        if (leaves) leaves.textContent = leavesOf(data.freedom);

        /*
         * Privacy named on this page, and only as a pointer.
         *
         * Somebody looking at permissions wants to know the other half exists
         * without the two being run together again.
         */
        const lookingUp = el('look-online');

        if (lookingUp && document.activeElement !== lookingUp) {
            lookingUp.checked = data.look_online !== false;
        }

        const note = el('permissions-privacy-note');

        if (note) {
            /*
             * Privacy is its own setting again, so this points at it rather
             * than claiming this switch decides it. For a while it said the
             * opposite, which was true then and is not now.
             */
            const mode = data.privacy ? T('privacy.' + data.privacy) : '—';

            note.textContent = T('control.privacyNote', { p: mode.startsWith('privacy.') ? data.privacy : mode });
        }

        const cite = el('cite-sources');

        if (cite && document.activeElement !== cite) {
            cite.checked = data.cite_sources !== false;
        }

        last = data.capabilities || [];
        draw(last);
        loaded = true;
    }

    function draw(list) {
        const host = el('permissions-list');

        if (!host) return;

        host.textContent = '';

        const shown = filtered(list);

        // What changes something first: those are the ones a decision is
        // about. The rest is there to be complete, not to be read.
        const order = [...shown].sort((a, b) =>
            (b.changes_something ? 1 : 0) - (a.changes_something ? 1 : 0)
            || a.name.localeCompare(b.name));

        for (const c of order) host.appendChild(row(c));

        const summary = el('control-summary');

        if (summary) {
            summary.textContent = T('control.summary', {
                n: list.length,
                l: list.filter((c) => c.leaves).length,
                o: list.filter((c) => c.off).length,
            });
        }

        const count = el('permissions-count');

        if (count) {
            const asked = list.filter((c) => c.changes_something).length;

            count.textContent = asked;
            count.hidden = asked === 0;
        }

        const empty = el('permissions-empty');

        if (empty) {
            empty.hidden = shown.length > 0;
            empty.textContent = list.length ? T('control.nothingHere') : T('control.empty');
        }
    }

    // What the "Show" choice keeps.
    function filtered(list) {
        const which = (el('control-filter') || {}).value || 'all';

        if (which === 'leaves') return list.filter((c) => c.leaves);
        if (which === 'changes') return list.filter((c) => c.changes_something);
        if (which === 'off') return list.filter((c) => c.off);

        return list;
    }

    function row(c) {
        const item = document.createElement('div');
        item.className = 'permit';
        item.dataset.decision = c.decision;

        const text = document.createElement('span');
        text.className = 'permit-text';

        const name = document.createElement('span');
        name.className = 'permit-name human';
        name.textContent = window.pnToolName ? window.pnToolName(c.name) : c.name.replace(/_/g, ' ');
        name.title = c.name;

        const what = document.createElement('span');
        what.className = 'permit-what';
        // The tool's own words, cut to a line: the full description is written
        // for a model and reads as a paragraph. Written in English, so shown
        // only on the English page; the name above is in either language.
        what.textContent = window.pnLang && window.pnLang() === 'bg' ? '' : (c.what || '').split('.')[0];
        what.hidden = what.textContent === '';

        text.append(name, what);

        /*
         * Whether using it can send anything off this machine, and what —
         * the question somebody looking at this list is actually asking.
         */
        const leaves = document.createElement('span');
        leaves.className = 'permit-leaves' + (c.leaves ? ' sends' : '');
        leaves.textContent = c.leaves
            ? T('control.canSend', { what: window.pnLeavesWhat ? window.pnLeavesWhat(c.leaves_what) : c.leaves_what })
            : T('control.stays');
        text.appendChild(leaves);

        if (c.hidden) {
            const why = document.createElement('span');
            why.className = 'permit-note';
            // "Why can it not do that" has several different answers — a
            // privacy setting, a decision about this machine, something not
            // installed — and somebody deserves to know which one they are
            // looking at. The reason comes with the answer now.
            why.textContent = c.hidden_by ? T('hidden.' + c.hidden_by) : (c.hidden_why || '');
            text.appendChild(why);
        }

        if (c.only_this_run) {
            const why = document.createElement('span');
            why.className = 'permit-note';
            why.textContent = T('control.thisRun');
            text.appendChild(why);
        }

        item.appendChild(text);
        item.appendChild(toggle(c, item));

        if (!c.changes_something) {
            const looks = document.createElement('span');
            looks.className = 'permit-reading';
            looks.textContent = T('control.onlyLooks');
            item.appendChild(looks);

            return item;
        }

        const choice = document.createElement('select');
        choice.className = 'permit-choice';

        for (const [value, label] of [
            ['ask', T('control.askMe')],
            ['allow', T('control.allow')],
            ['refuse', T('control.never')],
        ]) {
            const option = document.createElement('option');
            option.value = value;
            option.textContent = label;
            choice.appendChild(option);
        }

        choice.value = c.decision;

        choice.onchange = async () => {
            try {
                await post('/api/permissions/decide', {
                    tool: c.name,
                    answer: choice.value,
                    why: (c.what || '').split('.')[0],
                });

                item.dataset.decision = choice.value;
            } catch (err) {
                // Put it back rather than showing a decision that was not made.
                choice.value = c.decision;
            }
        };

        choice.disabled = c.off;
        item.appendChild(choice);

        return item;
    }

    /*
     * On or off, for one capability.
     *
     * Off takes it away completely — hidden from the model and refused if
     * named — through the same list "What is on this machine" keeps
     * (BRAIN_TURNED_OFF), so the two pages cannot disagree.
     */
    function toggle(c, item) {
        const label = document.createElement('label');
        label.className = 'switch';

        const box = document.createElement('input');
        box.type = 'checkbox';
        box.checked = !c.off;
        box.setAttribute('role', 'switch');
        box.setAttribute('aria-label', T('control.switch', {
            name: window.pnToolName ? window.pnToolName(c.name) : c.name,
        }));

        const word = document.createElement('span');
        word.className = 'switch-word';
        word.textContent = c.off ? T('control.off') : T('control.on');

        item.dataset.off = c.off ? 'yes' : 'no';

        box.onchange = async () => {
            const off = !box.checked;

            box.disabled = true;

            try {
                await post('/api/environment/off', { id: 'tool:' + c.name, off });

                c.off = off;
                load();
            } catch (err) {
                box.checked = !off;
                word.textContent = T('control.failed');
            } finally {
                box.disabled = false;
            }
        };

        label.append(box, word);

        return label;
    }

    const freedom = el('freedom');

    if (freedom) {
        freedom.onchange = async () => {
            try {
                const answer = await post('/api/permissions/freedom', { level: freedom.value });

                const summary = el('freedom-summary');

                if (summary) summary.textContent = means(answer.freedom);

                const leaves = el('freedom-means');

                if (leaves) leaves.textContent = leavesOf(answer.freedom);
            } catch (err) {
                load();
            }
        };
    }

    const cite = el('cite-sources');

    if (cite) {
        cite.onchange = async () => {
            try {
                await post('/api/settings/cite-sources', { on: cite.checked });
            } catch (err) {
                cite.checked = !cite.checked;
            }
        };
    }

    const filter = el('control-filter');

    if (filter) filter.onchange = () => { if (last) draw(last); };

    // Drawn again in the other language when it changes.
    window.addEventListener('pn-language', () => load());

    /*
     * Loaded when the page is opened and then left alone.
     *
     * A permissions list does not change on its own — everything on it is a
     * decision somebody made — so polling it would only be a way to fight the
     * dropdown they are using.
     */
    for (const item of document.querySelectorAll('[data-view="permissions"]')) {
        item.addEventListener('click', () => load());
    }

    document.addEventListener('DOMContentLoaded', () => {
        if (!loaded) load();
    });

    load();
}());
