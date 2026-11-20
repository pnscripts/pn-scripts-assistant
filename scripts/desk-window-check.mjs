/*
 * desk-window-check.mjs: the assistant's own window takes its key, proved in
 * the real window on Windows (ci.yml, "The Windows window takes its key").
 *
 *   node scripts/desk-window-check.mjs <debugging port> <assistant address>
 *
 * The program opens its window at a one-time address (server/desk.go); the
 * page there swaps the code for the window's key, keeps it in the tab's
 * session storage and goes on to the first page; whatever the page posts
 * afterwards carries the key (desk.js). Everything else on this computer is
 * refused. Unit tests prove the scripts in node and the guard in Go; only a
 * real WebView2 proves that its window does this, so this script reaches the
 * window the way a test tool reaches a browser (the DevTools protocol, which
 * WebView2 opens when WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS asks for a
 * debugging port: set by the CI step for this run, never by the program),
 * and does what a person does: opens "Programs" and ticks a switch with the
 * mouse. Then, as a program here: the same post without the key is refused.
 *
 * The key itself is never printed. Needs Node 22 or newer (WebSocket).
 * Exit 0 and the line "desk window check OK" when all of it held.
 */

const [port, address] = process.argv.slice(2);

if (!port || !address) {
    console.error('usage: node desk-window-check.mjs <debugging port> <assistant address, like 127.0.0.1:8895>');
    process.exit(2);
}

const base = `http://${address}`;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function fail(why) {
    console.log(`desk window check FAILED: ${why}`);
    process.exit(1);
}

// The window's page, once it has left the one-time address.
async function findThePage() {
    const until = Date.now() + 90_000;
    let seen = [];
    let answered = false;

    while (Date.now() < until) {
        try {
            const res = await fetch(`http://127.0.0.1:${port}/json/list`, { signal: AbortSignal.timeout(5000) });
            seen = await res.json();
            answered = true;

            const page = seen.find((t) => t.type === 'page' && typeof t.url === 'string'
                && t.url.startsWith(base + '/') && !t.url.includes('/desk/'));

            if (page && page.webSocketDebuggerUrl) return page;
        } catch {
            /* the window is still opening */
        }

        await sleep(1000);
    }

    // Never the one-time address itself: its code may still be unused.
    const said = seen.map((t) => (String(t.url).includes('/desk/') ? `${base}/desk/(code not shown)` : t.url));
    fail(answered
        ? `no window showed ${base}/ within 90 s (pages: ${said.join(', ') || 'none'})`
        : `nothing answered on the debugging port ${port} within 90 s: the web view was not started with it`);
}

function connect(url) {
    return new Promise((resolve, reject) => {
        const ws = new WebSocket(url);
        let next = 0;
        const waiting = new Map();

        ws.onmessage = (event) => {
            const msg = JSON.parse(String(event.data));
            const w = waiting.get(msg.id);

            if (!w) return;
            waiting.delete(msg.id);
            msg.error ? w.reject(new Error(msg.error.message)) : w.resolve(msg.result);
        };
        ws.onerror = () => reject(new Error('the debugging connection failed'));
        ws.onopen = () => resolve({
            send(method, params = {}) {
                const id = ++next;
                ws.send(JSON.stringify({ id, method, params }));

                return new Promise((res, rej) => {
                    waiting.set(id, { resolve: res, reject: rej });
                    setTimeout(() => {
                        if (waiting.delete(id)) rej(new Error(`${method} got no answer`));
                    }, 30_000);
                });
            },
            close: () => ws.close(),
        });
    });
}

const page = await findThePage();
const cdp = await connect(page.webSocketDebuggerUrl);

async function evaluate(expression) {
    const out = await cdp.send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true });

    if (out.exceptionDetails) throw new Error(out.exceptionDetails.text || 'the page threw');

    return out.result.value;
}

