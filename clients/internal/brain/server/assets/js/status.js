/*
 * One name and one colour per status, for the whole interface.
 *
 * There used to be three separate answers to "what is the brain doing, and what
 * colour is that". The core read four states from the server's palette; the
 * feed knew nine and hard-coded five tones of its own; the talk button had a
 * third set written into CSS. So a single moment was violet in one corner of
 * the screen, cyan in another and amber in a third, and nothing agreed with
 * anything else.
 *
 * This is the only place either question is answered. The server supplies the
 * colours, they are written into CSS variables once, and every part of the page
 * asks for a status by name and gets the same colour — including the core,
 * which no longer keeps a private palette.
 */

/*
 * The statuses, in the order they occur in a turn.
 *
 * Deliberately few. Every one has to be distinguishable at a glance from
 * across a room, and a list of fifteen shades is a list nobody learns — it
 * degrades to "something is happening", which is what a single colour would
 * have said for free.
 */
const STATUSES = [
    'idle',       // nothing happening
    'listening',  // microphone open, waiting for you
    'hearing',    // making out the words just said
    'thinking',   // a model is working
    'tool',       // doing something in the world
    'speaking',   // talking back
    'waiting',    // stopped, needs a decision from you
    'learning',   // its own background work, nobody waiting
];

/*
 * Everything the rest of the program calls a state, mapped to one of those.
 *
 * The server reports progress kinds, the page's own loop reports button states,
 * and they were never the same words. Both are listed here so neither has to
 * know about the other.
 */
const STATUS_OF = {
    listening: 'listening',
    transcribing: 'hearing',
    thinking: 'thinking',
    tool: 'tool',
    answering: 'thinking',
    speaking: 'speaking',
    waiting: 'waiting',
    learning: 'learning',
    embedding: 'learning',
    model: 'thinking',
};

/*
 * The page's own button states, which are a different vocabulary.
 *
 * Kept separate because one word means two things. The brain says "waiting"
 * when it has stopped and needs a decision — a red status, something is
 * blocked. The button says "waiting" when it is sitting there ready, listening
 * for its name, which is the calmest state it has. Running both through one
 * table painted an idle assistant in the colour reserved for needing help.
 */
const BUTTON_STATUS = {
    idle: 'idle',
    waiting: 'idle',
    stopping: 'idle',
    listening: 'listening',
    thinking: 'thinking',
    speaking: 'speaking',
};

/** The canonical status for a progress step, or for a bare state name. */
function statusOf(step) {
    if (!step) return 'idle';

    /*
     * A bare name, which may be either vocabulary.
     *
     * Already a canonical status — the sync passes those straight through —
     * or one of the page's own button states, which need translating. Checked
     * in that order because the page's "waiting" means the opposite of the
     * brain's, and only the translation table knows that.
     */
    if (typeof step === 'string') {
        if (BUTTON_STATUS[step]) return BUTTON_STATUS[step];

        return STATUSES.includes(step) ? step : 'idle';
    }

    if (!step.busy) return 'idle';

    // Work the brain gave itself is never coloured as though somebody is
    // waiting on it — except listening, which is reported the same way and is
    // exactly the status the interface exists to show.
    if (step.background && step.kind !== 'listening') return 'learning';

    return STATUS_OF[step.kind] || 'thinking';
}

/*
 * The palette, written into CSS variables so stylesheets can use it directly.
 *
 * Set on the document root rather than passed around, because most of what
 * needs a status colour is a border or a dot in a stylesheet, and reaching
 * those from script would mean re-implementing the cascade.
 */
function paintStatusPalette(look) {
    const root = document.documentElement;

    STATUSES.forEach((status) => {
        const colour = look[status === 'thinking' ? 'thinking_core' : status];

        if (colour) root.style.setProperty(`--status-${status}`, colour);
    });

    // The sweeping line has its own colour and is not a status.
    if (look.thinking_line) {
        root.style.setProperty('--status-line', look.thinking_line);
    }
}

async function loadStatusPalette() {
    try {
        const look = await fetch('/api/appearance').then((r) => r.json());

        paintStatusPalette(look);

        // Kept for the core, which needs the numbers rather than the variables.
        window.brainLook = look;
    } catch {
        // The stylesheet carries the same defaults, so the page is coloured
        // correctly even when this never arrives.
    }
}

// Re-read periodically: the brain can be asked to change these by voice, and a
// colour that only takes effect after a restart is one nobody believes changed.
loadStatusPalette();
setInterval(loadStatusPalette, 10000);

window.brainStatusOf = statusOf;
window.brainRepaintStatuses = loadStatusPalette;
window.brainStatuses = STATUSES;

/*
 * What to call each status, in words.
 *
 * One list, for the same reason there is one list of colours. The talk button
 * had its own names and its own collapsing — it called both "making out the
 * words" and "thinking" Thinking — so at any moment mid-turn the button and
 * the panel beside it named the same instant differently and coloured it
 * differently too. Two labels for one state is the same bug as two colours
 * for one state, and it is more obvious to read.
 */
const STATUS_LABEL = {
    idle: 'cc.status.idle',
    listening: 'cc.status.listening',
    hearing: 'cc.status.hearing',
    thinking: 'cc.status.thinking',
    tool: 'cc.status.tool',
    speaking: 'cc.status.speaking',
    waiting: 'cc.status.waiting',
    learning: 'cc.status.learning',
};

function statusWords(key, vars) {
    return window.pnT ? window.pnT(key, vars) : key;
}

