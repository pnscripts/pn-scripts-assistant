/*
 * The diary, which lives in this database rather than in an account.
 *
 * Nothing here reaches a network. A calendar file goes in when somebody says
 * so and comes out when they ask, and that is the whole of the interop — which
 * is the point: a calendar is the kind of thing that quietly ends up on
 * somebody else's server, and this one cannot.
 */
(function () {
    'use strict';

    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
    const locale = () => (window.pnLang && window.pnLang() === 'bg' ? 'bg-BG' : 'en-GB');

    function say(text) {
        const node = el('diary-said');
        if (node) node.textContent = text || '';
    }

    /*
     * When it is, said the way somebody would say it.
     *
     * "Today 14:00" rather than a date they have to work out is today. A diary
     * that makes you count is a diary you stop looking at.
     */
    function when(event) {
        const starts = new Date(event.starts_at);
        const ends = new Date(event.ends_at);

        const today = new Date();
        today.setHours(0, 0, 0, 0);

        const days = Math.round((new Date(starts).setHours(0, 0, 0, 0) - today) / 86400000);

        let day;
        if (days === 0) day = T('room.today');
        else if (days === 1) day = T('room.tomorrow');
        else day = starts.toLocaleDateString(locale(), { weekday: 'short', day: 'numeric', month: 'short' });

        if (event.all_day) return T('room.allDay', { day: day });

        const clock = (d) => d.toLocaleTimeString(locale(), { hour: '2-digit', minute: '2-digit' });

        return T('room.fromTo', { day: day, from: clock(starts), to: clock(ends) });
    }

    function eventRow(event) {
        const row = document.createElement('div');
        row.className = 'task-row';

        const time = document.createElement('span');
        time.className = 'diary-when';
        time.textContent = when(event);
        row.appendChild(time);

        const title = document.createElement('span');
        title.className = 'task-state';
        title.textContent = event.title + (event.place ? ' · ' + event.place : '');
        row.appendChild(title);

        // Where it came from, so something that arrived in a file can be told
        // from something somebody typed.
        if (event.came_from) {
            const from = document.createElement('span');
            from.className = 'skill-origin';
            from.textContent = T('room.imported');
            row.appendChild(from);
        }

        const cancel = document.createElement('button');
        cancel.type = 'button';
        cancel.className = 'task-button';
        cancel.textContent = T('room.cancelIt');
        cancel.addEventListener('click', () => cancelEvent(event.id));
        row.appendChild(cancel);

        return row;
    }

    async function cancelEvent(id) {
        try {
            const res = await fetch('/api/diary/' + id, { method: 'DELETE' });

            if (!res.ok) return;

            say(T('room.takenOut'));
            load();
        } catch (err) {
            say(T('room.failed', { why: err.message }));
        }
    }

    async function save() {
        const title = el('diary-title')?.value.trim();
        const at = el('diary-when')?.value;

        if (!title || !at) {
            say(T('room.needsNameTime'));
            return;
        }

        const minutes = Number(el('diary-length')?.value || 60);
        const starts = new Date(at);
        const ends = new Date(starts);

        if (minutes === 0) {
            starts.setHours(0, 0, 0, 0);
            ends.setTime(starts.getTime());
            ends.setDate(ends.getDate() + 1);
        } else {
            ends.setMinutes(ends.getMinutes() + minutes);
        }

        try {
            const res = await fetch('/api/diary', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    title: title,
                    starts_at: starts.toISOString(),
                    ends_at: ends.toISOString(),
                    all_day: minutes === 0,
                    place: el('diary-place')?.value.trim() || ''
                })
            });

            const body = await res.json().catch(() => ({}));

            if (!res.ok) {
                say(body.error ? T('room.notInWhy', { why: body.error }) : T('room.notIn'));
                return;
            }

            el('diary-title').value = '';
            el('diary-place').value = '';
            const details = el('diary-new');
            if (details) details.open = false;

            say(T('room.inDiary'));
            load();
        } catch (err) {
            say(T('room.notInWhy', { why: err.message }));
        }
    }

    async function bringIn() {
        const path = el('diary-path')?.value.trim();

        if (!path) {
            say(T('room.sayFile'));
            return;
        }

        say(T('room.reading'));

        try {
            const res = await fetch('/api/diary/import', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ path: path })
            });

            const body = await res.json().catch(() => ({}));

            if (!res.ok) {
                say(body.error ? T('room.unreadableWhy', { why: body.error }) : T('room.unreadable'));
                return;
            }

            // Said from the counts, in the page's language.
            say(typeof body.added === 'number'
                ? T('room.readInCount', { n: body.added })
                  + (body.skipped ? T('room.readInSkipped', { n: body.skipped }) : '')
                : T('room.readInDone'));
            load();
        } catch (err) {
            say(T('room.unreadableWhy', { why: err.message }));
        }
    }

    /*
     * Reloading the list never touches what somebody is typing.
     *
     * There was a guard here that stopped the whole reload while any field had
     * been typed in, copied from a panel where the reload does rebuild its
     * form. This one rebuilds only the list of events, which contains no
     * fields at all — so the guard did nothing but make the list stop updating
     * the moment somebody typed a path into the import box, which is exactly
     * when they are about to want it updated.
     */
    async function load() {
        let body;

        try {
            const res = await fetch('/api/diary?days=14');
            if (!res.ok) return;
            body = await res.json();
        } catch (err) {
            return;
        }

        const list = el('diary-list');
        if (!list) return;

        list.textContent = '';

        const events = body.events || [];

        if (!events.length) {
            const p = document.createElement('p');
            p.className = 'note';
            p.textContent = body.total
                ? T('room.diaryEmptyOf', { n: body.total })
                : T('room.diaryNone');
            list.appendChild(p);
            return;
        }

        for (const event of events) list.appendChild(eventRow(event));
    }

    function wire() {
        el('diary-save')?.addEventListener('click', save);
        el('diary-import')?.addEventListener('click', bringIn);

        // A plain link would be blocked from starting a download in some
        // contexts; opening it is what actually reaches the file.
        el('diary-export')?.addEventListener('click', () => {
            window.open('/api/diary/export', '_blank');
        });

        load();

        document.querySelector('.nav-item[data-view="room"]')
            ?.addEventListener('click', load);

        window.addEventListener('pn-language', load);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
})();
