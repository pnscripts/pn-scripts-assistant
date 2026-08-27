/*
 * PN Brain console.
 *
 * Vanilla, no framework and no build step: this ships inside a single
 * self-contained binary, and the whole surface is one chat plus a few polled
 * panels. A bundler would add a build stage to the static image for very
 * little.
 *
 * Everything here talks to the same JSON API the CLI and desktop app use, so
 * the interface stays one client among several rather than a privileged path.
 */

const api = {
    async get(path) {
        const res = await fetch(path, { headers: { Accept: 'application/json' } });
        if (!res.ok) throw new Error(`${path} -> ${res.status}`);
        return res.json();
    },
    async post(path, body) {
        const res = await fetch(path, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
            body: body === undefined ? undefined : JSON.stringify(body),
        });
        if (!res.ok) throw new Error(`${path} -> ${res.status}`);
        return res.json();
    },
};

const el = (id) => document.getElementById(id);

const state = {
    conversationId: null,
    brainName: 'PN Brain',
    busy: false,
};

/* ---------- transcript ---------- */

function clearBoot() {
    el('boot')?.remove();
}

function addMessage(who, text, { cssClass = '', meta = '', actions = [] } = {}) {
    clearBoot();

    const wrap = document.createElement('div');
    wrap.className = `msg ${cssClass}`;

    const label = document.createElement('div');
    label.className = 'who';
    label.textContent = who;
    wrap.appendChild(label);

    const body = document.createElement('div');
    body.className = 'body';
    // textContent, never innerHTML: replies can contain fetched web content,
    // and rendering that as markup would turn a page the model read into
    // script running in this console.
    body.textContent = text;
    wrap.appendChild(body);

    if (actions.length) {
        const row = document.createElement('div');
        row.className = 'actions';
        actions.forEach((a) => {
            const chip = document.createElement('span');
            chip.className = 'action-chip';
            chip.textContent = a;
            row.appendChild(chip);
        });
        wrap.appendChild(row);
    }

    if (meta) {
        const m = document.createElement('div');
        m.className = 'meta';
        m.textContent = meta;
        wrap.appendChild(m);
    }

    const transcript = el('transcript');
    transcript.appendChild(wrap);
    transcript.scrollTop = transcript.scrollHeight;
    return wrap;
}

function showThinking() {
    clearBoot();
    const wrap = document.createElement('div');
    wrap.className = 'msg brain thinking';
    wrap.innerHTML = '<div class="who"></div><div class="thinking-line">thinking…</div>';
    wrap.querySelector('.who').textContent = state.brainName;
    el('transcript').appendChild(wrap);
    el('transcript').scrollTop = el('transcript').scrollHeight;
    el('orb').classList.add('thinking');
    return wrap;
}

/* ---------- sending ---------- */

async function send(text) {
    if (state.busy || !text.trim()) return;

    state.busy = true;
    el('send').disabled = true;
    addMessage('you', text, { cssClass: 'you' });

    const placeholder = showThinking();

    // A typed message lights the reactor too, so the picture reflects what the
    // brain is doing whether or not anybody is talking to it.
    if (window.brainMapState) window.brainMapState('thinking');

    try {
        const data = await api.post('/api/chat', {
            conversation_id: state.conversationId,
            message: text,
            provider: el('provider').value || null,
        });

        state.conversationId = data.conversation_id;
        placeholder.remove();

        addMessage(state.brainName, data.reply, {
            cssClass: 'brain',
            meta: `${data.provider} · ${data.model}`,
            actions: data.actions_taken || [],
        });

        // A pause for approval is the interesting case, so surface it
        // immediately instead of waiting for the next poll.
        // Show which memories the brain actually reached for.
        if (data.recalled?.length && window.brainMapRecall) {
            window.brainMapRecall(data.recalled);
        }

        speak(data.reply);

        if (window.brainMapState) {
            window.brainMapState(talking.on ? 'listening' : 'idle');
        }

        if (data.pending_approvals?.length) refreshApprovals();
        refreshStatus();
        refreshActivity();
    } catch (err) {
        placeholder.remove();
        addMessage('error', String(err.message || err), { cssClass: 'error' });
    } finally {
        state.busy = false;
        el('send').disabled = false;
        el('orb').classList.remove('thinking');
        el('talk').onclick = toggleTalking;
    el('input').focus();
    }
}

/* ---------- panels ---------- */

