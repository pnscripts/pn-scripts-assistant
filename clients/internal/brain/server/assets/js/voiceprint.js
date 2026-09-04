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

    if (!res.ok) throw new Error(data.error || `that did not work (${res.status})`);

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
     * Three separate facts, because they fail separately and the fix is
     * different for each: the model can be missing, no voice can have been
     * taught, and acting on it can be switched off.
     */
    const lines = [
        ['what it needs', state.installed ? 'installed' : `not here yet (${state.megabytes}MB)`],
        ['your voice', state.enrolled
            ? `taught from ${state.samples} recording${state.samples === 1 ? '' : 's'}`
            : 'not taught yet'],
    ];

    if (state.enrolled && typeof state.agreement === 'number' && state.samples > 1) {
        /*
         * How alike the recordings were to each other.
         *
         * The honest measure of whether the enrolment is any good: several
         * recordings of one person in one room sit close together, and if they
         * do not then something else was in the room and the print describes a
         * blend of two voices.
         */
        lines.push(['how alike they were', `${state.agreement.toFixed(2)}${
            state.agreement < 0.6 ? ' — low; teach it again somewhere quieter' : ''}`]);
    }

    rows.innerHTML = lines.map(([what, value]) => `<div class="row">
        <span class="row-label">${what}</span>
        <span class="row-value">${value}</span>
    </div>`).join('');

    el('voice-get').hidden = state.installed;
    el('voice-teach').hidden = !state.installed;
    el('voice-forget').hidden = !state.enrolled;
    el('voice-only-field').hidden = !state.enrolled;
    el('voice-only').checked = !!state.only_me;

    el('voice-teach').textContent = state.enrolled
        ? 'Teach it more of my voice'
        : 'Teach it my voice';
}

el('voice-get').onclick = async () => {
    el('voice-get').disabled = true;
    say(`Fetching about ${state.megabytes}MB. It carries on in the background — watch Doing now.`);

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
            say('Ready. Teach it your voice next.', 'ok');
        }
    }, 4000);
};

el('voice-teach').onclick = async () => {
    const button = el('voice-teach');

    button.disabled = true;
    say('Say a sentence — anything, in your normal voice. Listening…');

    try {
        const done = await send('/api/voiceprint/teach', {
            device: (el('microphone') || {}).value || '',
        });

        say(done.enough
            ? `Got it — ${done.samples} recordings, and that is enough. You can add more any time.`
            : `Got it — ${done.samples} of ${done.wanted}. Press again and say something else.`,
            'ok');
    } catch (err) {
        say(String(err.message || err), 'bad');
    }

    button.disabled = false;
    load();
};

el('voice-forget').onclick = async () => {
    const agreed = window.confirm(
        'Forget your voice?\n\nThe voiceprint is deleted and it goes back to '
        + 'answering any voice that says its name. Nothing else is affected.'
    );

    if (!agreed) return;

    try {
        await send('/api/voiceprint/forget', {});
        say('Forgotten. Nothing about your voice is left on this machine.', 'ok');
    } catch (err) {
        say(String(err.message || err), 'bad');
    }

    load();
};

el('voice-only').onchange = async () => {
    const on = el('voice-only').checked;

    try {
        await fetch('/api/settings', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ only_me: on }),
        });

        say(on
            ? 'It will answer your voice and ignore the rest of the room.'
            : 'It will answer any voice that addresses it.', 'ok');
    } catch (err) {
        say(String(err.message || err), 'bad');
    }

    load();
};

load();

}());
