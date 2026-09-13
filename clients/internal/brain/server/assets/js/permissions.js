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
     * What each setting means, in both halves.
     *
     * Both, because this is one switch now: it decides what the program may do
     * on this machine and what may leave it. Saying only the first half was
     * how somebody ended up with a program that had stopped asking and was
     * still refusing.
     */
    const MEANS = {
        ask: 'It asks before anything that changes something, and nothing leaves this '
            + 'machine — no hosted model, no web.',
        granted: 'It does what you have already allowed and asks about the rest. The web '
            + 'is open; the model answering you stays on this machine.',
        everything: 'It does anything it can, without asking, and nothing is held back: '
            + 'hosted models, the web, and what it has learned about you may all be sent. '
            + 'Everything is still recorded.',
    };

    // The second half again, under the control itself, because the consequence
    // of the last option is the one worth reading twice.
    const LEAVES = {
        ask: 'Nothing leaves this machine.',
        granted: 'Searches leave. Your conversations and what it knows about you do not.',
        everything: 'Anything may leave, including what it has learned about you.',
    };

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

        if (summary) summary.textContent = MEANS[data.freedom] || MEANS.ask;

        const leaves = el('freedom-means');

        if (leaves) leaves.textContent = LEAVES[data.freedom] || LEAVES.ask;

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
             * It used to say privacy was a different question, set elsewhere,
             * and that nothing on this page changed it. All three were true
             * once and none of them is now — this control is the one that
             * decides, and a panel that says otherwise is worse than a panel
             * that says nothing.
             */
            note.textContent = 'What may leave this machine follows this same switch, '
                + 'and is currently “' + (data.privacy || '—') + '”. The Privacy page '
                + 'shows what that means and what it is protecting; this is where it '
                + 'is changed.';
        }

        draw(data.capabilities || []);
        loaded = true;
    }

    function draw(list) {
        const host = el('permissions-list');

        if (!host) return;

        host.textContent = '';

        // What changes something first: those are the ones a decision is
        // about. The rest is there to be complete, not to be read.
        const order = [...list].sort((a, b) =>
            (b.changes_something ? 1 : 0) - (a.changes_something ? 1 : 0)
            || a.name.localeCompare(b.name));

        for (const c of order) host.appendChild(row(c));

        const count = el('permissions-count');

        if (count) {
            const asked = list.filter((c) => c.changes_something).length;

            count.textContent = asked;
            count.hidden = asked === 0;
        }

        const empty = el('permissions-empty');

        if (empty) empty.hidden = list.length > 0;
    }

    function row(c) {
        const item = document.createElement('div');
        item.className = 'permit';
        item.dataset.decision = c.decision;

        const text = document.createElement('span');
        text.className = 'permit-text';

        const name = document.createElement('span');
        name.className = 'permit-name';
        name.textContent = c.name.replace(/_/g, ' ');

        const what = document.createElement('span');
        what.className = 'permit-what';
        // The tool's own words, cut to a line: the full description is written
        // for a model and reads as a paragraph.
        what.textContent = (c.what || '').split('.')[0];

        text.append(name, what);

        if (c.hidden_by_privacy) {
            const why = document.createElement('span');
            why.className = 'permit-note';
            // "Why can it not do that" has two different answers and somebody
            // deserves to know which one they are looking at.
            why.textContent = 'out of reach at this privacy setting';
            text.appendChild(why);
        }

        if (c.only_this_run) {
            const why = document.createElement('span');
            why.className = 'permit-note';
            why.textContent = 'allowed until the program stops';
            text.appendChild(why);
        }

        item.appendChild(text);

        if (!c.changes_something) {
            const looks = document.createElement('span');
            looks.className = 'permit-reading';
            looks.textContent = 'only looks';
            item.appendChild(looks);

            return item;
        }

        const choice = document.createElement('select');
        choice.className = 'permit-choice';

        for (const [value, label] of [
            ['ask', 'Ask me'],
            ['allow', 'Always allow'],
            ['refuse', 'Never'],
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

        item.appendChild(choice);

        return item;
    }

    const freedom = el('freedom');

    if (freedom) {
        freedom.onchange = async () => {
            try {
                const answer = await post('/api/permissions/freedom', { level: freedom.value });

                const summary = el('freedom-summary');

                if (summary) summary.textContent = answer.means || MEANS[answer.freedom];

                const leaves = el('freedom-means');

                if (leaves) leaves.textContent = LEAVES[answer.freedom] || LEAVES.ask;
            } catch (err) {
                load();
            }
        };
    }

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
