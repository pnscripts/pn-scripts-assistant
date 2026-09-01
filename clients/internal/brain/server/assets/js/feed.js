/*
 * What the brain is doing, as it does it.
 *
 * The panel this replaced listed things that change perhaps twice a day —
 * which model, how much is remembered, whether voice is ready — in the best
 * position on the page, while the thing that changes every few seconds had
 * nowhere to appear at all. Somebody who spoke to it saw nothing happen until
 * the answer arrived, which on this machine can be minutes later, and there
 * was no way to tell work from a hang.
 *
 * Built like an installer's log because that is the shape that answers the
 * question: a line per step, the one in progress still moving, the ones before
 * it settled with how long they took. The times are the point. A step that has
 * been running four minutes and one that took eighty milliseconds look
 * identical without them, and they mean opposite things.
 */

const FEED_INTERVAL = 700;

const feedBox = document.getElementById('feed');
const feedIdle = document.getElementById('feed-idle');

/*
 * How to introduce each kind of step.
 *
 * The names only. Colour comes from the shared status — see status.js — so a
 * step is the same colour here as it is on the talk button and in the core.
 * This list used to carry its own tones, which is how the same moment came to
 * be three different colours in three places.
 */
const FEED_KINDS = {
    listening: 'Listening',
    transcribing: 'Making out the words',
    thinking: 'Thinking',
    tool: 'Running a tool',
    answering: 'Writing the answer',
    speaking: 'Speaking',
    waiting: 'Waiting for you',
    learning: 'Learning',
    model: 'Testing a model',
    embedding: 'Re-indexing',
};

/** A duration a person can read at a glance. */
function feedTime(seconds) {
    if (!seconds || seconds < 0.1) return '';
    if (seconds < 1) return `${Math.round(seconds * 1000)}ms`;
    if (seconds < 60) return `${seconds.toFixed(1)}s`;

    const mins = Math.floor(seconds / 60);

    return `${mins}m ${String(Math.round(seconds % 60)).padStart(2, '0')}s`;
}

let feedShown = '';

/*
 * What kinds of step count as the brain working on something asked of it.
 *
 * Listening is not one of them. It happens continuously and produces an entry
 * every time the room makes a noise, so on a machine that takes a minute to
 * answer, a genuine multi-step task arrives interleaved with — and buried
 * under — a running commentary on the fan.
 */
const WORKING_KINDS = new Set([
    'thinking', 'tool', 'answering', 'speaking', 'waiting', 'learning',
    'model', 'embedding',
]);

function isWork(step) {
    return WORKING_KINDS.has(step.kind) || !!step.tool;
}

/*
 * The one line that answers "is it still going".
 *
 * A list of finished steps answers what happened; it does not say whether
 * anything is happening now, and on a machine where an answer takes a minute
 * that is the question actually being asked. It was inferable — from a clock
 * ticking on the last row — and inferable is not the same as said.
 */
function sayWhetherItIsWorking(all, working) {
    const line = document.getElementById('working-on');
    const text = document.getElementById('working-on-text');
    const clock = document.getElementById('working-on-clock');

    if (!line || !text || !clock) return;

    /*
     * Whether a question is being answered — not what is happening this
     * instant.
     *
     * "busy" is true of a silent room being listened to, and during a real
     * turn the current step alternates between answering and listening several
     * times a second, because the microphone reopens as soon as an answer
     * starts being written. A line driven by that flickered on and off while
     * the brain worked steadily, which is worse than not having one.
     */
    const now = window.brainWork ? window.brainWork() : {};

    if (!now.in_a_turn) {
        line.hidden = true;

        return;
    }

    const last = all[all.length - 1];
    const step = working[working.length - 1] || last || {};
    const rounds = all.filter((s) => s.kind === 'tool' || s.tool).length;

    let what = FEED_KINDS[step.kind] || 'Working';

    if (step.tool) what = 'Running ' + step.tool;

    /*
     * How many tools it has run, because a task that needs several is the one
     * where somebody most wants to know it is progressing rather than stuck.
     */
    if (rounds > 0) what += ' · ' + rounds + (rounds === 1 ? ' step' : ' steps');

    text.textContent = 'Working on your question — ' + what;
    clock.textContent = feedTime(step.seconds) || '';
    line.dataset.status = window.brainStatusOf
        ? window.brainStatusOf({ ...step, busy: true })
        : 'thinking';
    line.hidden = false;
}

