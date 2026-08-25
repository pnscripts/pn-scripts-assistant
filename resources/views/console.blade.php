<!doctype html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>{{ config('brain.name') }}</title>
    <link rel="stylesheet" href="{{ asset('css/console.css') }}">
</head>
<body>
<div class="shell">

    <aside class="rail">
        <div class="identity">
            <div class="orb" id="orb" aria-hidden="true"><span></span><span></span><span></span></div>
            <div class="identity-text">
                <h1 id="brain-name">{{ config('brain.name') }}</h1>
                <p class="status-line"><span class="dot" id="conn-dot"></span><span id="conn-text">connecting…</span></p>
            </div>
        </div>

        <section class="panel">
            <h2>Memory</h2>
            <div class="stat"><span class="stat-value" id="stat-facts">—</span><span class="stat-label">facts known</span></div>
            <div class="stat"><span class="stat-value" id="stat-pending">—</span><span class="stat-label">awaiting review</span></div>
            <div class="stat"><span class="stat-value" id="stat-convos">—</span><span class="stat-label">conversations</span></div>
        </section>

        <section class="panel" id="storage-panel">
            <h2>Storage</h2>
            <div class="storage-bar"><div id="storage-fill"></div></div>
            <p class="storage-text" id="storage-text">—</p>
            <div id="storage-warning" hidden>
                <p class="storage-advice" id="storage-advice"></p>
                <p class="storage-hint">Run <code>scripts/extend-storage.sh</code> to move the brain to another drive.</p>
            </div>
        </section>

        <section class="panel" id="approvals-panel" hidden>
            <h2>Needs you <span class="badge" id="approval-count">0</span></h2>
            <div id="approvals"></div>
        </section>

        <section class="panel grow">
            <h2>Activity</h2>
            <div id="activity" class="activity"></div>
        </section>

        <section class="panel">
            <h2>Engine</h2>
            <label class="field">
                <span>Provider</span>
                <select id="provider">
                    <option value="">default</option>
                    <option value="ollama">ollama · local</option>
                    <option value="anthropic">anthropic · api</option>
                </select>
            </label>
            <p class="engine-meta" id="engine-meta">—</p>
        </section>
    </aside>

    <main class="main">
        <div class="transcript" id="transcript">
            <div class="boot" id="boot">
                <p class="boot-title">{{ config('brain.name') }}</p>
                <p class="boot-sub">Ask something, or tell me to look at something.</p>
            </div>
        </div>

        <form class="composer" id="composer" autocomplete="off">
            <textarea id="input" rows="1" placeholder="Say something…" aria-label="Message"></textarea>
            <button type="submit" id="send" aria-label="Send">
                <svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true">
                    <path d="M3 12l18-9-9 18-2-7-7-2z" fill="currentColor"/>
                </svg>
            </button>
        </form>
        <p class="hint">Reads run on their own. Anything that changes something waits for you.</p>
    </main>
</div>

<script src="{{ asset('js/console.js') }}"></script>
</body>
</html>
