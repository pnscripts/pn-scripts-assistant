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
        el('input').focus();
    }
}

/* ---------- panels ---------- */

async function refreshStatus() {
    try {
        const s = await api.get('/api/status');
        state.brainName = s.name || state.brainName;
        el('brain-name').textContent = s.name;
        el('stat-facts').textContent = s.memory.facts;
        el('stat-pending').textContent = s.memory.pending_lessons;
        el('stat-convos').textContent = s.memory.conversations;
        el('engine-meta').textContent = `${s.provider} · ${s.model}`;
        renderPrivacy(s.privacy);
        renderStorage(s.storage);
        el('conn-dot').className = 'dot online';
        el('conn-text').textContent = `online · ${s.capabilities.length} capabilities`;
        // Only offer to speak when there is something that can. A control for
        // a capability the machine lacks is a promise the app cannot keep.
        el('speak-field').hidden = !s.capabilities.includes('speech');
        document.title = s.name;
    } catch {
        el('conn-dot').className = 'dot offline';
        el('conn-text').textContent = 'offline';
    }
}

function renderPrivacy(privacy) {
    if (!privacy) return;

    const summary = el('privacy-summary');
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
function speak(text) {
    if (!el('speak-toggle') || !el('speak-toggle').checked || !text) return;

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

    items.forEach((item) => {
        const row = document.createElement('div');
        row.className = `activity-item ${item.status}`;

        const mark = document.createElement('span');
        mark.className = 'mark';
        mark.textContent = MARKS[item.status] || '·';

        const text = document.createElement('span');
        text.textContent = item.summary || item.tool;

        row.append(mark, text);
        box.appendChild(row);
    });
}

async function restoreConversation() {
    try {
        const convo = await api.get('/api/conversations/latest');
        if (!convo.id || !convo.messages.length) return;

        state.conversationId = convo.id;
        convo.messages.forEach((m) => {
            addMessage(
                m.role === 'user' ? 'you' : state.brainName,
                m.content,
                { cssClass: m.role === 'user' ? 'you' : 'brain' }
            );
        });
    } catch {
        /* An empty console is a fine fallback; don't block startup on history. */
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

(async function start() {
    await refreshStatus();
    await restoreConversation();
    refreshApprovals();
    refreshLessons();
    refreshDrives();
    refreshActivity();
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