function drawFeed(steps) {
    if (!feedBox) return;

    /*
     * While there is real work in the list, the microphone noise steps aside.
     *
     * Asked to read a folder, the brain called the tool, read the answer and
     * went back for more — and none of that was findable on screen, because
     * between each round sat several "no words made out" entries from a quiet
     * room. The question "is it working on my task or not" had no answer
     * anywhere, while the panel whose whole job is to answer it scrolled.
     *
     * Only while work is in flight. With nothing being asked of it, what the
     * microphone is doing is the most interesting thing there is, and the
     * panel goes back to showing it.
     */
    const working = steps.filter(isWork);

    sayWhetherItIsWorking(steps, working);

    if (working.length) steps = working;

    if (!steps.length) {
        if (feedIdle) feedIdle.hidden = false;

        // Anything drawn before is cleared, or the last turn's steps sit there
        // looking like they are still happening.
        feedBox.querySelectorAll('.feed-line').forEach((n) => n.remove());
        feedShown = '';

        return;
    }

    if (feedIdle) feedIdle.hidden = true;

    /*
     * Redrawn only when something actually changed.
     *
     * The running step's duration changes on every poll, and rebuilding the
     * list for that restarts every CSS animation — so the whole panel flashed
     * twice a second. The signature deliberately leaves the last duration out,
     * and that one line is updated in place instead.
     */
    const signature = steps
        .map((s) => `${s.kind}|${s.note}|${s.tool}|${(s.detail || []).join('~')}`)
        .join('\n');

    if (signature !== feedShown) {
        feedBox.querySelectorAll('.feed-line').forEach((n) => n.remove());

        steps.forEach((step, i) => {
            const title = FEED_KINDS[step.kind] || step.kind || 'Working';
            const running = i === steps.length - 1;

            const line = document.createElement('div');
            line.className = 'feed-line';

            /*
             * Work nobody was waiting on reads more quietly.
             *
             * A listening turn that contained no words is announced before
             * anybody can know it contained none — the level crossed the
             * threshold, so the microphone opened. At full volume a run of
             * those looks like the assistant hearing voices in an empty room,
             * which is a far more alarming thing than what is actually
             * happening. Still shown, because it is the honest record of what
             * the microphone made of the room.
             */
            if (step.background) line.dataset.background = 'yes';

            // The one shared answer to "what colour is this status".
            line.dataset.status = window.brainStatusOf
                ? window.brainStatusOf({ ...step, busy: true })
                : 'thinking';

            line.dataset.running = running ? 'yes' : 'no';

            const dot = document.createElement('span');
            dot.className = 'feed-dot';
            line.appendChild(dot);

            const body = document.createElement('span');
            body.className = 'feed-body';

            const name = document.createElement('span');
            name.className = 'feed-name';
            name.textContent = step.note || title;
            body.appendChild(name);

            // The model, when one was answering — a switch mid-turn is most of
            // the difference in how long a turn takes, and it belongs beside
            // the step it applied to rather than only in the settings.
            if (step.model) {
                const which = document.createElement('span');
                which.className = 'feed-model';
                which.textContent = step.model;
                body.appendChild(which);
            }

            /*
             * The specifics, under the name of the step.
             *
             * This is the part worth watching. "Thinking" for four minutes is
             * the same picture whether it is working or wedged; "weather in
             * Sofia today · 10 results back in 1.2s" is not a picture of
             * anything else.
             */
            (step.detail || []).forEach((text) => {
                const detail = document.createElement('span');
                detail.className = 'feed-detail';
                detail.textContent = text;
                body.appendChild(detail);
            });

            line.appendChild(body);

            const took = document.createElement('span');
            took.className = 'feed-took';
            took.textContent = feedTime(step.took);
            line.appendChild(took);

            feedBox.appendChild(line);
        });

        feedShown = signature;
        feedBox.scrollTop = feedBox.scrollHeight;

        return;
    }

    // Unchanged, so only the running line's clock moves.
    const lines = feedBox.querySelectorAll('.feed-line');
    const last = lines[lines.length - 1];

    if (last) {
        const took = last.querySelector('.feed-took');

        if (took) took.textContent = feedTime(steps[steps.length - 1].took);
    }
}

