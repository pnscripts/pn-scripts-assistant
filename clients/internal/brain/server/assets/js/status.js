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
    idle: 'Talk',
    listening: 'Listening',
    hearing: 'Hearing you',
    thinking: 'Thinking',
    tool: 'Working',
    speaking: 'Speaking',
    waiting: 'Needs you',
    learning: 'Learning',
};

window.brainStatusLabel = (status) => STATUS_LABEL[status] || 'Working';

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

setInterval(followStatus, 500);
followStatus();
