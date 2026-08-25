/*
 * Pnexus console.
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
    brainName: 'Pnexus',
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
        el('conn-dot').className = 'dot online';
        el('conn-text').textContent = `online · ${s.capabilities.length} capabilities`;
        document.title = s.name;
    } catch {
        el('conn-dot').className = 'dot offline';
        el('conn-text').textContent = 'offline';
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
    refreshActivity();
    el('input').focus();

    // Polling rather than websockets: approvals can be decided from the Filament
    // admin or another window, and a few seconds of staleness costs nothing here.
    setInterval(() => {
        refreshStatus();
        refreshApprovals();
        refreshActivity();
    }, 5000);
})();
