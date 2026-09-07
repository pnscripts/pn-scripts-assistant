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
    read_file: 'I am reading it.',
    write_file: 'I am writing that.',
    edit_file: 'I am editing it.',
    list_directory: 'I am looking at the folder.',
    search_files: 'I am searching your files.',

    // The world outside.
    fetch_url: 'I am fetching the page.',
    web_search: 'I am searching the web.',

    // Things that take a while and can surprise you.
    run_command: 'I am running it.',
    do_in_background: 'I am starting that in the background.',

    // Documents.
    read_document: 'I am reading the document.',
    write_document: 'I am writing the document.',

    // Mail, where the wait is the network.
    read_email: 'I am checking your mail.',
    send_email: 'I am sending it.',

    // The screen and the desktop.
    look_at_screen: 'I am looking at your screen.',
    list_windows: 'I am checking what is open.',
    open_app: 'I am opening it.',

    // Reminders.
    remind_me: 'I am noting it.',
    list_reminders: 'I am checking your reminders.',

    // Models, which is the longest wait in the program.
    list_models: 'I am checking the models.',

    // The ones added since: looking things up about the machine itself.
    list_drives: 'I am checking your drives.',
    learn_from_folder: 'I am learning that folder.',
};

/*
 * And the steps that are not tools, which are most of the wait.
 *
 * Thinking is the longest thing that happens here and had nothing to say for
 * itself: a minute of silence between the question and the answer, with no way
 * to tell it from the program having stopped. Naming the step is not
 * decoration on this hardware, it is the only evidence there is.
 *
 * Listening is deliberately absent. It is what the brain does when nobody has
 * asked it anything, and announcing it would mean talking into an empty room
 * every few seconds.
 */
const SPOKEN_KINDS = {
    thinking: 'I am thinking about that.',
    transcribing: 'I am making out what you said.',
    answering: 'I am writing the answer.',
    learning: 'I am learning from that.',
    embedding: 'I am re-indexing what I know.',
    model: 'I am testing a model.',
    waiting: 'I need you to decide something.',
};

let announced = '';

/*
 * Whether the brain says what it is doing.
 *
 * It used to speak only while a conversation was being held by voice, on the
 * reasoning that somebody typing has the screen in front of them. That is a
 * fair default and it is not somebody's only preference: an assistant on a
 * machine where a turn takes a minute is most useful when you are doing
 * something else, and then the screen is exactly what you are not looking at.
 *
 * So it follows the same switch as reading answers out loud — "Read every
 * answer out loud" in the settings — and still speaks in a voice conversation
 * whatever that is set to, because there the screen is not an option at all.
 */
let alwaysSpeaks = true;

setInterval(async () => {
    try {
        const body = await fetch('/api/status', { headers: { Accept: 'application/json' } })
            .then((r) => r.json());

        const status = body && body.data !== undefined ? body.data : body;

        alwaysSpeaks = status.always_speak !== false;
    } catch {
        // Keep the last answer: a failed poll is not a preference.
    }
}, 10000);

function shouldSay() {
    if (window.brainIsTalking && window.brainIsTalking()) return true;

    return alwaysSpeaks;
}

/*
 * One line as a step begins, then quiet.
 *
 * Only at the start of each: narrating progress through a long job is worse
 * than saying nothing, because it talks over the person while they are
 * deciding whether to interrupt. Keyed on what would be said rather than on
 * the step, so a job whose note changes two thousand times says its line once.
 */
function announce(step) {
    if (!step) return;
    if (!shouldSay()) return;

    /*
     * Work nobody asked for stays quiet while it happens.
     *
     * The guard below says a line once and then holds its tongue — but it is
     * keyed on the step, and reading a drive is thousands of steps: one per
     * few files, each ending, each resetting the guard. So "I am learning from
     * that" was said every ninety seconds for an afternoon.
     *
     * The same reasoning that keeps listening off this list. Announcing the
     * middle of a long background job is worse than announcing nothing,
     * because it talks over the person the work was meant to stay out of the
     * way of. It is on the screen the whole time, and the brain says what it
     * learned when the folder is finished — see finishedReading.
     */
    if (step.background) return;

    const said = SPOKEN[step.tool] || SPOKEN_KINDS[step.kind];

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
    /*
     * Every step, not only the tools.
     *
     * Thinking is the longest thing that happens on this machine and said
     * nothing about itself, so the wait between a question and its answer was
     * a minute of silence indistinguishable from the program having stopped.
     * announce takes the whole step and picks the tool's line when there is
     * one, the kind's when there is not.
     */
    announce(step);
}

// Read from the one place that asks, rather than asking again. See signals.js.
function poll() {
    show(window.brainWork ? window.brainWork() : { busy: false });
}

setInterval(poll, INTERVAL);
poll();