async function refreshStatus() {
    try {
        const s = await api.get('/api/status');
        state.brainName = s.name || state.brainName;
        el('brain-name').textContent = s.name;
        el('engine-meta').textContent = `${s.provider} · ${s.model}`;
        renderPrivacy(s.privacy);
        renderStorage(s.storage);

        // The gauges around the reactor show these, so they move for a reason.
        if (window.brainMapGauges) {
            const used = s.storage && s.storage.total_bytes
                ? 1 - s.storage.free_bytes / s.storage.total_bytes
                : 0;

            window.brainMapGauges({
                storage: used,
                // A full queue is ten waiting; more than that is still full.
                queue: Math.min(1, (s.memory.pending_lessons || 0) / 10),
            });
        }
        el('conn-dot').className = 'dot online';
        el('conn-text').textContent = `online · ${s.capabilities.length} capabilities`;
        // Only offer to speak when there is something that can. A control for
        // a capability the machine lacks is a promise the app cannot keep.
        // Talking needs both halves: hearing you and answering aloud.
        const canTalk = s.capabilities.includes('listening')
            && s.capabilities.includes('speech');

        el('talk').hidden = !canTalk;
        el('microphone-field').hidden = !canTalk;

        el('voice-field').hidden = !s.capabilities.includes('speech');
        if (s.capabilities.includes('speech')) loadVoices();

        if (canTalk) {
            loadMicrophones();

            // On by default: somebody who has a microphone and a voice
            // installed wants to talk to it, and having to switch that on
            // every time is a small tax on the thing they came for. Started
            // once per session, and only if they have not already stopped it.
            if (!talkingStartedOnce) {
                talkingStartedOnce = true;
                talking.on = true;
                talkLoop();
            }
        }
        document.title = s.name;
    } catch {
        el('conn-dot').className = 'dot offline';
        el('conn-text').textContent = 'offline';
    }
}

function renderPrivacy(privacy) {
    if (!privacy) return;

    const summary = el('privacy-summary');

    // The panels live in views that can be switched away from, so anything
    // that renders into one has to cope with it not being there. Throwing here
    // would abort the rest of refreshStatus and quietly stop the whole status
    // panel updating.
    if (!summary) return;

    summary.textContent = privacy.summary;
    summary.className = 'privacy-summary ' + privacy.mode;
    el('privacy-detail').textContent = privacy.detail;
}

const GB = 1024 ** 3;

function renderStorage(storage) {
    if (!storage) return;

    const usedPct = storage.total_bytes
        ? Math.round((storage.used_bytes / storage.total_bytes) * 100)
        : 0;

    const fill = el('storage-fill');
    fill.style.width = usedPct + '%';
    fill.className = storage.level === 'ok' ? '' : storage.level;

    el('storage-text').textContent =
        `${(storage.free_bytes / GB).toFixed(0)}GB free · brain using ` +
        `${(storage.database_bytes / (1024 ** 2)).toFixed(0)}MB`;

    const warning = el('storage-warning');
    warning.hidden = !storage.advice;
    if (storage.advice) el('storage-advice').textContent = storage.advice;
}

/*
 * Lessons the brain proposes to remember.
 *
 * These are inferences a model made about what Petar meant, and nothing can
 * check them — which is why they wait here instead of promoting themselves.
 * Accepting one runs the same duplicate check as an automatic promotion, so
 * the one path a person touches is not the one path that creates duplicates.
 */
/*
 * Reading a reply aloud.
 *
 * Local engines only — a cloud voice would post every reply, including whatever
 * the brain recalled from this machine, to a third party to be turned into
 * audio. Failure is deliberately quiet: not hearing an answer that is already
 * on screen is a small thing, and an error box about it would be a larger one.
 */
/*
 * Listening.
 *
 * What comes back goes into the input box, not straight to the brain. A
 * recogniser that mishears "delete the backups" should not have that reach
 * something with tools before a person has read it.
 */
/*
 * Conversation mode.
 *
 * Listen until the speaker stops, send, speak the reply, listen again. The loop
 * is what makes it a conversation rather than a series of dictations, and every
 * step of it is visible in the transcript so it is never unclear whether the
 * brain is listening, thinking, or waiting.
 *
 * Transcripts are sent without confirmation here, which the push-to-talk button
 * deliberately does not do. That is safe for the same reason it is safe to let
 * the model choose tools at all: anything that changes something still stops for
 * approval. A misheard sentence can waste a reply; it cannot delete a file.
 */
