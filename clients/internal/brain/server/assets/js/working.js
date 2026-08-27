/*
 * What the brain is doing while you wait.
 *
 * A reply on this machine can take minutes. The model runs on the processor
 * here, and a turn that uses a tool is several model calls with work between
 * them — measured at about eight minutes for "list this directory". For all of
 * that the interface used to show a single unchanging line, which cannot answer
 * the only question somebody waiting has: is this working, or is it stuck?
 *
 * So it says what it is doing, and how long it has been doing it. The elapsed
 * time is the important half. A step that has been running for twelve seconds
 * and a step that has been running for four minutes look identical without it,
 * and they mean completely different things.
 */

const line = document.getElementById('working');
const label = document.getElementById('working-note');
const clock = document.getElementById('working-clock');

/** How often to ask. Cheap — it reads one value the brain already holds. */
const INTERVAL = 700;

/** Tools whose start is worth saying out loud, and how to say it. */
const SPOKEN = {
    read_file: 'Reading a file.',
    list_directory: 'Looking at a folder.',
    run_command: 'Running that now.',
    fetch_url: 'Fetching that page.',
    web_search: 'Searching the web.',
};

let announced = '';

/*
 * Said aloud only while the conversation is being held by voice.
 *
 * Somebody typing has the screen in front of them and has not asked to be
 * talked at; announcing every step to them would be an interruption rather than
 * an answer. Somebody talking cannot see the screen and has nothing else to go
 * on, which is exactly when it is worth saying.
 */
function announce(note) {
    if (!window.brainIsTalking || !window.brainIsTalking()) return;

    const key = note.split(' ')[0].toLowerCase();
    const said = SPOKEN[key] || SPOKEN[note.toLowerCase()];

    if (!said || said === announced) return;

    announced = said;
    fetch('/api/speak', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text: said }),
    }).catch(() => {});
}

function show(step) {
    if (!line) return;

    if (!step.busy) {
        line.hidden = true;
        announced = '';

        return;
    }

    line.hidden = false;
    label.textContent = step.note || 'Working';

    const seconds = Math.round(step.seconds || 0);
    const time = seconds < 60
        ? `${seconds}s`
        : `${Math.floor(seconds / 60)}m ${String(seconds % 60).padStart(2, '0')}s`;

    // The round count is what makes a long turn make sense: a tool-using turn
    // is several model calls, and knowing it is on the third explains the wait
    // in a way a spinner never can.
    clock.textContent = step.round > 1 ? `${time} · step ${step.round}` : time;

    if (step.kind === 'tool') announce(step.note);
}

async function poll() {
    if (document.hidden) return;

    try {
        show(await fetch('/api/progress').then((r) => r.json()));
    } catch {
        show({ busy: false });
    }
}

setInterval(poll, INTERVAL);
poll();
