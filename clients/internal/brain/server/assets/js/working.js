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
 * Nothing is said out loud as a tool starts.
 *
 * There were twenty lines here, one per tool: "I am reading it", "I am looking
 * at the folder", "I am checking your reminders". Every one of them describes
 * something already on the screen, most of the tools they cover return in
 * under a second, and a turn that uses three tools said three of them before
 * the answer. An assistant that narrates its own work is not being helpful, it
 * is reading its to-do list aloud at somebody.
 *
 * The reasoning they were written under still holds and is answered better
 * elsewhere: silence during a long wait is frightening, so there is one line
 * at forty seconds — see stillWorking — rather than a line per step. And
 * anything that needs a decision speaks, because that is the one thing
 * somebody may not be looking at and cannot act on if they miss it.
 *
 * Kept as an empty map rather than removed so the shape of announce() still
 * says what it is for: a tool could earn a line here, and would have to earn
 * it by being slow and by not being visible anywhere else.
 */
const SPOKEN = {};

/*
 * Almost nothing is said out loud.
 *
 * This list had seven entries and a turn used three of them: "I am making out
 * what you said", "I am thinking about that", "I am writing the answer" — all
 * before the answer itself. Four spoken sentences for one question, three of
 * them describing what was already on the screen in front of the person
 * hearing them. That is not company, it is a colleague reading their own
 * to-do list aloud.
 *
 * What is left is the one thing somebody might not be looking at and does
 * need: something is waiting on a decision from them. Everything else stays on
 * the screen, where it always was.
 *
 * The wait is handled separately and better. Silence after a question is only
 * frightening when it goes on, so instead of narrating each step there is one
 * line when a turn has been going a long time — see stillWorking below.
 */
const SPOKEN_KINDS = {
    waiting: 'I need you to decide something.',
};

/*
 * How long a turn runs before it says anything about itself.
 *
 * The reason the narration existed: on this hardware an answer can take a
 * minute, and a minute of silence is indistinguishable from a program that has
 * stopped. That is a real problem and it needed one sentence, not four — and
 * it needed them at forty seconds rather than at the first step, because
 * almost every turn finishes before that and never needs saying anything at
 * all.
 */
const LONG_ENOUGH_TO_MENTION = 40;

let mentioned = false;

function stillWorking(step) {
    if (!step.busy || step.background) {
        mentioned = false;

        return;
    }

    if (mentioned || (step.seconds || 0) < LONG_ENOUGH_TO_MENTION) return;
    if (!shouldSay()) return;

    mentioned = true;

    // Said once per turn, and it says what is actually taking the time.
    fetch('/api/speak', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
            text: `Still ${(step.note || 'working').toLowerCase()}. This machine takes a while.`,
        }),
    }).catch(() => {});
}

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
    stillWorking(step);
}

// Read from the one place that asks, rather than asking again. See signals.js.
function poll() {
    show(window.brainWork ? window.brainWork() : { busy: false });
}

setInterval(poll, INTERVAL);
poll();