const talking = { on: false, running: false };

// Whether the loop has been started automatically this session. Without it,
// every status refresh — every five seconds — would restart a conversation the
// user had just switched off.
let talkingStartedOnce = false;

function toggleTalking() {
    if (talking.running) {
        // Stops after the turn in flight; cutting a reply off mid-sentence
        // would be worse than a moment's wait.
        talking.on = false;
        setTalkButton('stopping');
        setVoiceStatus('Finishing this turn…');

        return;
    }

    talking.on = true;
    talkLoop();
}

/*
 * The two things the command centre's quick commands need.
 *
 * Exposed here rather than reimplemented there: starting a conversation means
 * forgetting the current one's identity so the next message opens a new one,
 * and only this file knows that.
 */
window.brainCommands = {
    talk: toggleTalking,
    newConversation() {
        state.conversationId = null;

        const transcript = el('transcript');

        if (transcript) transcript.innerHTML = '';

        addMessage(state.brainName, 'New conversation. What would you like to do?',
            { cssClass: 'brain' });
    },
};

function setTalkButton(state) {
    // The reactor shows this too, so the centre of the picture says what the
    // brain is doing without anybody reading the button.
    if (window.brainMapState) window.brainMapState(state);

    const button = el('talk');
    const label = el('talk-label');

    if (!button || !label) return;

    button.dataset.state = state;
    button.setAttribute('aria-pressed', state === 'idle' ? 'false' : 'true');

    label.textContent = {
        idle: 'Talk',
        listening: 'Listening',
        thinking: 'Thinking',
        speaking: 'Speaking',
        stopping: 'Stopping',
    }[state] || 'Talk';
}

async function talkLoop() {
    if (talking.running) return;

    talking.running = true;

    try {
        while (talking.on) {
            setTalkButton('listening');
            setVoiceStatus('Speak when ready');

            let heard;
            try {
                heard = await api.post('/api/turn', { device: el('microphone').value || '' });
            } catch (err) {
                addMessage('error', String(err.message || err), { cssClass: 'error' });
                break;
            }

            if (!talking.on) break;

            const said = (heard.text || '').trim();

            if (!said) {
                // Nothing was said. Say why once and keep listening, rather
                // than filling the transcript with the same notice.
                setVoiceStatus(heard.advice || 'Heard nothing — still listening');
                continue;
            }

            addMessage('you', said);
            setTalkButton('thinking');
            setVoiceStatus('Thinking…');

            let reply;
            try {
                reply = await api.post('/api/chat', {
                    conversation_id: state.conversationId,
                    message: said,
                    provider: el('provider').value || '',
                    spoken: true,
                });
            } catch (err) {
                addMessage('error', String(err.message || err), { cssClass: 'error' });
                break;
            }

            state.conversationId = reply.conversation_id;
            addMessage(state.brainName, reply.reply, { cssClass: 'brain' });

            if (window.brainMapRecall && reply.recalled) window.brainMapRecall(reply.recalled);
        if (window.brainMapGauges) {
            window.brainMapGauges({ recall: Math.min(1, (reply.recalled || []).length / 12) });
        }

            if (!talking.on) break;

            // Written and spoken, always — the transcript is the record and
            // the voice is how you hear it without looking.
            setTalkButton('speaking');
            setVoiceStatus('Speaking…');

            try {
                // wait: true holds until the voice has actually stopped. On
                // speakers the microphone hears the brain, and estimating the
                // duration from the word count — which this replaced — was
                // wrong in both directions: too short and it transcribed
                // itself, too long and every exchange dragged.
                await api.post('/api/speak', { text: reply.reply, wait: true });
            } catch {
                // Not being heard is not a reason to end the conversation.
            }

            // A short settle for the tail of the audio and the room's echo.
            await new Promise((r) => setTimeout(r, ECHO_SETTLE_MS));
        }
    } finally {
        talking.on = false;
        talking.running = false;
        setTalkButton('idle');
        setVoiceStatus('');
    }
}

// Long enough for the tail of the audio and a room's echo to die away, short
// enough not to be felt as a pause.
const ECHO_SETTLE_MS = 350;

function setVoiceStatus(text) {
    const hint = el('voice-status');
    if (hint) hint.textContent = text;
}

let voicesLoaded = false;

