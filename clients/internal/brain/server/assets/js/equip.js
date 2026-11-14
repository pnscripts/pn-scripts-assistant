(function () {
    'use strict';

    /*
     * Equipping the organisation: starting a project, the capability packages,
     * what the machine has, and the integrations.
     *
     * Every button here that changes something is its owner's approval, given
     * at the computer — the server refuses these from anywhere else — and each
     * says in its label exactly what it does. Read when the Organisation view
     * is opened, like the rest of it, rather than on a timer.
     */

    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
    const bg = () => window.pnLang && window.pnLang() === 'bg';

    // A recipe's source and cost as provision/recipes.go says them; anything
    // else it says stays as said.
    function sourceSaid(source) {
        const m = /^your distribution's package (.+)$/.exec(source || '');

        return m ? T('org.distroPackage', { pkg: m[1] }) : source;
    }

    function costSaid(cost) {
        return cost === 'free' ? T('org.costFree') : cost;
    }

    // A string of i18n.js when there is one, and the server's word when not.
    function named(key, fallback) {
        const said = T(key);

        return said === key ? fallback : said;
    }

    function toolName(name) {
        return window.pnToolName ? window.pnToolName(name) : String(name || '').replace(/_/g, ' ');
    }

    function node(tag, className, text) {
        const n = document.createElement(tag);

        if (className) n.className = className;
        if (text !== undefined) n.textContent = text;

        return n;
    }

    function button(label, run) {
        const b = node('button', 'task-button', label);

        b.type = 'button';
        b.addEventListener('click', run);

        return b;
    }

    async function get(url) {
        const res = await fetch(url);
        const body = await res.json().catch(() => ({}));

        if (!res.ok) throw new Error(body.error || T('org.failed'));

        return body;
    }

    async function post(url, body) {
        const res = await fetch(url, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body || {}),
        });

        const answer = await res.json().catch(() => ({}));

        if (!res.ok) throw new Error(answer.error || T('org.failed'));

        return answer;
    }

    /* ---------- a project ---------- */

    let project = null;

    function showProject(p) {
        project = p;

        const box = el('proj-proposal');
        const options = el('proj-options');

        box.hidden = false;
        el('proj-proposal-text').textContent = projectText(p);
        options.textContent = '';

        // The engine question answered with a button per engine, each saying
        // what choosing it would take on this machine.
        for (const o of p.options || []) {
            const choice = button(o.title, () => propose(o.package));

            choice.title = o.says;
            options.appendChild(choice);
        }

        const ready = !!p.ready && p.state === 'open';

        el('proj-start').hidden = !ready;
        el('proj-decline').hidden = p.state !== 'open';
    }

    /*
     * The question "where?" is the common one and is said in the page's
     * language with its choices. A worked-out plan is the server's own text:
     * most of it is the packages' and the folder's words, which are English.
     */
    function projectText(p) {
        if (p.question_key) {
            const lines = [named('org.proj.' + p.question_key, p.question)];

            for (const o of p.options || []) lines.push('  · ' + o.title + ': ' + o.says);

            return lines.join('\n');
        }

        return p.text || '';
    }

    async function propose(pkg) {
        const request = el('proj-request').value.trim();
        const folder = el('proj-folder').value.trim();
        const said = el('proj-said');

        if (!request) {
            said.textContent = T('org.proj.sayWhat');

            return;
        }

        said.textContent = T('org.proj.working');

        try {
            const p = await post('/api/projects/propose', { request, folder, package: pkg || '' });

            said.textContent = p.question ? '' : T('org.proj.proposal', { id: p.id });
            showProject(p);
        } catch (err) {
            said.textContent = err.message;
        }
    }

    async function startProject() {
        if (!project) return;

        const said = el('proj-said');

        try {
            await post('/api/projects/' + project.id + '/start');

            said.textContent = startedWords(project);
            el('proj-start').hidden = true;
            el('proj-decline').hidden = true;
        } catch (err) {
            said.textContent = err.message;
        }
    }

    // What the server said in English, said from the proposal started.
    function startedWords(row) {
        const p = row.proposal || {};
        const vars = { name: p.name || '', dir: p.dir || '', list: (p.installs || []).join(', ') };

        return T((p.installs || []).length ? 'org.proj.startingInstalls' : 'org.proj.starting', vars);
    }

    async function declineProject() {
        if (!project) return;

        try {
            await post('/api/projects/' + project.id + '/decline');
            el('proj-said').textContent = T('org.proj.declined');
            el('proj-proposal').hidden = true;
        } catch (err) {
            el('proj-said').textContent = err.message;
        }
    }

    /* ---------- packages ---------- */

    function statusLine(need) {
        const s = need.status || {};
        let text = s.title + (s.wanted ? ' ' + s.wanted : '');

        if (s.present && s.compatible) {
            text += s.version ? T('org.need.ready', { version: s.version }) : T('org.need.here');
        } else if (s.present) {
            text += ': ' + s.problem;
        } else {
            text += T('org.need.notHere');
        }

        if (need.optional) text += ' · ' + T('org.need.isOptional');

        return text;
    }

    async function packages() {
        const list = el('pkg-list');

        if (!list) return;

        try {
            const body = await get('/api/packages');

            list.textContent = '';

            for (const p of body.packages || []) {
                const item = node('details', 'equip-item');
                const head = node('summary', 'equip-head');

                head.append(node('span', 'equip-title', p.title),
                    node('span', 'org-kind', named('org.work.' + p.work, p.work)),
                    node('span', ['org-risk', 'org-risk-' + (p.risk || 'low')].join(' '), named('org.risk.' + (p.risk || 'low'), p.risk || 'low')));

                item.appendChild(head);
                item.appendChild(node('p', 'note', p.summary));
                item.appendChild(node('p', 'note tiny', T(p.built_in ? 'org.pkgBuiltIn' : 'org.pkgYours',
                    { id: p.id, version: p.version, file: p.file })));

                const role = p.role || {};
                const vars = {
                    role: (role.title || '').toLowerCase(), title: role.title || '',
                    tools: (role.tools || []).map(toolName).join(', ') || T('org.judgementOnly'),
                    never: (role.never || []).map(toolName).join(', '),
                };

                item.appendChild(node('p', 'note', T((role.never || []).length ? 'org.pkgRoleNever' : 'org.pkgRole', vars)));

                for (const need of p.requires || []) {
                    item.appendChild(node('p', 'note tiny', T('org.pkgNeeds', { what: statusLine(need), why: need.why })));
                }

                for (const limit of p.limits || []) {
                    item.appendChild(node('p', 'note tiny equip-limit', T('org.mustNot', { what: limit })));
                }

                if (p.escalation) {
                    item.appendChild(node('p', 'note tiny equip-limit', T('org.qualified', { who: p.escalation })));
                }

                if ((p.evidence || []).length) {
                    item.appendChild(node('p', 'note tiny', T('org.finishedWhen', { list: p.evidence.join(', ') })));
                }

                list.appendChild(item);
            }

            for (const [file, why] of Object.entries(body.refused || {})) {
                list.appendChild(node('p', 'note equip-limit', T('org.pkgRefused', { file, why })));
            }
        } catch (err) {
            list.textContent = err.message;
        }
    }

    /* ---------- what the machine has ---------- */

    async function machine() {
        const list = el('prov-list');

        if (!list) return;

        try {
            const body = await get('/api/provision');

            list.textContent = '';

            for (const s of body.recipes || []) {
                const row = node('div', 'equip-row');
                let here = s.present
                    ? (s.version ? s.version : T('org.here'))
                    : T('org.notHere');

                // Installed is not usable: a licence that will not let it work
                // unattended is said, not hidden behind "here".
                if (s.present && s.licence_state === 'inactive') here += T('org.provUnlicensed');
                if (s.present && s.licence_state === 'unknown') here += T('org.provLicenceUnknown');

                row.appendChild(node('span', 'equip-title', s.title));
                row.appendChild(node('span', 'note tiny', here + ' · ' + named('org.recipe.' + s.kind, s.kind)));

                if (!s.present && s.installable) {
                    row.appendChild(button(s.size ? T('org.installSize', { size: s.size }) : T('org.install'), () => install(s)));
                }

                list.appendChild(row);

                const terms = [sourceSaid(s.source), s.licence && T('org.licence', { licence: s.licence }), costSaid(s.cost)].filter(Boolean).join(' · ');

                if (!s.present) {
                    list.appendChild(node('p', 'note tiny equip-under', s.installable ? terms : (s.manual || terms)));
                }
            }
        } catch (err) {
            list.textContent = err.message;
        }
    }

    async function install(s) {
        const said = el('prov-said');

        try {
            const body = await post('/api/provision/install', { recipe: s.id });

            // The server's English sentence, said from the recipe: in the
            // background unless goal mode installed it there and then.
            said.textContent = body.background === false
                ? T('org.installedNow', { title: s.title })
                : T(s.size ? 'org.installingSize' : 'org.installing', { title: s.title, size: s.size || '' });
        } catch (err) {
            said.textContent = err.message;
        }
    }

    /* ---------- integrations ---------- */

    async function act(id, action, body) {
        const said = el('int-said');

        try {
            said.textContent = action === 'activate' ? T('org.intStarting', { id }) : '';
            await post('/api/integrations/' + encodeURIComponent(id) + '/' + action, body);
            said.textContent = '';
            integrations();
        } catch (err) {
            said.textContent = err.message;
        }
    }

    // Where an integration runs, as mcp.Server.Where says it in English.
    function whereWords(v) {
        const command = (v.command || []).join(' ');

        if (v.transport === 'http' && v.leaves_the_machine) return T('org.int.whereRemote', { url: v.url || '' });
        if (v.transport === 'http') return T('org.int.whereLocal', { url: v.url || '' });
        if (v.network) return T('org.int.whereNetwork', { command });

        return T('org.int.whereProgram', { command });
    }

    async function integrations() {
        const list = el('int-list');

        if (!list) return;

        try {
            const body = await get('/api/integrations');

            list.textContent = '';

            for (const v of body.integrations || []) {
                const item = node('div', 'equip-item');
                const head = node('div', 'equip-head');

                let state = T('org.int.catalogue');

                if (v.from && v.from.gone_since) state = T('org.int.goneBadge');
                else if (v.running) state = T('org.int.running', { era: v.era || '', protocol: v.protocol || '' });
                else if (v.approved) state = T('org.int.stopped');
                else if (v.listed) state = T('org.int.added');

                head.append(node('span', 'equip-title', v.title), node('span', 'org-kind', state));

                if (v.leaves_the_machine) head.appendChild(node('span', 'org-risk org-risk-high', T('org.int.leaves')));

                item.appendChild(head);
                item.appendChild(node('p', 'note', v.why || ''));
                item.appendChild(node('p', 'note tiny', whereWords(v) + (v.version ? ' · ' + v.version : '')
                    + (v.licence ? ' · ' + T('org.licence', { licence: v.licence }) : '')));

                if (v.missing) item.appendChild(node('p', 'note tiny equip-limit', T('org.int.missing', { what: v.missing })));

                const gone = !!(v.from && v.from.gone_since);

                if (v.from) {
                    item.appendChild(node('p', 'note tiny', T('org.int.from', { client: clientTitle(v.from.client) })));
                }

                if (gone) {
                    item.appendChild(node('p', 'note tiny equip-limit',
                        T('org.int.gone', { client: clientTitle(v.from.client), when: whenSaid(v.from.gone_since) })));
                }

                if (v.approved) {
                    item.appendChild(node('p', 'note tiny', T('org.int.granted', { list: (v.agents || []).join(', ') || T('org.int.nobody') })));
                }

                for (const t of v.tools || []) {
                    item.appendChild(node('p', 'note tiny', T(t.read_only ? 'org.int.readsOnly' : 'org.int.asksFirst') + ' · ' + t.as
                        + (t.description ? ': ' + t.description : '') + (t.hint ? ' (' + t.hint + ')' : '')));
                }

                const actions = node('div', 'org-actions');

                if (!v.approved) {
                    actions.appendChild(button(T('org.int.approve'), () => act(v.id, 'approve', { agents: ['assistant'] })));
                }

                if (v.approved && !v.running && !gone) actions.appendChild(button(T('org.startIt'), () => act(v.id, 'activate')));
                if (v.running) actions.appendChild(button(T('org.int.stop'), () => act(v.id, 'deactivate')));
                if (v.approved) actions.appendChild(button(T('org.int.revoke'), () => act(v.id, 'revoke')));
                if (v.listed) actions.appendChild(button(T('org.int.remove'), () => act(v.id, 'remove')));

                item.appendChild(actions);
                list.appendChild(item);
            }

            found(body);
        } catch (err) {
            list.textContent = err.message;
        }
    }

    /* ---------- found in the user's other programs ---------- */

    // A program's name, the same in every language.
    function clientTitle(client) {
        return named('org.imp.c.' + client, client);
    }

    function whenSaid(iso) {
        const d = new Date(iso);

        if (isNaN(d)) return iso;

        return d.toLocaleString(bg() ? 'bg-BG' : 'en-GB',
            { day: 'numeric', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' });
    }

    function countSaid(n) {
        return new Intl.NumberFormat(bg() ? 'bg-BG' : 'en-GB').format(n);
    }

    async function importFound(key) {
        const said = el('int-said');

        try {
            said.textContent = '';
            await post('/api/integrations/import', { key });
            integrations();
        } catch (err) {
            said.textContent = err.message;
        }
    }

    // What /api/integrations found in the other programs' settings.
    function found(body) {
        const box = el('int-found');
        const files = el('int-found-files');

        if (!box || !files) return;

        box.textContent = '';

        const list = body.found || [];

        if (!list.length) box.appendChild(node('p', 'note', T('org.imp.none')));

        for (const f of list) {
            const item = node('div', 'equip-item');
            const head = node('div', 'equip-head');

            head.append(node('span', 'equip-title', f.name), node('span', 'org-kind', clientTitle(f.client)));

            if (f.leaves_the_machine) head.appendChild(node('span', 'org-risk org-risk-high', T('org.int.leaves')));

            item.appendChild(head);

            if (f.scope) item.appendChild(node('p', 'note tiny', T('org.imp.folder', { folder: f.scope })));

            const server = f.server || {};

            if (!f.problem) {
                item.appendChild(node('p', 'note tiny', whereWords({
                    transport: server.transport, url: server.url, command: server.command,
                    network: server.network, leaves_the_machine: f.leaves_the_machine,
                })));
            }

            // What is kept sealed, by the name the person wrote there: a
            // setting's name, or the header's.
            if (!f.problem && (server.secrets || []).length) {
                const names = server.header ? [server.header] : server.secrets;

                item.appendChild(node('p', 'note tiny', T('org.imp.secrets', { names: names.join(', ') })));
            }

            if (f.problem) {
                const why = named('org.imp.p.' + f.problem, f.problem).replace('{detail}', f.detail || '');

                item.appendChild(node('p', 'note tiny equip-limit', T('org.imp.cannot', { why })));
            }

            const actions = node('div', 'org-actions');

            if (f.imported) {
                item.appendChild(node('p', 'note tiny', T('org.imp.imported', { id: f.imported })));
            } else if (!f.problem) {
                actions.appendChild(button(T('org.imp.import'), () => importFound(f.key)));
            }

            if (actions.childNodes.length) item.appendChild(actions);

            box.appendChild(item);
        }

        const said = (body.found_files || []).map((f) => {
            const key = 'org.imp.f.' + f.state;

            return named(key, f.state).replace('{client}', clientTitle(f.client)).replace('{n}', countSaid(f.count));
        });

        files.textContent = said.length ? T('org.imp.files', { list: said.join(' · ') }) + ' ' + T('org.imp.watched') : '';
    }

    /* ---------- a task's evidence ---------- */

    // The Tasks view calls this when it opens a task.
    let evidenceOf = null;

    window.pnTaskEvidence = async function (taskID) {
        evidenceOf = taskID;

        const box = el('task-detail-evidence');

        if (!box) return;

        try {
            const body = await get('/api/tasks/' + taskID + '/evidence');
            const rows = body.evidence || [];

            box.textContent = '';
            box.hidden = rows.length === 0;

            if (!rows.length) return;

            box.appendChild(node('h3', 'equip-subhead', T('org.ev.title')));

            for (const e of rows) {
                // The settings as the task found them are for comparing, not
                // for reading: said in a line, never shown as the file.
                if (e.kind === 'settings') {
                    box.appendChild(node('p', 'note tiny', T('org.ev.settings')));
                    continue;
                }

                // How the work is being done, and every change of who does
                // it, in full: the reasons are the point.
                if (e.kind === 'decision' || e.kind === 'switch' || e.kind === 'install') {
                    const d = node('details', 'equip-decision');
                    d.open = e.kind !== 'install';
                    d.appendChild(node('summary', 'note tiny' + (e.ok || e.kind === 'decision' ? '' : ' equip-limit'),
                        named('org.ev.' + e.kind, e.kind) + ': ' + e.subject));
                    d.appendChild(node('pre', 'proposal-text', e.detail || ''));
                    box.appendChild(d);
                    continue;
                }

                const line = node('p', 'note tiny' + (e.ok ? '' : ' equip-limit'),
                    named('org.ev.' + e.kind, e.kind) + ': ' + e.subject + (e.detail ? ', ' + e.detail.split('\n')[0] : ''));

                box.appendChild(line);
            }
        } catch (err) {
            box.hidden = true;
        }
    };

    /* ---------- how work is done ---------- */

    // An integration nobody has approved is not unusable, only not yet
    // allowed: it is the one reason an integration is listed that way.
    function stateWord(r) {
        if (r.kind === 'integration' && r.state === 'not_supported') return T('org.rs.notApproved');

        return named('org.rs.' + r.state, r.state);
    }

    const gb = 1024 * 1024 * 1024;

    let policy = null;

    function capacity(r) {
        if (!r.usage) return r.kind === 'agent' ? T('org.capacityUnknown') : '';

        const left = r.usage.remaining;
        let text = (left === undefined || left === null) ? T('org.capacityUnknown') : T('org.left', { n: Math.round(left) });

        if (r.usage.resets_at && !r.usage.resets_at.startsWith('0001')) {
            text += T('org.resets', { when: new Date(r.usage.resets_at).toLocaleString(bg() ? 'bg-BG' : 'en-GB') });
        }

        return text;
    }

    function machineLine(m) {
        const ram = m.ram_bytes ? Math.round(m.ram_bytes / gb) + ' GB' : '? GB';
        const parts = [m.os, T('org.m.cores', { cpu: m.cpu, n: m.cores }), ram + ' RAM',
            (m.gpu || []).join(', ') || T('org.m.noGpu'),
            T('org.m.display', { display: m.display }), T('org.m.private', { display: m.virtual_display }),
            m.landlock_abi ? T('org.m.landlock', { abi: m.landlock_abi }) : T('org.m.noLandlock')];

        return parts.join(' · ');
    }

    function showPolicy(p) {
        policy = p;

        el('res-privacy').value = p.privacy;
        el('res-notify').value = p.notify;
        el('res-auto-models').checked = !!p.autonomy.install_local_models;
        el('res-auto-tools').checked = !!p.autonomy.install_tools;
        el('res-max-download').value = (p.budget.max_download_bytes / gb).toFixed(1);
        el('res-min-free').value = Math.round(p.budget.min_free_disk_bytes / gb);
        el('res-budget').value = p.budget.monthly_api_usd;
        el('res-warn').value = p.thresholds.warning;
        el('res-low').value = p.thresholds.low;
        el('res-critical').value = p.thresholds.critical;
    }

    async function resources(refresh) {
        const list = el('res-list');

        if (!list) return;

        el('res-said').textContent = T('org.looking');

        try {
            const body = refresh ? await post('/api/resources/refresh') : await get('/api/resources');

            el('res-machine').textContent = machineLine(body.machine);
            el('res-coding').textContent = T('org.codingNow') + '\n' + body.coding.explained;

            list.textContent = '';

            for (const r of body.resources || []) {
                const row = node('div', 'equip-row');

                row.appendChild(node('span', 'equip-title', r.title));
                row.appendChild(node('span', 'note tiny',
                    [named('org.rk.' + r.kind, r.kind), r.version && 'v' + r.version, stateWord(r), capacity(r),
                        r.plan && T('org.plan', { plan: r.plan })].filter(Boolean).join(' · ')));
                list.appendChild(row);

                if (r.why) list.appendChild(node('p', 'note tiny equip-under', r.why));
            }

            const runs = el('res-runs');
            runs.textContent = '';

            if ((body.runs || []).length) {
                runs.appendChild(node('p', 'note', T('org.recentWork')));

                for (const run of body.runs.slice(0, 10)) {
                    runs.appendChild(node('p', 'note tiny',
                        [run.resource, run.work, run.result, run.failure, run.detail].filter(Boolean).join(' · ')));
                }
            }

            showPolicy(body.policy);
            el('res-said').textContent = '';
        } catch (err) {
            el('res-said').textContent = err.message;
        }
    }

    async function savePolicy() {
        if (!policy) return;

        const p = JSON.parse(JSON.stringify(policy));

        p.privacy = el('res-privacy').value;
        p.notify = el('res-notify').value;
        p.autonomy.install_local_models = el('res-auto-models').checked;
        p.autonomy.install_tools = el('res-auto-tools').checked;
        p.budget.max_download_bytes = Math.round(parseFloat(el('res-max-download').value || '0') * gb);
        p.budget.max_model_bytes = p.budget.max_download_bytes;
        p.budget.min_free_disk_bytes = Math.round(parseFloat(el('res-min-free').value || '0') * gb);
        p.budget.monthly_api_usd = parseFloat(el('res-budget').value || '0');
        p.thresholds.warning = parseFloat(el('res-warn').value);
        p.thresholds.low = parseFloat(el('res-low').value);
        p.thresholds.critical = parseFloat(el('res-critical').value);

        try {
            await post('/api/resources/policy', p);
            el('res-said').textContent = T('org.policySaved');
            resources(false);
        } catch (err) {
            el('res-said').textContent = err.message;
        }
    }

    function load() {
        packages();
        machine();
        integrations();
        resources(false);
    }

    function wire() {
        document.querySelector('.nav-item[data-view="organisation"]')?.addEventListener('click', load);

        el('proj-go')?.addEventListener('click', () => propose(''));
        el('res-refresh')?.addEventListener('click', () => resources(true));
        el('res-save')?.addEventListener('click', savePolicy);
        el('proj-start')?.addEventListener('click', startProject);
        el('proj-decline')?.addEventListener('click', declineProject);

        for (const id of ['proj-request', 'proj-folder']) {
            el(id)?.addEventListener('keydown', (e) => {
                if (e.key === 'Enter') { e.preventDefault(); propose(''); }
            });
        }

        // Said again in the other language.
        window.addEventListener('pn-language', () => {
            load();

            if (project && !el('proj-proposal').hidden) showProject(project);
            if (evidenceOf !== null && el('task-detail-evidence') && !el('task-detail-evidence').hidden) {
                window.pnTaskEvidence(evidenceOf);
            }
        });

        load();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
})();
