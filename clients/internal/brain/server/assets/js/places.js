/*
 * The drives and folders the brain looks after.
 *
 * Learning used to be a thing you asked for, once, about one folder — which
 * works for one folder and falls apart at the scale this is used at: work on
 * one drive, films on another, documents in a third, each plugged in and
 * unplugged through the week. Nobody asks again after every change, so what
 * the brain knew was whatever it had been told months ago.
 *
 * A place is somewhere it looks after. The panel's job is to make two things
 * obvious that are otherwise invisible: how much of each place it has still to
 * get through, and that a drive in a drawer is not a fault.
 */
(function () {

/** How often the places are looked at. Rarely: a bite takes minutes, and each
 *  refresh stats a drive that may be asleep. */
const PLACES_INTERVAL = 20000;

const GB = 1024 * 1024 * 1024;

function el(id) {
    return document.getElementById(id);
}

const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
const bg = () => window.pnLang && window.pnLang() === 'bg';

// The knowledge folder keeps the name the server gives it (places.KnowledgeName)
// until somebody renames it; that one name is said in the page's language.
const placeName = (p) => (window.pnEnglish && p.name === window.pnEnglish('sys.knowledgeName') ? T('sys.knowledgeName') : p.name);

// A count, grouped the way the page's language groups it.
const num = (n) => Number(n || 0).toLocaleString(bg() ? 'bg-BG' : 'en-GB');

// So many things, with "thing" said once for one.
const things = (n) => (n === 1 ? T('sys.thing1') : T('sys.things', { n: num(n) }));

function say(what, kind) {
    const note = el('place-note');

    if (!note) return;

    note.textContent = what || '';
    note.dataset.kind = kind || '';
}

function ago(seconds) {
    if (!isFinite(seconds) || seconds < 0) return '';
    if (seconds < 90) return T('sys.justNow');
    if (seconds < 3600) return T('sys.minutesAgo', { n: Math.round(seconds / 60) });

    const hours = Math.round(seconds / 3600);

    if (seconds < 86400) return hours === 1 ? T('sys.hourAgo') : T('sys.hoursAgo', { n: hours });

    const days = Math.round(seconds / 86400);

    return days === 1 ? T('sys.dayAgo') : T('sys.daysAgo', { n: days });
}

async function send(where, body) {
    const res = await fetch(where, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body || {}),
    });

    const data = await res.json().catch(() => ({}));

    if (!res.ok) throw new Error(data.error || T('sys.failed', { status: res.status }));

    return data;
}

async function load() {
    let data;

    try {
        data = await fetch('/api/places').then((r) => r.json());
    } catch {
        return;
    }

    const list = el('places');
    const empty = el('places-empty');

    if (!list) return;

    list.textContent = '';

    const places = data.places || [];

    if (empty) empty.hidden = places.length > 0;

    places.forEach((p) => list.appendChild(row(p)));

    suggestions(data.drives || []);
}

function row(p) {
    const row = document.createElement('div');
    row.className = 'drive';

    if (!p.reachable) row.classList.add('copy-away');

    const where = document.createElement('span');
    where.className = 'drive-path';
    where.textContent = placeName(p);
    where.title = p.path;

    const state = document.createElement('span');
    state.className = 'drive-free';

    /*
     * What it knows from here, and what it has left.
     *
     * The second number is the one with no other home in the interface. A
     * place is read a little at a time over hours, so "1,240 still to read" is
     * the difference between a brain that is working through your drive and
     * one that has quietly stopped.
     */
    state.textContent = p.never && !p.learned
        ? (p.waiting ? T('sys.placeToRead', { n: num(p.waiting) }) : T('sys.placeUnread'))
        : T('sys.placeLearned', { n: num(p.learned) })
            + (p.waiting ? ' · ' + T('sys.placeToGo', { n: num(p.waiting) }) : '');

    row.append(where, state);

    const note = document.createElement('span');
    note.className = 'drive-note';

    if (p.trouble) {
        note.textContent = p.trouble_key ? T(p.trouble_key) : p.trouble;
    } else {
        const bits = [p.short];

        if (!p.never) bits.push(T('sys.placeLastRead', { ago: ago(p.age_seconds) }));
        if (!p.waiting && !p.never) bits.push(T('sys.placeAllRead'));

        note.textContent = bits.join(' · ');
    }

    row.appendChild(note);
    row.appendChild(actions(p));

    return row;
}

