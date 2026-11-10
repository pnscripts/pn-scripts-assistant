/*
 * Whose voice the brain answers.
 *
 * Every test before this one asks whether speech happened — loud enough,
 * shaped like a voice, addressed by name. None of them can say whose voice it
 * was, which is the question actually being asked by "listen to me, not to
 * that". A television says the name as readily as a person does.
 *
 * So a model measures the voice itself and the result is five hundred numbers.
 * No audio is kept, nothing about them leaves this machine, and one press
 * deletes them — which is the only thing that makes keeping a record of
 * somebody's voice reasonable in the first place.
 */
(function () {

// Every word from i18n.js, in the page's language.
const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);

function el(id) {
    return document.getElementById(id);
}

function say(what, kind) {
    const line = el('voice-progress');

    if (!line) return;

    line.textContent = what || '';
    line.dataset.kind = kind || '';
}

async function send(where, body) {
    const res = await fetch(where, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body || {}),
    });

    const data = await res.json().catch(() => ({}));

    if (!res.ok) throw new Error(data.error || T('voice.failed', { status: res.status }));

    return data;
}

let state = {};

async function load() {
    try {
        state = await fetch('/api/voiceprint').then((r) => r.json());
    } catch {
        return;
    }

    draw();
}

function draw() {
    const rows = el('voice-state-rows');

    if (!rows) return;

    /*
     * Withdrawn since 1.0.1: no model tells voices apart, so the card says
     * why, and a voice taught before can only be forgotten.
     */
    const withdrawn = !!state.withdrawn;

    el('voice-note').hidden = withdrawn;
    el('voice-withdrawn').hidden = !withdrawn;
    el('voice-withdrawn-kept').hidden = !(withdrawn && state.enrolled);

    /*
     * Separate facts, because they fail separately and the fix is different
     * for each: the model can be missing, no voice can have been taught, and
     * acting on it can be switched off.
     */
    const taught = state.samples === 1
        ? T('voice.taughtOne')
        : T('voice.taughtMany', { n: state.samples });

    const lines = withdrawn
        ? (state.enrolled ? [[T('voice.yours'), T('voice.kept')]] : [])
        : [
            [T('voice.needs'), state.installed ? T('voice.installed')
                : state.downloading ? T('voice.downloading') : T('voice.missing', { mb: state.megabytes })],
            [T('voice.yours'), state.enrolled ? taught : T('voice.notTaught')],
        ];

    if (!withdrawn && state.enrolled && typeof state.agreement === 'number' && state.samples > 1) {
        /*
         * How alike the recordings were to each other.
         *
         * The honest measure of whether the enrolment is any good: several
         * recordings of one person in one room sit close together, and if they
         * do not then something else was in the room and the print describes a
         * blend of two voices.
         */
        const alike = state.agreement.toFixed(2);

        lines.push([T('voice.alike'), `${window.pnLang && window.pnLang() === 'bg' ? alike.replace('.', ',') : alike}${
            state.agreement < 0.6 ? T('voice.low') : ''}`]);
    }

    /*
     * "Answer only me" on, and the voice cannot be checked.
     *
     * Then nothing it hears is acted on, rather than everything: the switch
     * still shows on, so it has to say here why the brain is not answering.
     */
    if (state.only_me && state.only_me_blocked) {
        // onlyme.js is loaded before this script and says it in the page's language.
        lines.push(window.pnOnlyMe.blocked(state));
    }

    // Text, never markup: a reason the server gives is shown as it is written.
    rows.replaceChildren(...lines.map(([what, value]) => {
        const row = document.createElement('div');
        const label = document.createElement('span');
        const shown = document.createElement('span');

        row.className = 'row';
        label.className = 'row-label';
        label.textContent = what;
        shown.className = 'row-value';
        shown.textContent = value;
        row.append(label, shown);

        return row;
    }));

    el('voice-get').hidden = withdrawn || state.installed;
    // One download at a time: the server refuses a second, and the button
    // says so by not being pressable while one runs.
    el('voice-get').disabled = !!state.downloading;
    el('voice-teach').hidden = withdrawn || !state.installed;
    el('voice-forget').hidden = !state.enrolled;
    el('voice-only-field').hidden = withdrawn || !state.enrolled;
    el('voice-only').checked = !!state.only_me;

    /*
     * Not gated on having taught it a voice.
     *
     * This one is about what the room sounds like, not about who is in it, and
     * it is worth switching on the first evening — long before anybody gets
     * round to teaching it a voice.
     */
    el('voice-room').checked = !!state.cancel_room;
    el('voice-quiet').checked = !!state.keep_quiet;

    el('voice-teach').textContent = state.enrolled ? T('voice.teachMore') : T('voice.teach');
}

el('voice-get').onclick = async () => {
    el('voice-get').disabled = true;
    say(T('voice.fetching', { mb: state.megabytes }));

    try {
        await send('/api/voiceprint/install', {});
    } catch (err) {
        say(String(err.message || err), 'bad');
        el('voice-get').disabled = false;

        return;
    }

    // It arrives in its own time, so the panel keeps looking rather than
    // claiming it is there.
    const watching = setInterval(async () => {
        await load();

        if (state.installed) {
            clearInterval(watching);
            el('voice-get').disabled = false;
            say(T('voice.ready'), 'ok');
        }
    }, 4000);
};

el('voice-teach').onclick = async () => {
    const button = el('voice-teach');

    button.disabled = true;
    say(T('voice.listening'));

    try {
        const done = await send('/api/voiceprint/teach', {
            device: (el('microphone') || {}).value || '',
        });

        say(done.enough
            ? T('voice.gotEnough', { n: done.samples })
            : T('voice.gotSome', { n: done.samples, wanted: done.wanted }),
            'ok');
    } catch (err) {
        say(String(err.message || err), 'bad');
    }

    button.disabled = false;
    load();
};

el('voice-forget').onclick = async () => {
    const agreed = window.confirm(T('voice.forgetConfirm'));

    if (!agreed) return;

    try {
        await send('/api/voiceprint/forget', {});
        say(T('voice.forgotten'), 'ok');
    } catch (err) {
        say(String(err.message || err), 'bad');
    }

    load();
};

el('voice-quiet').onchange = async () => {
    const on = el('voice-quiet').checked;

    try {
        await send('/api/settings', { keep_quiet: on });

        say(on ? T('voice.quietOn') : T('voice.quietOff'), 'ok');
    } catch (err) {
        say(String(err.message || err), 'bad');
    }

    load();
};

el('voice-room').onchange = async () => {
    const on = el('voice-room').checked;

    say(on ? T('voice.roomMoving') : T('voice.roomRestoring'));

    try {
        await send('/api/settings', { cancel_room: on });

        say(on ? T('voice.roomOn') : T('voice.roomOff'), 'ok');
    } catch (err) {
        say(String(err.message || err), 'bad');
    }

    load();
};

el('voice-only').onchange = async () => {
    const on = el('voice-only').checked;

    try {
        // send, not fetch: a refusal (withdrawn, nothing taught) is said
        // rather than reported as done.
        await send('/api/settings', { only_me: on });

        say(on ? T('voice.onlyOn') : T('voice.onlyOff'), 'ok');
    } catch (err) {
        say(String(err.message || err), 'bad');
    }

    load();
};

// The whole card follows the page's language when it changes.
window.addEventListener('pn-language', () => draw());

load();

}());
