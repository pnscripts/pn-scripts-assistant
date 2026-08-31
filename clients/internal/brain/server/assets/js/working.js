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

/** How often to redraw from the shared reading. */
const INTERVAL = 400;

/*
 * What to say out loud as each tool starts.
 *
 * Somebody who asked by voice is not looking at the screen, so a tool that
 * takes twenty seconds is twenty seconds of silence that is indistinguishable
 * from the program having died. One short line at the start fixes that, and
 * only at the start: narrating progress through a long job is worse than
 * saying nothing, because it talks over the person while they are deciding
 * whether to interrupt.
 *
 * Written the way somebody competent says it in passing — what is happening,
 * in one clause, then quiet. Not "I'll go ahead and take a look at that file
 * for you now", which says the same thing and takes four times as long to get
 * out of the way.
 *
 * Tools missing from this list are deliberately silent. Anything that returns
 * instantly says nothing, because the line would arrive after the answer it
 * was meant to cover.
 */
const SPOKEN = {
    // Files and folders.
    read_file: 'Reading it.',
    write_file: 'Writing that.',
    edit_file: 'Editing it.',
    list_directory: 'Looking at the folder.',
    search_files: 'Searching your files.',

    // The world outside.
    fetch_url: 'Fetching the page.',
    web_search: 'Searching the web.',

    // Things that take a while and can surprise you.
    run_command: 'Running it.',
    do_in_background: 'Starting that in the background.',

    // Documents.
    read_document: 'Reading the document.',
    write_document: 'Writing the document.',

    // Mail, where the wait is the network.
    read_email: 'Checking your mail.',
    send_email: 'Sending it.',

    // The screen and the desktop.
    look_at_screen: 'Looking at your screen.',
    list_windows: 'Checking what is open.',
    open_app: 'Opening it.',

    // Reminders.
    remind_me: 'Noting it.',
    list_reminders: 'Checking your reminders.',

    // Models, which is the longest wait in the program.
    list_models: 'Checking the models.',
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
function announce(name) {
    if (!name) return;
    if (!window.brainIsTalking || !window.brainIsTalking()) return;

    const said = SPOKEN[name];

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

    // One status, one colour, everywhere. See status.js.
    line.dataset.status = window.brainStatusOf ? window.brainStatusOf(step) : 'thinking';

    /*
     * Which model, beside what it is doing.
     *
     * The brain moves between models within a session — small talk to a quick
     * one, work to the one that can use tools — and the switch is most of the
     * difference in how long an answer takes. Somebody waiting deserves to know
     * which of them they are waiting for, while they wait rather than after.
     */
    label.textContent = step.model
        ? `${step.note || 'Working'} \u00b7 ${step.model}`
        : step.note || 'Working';

    const seconds = Math.round(step.seconds || 0);
    const time = seconds < 60
        ? `${seconds}s`
        : `${Math.floor(seconds / 60)}m ${String(seconds % 60).padStart(2, '0')}s`;

    // The round count is what makes a long turn make sense: a tool-using turn
    // is several model calls, and knowing it is on the third explains the wait
    // in a way a spinner never can.
    clock.textContent = step.round > 1 ? `${time} · step ${step.round}` : time;

    /*
     * Announced by tool name, not by the summary.
     *
     * Guessing from the summary's first word is what this used to do: the
     * summary reads "Read /etc/hosts", the first word is "read", and the list
     * is keyed by read_file — so nothing ever matched and the line meant to
     * cover a slow tool never once played.
     */
    if (step.kind === 'tool') announce(step.tool);
}

// Read from the one place that asks, rather than asking again. See signals.js.
function poll() {
    show(window.brainWork ? window.brainWork() : { busy: false });
}

setInterval(poll, INTERVAL);
poll();