async function waitFor(what, expression, seconds) {
    const until = Date.now() + seconds * 1000;

    while (Date.now() < until) {
        try {
            const value = await evaluate(expression);
            if (value) return value;
        } catch {
            /* the page is loading */
        }

        await sleep(500);
    }

    fail(`${what} within ${seconds} s`);
}

// 1. The window holds a key for this tab and is on the first page.
await waitFor('the page did not load', `document.readyState === 'complete' && !!window.pnDesk`, 60);

const held = await evaluate(`({ here: window.pnDesk.here(), path: location.pathname,
    kept: (sessionStorage.getItem('pn-desk') || '').length })`);

if (!held.here || held.kept < 32 || held.path !== '/') {
    fail(`the window did not take its key (holds one: ${held.here}, ${held.kept} characters, on ${held.path})`);
}

console.log(`the window holds its key (${held.kept} characters, not shown) and left the one-time address for ${held.path}`);

// 2. A program on this computer: the switch, posted without the key, is refused.
const switchState = async () => {
    const res = await fetch(`${base}/api/outside`, { signal: AbortSignal.timeout(10_000) });
    const body = await res.json();
    const all = (body && body.data ? body.data : body).subjects || [];
    const row = all.find((s) => s.subject === 'programs' || s.id === 'programs' || s.name === 'programs');

    return row ? Boolean(row.allowed) : null;
};

if (await switchState() !== false) fail('the switch "which programs are installed" should start off');

const program = await fetch(`${base}/api/outside/allow`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ subject: 'programs', allowed: true }),
    signal: AbortSignal.timeout(10_000),
});
const refusal = await program.json().catch(() => ({}));

// Refused for the key, not for something else on the way (an Origin the
// guard did not like would be a 403 too, and would prove nothing here).
if (program.status !== 403 || !refusal || refusal.desk !== 'not the window' || await switchState() !== false) {
    fail(`a program without the key was not refused for the key (answer ${program.status}, ${JSON.stringify(refusal)})`);
}

console.log('a program on this computer without the key was refused for not being the window (403) and the switch stayed off');

// 3. A person in the window: opens Programs and ticks the switch with the mouse.
await evaluate(`(() => {
    const open = [...document.querySelectorAll('button[data-view="programs"]')];
    if (open.length === 0) throw new Error('no way to the Programs view');
    open[0].click();
    return true;
})()`);

await waitFor('the switch did not appear in the Programs view', `!!document.getElementById('outside-programs')`, 60);

const box = await evaluate(`(() => {
    const b = document.getElementById('outside-programs');
    b.scrollIntoView({ block: 'center' });
    const r = b.getBoundingClientRect();
    return { x: r.left + r.width / 2, y: r.top + r.height / 2, w: r.width, h: r.height, checked: b.checked };
})()`);

if (box.w <= 0 || box.h <= 0) fail('the switch is not on the screen to be ticked');
if (box.checked) fail('the switch was already ticked');

await sleep(500);

for (const type of ['mouseMoved', 'mousePressed', 'mouseReleased']) {
    await cdp.send('Input.dispatchMouseEvent', { type, x: box.x, y: box.y, button: 'left', clickCount: type === 'mouseMoved' ? 0 : 1 });
}

const until = Date.now() + 30_000;
while (Date.now() < until && await switchState() !== true) await sleep(500);

if (await switchState() !== true) {
    const said = await evaluate(`(document.getElementById('outside-note') || {}).textContent || ''`);
    fail(`the switch ticked in the window was not kept; the page says: ${said || 'nothing'}`);
}

const shown = await waitFor('the page did not show the switch on',
    `document.getElementById('outside-programs') && document.getElementById('outside-programs').checked`, 15);

console.log(`ticked with the mouse in the window: the assistant keeps it (${shown ? 'shown ticked' : ''})`);

cdp.close();
console.log('desk window check OK: the window took its key, a program without it was refused');
process.exit(0);
