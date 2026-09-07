/*
 * What is out of date, and doing something about it.
 *
 * One check covering the program, the engine, and everything the requirements
 * know about — because "is anything out of date" was a question with four
 * answers in four places and no way to ask it once.
 *
 * Checked when the page is opened and once an hour, never faster. Every one of
 * these asks somebody else's server, and a program that polls a release API
 * every minute is a program that gets rate-limited and then reports "up to
 * date" because it was refused.
 */
(function () {
    const el = (id) => document.getElementById(id);

    const EVERY = 60 * 60 * 1000;

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

    async function check() {
        let data;

        try {
            data = await call('/api/upkeep');
        } catch (err) {
            return;
        }

        const waiting = data.waiting || 0;

        // The badge on the rail, so it is noticed without going looking.
        for (const id of ['nav-upkeep', 'upkeep-count']) {
            const badge = el(id);

            if (!badge) continue;

            badge.textContent = waiting;
            badge.hidden = waiting === 0;
        }

        const note = el('upkeep-note');

        if (note) {
            note.textContent = data.why
                || (waiting === 0
                    ? 'Everything is current.'
                    : `${waiting} ${waiting === 1 ? 'thing is' : 'things are'} out of date.`);
        }

        draw(data.items || []);
    }

    function draw(items) {
        const host = el('upkeep-list');

        if (!host) return;

        if (host.contains(document.activeElement)) return;

        host.textContent = '';

        // What needs attention first. A list where the one out-of-date thing
        // is eighth is a list nobody reads to the end of.
        for (const i of [...items].sort((a, b) => (b.newer ? 1 : 0) - (a.newer ? 1 : 0))) {
            host.appendChild(row(i));
        }
    }

    function row(item) {
        const row = document.createElement('div');
        row.className = 'permit';
        row.dataset.state = item.newer ? 'outdated' : 'ok';

        const text = document.createElement('span');
        text.className = 'permit-text';

        const name = document.createElement('span');
        name.className = 'permit-name';
        name.textContent = item.name;

        const what = document.createElement('span');
        what.className = 'permit-what';

        /*
         * Both versions, always.
         *
         * "Out of date" without the two numbers is an assertion; with them it
         * is a fact somebody can check, and they can see how far behind they
         * actually are before deciding to spend ten minutes on it.
         */
        what.textContent = item.why
            || (item.newer
                ? `${item.have || 'yours'} → ${item.latest || 'newer'}`
                : `${item.have || 'installed'} — current`);

        text.append(name, what);
        row.appendChild(text);

        if (!item.newer) {
            const fine = document.createElement('span');
            fine.className = 'permit-reading';
            fine.textContent = item.why ? 'cannot tell' : 'current';
            row.appendChild(fine);

            return row;
        }

        const button = document.createElement('button');
        button.className = 'model-action';
        button.textContent = 'Update';

        button.onclick = async () => {
            const was = button.textContent;

            button.disabled = true;
            button.textContent = 'updating…';

            try {
                if (item.how === 'self') {
                    await call('/api/upkeep/self', {});
                } else if (item.how.startsWith('part:')) {
                    await call('/api/parts/install', { name: item.how.slice(5) });
                }

                button.textContent = 'started';
            } catch (err) {
                button.textContent = String(err.message || err).slice(0, 46);
                button.disabled = false;

                setTimeout(() => { button.textContent = was; }, 4000);
            }
        };

        row.appendChild(button);

        return row;
    }

    const again = el('upkeep-again');

    if (again) {
        again.onclick = () => {
            again.disabled = true;
            again.textContent = 'checking…';

            check().finally(() => {
                again.disabled = false;
                again.textContent = 'Check again';
            });
        };
    }

    for (const item of document.querySelectorAll('[data-view="updates"]')) {
        item.addEventListener('click', () => check());
    }

    check();
    setInterval(check, EVERY);
}());
