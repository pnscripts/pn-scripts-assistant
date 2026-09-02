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

function say(what, kind) {
    const note = el('place-note');

    if (!note) return;

    note.textContent = what || '';
    note.dataset.kind = kind || '';
}

function ago(seconds) {
    if (!isFinite(seconds) || seconds < 0) return '';
    if (seconds < 90) return 'just now';
    if (seconds < 3600) return `${Math.round(seconds / 60)} minutes ago`;
    if (seconds < 86400) return `${Math.round(seconds / 3600)} hours ago`;

    return `${Math.round(seconds / 86400)} days ago`;
}

async function send(where, body) {
    const res = await fetch(where, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body || {}),
    });

    const data = await res.json().catch(() => ({}));

    if (!res.ok) throw new Error(data.error || `that did not work (${res.status})`);

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
    where.textContent = p.name;
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
        ? (p.waiting ? `${p.waiting.toLocaleString()} to read` : 'not read yet')
        : `${p.learned.toLocaleString()} learned`
            + (p.waiting ? ` · ${p.waiting.toLocaleString()} to go` : '');

    row.append(where, state);

    const note = document.createElement('span');
    note.className = 'drive-note';

    if (p.trouble) {
        note.textContent = p.trouble;
    } else {
        const bits = [p.short];

        if (!p.never) bits.push(`last read ${ago(p.age_seconds)}`);
        if (!p.waiting && !p.never) bits.push('all of it read');

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
        look.textContent = 'Read some now';

        look.onclick = async () => {
            look.disabled = true;
            say(`Reading ${p.name}…`);

            try {
                const done = await send('/api/places/look', { path: p.path });

                say(done.read
                    ? `${p.name}: ${done.learned} new, ${done.known} already known`
                        + (done.waiting
                            ? `, ${done.waiting.toLocaleString()} still to read.`
                            : ' — that is all of it.')
                    : `Nothing new in ${p.name}.`, 'ok');
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
    rename.textContent = 'Rename';

    rename.onclick = async () => {
        const name = window.prompt(`What do you call ${p.short}?`, p.name);

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
    forget.textContent = 'Stop looking after';

    forget.onclick = async () => {
        try {
            const done = await send('/api/places/forget', { path: p.path });
            say(done.note, 'ok');
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
            ? 'your home folder'
            : `${d.mount_point}${d.free_bytes ? ` · ${(d.free_bytes / GB).toFixed(0)}GB free` : ''}`;

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
            say('Type a folder, or pick a drive above.', 'bad');

            return;
        }

        add.disabled = true;
        say('Looking…');

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
            if (!done.counted) {
                say(`Looking after ${done.name}. ${done.note || ''}`.trim(), 'ok');
            } else if (done.waiting) {
                say(`Looking after ${done.name} — ${done.waiting.toLocaleString()} things to read,`
                    + ` about ${Math.max(1, done.minutes)} minutes of work. It starts on its own.`, 'ok');
            } else {
                say(`Looking after ${done.name} — nothing new in it to read.`, 'ok');
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

}());
