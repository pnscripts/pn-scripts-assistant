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
const THINKING_INTERVAL = 1500;

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
        /*
         * A row, with the conversation as a button inside it rather than being
         * one.
         *
         * Rename and delete are buttons too, and a button inside a button is
         * not valid markup — the browser closes the outer one early, which
         * puts the controls outside the row and the row's own click on
         * whatever is left.
         */
        const row = document.createElement('div');

        row.className = 'row history-row';

        const openIt = document.createElement('button');

        openIt.type = 'button';
        openIt.className = 'history-open';

        const opening = document.createElement('span');
        opening.className = 'history-said';
        opening.textContent = shorten(c.title || c.opening, 60);

        const meta = document.createElement('span');
        meta.className = 'history-when';
        meta.textContent = `${when(c.when)} · ${c.turns}`;

        openIt.appendChild(opening);
        openIt.appendChild(meta);

        // Opening one puts it back in the transcript to be read and carried on.
        openIt.addEventListener('click', () => {
            if (window.brainOpenConversation) window.brainOpenConversation(c.id);
        });

        row.appendChild(openIt);

        /*
         * Named by its first line until somebody names it.
         *
         * The opening message is a fair guess and often a poor name — a
         * conversation that started "can you hear me" is not about that — and
         * twenty of those are unsearchable.
         */
        const rename = document.createElement('button');

        rename.type = 'button';
        rename.className = 'history-act';
        rename.title = 'Rename this conversation';
        rename.textContent = '✎';

        rename.addEventListener('click', async (e) => {
            e.stopPropagation();

            const now = c.title || shorten(c.opening, 60);
            const name = window.prompt('Call this conversation:', now);

            if (name === null || name.trim() === '') return;

            await fetch('/api/conversations/' + c.id, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ title: name.trim() }),
            });

            loadHistory();
        });

        const remove = document.createElement('button');

        remove.type = 'button';
        remove.className = 'history-act history-delete';
        remove.title = 'Delete this conversation';
        remove.textContent = '×';

        remove.addEventListener('click', async (e) => {
            e.stopPropagation();

            /*
             * Asked before, because it cannot be undone.
             *
             * What the brain learned from the conversation stays — a lesson is
             * knowledge in its own right by then — but the transcript does not
             * come back, and the row is small and next to the one that opens
             * it.
             */
            const name = c.title || shorten(c.opening, 40);

            if (!window.confirm('Delete "' + name + '"? What it learned is kept; '
                + 'the conversation itself cannot be brought back.')) return;

            await fetch('/api/conversations/' + c.id, { method: 'DELETE' });

            loadHistory();
        });

        row.appendChild(rename);
        row.appendChild(remove);

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
    /*
     * "(on its own)" belongs to work the brain gave itself, not to listening.
     *
     * Both are marked background, because neither is something somebody is
     * waiting on — but only one of them is work, and "Listening (on its own)"
     * describes nothing.
     */
    const unattended = step.background && step.kind !== 'listening';
    let what = unattended ? `${step.note} (on its own)` : step.note || 'Working';

    if (step.model && !step.background) what += ` \u00b7 ${step.model}`;

    el('thinking-now-text').textContent = what;
    line.dataset.background = step.background ? 'yes' : 'no';

    // The same status, and therefore the same colour, as the feed, the talk
    // button and the core. See status.js.
    line.dataset.status = window.brainStatusOf ? window.brainStatusOf(step) : 'thinking';

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