/*
 * Which voice reads answers aloud.
 *
 * Loaded once and remembered on the server, so a voice somebody picked is
 * still theirs after a restart. Changing it speaks a sample immediately —
 * choosing a voice from a list of names without hearing any of them is
 * choosing blind.
 */
async function loadVoices() {
    if (voicesLoaded) return;

    let data;
    try {
        data = await api.get('/api/voices');
    } catch {
        return;
    }

    const select = el('voice');
    const voices = data.voices || [];

    select.textContent = '';

    voices.forEach((v) => {
        const option = document.createElement('option');
        option.value = v.id;
        option.textContent = v.name;
        select.appendChild(option);
    });

    if (data.current) select.value = data.current;

    // Which language is being spoken, which whisper needs told. Left to guess
    // it sometimes translates instead of transcribing, so Bulgarian speech
    // comes back as English prose with no error to explain it.
    const language = el('language');
    language.value = data.language || '';
    el('language-field').hidden = false;

    language.onchange = async () => {
        try {
            await api.post('/api/language', { code: language.value });
        } catch (err) {
            addMessage('error', String(err.message || err), { cssClass: 'error' });
        }
    };

    el('voice-field').hidden = voices.length < 2;
    voicesLoaded = true;

    select.onchange = async () => {
        try {
            await api.post('/api/voice', { id: select.value });
            await api.post('/api/speak', { text: 'This is how I sound.' });
        } catch (err) {
            addMessage('error', String(err.message || err), { cssClass: 'error' });
        }
    };
}

let microphonesLoaded = false;

/*
 * Which input to listen through.
 *
 * Offered as a choice rather than taken from the system default, because the
 * default is frequently wrong: on this machine it is the built-in analog jack,
 * which records near-silence while a USB microphone sits unused. That failure
 * is indistinguishable from a broken recogniser.
 */
async function loadMicrophones() {
    if (microphonesLoaded) return;

    let mics = [];
    try {
        mics = (await api.get('/api/microphones')).microphones || [];
    } catch {
        return;
    }

    const select = el('microphone');
    select.textContent = '';

    mics.forEach((m) => {
        const option = document.createElement('option');
        option.value = m.id;
        option.textContent = m.name + (m.default ? ' (system default)' : '');
        select.appendChild(option);
    });

    // Prefer something that is plainly a microphone over the system default,
    // which is often a jack with nothing in it.
    const named = mics.find((m) => /mic/i.test(m.name));
    if (named) select.value = named.id;

    el('microphone-field').hidden = mics.length === 0;
    microphonesLoaded = true;
}


/*
 * Reading a reply aloud.
 *
 * Not a separate preference any more. If you are talking to it, it talks back —
 * having to switch that on separately was a setting nobody wants to think
 * about. Typed messages stay silent, which is what typing means.
 *
 * Failure is deliberately quiet: not hearing an answer that is already on
 * screen is a small thing, and an error box about it would be a larger one.
 */
function speak(text) {
    if (!talking.on || !text) return;

    api.post('/api/speak', { text }).catch(() => {});
}

async function refreshLessons() {
    let pending = [];
    try {
        pending = await api.get('/api/lessons');
    } catch {
        return;
    }

    const panel = el('lessons-panel');
    const list = el('lessons');
    list.textContent = '';
    panel.hidden = pending.length === 0;
    el('lesson-count').textContent = pending.length;

    pending.forEach((lesson) => {
        const card = document.createElement('div');
        card.className = 'approval';

        const content = document.createElement('p');
        // textContent, never innerHTML: a language model wrote this string and
        // it may contain anything at all.
        content.textContent = lesson.content;

        if (lesson.confidence) {
            const badge = document.createElement('span');
            badge.className = 'tool';
            badge.textContent = lesson.confidence;
            content.prepend(badge);
        }

        const actions = document.createElement('div');
        actions.className = 'approval-actions';

        const accept = document.createElement('button');
        accept.className = 'approve';
        accept.textContent = 'Remember';
        accept.onclick = () => decideLesson(lesson.id, 'accept');

        const reject = document.createElement('button');
        reject.className = 'reject';
        reject.textContent = 'Discard';
        reject.onclick = () => decideLesson(lesson.id, 'reject');

        actions.append(accept, reject);
        card.append(content, actions);
        list.appendChild(card);
    });
}

