/*
 * The two things that happen to a brain kept on a drive.
 *
 * It gets carried somewhere else, and it runs out of room. Both had answers
 * already and both answers were a terminal command, which for this program is
 * the same as not having one — nobody moves their assistant to a bigger disk
 * by remembering a flag, and nobody repairs a thousand memories by hand.
 *
 * The journey is the more dangerous of the two because it is invisible. The
 * drive lands at a different path on every machine, so every memory that names
 * a file names one that does not exist here — and recall keeps working, the
 * sentences stay right, and every path in them points nowhere.
 */
(function () {

/** How often to look. Rarely: neither of these changes without a restart. */
const JOURNEY_INTERVAL = 60000;

const GB = 1024 * 1024 * 1024;
const MB = 1024 * 1024;

function el(id) {
    return document.getElementById(id);
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

async function checkTheJourney() {
    const banner = el('travelled');

    if (!banner) return;

    let data;

    try {
        data = await fetch('/api/journey').then((r) => r.json());
    } catch {
        return;
    }

    banner.hidden = !data.travelled;

    if (!data.travelled) return;

    /*
     * The count first, because it is the size of the problem, and the paths
     * only in the tooltip.
     *
     * A mount point is forty characters of nothing anybody reads, and two of
     * them wrap this banner over the panels underneath. What somebody needs
     * from the top of the screen is what broke, how much, and the button.
     */
    const note = el('travelled-note');

    note.textContent =
        `${data.affected.toLocaleString()} things it knows name the old path and cannot be `
        + 'opened until they are repaired. Nothing is lost meanwhile.';

    note.title = `Was: ${data.old_prefix}\nNow: ${data.new_prefix}`;
}

function wireTheRepair() {
    const button = el('travelled-fix');
    const no = el('travelled-no');

    /*
     * And a way to say no.
     *
     * Somebody whose old paths do not matter any more — a drive they have
     * finished with, a folder they have moved on from — should not be asked
     * the same question at every start for the rest of the program's life. A
     * banner with only one button is not an offer.
     */
    if (no) {
        no.onclick = async () => {
            try {
                await send('/api/journey/forget', {});
            } catch {
                /* Nothing to do about it; the banner is not worth an error. */
            }

            el('travelled').hidden = true;
        };
    }

    if (!button) return;

    button.onclick = async () => {
        button.disabled = true;
        button.textContent = 'Repairing…';

        /*
         * Repairing means re-embedding every memory that changed, which is
         * seconds each — so this can be minutes and the button has to say so
         * rather than looking stuck.
         */
        try {
            const done = await send('/api/journey/repair', {});

            el('travelled-note').textContent =
                `Repaired ${done.repaired.toLocaleString()} memories `
                + `(${done.re_embedded.toLocaleString()} re-read) from ${done.from} to ${done.to}.`;

            button.hidden = true;
        } catch (err) {
            el('travelled-note').textContent = String(err.message || err);
            button.disabled = false;
            button.textContent = 'Try again';
        }
    };
}

// How much room there is, and — only when there is somewhere to go — the
// offer to move.
async function checkTheRoom() {
    let data;

    try {
        data = await fetch('/api/home').then((r) => r.json());
    } catch {
        return;
    }

    const outlook = el('storage-outlook');

    if (outlook) {
        outlook.textContent = data.outlook || '';
        outlook.dataset.kind = data.level === 'ok' ? '' : 'bad';
    }

    const holder = el('move-home');
    const where = el('move-where');

    if (!holder || !where) return;

    const drives = (data.drives || []).filter((d) => d.fits);

    // Nowhere to move to is not a control worth showing.
    holder.hidden = drives.length === 0;

    if (!drives.length) return;

    const chosen = where.value;

    where.textContent = '';

    drives.forEach((d) => {
        const option = document.createElement('option');
        option.value = d.suggested;
        option.textContent = (d.home ? 'your home folder' : d.mount_point)
            + ` · ${(d.free_bytes / GB).toFixed(0)}GB free`
            + (d.removable ? ' · removable' : '');
        where.appendChild(option);
    });

    if (chosen) where.value = chosen;

    const note = el('move-note');

    if (note && !note.textContent) {
        note.textContent = `Everything it knows is ${Math.max(1, Math.round(data.database_bytes / MB))}MB.`
            + ' Moving copies it, checks it, and leaves the old one in place for you to delete.';
    }
}

function wireTheMove() {
    const button = el('move-go');

    if (!button) return;

    button.onclick = async () => {
        const where = el('move-where').value;

        if (!where) return;

        /*
         * Asked before it happens, and the wording is the whole of the
         * confirmation: what moves, what is left behind, and that it takes a
         * restart. Somebody moving their entire memory to another disk is
         * entitled to know all three before pressing anything.
         */
        const agreed = window.confirm(
            `Move the brain to ${where}?\n\n`
            + 'Everything it knows is copied there and checked. The old folder is left '
            + 'exactly as it is — nothing is deleted — but it stops being the brain, so '
            + 'nothing opens it by accident.\n\n'
            + 'It takes effect when you restart.'
        );

        if (!agreed) return;

        button.disabled = true;
        el('move-note').textContent = 'Moving…';

        try {
            const done = await send('/api/home', { path: where });
            el('move-note').textContent = `Moved to ${done.moved}. ${done.note}`;
            el('move-note').dataset.kind = 'ok';
        } catch (err) {
            el('move-note').textContent = String(err.message || err);
            el('move-note').dataset.kind = 'bad';
            button.disabled = false;
        }
    };
}

wireTheRepair();
wireTheMove();
checkTheJourney();
checkTheRoom();
setInterval(() => {
    checkTheJourney();
    checkTheRoom();
}, JOURNEY_INTERVAL);

}());
