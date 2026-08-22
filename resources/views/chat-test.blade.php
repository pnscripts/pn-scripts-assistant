<!doctype html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <title>AI Brain — smoke test</title>
    <style>
        body { font-family: system-ui, sans-serif; max-width: 720px; margin: 40px auto; padding: 0 16px; }
        h1 { font-size: 18px; }
        p.note { color: #666; font-size: 13px; }
        #log { border: 1px solid #ddd; border-radius: 8px; padding: 12px; min-height: 300px; margin-bottom: 12px; }
        .msg { margin-bottom: 10px; }
        .msg .role { font-weight: 600; }
        .msg .meta { color: #888; font-size: 11px; margin-left: 6px; }
        form { display: flex; gap: 8px; }
        input[type=text] { flex: 1; padding: 8px; }
        select, button { padding: 8px; }
    </style>
</head>
<body>
    <h1>AI Brain — smoke test</h1>
    <p class="note">Temporary test page for Phase 1. The real Jarvis-style UI lands in Phase 3.</p>
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
        const log = document.getElementById('log');
        const form = document.getElementById('form');
        const messageInput = document.getElementById('message');
        const providerSelect = document.getElementById('provider');

        function append(role, content, meta = '') {
            const div = document.createElement('div');
            div.className = 'msg';
            div.innerHTML = `<span class="role">${role}:</span> ${content} <span class="meta">${meta}</span>`;
            log.appendChild(div);
            log.scrollTop = log.scrollHeight;
        }

        form.addEventListener('submit', async (e) => {
            e.preventDefault();
            const message = messageInput.value.trim();
            if (!message) return;
            append('you', message);
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
                append('error', `Request failed (${res.status})`);
                return;
            }

            const data = await res.json();
            conversationId = data.conversation_id;
            append('brain', data.reply, `${data.provider} / ${data.model}`);
        });
    </script>
</body>
</html>