function actions(p) {
    const holder = document.createElement('div');
    holder.className = 'drive-actions';

    // Reading on request is only offered where there is something to read: a
    // drive that is not attached, or one already finished, has nothing.
    if (p.reachable && (p.waiting > 0 || p.never)) {
        const look = document.createElement('button');
        look.type = 'button';
        look.className = 'model-action';
        look.textContent = T('sys.placeReadNow');

        look.onclick = async () => {
            look.disabled = true;
            say(T('sys.placeReading', { name: placeName(p) }));

            try {
                const done = await send('/api/places/look', { path: p.path });

                say(done.read
                    ? T('sys.placeRead', { name: placeName(p), learned: num(done.learned), known: num(done.known) })
                        + (done.waiting
                            ? T('sys.placeStill', { n: num(done.waiting) })
                            : T('sys.placeAll'))
                    : T('sys.placeNothingNew', { name: placeName(p) }), 'ok');
            } catch (err) {
                say(String(err.message || err), 'bad');
            }

            look.disabled = false;
            load();
        };

        holder.appendChild(look);
    }

    const rename = document.createElement('button');
    rename.type = 'button';
    rename.className = 'model-action';
    rename.textContent = T('sys.placeRename');

    rename.onclick = async () => {
        const name = window.prompt(T('sys.placeCall', { place: p.short }), p.name);

        if (name === null) return;

        try {
            await send('/api/places/rename', { path: p.path, name });
        } catch (err) {
            say(String(err.message || err), 'bad');
        }

        load();
    };

    const forget = document.createElement('button');
    forget.type = 'button';
    forget.className = 'model-action';
    forget.textContent = T('sys.placeForget');

    forget.onclick = async () => {
        try {
            await send('/api/places/forget', { path: p.path });
            say(T('sys.placeForgot'), 'ok');
        } catch (err) {
            say(String(err.message || err), 'bad');
        }

        load();
    };

    holder.append(rename, forget);

    return holder;
}

function suggestions(drives) {
    const holder = el('place-suggestions');

    if (!holder) return;

    holder.textContent = '';

    drives.forEach((d) => {
        const chip = document.createElement('button');
        chip.type = 'button';
        chip.className = 'copy-suggestion';

        chip.textContent = d.home
            ? T('sys.homeFolder')
            : d.mount_point + (d.free_bytes ? ' · ' + T('sys.gbFree', { n: (d.free_bytes / GB).toFixed(0) }) : '');

        // The mount point is where to start typing, not the answer: the folder
        // worth learning is almost always one inside it.
        chip.onclick = () => {
            const path = el('place-path');
            path.value = d.mount_point.endsWith('/') ? d.mount_point : `${d.mount_point}/`;
            path.focus();
            say('');
        };

        holder.appendChild(chip);
    });
}

function wire() {
    const add = el('place-add');

    if (!add) return;

    add.onclick = async () => {
        const path = (el('place-path').value || '').trim();

        if (!path) {
            say(T('sys.typeFolder'), 'bad');

            return;
        }

        add.disabled = true;
        say(T('sys.looking'));

        try {
            const done = await send('/api/places', {
                path,
                kind: el('place-kind').value,
            });

            /*
             * The size first, because it is the answer to "what did that just
             * commit me to". A folder of two thousand things is hours of
             * reading, and being told that up front is the difference between
             * patience and a bug report.
             */
            const minutes = Math.max(1, done.minutes);

            if (!done.counted) {
                say(T('sys.placeAdded', { name: done.name })
                    + (done.note ? ' ' + T('sys.placeAway') : ''), 'ok');
            } else if (done.waiting) {
                say(T('sys.placeCounted', {
                    name: done.name,
                    things: things(done.waiting),
                    work: minutes === 1 ? T('sys.aboutMinute') : T('sys.aboutMinutes', { n: minutes }),
                }), 'ok');
            } else {
                say(T('sys.placeNothingToRead', { name: done.name }), 'ok');
            }

            el('place-path').value = '';
        } catch (err) {
            say(String(err.message || err), 'bad');
        }

        add.disabled = false;
        load();
    };
}

wire();
load();
setInterval(load, PLACES_INTERVAL);
window.addEventListener('pn-language', load);

}());
