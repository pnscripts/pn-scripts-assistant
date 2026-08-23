<!doctype html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <title>Brain — smoke test</title>
    <style>
        :root { color-scheme: dark; }
        body {
            font-family: system-ui, sans-serif;
            max-width: 720px;
            margin: 48px auto;
            padding: 0 16px;
            background: #0b0e14;
            color: #d7dce2;
        }
        h1 { font-size: 20px; letter-spacing: 0.02em; color: #7fd4ff; margin-bottom: 2px; }
        h1 .dot { color: #3ddc84; }
        p.note { color: #6b7280; font-size: 13px; margin-top: 0; }
        #log {
            border: 1px solid #1f2733;
            background: #0f141d;
            border-radius: 10px;
            padding: 14px;
            min-height: 320px;
            margin-bottom: 12px;
        }
        .msg { margin-bottom: 12px; line-height: 1.4; }
        .msg .role { font-weight: 600; }
        .msg.you .role { color: #d7dce2; }
        .msg.brain .role { color: #7fd4ff; }
        .msg.error .role { color: #ff6b6b; }
        .msg .meta { color: #4b5563; font-size: 11px; margin-left: 6px; }
        form { display: flex; gap: 8px; }
        input[type=text] {
            flex: 1; padding: 10px; border-radius: 6px; border: 1px solid #1f2733;
            background: #0f141d; color: #d7dce2;
        }
        select, button {
            padding: 10px; border-radius: 6px; border: 1px solid #1f2733;
            background: #151b26; color: #d7dce2;
        }
        button { cursor: pointer; }
        button:hover { border-color: #7fd4ff; }
    </style>
</head>
<body>
    <h1 id="title"><span class="dot">●</span> Brain</h1>
    <p class="note">Temporary smoke-test page for Phase 1 — the real UI lands in Phase 3.</p>
    <div id="log"></div>
    <form id="form">
        <select id="provider">
            <option value="">default</option>
            <option value="ollama">ollama (local)</option>
            <option value="anthropic">anthropic (API)</option>
        </select>
        <input type="text" id="message" placeholder="Say something..." autocomplete="off" required>
        <button type="submit">Send</button>
    </form>

    <script>
        let conversationId = null;
        let brainName = 'brain';
        const log = document.getElementById('log');
        const title = document.getElementById('title');
        const form = document.getElementById('form');
        const messageInput = document.getElementById('message');
        const providerSelect = document.getElementById('provider');

        function append(cls, role, content, meta = '') {
            const div = document.createElement('div');
            div.className = `msg ${cls}`;
            div.innerHTML = `<span class="role">${role}:</span> ${content} <span class="meta">${meta}</span>`;
            log.appendChild(div);
            log.scrollTop = log.scrollHeight;
        }

        fetch('/api/brain').then(r => r.json()).then(info => {
            brainName = info.name || 'brain';
            title.innerHTML = `<span class="dot">●</span> ${brainName}`;
            document.title = `${brainName} — smoke test`;
        }).catch(() => {});

        form.addEventListener('submit', async (e) => {
            e.preventDefault();
            const message = messageInput.value.trim();
            if (!message) return;
            append('you', 'you', message);
            messageInput.value = '';

            const res = await fetch('/api/chat', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', 'Accept': 'application/json' },
                body: JSON.stringify({
                    conversation_id: conversationId,
                    message,
                    provider: providerSelect.value || null,
                }),
            });

            if (!res.ok) {
                append('error', 'error', `Request failed (${res.status})`);
                return;
            }

            const data = await res.json();
            conversationId = data.conversation_id;
            append('brain', brainName, data.reply, `${data.provider} / ${data.model}`);
        });
    </script>
</body>
</html>