async function decideLesson(id, decision) {
    try {
        const result = await api.post(`/api/lessons/${id}/${decision}`);
        addMessage('system', result.message || 'Done.', { cssClass: 'brain' });
    } catch (err) {
        addMessage('error', String(err.message || err), { cssClass: 'error' });
    }

    refreshLessons();
    refreshDrives();
    refreshStatus();

    // Remembering something adds a node, so the map is no longer current.
    if (window.brainMapReload) window.brainMapReload();
}

/*
 * Where the brain could live.
 *
 * Shown always rather than only when the disk is filling: somebody deciding
 * whether to plug a drive in wants to know what it would gain them before the
 * warning appears, not after.
 */
async function refreshDrives() {
    let data;

    try {
        data = await api.get('/api/drives');
    } catch {
        return;
    }

    const list = el('drives');
    list.textContent = '';

    (data.drives || []).forEach((d) => {
        const row = document.createElement('div');
        row.className = 'drive' + (d.current ? ' current' : '');

        const where = document.createElement('span');
        where.className = 'drive-path';
        where.textContent = d.mount_point;

        const free = document.createElement('span');
        free.className = 'drive-free';
        free.textContent = (d.free_bytes / GB).toFixed(0) + 'GB free';

        row.append(where, free);

        const marks = [];
        if (d.current) marks.push('in use');
        if (d.removable) marks.push('removable');
        if (!d.writable) marks.push('not writable');

        if (marks.length) {
            const note = document.createElement('span');
            note.className = 'drive-note';
            note.textContent = marks.join(' · ');
            row.appendChild(note);
        }

        list.appendChild(row);
    });

    if (data.how && (data.drives || []).length > 1) {
        const how = document.createElement('p');
        how.className = 'storage-hint';
        how.textContent = data.how;
        list.appendChild(how);
    }
}

async function refreshApprovals() {
    let pending = [];
    try {
        pending = await api.get('/api/approvals');
    } catch {
        return;
    }

    const panel = el('approvals-panel');
    const list = el('approvals');
    list.textContent = '';
    panel.hidden = pending.length === 0;
    el('approval-count').textContent = pending.length;

    pending.forEach((item) => {
        const card = document.createElement('div');
        card.className = 'approval';

        const tool = document.createElement('span');
        tool.className = 'tool';
        tool.textContent = item.tool;

        const summary = document.createElement('p');
        // The summary is the whole point of the prompt: it is what a person
        // actually judges, so it is rendered as text and never as markup.
        summary.textContent = item.summary;
        summary.prepend(tool);

        const actions = document.createElement('div');
        actions.className = 'approval-actions';

        const approve = document.createElement('button');
        approve.className = 'approve';
        approve.textContent = 'Approve';
        approve.onclick = () => decide(item.id, 'approve');

        const reject = document.createElement('button');
        reject.className = 'reject';
        reject.textContent = 'Reject';
        reject.onclick = () => decide(item.id, 'reject');

        actions.append(approve, reject);
        card.append(summary, actions);
        list.appendChild(card);
    });
}

async function decide(id, decision) {
    try {
        const result = await api.post(`/api/approvals/${id}/${decision}`);
        const note = decision === 'approve'
            ? (result.error ? `Failed: ${result.error}` : `Done — ${result.result}`)
            : 'Rejected. Nothing was changed.';
        addMessage('system', note, { cssClass: 'brain' });
    } catch (err) {
        addMessage('error', String(err.message || err), { cssClass: 'error' });
    }
    refreshApprovals();
    refreshLessons();
    refreshDrives();
    refreshActivity();
    refreshStatus();
}

const MARKS = { executed: '✓', pending: '•', rejected: '×', failed: '!' };

