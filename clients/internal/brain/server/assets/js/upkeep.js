/*
 * What is out of date, and doing something about it.
 *
 * One check covering the program, the engine, and everything the requirements
 * know about — because "is anything out of date" was a question with four
 * answers in four places and no way to ask it once.
 *
 * Checked when the page is opened and once an hour, never faster; the server
 * keeps the answer about the program's own releases for a day, so GitHub is
 * asked at most once a day unless somebody presses "Check again". A program
 * that polls a release API every minute is a program that gets rate-limited
 * and then reports "up to date" because it was refused.
 *
 * The program's own row, and the banner across the top, offer the update the
 * way this copy was installed: the Windows installer downloads, checks and
 * runs the new setup; a copy from apt opens Software Updater; a copy nothing
 * installed opens the release page. Nothing fights the package manager.
 */
(function () {
    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);

    const EVERY = 60 * 60 * 1000;

    let last = null;

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

    async function check(fresh) {
        let data;

        try {
            data = await call('/api/upkeep' + (fresh ? '?fresh=1' : ''));
        } catch (err) {
            return;
        }

        last = data;
        draw();
    }

    // The program's own row, when it is there.
    function program(items) {
        return (items || []).find((i) => i.channel);
    }

    function draw() {
        if (!last) return;

        const data = last;
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
            if (data.why) {
                note.textContent = data.why_key ? T(data.why_key) : data.why;
            } else if (waiting === 0 && (data.items || []).some((i) => i.why)) {
                // "Everything is current" would be a claim about what was not
                // looked at — offline, say, or checks switched off.
                note.textContent = T('upd.partly');
            } else if (waiting === 0) {
                note.textContent = T('upd.allCurrent');
            } else {
                note.textContent = waiting === 1 ? T('upd.oneWaiting') : T('upd.manyWaiting', { n: waiting });
            }
        }

        const notice = el('upkeep-notice');

        if (notice && typeof data.notice === 'boolean' && document.activeElement !== notice) {
            notice.checked = data.notice;
        }

        list(data.items || []);
        banner(program(data.items));
    }

    function list(items) {
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

    // What the button says, for how this copy was installed.
    function actionLabel(item) {
        switch (item.channel) {
        case 'installer': return T('upd.updateNow');
        case 'apt': return T('upd.openUpdater');
        case 'snap': return T('upd.how');
        case 'deb':
        case 'portable': return T('upd.getIt');
        default: return T('upd.update');
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
        if (item.why) {
            what.textContent = item.why_key ? T(item.why_key) : item.why;
        } else if (item.newer) {
            what.textContent = T('upd.rowNewer', { have: item.have || '?', latest: item.latest || '?' });
        } else {
            what.textContent = T('upd.rowCurrent', { have: item.have || '' });
        }

        text.append(name, what);
        row.appendChild(text);

        if (!item.newer) {
            const fine = document.createElement('span');
            fine.className = 'permit-reading';
            fine.textContent = item.why ? T('upd.cannotTellShort') : T('upd.isCurrent');
            row.appendChild(fine);

            return row;
        }

        const button = document.createElement('button');
        button.className = 'model-action';
        button.textContent = actionLabel(item);
        button.onclick = () => run(item, button);

        row.appendChild(button);

        return row;
    }

    // Says what happened under the list: a sentence, and a command to copy
    // when there is one.
    function say(sentence, command) {
        const said = el('upkeep-said');
        const code = el('upkeep-command');

        if (said) {
            said.textContent = sentence || '';
            said.hidden = !sentence;
        }

        if (code) {
            code.textContent = command || '';
            code.hidden = !command;
        }
    }

    async function run(item, button) {
        const was = button.textContent;

        button.disabled = true;
        button.textContent = T('upd.updating');

        try {
            if (item.how === 'self') {
                const out = await call('/api/upkeep/self', {});

                // A copy built from source answers with "started" text of
                // its own; a released copy with an outcome.
                if (typeof out.started === 'string') {
                    say(out.started);
                } else {
                    say(out.key ? T(out.key, { v: item.latest || '' }) : out.say, out.command);

                    if (out.open) window.open(out.open, '_blank', 'noopener,noreferrer');
                }

                button.textContent = out.started ? T('upd.started') : was;
                button.disabled = !!out.started;

                // So the Updates page shows what was said.
                if (button.closest('#update-banner')) show('updates');
            } else if (item.how && item.how.startsWith('part:')) {
                await call('/api/parts/install', { name: item.how.slice(5) });
                button.textContent = T('upd.started');
            }
        } catch (err) {
            say(T('upd.failed', { why: String(err.message || err) }));
            button.textContent = was;
            button.disabled = false;
        }
    }

    function show(view) {
        const nav = document.querySelector(`.nav-item[data-view="${view}"]`);

        if (nav) nav.click();
    }

    /*
     * The banner: only for the program itself, only when it is newer, and
     * not again for a version somebody said "Later" to. Remembered in this
     * browser only, and the page works the same when it cannot remember.
     */
    function dismissed(version) {
        try {
            return window.localStorage.getItem('pn-update-later') === version;
        } catch (err) {
            return false;
        }
    }

    function banner(item) {
        const box = el('update-banner');

        if (!box) return;

        if (!item || !item.newer || !item.latest || dismissed(item.latest)) {
            box.hidden = true;

            return;
        }

        el('update-banner-text').textContent = T('upd.banner', {
            name: item.name, latest: item.latest, have: item.have,
        });

        const go = el('update-banner-go');

        go.textContent = actionLabel(item);
        go.disabled = false;
        go.onclick = () => run(item, go);

        el('update-banner-later').onclick = () => {
            try {
                window.localStorage.setItem('pn-update-later', item.latest);
            } catch (err) { /* hidden for now all the same */ }

            box.hidden = true;
        };

        box.hidden = false;
    }

    const again = el('upkeep-again');

    if (again) {
        again.onclick = () => {
            again.disabled = true;
            again.textContent = T('upd.checkingNow');

            check(true).finally(() => {
                again.disabled = false;
                again.textContent = T('upd.again');
            });
        };
    }

    const notice = el('upkeep-notice');

    if (notice) {
        notice.onchange = async () => {
            notice.disabled = true;

            try {
                await call('/api/upkeep/notice', { on: notice.checked });
            } catch (err) {
                notice.checked = !notice.checked;
                say(T('upd.failed', { why: String(err.message || err) }));
            } finally {
                notice.disabled = false;
            }
        };
    }

    for (const item of document.querySelectorAll('[data-view="updates"]')) {
        item.addEventListener('click', () => check(false));
    }

    window.addEventListener('pn-language', draw);

    check(false);
    setInterval(() => check(false), EVERY);
}());
