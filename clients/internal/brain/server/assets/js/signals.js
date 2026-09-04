/*
 * What the brain is doing, for anything that draws it.
 *
 * One place asks, so that the core and the memory map cannot disagree about
 * whether the brain is speaking, and so that three pictures of the same number
 * do not become three requests for it.
 *
 * Every value here is measured. The level is the amplitude of real samples —
 * the microphone's or the brain's own voice — and the recall pulse rises only
 * when a reply actually used memories. Nothing is driven by a timer, which is
 * the point: when the display is still, nothing is happening.
 */

/** How often the level is asked for. */
const LEVEL_INTERVAL = 90;

/** Roughly six seconds of history, enough to see a sentence as a shape. */
const WAVE_POINTS = 72;

export const signals = {
    /** "mic", "voice", or "" when the room is quiet. */
    source: '',
    /** 0 to 1, with this room's own noise already taken off. */
    level: 0,
    /** The same, eased, for anything that would otherwise twitch. */
    smooth: 0,
    /** listening | thinking | speaking | idle, from the conversation loop. */
    state: 'idle',
    /** Rises when memories are recalled, then falls away. */
    recall: 0,
    /** The recent history, oldest first. */
    wave: new Array(WAVE_POINTS).fill(0),
    position: 0,
    points: WAVE_POINTS,

    /*
     * What the brain is working on, from the same place the conversation line
     * reads it.
     *
     * Asked here rather than in each thing that draws it, because there are now
     * three: the line above the transcript, the core, and the voice card. Three
     * pollers asking the same question three times a second to get the same
     * answer would be silly, and worse, they could disagree.
     */
    work: { busy: false, in_a_turn: false, kind: '', note: '', seconds: 0, round: 0, background: false, model: '' },

    /*
     * What the core is coloured with.
     *
     * Kept here because it can change while the program is running: its owner
     * can ask for a different colour out loud and the brain sets it, so the
     * page has to notice rather than reading it once at startup.
     */
    look: {
        thinking_line: '#7bffa8',
        thinking_core: '#f0b26b',
        speaking: '#7bffa8',
        listening: '#5fe3f5',
        idle: '#5fe3f5',
    },
};

/*
 * How much of a reading is sound somebody made.
 *
 * The raw level includes the room — a fan, a drive, traffic outside. The brain
 * measures this room's floor at the start of every turn because it has to, in
 * order to know when a sentence has ended, so the same measured number is
 * subtracted here rather than a guessed one.
 */
/*
 * How much of what the microphone hears is worth drawing.
 *
 * Measured from where speech starts, not from where the room sits. Scaling
 * from the noise floor drew a bar for anything above it at all, so a quiet
 * room produced a busy waveform — the fan, the drive, the traffic outside —
 * while the listener, using a margin above that same floor, correctly treated
 * all of it as nothing. Two answers to "is anybody talking", disagreeing, and
 * the wrong one was the one on screen.
 *
 * The floor is kept as the fallback for a reading that carries no speech bar,
 * which is the old behaviour rather than a blank display.
 */
function audible(level, floor, speech) {
    const start = speech > 0 ? speech : floor;

    if (level <= start) return 0;

    return (level - start) / Math.max(0.05, 1 - start);
}

async function poll() {
    if (document.hidden) return;

    try {
        const reading = await fetch('/api/level').then((r) => r.json());

        signals.source = reading.source || '';
        signals.level = signals.source
            ? audible(reading.level || 0, reading.floor || 0, reading.speech || 0)
            : 0;
    } catch {
        // A failed poll means no reading, not the last reading forever.
        signals.source = '';
        signals.level = 0;
    }

    signals.wave[signals.position] = signals.level;
    signals.position = (signals.position + 1) % WAVE_POINTS;
}

setInterval(poll, LEVEL_INTERVAL);
poll();

/** How often to ask what it is working on. */
const WORK_INTERVAL = 700;

async function pollWork() {
    if (document.hidden) return;

    try {
        signals.work = await fetch('/api/progress').then((r) => r.json());
    } catch {
        signals.work = { busy: false, in_a_turn: false, kind: '', note: '', seconds: 0, round: 0, background: false, model: '' };
    }
}

setInterval(pollWork, WORK_INTERVAL);
pollWork();

async function pollLook() {
    if (document.hidden) return;

    try {
        const look = await fetch('/api/appearance').then((r) => r.json());

        if (look && look.idle) signals.look = look;
    } catch {
        // Keeping the colours it already has is the right failure here.
    }
}

// Slowly: this changes when somebody asks for it to, which is rare, but it has
// to change without a restart or asking would not feel like doing.
setInterval(pollLook, 2000);
pollLook();

/** Called every frame by whichever scene is running. */
export function easeSignals(delta) {
    signals.smooth += (signals.level - signals.smooth) * Math.min(1, delta * 9);
    signals.recall = Math.max(0, signals.recall - delta * 0.4);
}

/* ---------- what the conversation reports ---------- */

const recallListeners = [];

/** Called when a reply lands, with the memories it used. */
export function onRecall(fn) {
    recallListeners.push(fn);
}

/*
 * Passed on rather than taken over.
 *
 * This file is a module, so it runs after every plain script on the page —
 * including the one the voice panel uses to colour its state. Assigning here
 * replaced that handler instead of joining it, and the word under the
 * microphone stopped changing at that moment: it read "Waiting for its name"
 * in the resting colour through thinking, speaking and everything else, while
 * the label an inch below it said something different from the same event.
 *
 * Whoever was here first is still called. Being loaded last is not a reason to
 * be the only one.
 */
const alsoTell = window.brainMapState;

window.brainMapState = function (state) {
    signals.state = state || 'idle';

    if (alsoTell) alsoTell(state);
};

const alsoTellRecall = window.brainMapRecall;

window.brainMapRecall = function (ids) {
    const used = ids || [];

    if (alsoTellRecall) alsoTellRecall(ids);

    // Scaled by how much was recalled, so a question answered from one memory
    // does not look like one answered from a dozen.
    signals.recall = Math.min(1, used.length / 10);

    for (const fn of recallListeners) fn(used);
};

/** The working line reads this. */
window.brainWork = function () {
    return signals.work;
};

/** The traces beside the message box read this. */
window.brainLevel = function () {
    return {
        source: signals.source,
        level: signals.level,
        smooth: signals.smooth,
        wave: signals.wave,
        pos: signals.position,
        points: WAVE_POINTS,
    };
};