window.brainStatusLabel = (status) => statusWords(STATUS_LABEL[status] || 'cc.status.tool');

/*
 * The lines the brain writes about what it is doing, in the page's language.
 *
 * The server writes each step for a person in English ("Reading notes.txt:
 * passage 3 of 12"), and the same line is read by the log and the tests. The
 * page says it in Bulgarian by recognising the shape of the line: the English
 * of each of these strings in i18n.js is a shape the Go code writes, with
 * {1}, {2} where its values go, and TestEveryStepLineHasItsBulgarian checks
 * the shapes against the lines the Go code writes. A line nobody has listed
 * (a tool's own summary of what it was asked) is shown as it came.
 */
const STEP_KEYS = [
    'cc.step.thinking',
    'cc.step.listening',
    'cc.step.hearing',
    'cc.step.speaking',
    'cc.step.answering',
    'cc.step.doing',
    'cc.step.lastConversation',
    'cc.step.voicesApart',
    'cc.step.waitingFor',
    'cc.step.passage',
    'cc.step.learningMany',
    'cc.step.learningFrom',
    'cc.step.waitingList',
    'cc.step.reindexing',
    'cc.step.testing',
    'cc.step.listeningTo',
    'cc.step.couldNotRead',
    'cc.step.reading',
    'cc.step.passOneMore',
    'cc.step.passManyMore',
    'cc.step.passOneAll',
    'cc.step.passManyAll',
    'cc.step.prompt',
    'cc.step.rate',
    'cc.step.words',
    'cc.step.nothingBack',
    'cc.step.linesBack',
    'cc.step.failed',
    'cc.step.stillTalking',
    'cc.step.joined',
    'cc.step.noise',
    'cc.step.cutIn',
    'cc.step.speech',
    'cc.step.film',
    'cc.step.ownVoice',
    'cc.step.heard',
    'cc.step.noWords',
    'cc.step.kept',
    'cc.step.unfamiliar',
    'cc.step.referenceFor',
    'cc.step.reference',
    'cc.step.checking',
    'cc.step.runningAWhile',
    'cc.step.running',
    'cc.step.building',
    'cc.step.exporting',
    'cc.step.workingIn',
];

let stepShapes = null;

// The shapes, longest first, so "Reading {1}: passage {2} of {3}" is tried
// before "Reading {1}".
function shapes() {
    if (stepShapes) return stepShapes;

    stepShapes = STEP_KEYS.map((key) => {
        const english = window.pnEnglish ? window.pnEnglish(key) : key;
        const pattern = english
            .split(/\{\d\}/)
            .map((piece) => piece.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
            .join('(.+?)');

        return { key, size: english.length, pattern: new RegExp(`^${pattern}$`) };
    }).sort((a, b) => b.size - a.size);

    return stepShapes;
}

// A number or a duration as the page's language writes it: "1.5s" is
// "1,5 с" in Bulgarian.
function localNumber(text) {
    const bg = window.pnLang && window.pnLang() === 'bg';
    const value = String(text);

    if (!bg) return value;

    const took = value.match(/^(?:(\d+)m )?(\d+(?:\.\d+)?)(ms|s)$/);

    if (took) {
        const unit = took[3] === 'ms' ? 'мс' : 'с';
        const rest = `${took[2].replace('.', ',')} ${unit}`;

        return took[1] ? `${took[1]} мин ${rest}` : rest;
    }

    return /^[\d.]+k?$/.test(value) ? value.replace('.', ',').replace(/k$/, ' хил.') : value;
}

window.pnLocalNumber = localNumber;

/*
 * stepWords says one line about a step in the page's language: the note
 * when no step is given, or the step's own headline. On a Bulgarian page a
 * tool's step is named by the tool, since its summary is the tool's own
 * English.
 */
window.pnStepLine = function (line, step) {
    const bg = window.pnLang && window.pnLang() === 'bg';
    const text = String(line || '');

    if (!bg) return text;

    if (step && step.tool && window.pnToolName && (step.kind === 'tool' || !step.kind)) {
        return window.pnToolName(step.tool);
    }

    for (const { pattern, key } of shapes()) {
        const m = text.match(pattern);

        if (!m) continue;

        const vars = {};

        m.slice(1).forEach((value, i) => { vars[i + 1] = localNumber(value); });

        return statusWords(key, vars);
    }

    return text;
};

/*
 * The whole interface follows the current status.
 *
 * The panels, their headings and their borders were a fixed cyan whatever the
 * brain was doing, so the core could be violet for listening while every frame
 * around it stayed the colour of a different state. Setting the status on the
 * document root lets the chrome take its tint from the same variable as
 * everything else, and the room changes colour with the work.
 *
 * Deliberately a tint and not the full colour. Borders and headings at full
 * strength would compete with the core, which is the thing meant to be read
 * first; mixed down against the resting line colour they read as the room
 * having shifted rather than as eight things shouting.
 */
function followStatus() {
    const step = window.brainWork ? window.brainWork() : null;

    document.documentElement.dataset.status = statusOf(step);
}

/*
 * Twice a second was four times a second's worth of work for a colour.
 *
 * Every one of these asks the server what the brain is doing and repaints
 * whatever is showing it. On a machine drawing its own pixels with the
 * processor that is not free, and nothing on this page changes state faster
 * than a person can notice — a step that lasts a minute does not need to be
 * checked a hundred and twenty times while it runs.
 */
setInterval(followStatus, 1200);
followStatus();
