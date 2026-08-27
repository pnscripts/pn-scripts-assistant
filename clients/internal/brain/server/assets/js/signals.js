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
    work: { busy: false, kind: '', note: '', seconds: 0, round: 0 },
};

/*
 * How much of a reading is sound somebody made.
 *
 * The raw level includes the room — a fan, a drive, traffic outside. The brain
 * measures this room's floor at the start of every turn because it has to, in
 * order to know when a sentence has ended, so the same measured number is
 * subtracted here rather than a guessed one.
 */
function audible(level, floor) {
    if (level <= floor) return 0;

    return (level - floor) / Math.max(0.05, 1 - floor);
}

async function poll() {
    if (document.hidden) return;

    try {
        const reading = await fetch('/api/level').then((r) => r.json());

        signals.source = reading.source || '';
        signals.level = signals.source
            ? audible(reading.level || 0, reading.floor || 0)
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
        signals.work = { busy: false, kind: '', note: '', seconds: 0, round: 0 };
    }
}

setInterval(pollWork, WORK_INTERVAL);
pollWork();

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

window.brainMapState = function (state) {
    signals.state = state || 'idle';
};

window.brainMapRecall = function (ids) {
    const used = ids || [];

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