async function pollFeed() {
    try {
        const reply = await api.get('/api/steps');

        drawFeed(reply.steps || []);
    } catch {
        // Quiet. A panel that cannot reach the brain is already obvious from
        // everything else on the page going stale.
    }
}

setInterval(pollFeed, FEED_INTERVAL);
pollFeed();

/*
 * Work running beside the conversation, shown while it runs.
 *
 * Background work had no way to be seen at all. The only way to find out what
 * the brain was doing on its own was to ask it — a model call, a minute on
 * this machine, to answer a question the program already knew — so anything
 * started in the background was invisible unless somebody thought to ask.
 * Something invisible cannot be judged, or stopped, or trusted.
 *
 * It goes in the Activity panel, which was empty, and it carries what each job
 * is and how long it has been at it. A job running for four minutes and one
 * that finished in eighty milliseconds look identical without the time, and
 * they mean opposite things.
 */
const BACKGROUND_INTERVAL = 1500;

function drawBackground(jobs) {
    const host = document.getElementById('activity');

    if (!host) return;

    let box = host.querySelector('.jobs');

    if (!jobs.length) {
        if (box) box.remove();

        return;
    }

    if (!box) {
        box = document.createElement('div');
        box.className = 'jobs';
        host.prepend(box);
    }

    // Running first: what is happening now matters more than what happened.
    const order = { running: 0, failed: 1, done: 2, stopped: 3 };
    const sorted = [...jobs].sort((a, b) =>
        (order[a.state] ?? 9) - (order[b.state] ?? 9) || b.id - a.id);

    const signature = sorted.map((j) => `${j.id}|${j.state}`).join('\n');

    if (box.dataset.jobs === signature) {
        // Only the clocks move.
        sorted.forEach((job, i) => {
            const took = box.children[i]?.querySelector('.job-took');

            if (took && job.state === 'running') took.textContent = feedTime(job.seconds);
        });

        return;
    }

    box.textContent = '';
    box.dataset.jobs = signature;

    sorted.forEach((job) => {
        const row = document.createElement('div');
        row.className = 'job';
        row.dataset.state = job.state;

        // The same status colours as everything else, so a job reads the same
        // way as a step. See status.js.
        row.dataset.status = job.state === 'running' ? 'tool'
            : job.state === 'failed' ? 'waiting' : 'learning';

        const dot = document.createElement('span');
        dot.className = 'job-dot';

        const body = document.createElement('span');
        body.className = 'job-body';

        const what = document.createElement('span');
        what.className = 'job-what';
        what.textContent = job.what || 'Working';
        body.appendChild(what);

        // What came of it, when something did. A finished job with no result
        // shown is a job nobody can tell succeeded.
        const detail = job.error || job.result;

        if (detail) {
            const note = document.createElement('span');
            note.className = 'job-detail';
            note.textContent = String(detail).replace(/\s+/g, ' ').slice(0, 140);
            body.appendChild(note);
        }

        const took = document.createElement('span');
        took.className = 'job-took';
        took.textContent = feedTime(job.seconds);

        row.append(dot, body, took);
        box.appendChild(row);
    });
}

async function pollBackground() {
    try {
        const reply = await api.get('/api/background');

        drawBackground(reply.jobs || []);
    } catch {
        // Quiet: a panel that cannot reach the brain is already obvious from
        // everything else on the page going stale.
    }
}

setInterval(pollBackground, BACKGROUND_INTERVAL);
pollBackground();
