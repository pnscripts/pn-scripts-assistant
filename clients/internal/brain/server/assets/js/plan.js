/*
 * Plan: what is next, the to-do lists, and the online calendar.
 *
 * What is next is the diary, the online calendar, the reminders and the
 * to-dos that are due, in one time order — drawn as the server gathered it
 * (tools.Planner.Ahead). Ticking an item off is one click and undone the
 * same way; removing one asks a second press.
 */
(function () {
    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
    const bg = () => window.pnLang && window.pnLang() === 'bg';
    const locale = () => (bg() ? 'bg-BG' : 'en-GB');

    let lastPlan = null;
    let lastTodos = null;

    async function get(path) {
        const res = await fetch(path, { headers: { Accept: 'application/json' } });
        const body = await res.json().catch(() => ({}));

        if (!res.ok) throw new Error(body.error || `${res.status}`);

        return body.data !== undefined ? body.data : body;
    }

    async function send(path, body) {
        const res = await fetch(path, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
            body: JSON.stringify(body || {}),
        });
        const data = await res.json().catch(() => ({}));

        if (!res.ok) throw new Error(data.error || `${res.status}`);

        return data.data !== undefined ? data.data : data;
    }

    function say(id, text, bad) {
        const p = el(id);
        if (!p) return;

        p.textContent = text || '';
        p.classList.toggle('is-bad', !!bad);
    }

    const dayStart = (d) => new Date(d.getFullYear(), d.getMonth(), d.getDate());

    function dayTitle(d) {
        const today = dayStart(new Date());
        const diff = Math.round((dayStart(d) - today) / 86400000);
        const date = d.toLocaleDateString(locale(), { weekday: 'long', day: 'numeric', month: 'long' });

        if (diff === 0) return `${T('plan.today')} · ${date}`;
        if (diff === 1) return `${T('plan.tomorrow')} · ${date}`;

        return date;
    }

    const clock = (d) => d.toLocaleTimeString(locale(), { hour: '2-digit', minute: '2-digit' });

    function dueWords(iso) {
        if (!iso) return '';

        const d = new Date(iso);
        const diff = Math.round((dayStart(d) - dayStart(new Date())) / 86400000);

        if (diff < 0) return T('todo.overdue', { date: d.toLocaleDateString(locale(), { day: 'numeric', month: 'short' }) });
        if (diff === 0) return T('todo.dueToday');
        if (diff === 1) return T('todo.dueTomorrow');

        return T('todo.dueOn', { date: d.toLocaleDateString(locale(), { weekday: 'short', day: 'numeric', month: 'short' }) });
    }

    function listName(name) {
        return name === 'to do' ? T('todo.general') : name;
    }

    function chip(kind) {
        const span = document.createElement('span');
        span.className = `plan-chip plan-${kind}`;
        span.textContent = T(`plan.kind.${kind}`);

        return span;
    }

    function drawPlan(data) {
        lastPlan = data;

        const box = el('plan-agenda');
        box.replaceChildren();

        const a = data.agenda;

        if (a.overdue.length) {
            const h = document.createElement('h3');
            h.className = 'tr-sub plan-overdue';
            h.textContent = `${T('plan.overdue')} · ${a.overdue.length}`;

            const ul = document.createElement('ul');
            ul.className = 'writing-list';

            for (const item of a.overdue) ul.append(todoRow(item, true));

            box.append(h, ul);
        }

        let current = '';
        let ul = null;

        for (const c of a.coming) {
            const at = new Date(c.at);
            const key = dayStart(at).toISOString();

            if (key !== current) {
                current = key;

                const h = document.createElement('h3');
                h.className = 'tr-sub';
                h.textContent = dayTitle(at);

                ul = document.createElement('ul');
                ul.className = 'writing-list';

                box.append(h, ul);
            }

            const li = document.createElement('li');
            li.className = `writing-item plan-item is-${c.kind}`;

            const when = document.createElement('span');
            when.className = 'plan-when';
            when.textContent = c.kind === 'todo' || c.all_day ? T('plan.allDay')
                : c.kind === 'reminder' ? clock(at) : `${clock(at)}–${clock(new Date(c.ends))}`;

            const what = document.createElement('span');
            what.className = 'plan-what';
            what.textContent = c.what;

            if (c.place && c.kind !== 'todo') {
                const place = document.createElement('span');
                place.className = 'writing-why';
                place.textContent = ` · ${c.place}`;
                what.append(place);
            }

            li.append(when, what, chip(c.kind));
            ul.append(li);
        }

        if (!a.coming.length && !a.overdue.length) {
            const p = document.createElement('p');
            p.className = 'activity-empty';
            p.textContent = T('plan.nothing', { days: data.days });
            box.append(p);
        }

        if (a.undated.length) {
            const h = document.createElement('h3');
            h.className = 'tr-sub';
            h.textContent = `${T('plan.undated')} · ${a.undated.length}`;

            const p = document.createElement('p');
            p.className = 'note';
            p.textContent = a.undated.slice(0, 8).map((t) => t.text).join(' · ')
                + (a.undated.length > 8 ? ` · ${T('plan.more', { n: a.undated.length - 8 })}` : '');

            box.append(h, p);
        }

        const missing = (a.missing || []).map((m) => T('plan.missing', { what: m })).join(' ');
        say('plan-said', missing, !!missing);

        drawCalendar(data.calendar);
    }

    async function loadPlan() {
        try {
            drawPlan(await get(`/api/plan?days=${encodeURIComponent(el('plan-days').value)}`));
        } catch (err) {
            say('plan-said', T('writing.failed', { why: err.message }), true);
        }
    }

    function todoRow(item, compact) {
        const li = document.createElement('li');
        li.className = 'writing-item todo-item' + (item.done_at ? ' is-done' : '') + (compact ? ' is-warn' : '');

        const box = document.createElement('input');
        box.type = 'checkbox';
        box.checked = !!item.done_at;
        box.setAttribute('aria-label', T(item.done_at ? 'todo.undo' : 'todo.tick', { what: item.text }));
        box.addEventListener('change', async () => {
            box.disabled = true;

            try {
                await send(`/api/todos/${item.id}/tick`, { done: box.checked });
                await Promise.all([loadTodos(), loadPlan()]);
            } catch (err) {
                box.checked = !box.checked;
                say('todo-said', T('writing.failed', { why: err.message }), true);
            } finally {
                box.disabled = false;
            }
        });

        const text = document.createElement('span');
        text.className = 'todo-text-cell';

        const what = document.createElement('span');
        what.className = 'plan-what';
        what.textContent = item.text;

        const meta = document.createElement('span');
        meta.className = 'writing-why';
        meta.textContent = [listName(item.list), dueWords(item.due)].filter(Boolean).join(' · ');

        text.append(what, meta);
        li.append(box, text);

        if (!compact) {
            const remove = document.createElement('button');
            remove.type = 'button';
            remove.className = 'model-action';
            remove.textContent = T('gl.remove');
            remove.setAttribute('aria-label', T('todo.removeOne', { what: item.text }));
            remove.addEventListener('click', async () => {
                if (!remove.classList.contains('danger')) {
                    remove.classList.add('danger');
                    remove.textContent = T('gl.sure');
                    setTimeout(() => { remove.classList.remove('danger'); remove.textContent = T('gl.remove'); }, 4000);
                    return;
                }

                try {
                    await send(`/api/todos/${item.id}/remove`);
                    await Promise.all([loadTodos(), loadPlan()]);
                } catch (err) {
                    say('todo-said', T('writing.failed', { why: err.message }), true);
                }
            });

            li.append(remove);
        }

        return li;
    }

    function drawTodos(data) {
        lastTodos = data;

        const filter = el('todo-filter');
        const chosen = filter.value;
        const names = Object.keys(data.lists || {}).sort();

        [...filter.options].forEach((o, i) => { if (i > 0) o.remove(); });

        for (const name of names) {
            const o = document.createElement('option');
            o.value = name;
            o.textContent = `${listName(name)} (${data.lists[name]})`;
            filter.append(o);
        }

        filter.value = names.includes(chosen) ? chosen : '';

        const datalist = el('todo-lists');
        datalist.replaceChildren(...names.map((n) => Object.assign(document.createElement('option'), { value: n })));

        const only = filter.value;
        const open = data.items.filter((t) => !t.done_at && (!only || t.list === only));
        const done = data.items.filter((t) => t.done_at && (!only || t.list === only));

        el('todo-items').replaceChildren(...open.map((t) => todoRow(t, false)));
        el('todo-empty').hidden = open.length > 0;
        el('todo-done').replaceChildren(...done.slice(0, 30).map((t) => todoRow(t, false)));
        el('todo-done-box').hidden = done.length === 0;
    }

    async function loadTodos() {
        try {
            drawTodos(await get('/api/todos'));
        } catch (err) {
            say('todo-said', T('writing.failed', { why: err.message }), true);
        }
    }

    async function addTodo() {
        const text = el('todo-text').value.trim();

        if (!text) {
            say('todo-said', T('todo.needText'), true);
            el('todo-text').focus();
            return;
        }

        try {
            const item = await send('/api/todos', { text, list: el('todo-list').value, due: el('todo-due').value });

            say('todo-said', T('todo.added', { what: item.text, list: listName(item.list) }));
            el('todo-text').value = '';
            el('todo-due').value = '';
            el('todo-text').focus();
            await Promise.all([loadTodos(), loadPlan()]);
        } catch (err) {
            say('todo-said', T('writing.failed', { why: err.message }), true);
        }
    }

    function drawCalendar(cal) {
        if (!cal) return;

        el('cal-state').textContent = cal.configured ? T('cal.on', { url: cal.url }) : T('cal.off');
        el('cal-remove').hidden = !cal.configured;

        if (cal.configured && !el('cal-url').value) {
            el('cal-url').value = cal.url;
            el('cal-user').value = cal.user;
            el('cal-password').placeholder = cal.has_password ? T('cal.keep') : '';
        }
    }

    async function saveCalendar() {
        const button = el('cal-save');
        button.disabled = true;
        say('cal-said', T('cal.checking'));

        try {
            const res = await send('/api/settings/calendar', {
                url: el('cal-url').value, user: el('cal-user').value, password: el('cal-password').value,
            });

            el('cal-password').value = '';
            say('cal-said', T('cal.saved', { name: res.name }));
            await loadPlan();
        } catch (err) {
            say('cal-said', T('writing.failed', { why: err.message }), true);
        } finally {
            button.disabled = false;
        }
    }

    async function removeCalendar() {
        const button = el('cal-remove');

        if (!button.classList.contains('danger')) {
            button.classList.add('danger');
            button.textContent = T('gl.sure');
            setTimeout(() => { button.classList.remove('danger'); button.textContent = T('cal.remove'); }, 4000);
            return;
        }

        try {
            await send('/api/settings/calendar', { remove: true });
            el('cal-url').value = '';
            el('cal-user').value = '';
            el('cal-password').placeholder = '';
            say('cal-said', T('cal.removed'));
            await loadPlan();
        } catch (err) {
            say('cal-said', T('writing.failed', { why: err.message }), true);
        }
    }

    function wire() {
        if (!el('plan-agenda')) return;

        el('plan-refresh').addEventListener('click', () => { loadPlan(); loadTodos(); });
        el('plan-days').addEventListener('change', loadPlan);
        el('todo-add').addEventListener('click', addTodo);
        el('todo-filter').addEventListener('change', () => lastTodos && drawTodos(lastTodos));
        el('cal-save').addEventListener('click', saveCalendar);
        el('cal-remove').addEventListener('click', removeCalendar);

        for (const id of ['todo-text', 'todo-list']) {
            el(id).addEventListener('keydown', (e) => {
                if (e.key === 'Enter') {
                    e.preventDefault();
                    addTodo();
                }
            });
        }

        for (const item of document.querySelectorAll('.nav-item[data-view="plan"]')) {
            item.addEventListener('click', () => { loadPlan(); loadTodos(); });
        }

        window.addEventListener('pn-language', () => {
            if (lastPlan) drawPlan(lastPlan);
            if (lastTodos) drawTodos(lastTodos);
        });
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
}());
