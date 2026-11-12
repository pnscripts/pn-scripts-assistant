/*
 * Looking after this computer, on the System page (server/machinecare.go):
 * whether a restart is fine now and what stands in the way, what was left
 * running whose job is over — each with a Stop that asks once more on the
 * page — and how the last session ended when it ended badly.
 * Drawn with textContent: commands and folder names come from the machine.
 */
(function () {
    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
    const bg = () => document.documentElement.lang === 'bg';

    let data = null;

    function when(iso) {
        const d = new Date(iso);
        if (Number.isNaN(d.getTime())) return '';

        const time = d.toLocaleTimeString(bg() ? 'bg-BG' : 'en-GB', { hour: '2-digit', minute: '2-digit' });
        const day = `${d.getDate()} ${d.toLocaleDateString(bg() ? 'bg-BG' : 'en-GB', { month: 'long' })}`;

        return T('mcare.at', { time, day });
    }

    function say(text, bad) {
        el('mcare-said').textContent = text || '';
        el('mcare-said').classList.toggle('is-bad', !!bad);
    }

    function draw() {
        if (!data) return;

        const last = el('mcare-last');
        last.hidden = !(data.last_stop && data.last_stop.unclean);
        if (!last.hidden) last.textContent = T('mcare.unclean', { when: when(data.last_stop.last_seen) });

        const r = data.restart || { known: false, ready: false, items: [] };
        const verdict = el('mcare-verdict');
        verdict.textContent = T(!r.known ? 'mcare.unknown' : (r.ready ? 'mcare.ready' : 'mcare.notReady'));
        verdict.classList.toggle('is-bad', r.known && !r.ready);

        drawWatch(r);

        el('mcare-items').replaceChildren(...(r.items || []).map((item) => {
            const li = document.createElement('li');
            li.className = item.blocking ? 'care-blocking' : '';

            const text = T(`mcare.r.${item.key}`, item.args || {});
            li.textContent = text === `mcare.r.${item.key}` ? item.say : text;

            return li;
        }));

        const left = data.leftovers || [];

        el('mcare-left').replaceChildren(...left.map((l) => {
            const li = document.createElement('li');
            li.className = 'writing-item study-item care-leftover';

            const main = document.createElement('div');
            main.className = 'study-item-main';

            const strong = document.createElement('strong');
            strong.textContent = `${T(`mcare.k.${l.kind.replace(/ /g, '_')}`)} · ${l.pid}`;

            const command = document.createElement('code');
            command.className = 'care-command';
            command.textContent = l.command;

            const why = document.createElement('small');
            why.className = 'study-stats';
            why.textContent = T(l.ours ? 'mcare.why.ours' : 'mcare.why.orphan', { age: l.age });

            main.append(strong, command, why);

            const stop = document.createElement('button');
            stop.type = 'button';
            stop.className = 'model-action';
            stop.textContent = T('mcare.stop');
            stop.addEventListener('click', async () => {
                // Asked once more here, on the page: never a dialog.
                if (!stop.dataset.sure) {
                    stop.dataset.sure = '1';
                    stop.textContent = T('mcare.stopSure');

                    return;
                }

                stop.disabled = true;

                try {
                    const res = await fetch('/api/machine/care/stop', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
                        body: JSON.stringify({ pid: l.pid, command: l.command }),
                    });
                    const out = await res.json().catch(() => ({}));

                    if (!res.ok) throw new Error(out.error || `${res.status}`);

                    say(T('mcare.stopped', { pid: l.pid }));
                    await load(true);
                } catch (err) {
                    stop.disabled = false;
                    say(T('writing.failed', { why: err.message }), true);
                }
            });

            li.append(main, stop);

            return li;
        }));

        el('mcare-none').hidden = left.length > 0;
        el('mcare-none').textContent = T(data.here ? 'mcare.none' : 'mcare.notHere');
    }

    /*
     * Tell me when I can restart. Offered while a restart is not safe and
     * nothing is watching; while one is, it says until when and can be
     * stopped; once it has ended, how.
     */
    function drawWatch(r) {
        const w = data.watch;
        const going = !!(w && w.state === 'watching');
        const line = el('mcare-watching');

        el('mcare-watch').hidden = going || !r.known || r.ready;
        el('mcare-unwatch').hidden = !going;

        if (going) {
            line.textContent = T('mcare.watching', { since: clock(w.since), until: clock(w.until) });
        } else if (w && w.state) {
            line.textContent = T(`mcare.w.${w.state}`, { at: clock(w.ended) });
        }

        line.hidden = !(w && w.state);

        // While it watches, the line follows it: one small question a
        // minute, and none while the page is not being looked at.
        clearTimeout(follow);
        if (going) follow = setTimeout(followWatch, 60000);
    }

    let follow = 0;

    async function followWatch() {
        if (document.hidden || !el('mcare-card').offsetParent) {
            follow = setTimeout(followWatch, 60000);

            return;
        }

        try {
            const res = await fetch('/api/machine/care/watch', { headers: { Accept: 'application/json' } });
            const out = await res.json().catch(() => ({}));

            if (!res.ok) throw new Error(out.error || `${res.status}`);

            // Ended: the whole card is read again, since what stood in the
            // way has changed too.
            if (out.watch && out.watch.state !== 'watching') {
                await load(true);

                return;
            }

            data.watch = out.watch;
            draw();
        } catch {
            follow = setTimeout(followWatch, 60000);
        }
    }

    function clock(iso) {
        const d = new Date(iso);
        if (Number.isNaN(d.getTime())) return '';

        return d.toLocaleTimeString(bg() ? 'bg-BG' : 'en-GB', { hour: '2-digit', minute: '2-digit' });
    }

    async function watch(stop) {
        const button = el(stop ? 'mcare-unwatch' : 'mcare-watch');
        button.disabled = true;

        try {
            const res = await fetch('/api/machine/care/watch', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
                body: JSON.stringify(stop ? { stop: true } : { hours: 0 }),
            });
            const out = await res.json().catch(() => ({}));

            if (!res.ok) throw new Error(out.error || `${res.status}`);

            data.watch = out.watch;
            draw();
            say('');
        } catch (err) {
            say(T('writing.failed', { why: err.message }), true);
        } finally {
            button.disabled = false;
        }
    }

    async function load(quiet) {
        el('mcare-look').disabled = true;

        try {
            const res = await fetch('/api/machine/care', { headers: { Accept: 'application/json' } });
            const out = await res.json().catch(() => ({}));

            if (!res.ok) throw new Error(out.error || `${res.status}`);

            data = out;
            draw();
            if (!quiet) say('');
        } catch (err) {
            say(T('writing.failed', { why: err.message }), true);
        } finally {
            el('mcare-look').disabled = false;
        }
    }

    function wire() {
        if (!el('mcare-card')) return;

        el('mcare-look').addEventListener('click', () => load());
        el('mcare-watch').addEventListener('click', () => watch(false));
        el('mcare-unwatch').addEventListener('click', () => watch(true));

        for (const item of document.querySelectorAll('[data-view="system"]')) {
            if (item.classList.contains('nav-item') || item.classList.contains('icon-button')) {
                item.addEventListener('click', () => load());
            }
        }

        window.addEventListener('pn-language', draw);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
}());
