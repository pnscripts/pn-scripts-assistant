/*
 * The brain, kept in more than one place.
 *
 * Everything it knows is a single file on a single disk that gets carried
 * around, and until this existed the answer to "what if that drive dies" was
 * nothing at all.
 *
 * One rule runs through all of it and the wording here has to keep saying so:
 * a copy is a copy. Copies are written with a different marker from the brain
 * itself, so nothing goes looking for a brain and finds one — a backup drive
 * plugged into another machine cannot quietly start being a second brain that
 * then grows apart from this one. Making a copy into the brain is a button
 * somebody presses, having been shown how old it is and how much is in it.
 */
(function () {

/** How often the copies are looked at. Rarely: a copy changes every few minutes
 *  at most, and each refresh stats a drive that may be asleep. */
const COPIES_INTERVAL = 30000;

const GB = 1024 * 1024 * 1024;
const MB = 1024 * 1024;

function el(id) {
    return document.getElementById(id);
}

function say(what, kind) {
    const note = el('copy-note');

    if (!note) return;

    note.textContent = what || '';
    note.dataset.kind = kind || '';
}

// How long ago, in words. Rounded: "an hour ago" is what somebody wants from a
// backup, and a timestamp to the second is a thing to decode.
function ago(seconds) {
    if (!isFinite(seconds) || seconds < 0) return '';
    if (seconds < 90) return 'just now';
    if (seconds < 3600) return `${Math.round(seconds / 60)} minutes ago`;
    if (seconds < 86400) return `${Math.round(seconds / 3600)} hours ago`;

    return `${Math.round(seconds / 86400)} days ago`;
}

function size(bytes) {
    if (!bytes) return '';

    return bytes >= GB
        ? `${(bytes / GB).toFixed(1)}GB`
        : `${Math.max(1, Math.round(bytes / MB))}MB`;
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
        data = await fetch('/api/copies').then((r) => r.json());
    } catch {
        return;
    }

    const list = el('copies');
    const empty = el('copies-empty');

    if (!list) return;

    list.textContent = '';

    const copies = data.copies || [];

    if (empty) empty.hidden = copies.length > 0;

    copies.forEach((c) => list.appendChild(row(c)));

    suggestions(data.drives || []);
}

function row(c) {
    const row = document.createElement('div');
    row.className = 'drive';

    if (!c.reachable) row.classList.add('copy-away');
    if (c.reachable && c.trouble && !c.never) row.classList.add('copy-trouble');

    const where = document.createElement('span');
    where.className = 'drive-path';
    where.textContent = c.short || c.path;
    where.title = c.path;

    const when = document.createElement('span');
    when.className = 'drive-free';

    /*
     * What is in it and how old it is, in that order.
     *
     * The number that matters about a backup is not its size — it is whether
     * it holds what you would miss. "1,029 things · 2 hours ago" answers the
     * question somebody actually has.
     */
    when.textContent = c.never
        ? '—'
        : `${c.facts.toLocaleString()} things · ${ago(c.age_seconds)}`;

    row.append(where, when);

    const note = document.createElement('span');
    note.className = 'drive-note';

    if (c.trouble) {
        note.textContent = c.trouble;
    } else if (!c.never) {
        note.textContent = `${size(c.bytes)} copied`;
    }

    if (note.textContent) row.appendChild(note);

    row.appendChild(actions(c));

    return row;
}

function actions(c) {
    const holder = document.createElement('div');
    holder.className = 'drive-actions';

    /*
     * Using a copy is offered only where it means something.
     *
     * On a drive that is not attached the button could not do anything, and on
     * a folder that has never been copied to there is nothing in it to use —
     * offering either would be offering to restore from a backup that does not
     * exist.
     */
    if (c.reachable && !c.never) {
        const use = document.createElement('button');
        use.type = 'button';
        use.className = 'model-action';
        use.textContent = 'Use this copy';

        use.onclick = async () => {
            const age = ago(c.age_seconds);
            const agreed = window.confirm(
                `Open the brain from this copy instead?\n\n${c.path}\n`
                + `${c.facts.toLocaleString()} things, copied ${age}.\n\n`
                + 'Anything learned since then is in the brain you are using now, '
                + 'and stays where it is — nothing is deleted. '
                + 'The change takes effect when you restart.'
            );

            if (!agreed) return;

            try {
                const done = await send('/api/copies/use', { path: c.path });
                say(done.note, 'ok');
            } catch (err) {
                say(String(err.message || err), 'bad');
            }

            load();
        };

        holder.appendChild(use);
    }

    const stop = document.createElement('button');
    stop.type = 'button';
    stop.className = 'model-action';
    stop.textContent = 'Stop copying here';

    stop.onclick = async () => {
        try {
            const done = await send('/api/copies/stop', { path: c.path });
            say(done.note, 'ok');
        } catch (err) {
            say(String(err.message || err), 'bad');
        }

        load();
    };

    holder.appendChild(stop);

    return holder;
}

/*
 * The drives a copy would fit on, as something to click.
 *
 * Typing a path is the reliable way and it stays; but somebody who has just
 * plugged a drive in does not know what the system called it, and asking them
 * to go and find out is asking them to open a file manager to use a program
 * that promised they would not have to.
 */
function suggestions(drives) {
    const holder = el('copy-suggestions');

    if (!holder) return;

    holder.textContent = '';

    drives.forEach((d) => {
        const chip = document.createElement('button');
        chip.type = 'button';
        chip.className = 'copy-suggestion';
        // The home folder is named as such. "/home/petar" in a row of drives
        // reads as a disk somebody has to recognise; "your home folder" is
        // what it actually is.
        chip.textContent = d.home
            ? `your home folder · ${(d.free_bytes / GB).toFixed(0)}GB free`
            : `${d.mount_point} · ${(d.free_bytes / GB).toFixed(0)}GB free`;
        chip.title = d.suggested;

        chip.onclick = () => {
            el('copy-path').value = d.suggested;
            say('');
        };

        holder.appendChild(chip);
    });
}

function wire() {
    const add = el('copy-add');

    if (add) {
        add.onclick = async () => {
            const path = (el('copy-path').value || '').trim();

            if (!path) {
                say('Type a folder, or pick a drive above.', 'bad');

                return;
            }

            add.disabled = true;
            say('Copying…');

            try {
                const done = await send('/api/copies', { path });

                say(done.copied
                    ? `Copied. ${done.facts.toLocaleString()} things, ${size(done.bytes)}.`
                    : `Added, but the copy did not finish: ${done.trouble}`,
                    done.copied ? 'ok' : 'bad');

                if (done.copied) el('copy-path').value = '';
            } catch (err) {
                say(String(err.message || err), 'bad');
            }

            add.disabled = false;
            load();
        };
    }

    const now = el('copy-now');

    if (now) {
        now.onclick = async () => {
            now.disabled = true;
            say('Copying…');

            try {
                const done = await send('/api/copies/now', {});
                const made = (done.copies || []).filter((c) => c.reachable && !c.trouble);
                const missed = (done.copies || []).length - made.length;

                say(made.length
                    ? `Copied to ${made.length} place${made.length === 1 ? '' : 's'}.`
                        + (missed ? ` ${missed} not reachable.` : '')
                    : 'Nothing could be copied to right now.',
                    made.length ? 'ok' : 'bad');
            } catch (err) {
                say(String(err.message || err), 'bad');
            }

            now.disabled = false;
            load();
        };
    }
}

wire();
load();
setInterval(load, COPIES_INTERVAL);

}());
