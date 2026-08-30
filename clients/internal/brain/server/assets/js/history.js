/*
 * What was said before, and what the brain is doing right now.
 *
 * This is where the quick commands were: four buttons for things already
 * reachable from the rail beside them, which is not what a panel that size is
 * for. What somebody actually wants to see there is the conversation they had
 * an hour ago, and whether the brain is working on something at this moment.
 *
 * The thinking line is here as well as over the transcript on purpose. An
 * answer on this machine takes long enough that the question "is it doing
 * anything?" is the one being asked, and it should be answerable from wherever
 * the eye happens to be.
 */
(function () {

/** How often the history is refreshed. Rarely: it changes once a conversation. */
const HISTORY_INTERVAL = 20000;

/** How often the thinking line is refreshed. */
const THINKING_INTERVAL = 700;

function el(id) {
    return document.getElementById(id);
}

function when(iso) {
    const at = new Date(iso);

    if (Number.isNaN(at.getTime())) return '';

    const today = new Date();
    const sameDay = at.toDateString() === today.toDateString();

    return sameDay
        ? at.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
        : at.toLocaleDateString([], { day: 'numeric', month: 'short' });
}

function shorten(text, most) {
    const clean = (text || '').replace(/\s+/g, ' ').trim();

    return clean.length > most ? `${clean.slice(0, most).trimEnd()}…` : clean;
}

async function loadHistory() {
    const rows = el('history-rows');
    const empty = el('history-empty');

    if (!rows) return;

    let data;

    try {
        data = await fetch('/api/conversations').then((r) => r.json());
    } catch {
        return;
    }

    const list = data.conversations || [];

    if (empty) empty.hidden = list.length > 0;

    rows.innerHTML = '';

    for (const c of list) {
        const row = document.createElement('button');

        row.type = 'button';
        row.className = 'row history-row';

        const opening = document.createElement('span');
        opening.className = 'history-said';
        opening.textContent = shorten(c.opening, 60);

        const meta = document.createElement('span');
        meta.className = 'history-when';
        meta.textContent = `${when(c.when)} · ${c.turns}`;

        row.appendChild(opening);
        row.appendChild(meta);

        // Opening one puts it back in the transcript to be read and carried on.
        row.addEventListener('click', () => {
            if (window.brainOpenConversation) window.brainOpenConversation(c.id);
        });

        rows.appendChild(row);
    }
}

function showThinking() {
    const line = el('thinking-now');

    if (!line) return;

    const step = window.brainWork ? window.brainWork() : { busy: false };

    /*
     * Nothing to say unless something is actually happening.
     *
     * Busy with no note and no clock is a reading left behind rather than work
     * in progress, and a panel that says "Thinking" while the brain sits idle
     * is worse than one that says nothing — it is the display disagreeing with
     * the machine, which makes every other thing it says less believable.
     */
    if (!step.busy || (!step.note && !step.seconds)) {
        line.hidden = true;

        return;
    }

    line.hidden = false;

    // Background work is the brain's own housekeeping, and saying so is the
    // difference between "it is busy with me" and "it is tidying up".
    const what = step.background ? `${step.note} (on its own)` : step.note || 'Working';

    el('thinking-now-text').textContent = what;
    line.dataset.background = step.background ? 'yes' : 'no';

    const seconds = Math.round(step.seconds || 0);

    el('thinking-now-clock').textContent = seconds < 60
        ? `${seconds}s`
        : `${Math.floor(seconds / 60)}m ${String(seconds % 60).padStart(2, '0')}s`;
}

setInterval(loadHistory, HISTORY_INTERVAL);
setInterval(showThinking, THINKING_INTERVAL);

loadHistory();
showThinking();

// Refreshed as soon as an exchange finishes, rather than up to twenty seconds
// later, so the conversation you just had appears in the list you are looking at.
window.brainConversationChanged = loadHistory;

})();