async function refreshActivity() {
    let items = [];
    try {
        items = await api.get('/api/activity');
    } catch {
        return;
    }

    const box = el('activity');
    box.textContent = '';

    if (!items.length) {
        const empty = document.createElement('div');
        empty.className = 'activity-empty';
        empty.textContent = 'Nothing yet.';
        box.appendChild(empty);
        return;
    }

    /*
     * Each entry is an icon, two lines and a tag.
     *
     * The tag is the entry's own kind and outcome — a tool that ran, a message,
     * something waiting on a decision — not a severity invented to give the
     * column some colour. When everything is ordinary, everything is the same
     * colour, and that is the useful state to be able to recognise.
     */
    const KINDS = {
        message: ['MSG', 'said'],
        tool: ['RUN', 'ran'],
        lesson: ['MEM', 'learned'],
        approval: ['ASK', 'waiting'],
    };

    items.forEach((item) => {
        const row = document.createElement('div');
        row.className = `feed-item ${item.status || ''}`;

        const icon = document.createElement('span');
        icon.className = 'row-icon';
        icon.textContent = (KINDS[item.kind] || ['·'])[0];

        const text = document.createElement('span');
        text.className = 'row-text';

        const title = document.createElement('span');
        title.className = 'row-name';
        title.textContent = item.summary || item.tool || '—';

        const when = document.createElement('span');
        when.className = 'row-state';
        when.textContent = [(KINDS[item.kind] || [null, item.kind])[1], ago(item.when)]
            .filter(Boolean).join(' · ');

        text.append(title, when);

        const tag = document.createElement('span');
        tag.className = `feed-tag ${item.status || ''}`;
        tag.textContent = (item.status || item.kind || '').toUpperCase();

        row.append(icon, text, tag);
        box.appendChild(row);
    });
}

/*
 * How long ago, in words.
 *
 * Rounded on purpose. "Four minutes ago" is what somebody wants from a feed;
 * a timestamp to the second is a thing to decode.
 */
function ago(when) {
    if (!when) return '';

    const seconds = (Date.now() - new Date(when).getTime()) / 1000;

    if (!isFinite(seconds) || seconds < 0) return '';
    if (seconds < 60) return 'just now';
    if (seconds < 3600) return `${Math.round(seconds / 60)}m ago`;
    if (seconds < 86400) return `${Math.round(seconds / 3600)}h ago`;

    return `${Math.round(seconds / 86400)}d ago`;
}

// Returns whether anything was restored, so the caller knows whether a
// greeting would be an opening or an interruption.
async function restoreConversation() {
    try {
        const convo = await api.get('/api/conversations/latest');
        if (!convo.id || !convo.messages.length) return false;

        state.conversationId = convo.id;
        convo.messages.forEach((m) => {
            addMessage(
                m.role === 'user' ? 'you' : state.brainName,
                m.content,
                { cssClass: m.role === 'user' ? 'you' : 'brain' }
            );
        });

        return true;
    } catch {
        /* An empty console is a fine fallback; don't block startup on history. */
        return false;
    }
}

/* ---------- wiring ---------- */

el('composer').addEventListener('submit', (e) => {
    e.preventDefault();
    const input = el('input');
    const text = input.value;
    input.value = '';
    input.style.height = 'auto';
    send(text);
});

// Enter sends, Shift+Enter breaks the line — the convention everywhere else.
el('input').addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        el('composer').requestSubmit();
    }
});

el('input').addEventListener('input', (e) => {
    e.target.style.height = 'auto';
    e.target.style.height = `${Math.min(e.target.scrollHeight, 190)}px`;
});

/*
 * What the brain says on opening.
 *
 * Shown once, and spoken when it can speak. Composed on the server from what
 * it already knows rather than generated by the model, so it arrives
 * immediately — a greeting that takes a minute to appear is not a greeting.
 *
 * Skipped when there is a conversation to restore: reintroducing itself on top
 * of a transcript somebody is already reading would be noise.
 */
async function greet(canSpeak) {
    let greeting;

    try {
        greeting = await api.get('/api/greeting');
    } catch {
        return;
    }

    if (!greeting.text) return;

    addMessage(state.brainName, greeting.text, { cssClass: 'brain' });

    if (canSpeak) api.post('/api/speak', { text: greeting.text }).catch(() => {});
}

(async function start() {
    await refreshStatus();
    const restored = await restoreConversation();

    if (!restored) {
        let canSpeak = false;

        try {
            canSpeak = (await api.get('/api/status')).capabilities.includes('speech');
        } catch {
            // A greeting nobody hears is still worth showing.
        }

        await greet(canSpeak);
    }

    refreshApprovals();
    refreshLessons();
    refreshDrives();
    refreshActivity();
    el('talk').onclick = toggleTalking;
    el('input').focus();

    // Polling rather than websockets: approvals can be decided from the Filament
    // admin or another window, and a few seconds of staleness costs nothing here.
    setInterval(() => {
        refreshStatus();
        refreshApprovals();
        refreshLessons();
        refreshDrives();
        refreshActivity();
    }, 5000);
})();
