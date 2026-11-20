/*
 * The Programs page: what is installed on this computer, what came and went,
 * and, for each program, what the assistant found and what it does with it.
 *
 * Read only. The server keeps the inventory current by itself (package
 * programs); this draws GET /api/programs. The words are worked out by small
 * functions on window.pnPrograms, so a test can run them in node on the real
 * server's answer in both languages; the drawing below only places them.
 */
(function () {
    'use strict';

    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
    const bg = () => (window.pnLang ? window.pnLang() === 'bg' : false);

    // How far back "What changed" looks, and how many rows a list shows
    // before it asks for a narrower search (said on the page, never cut
    // silently).
    const RECENT_DAYS = 14;
    const MOST_CHANGES = 20;
    const MOST_ROWS = 200;

    // The server's reasons for polling a folder (programs.Reasons), and the
    // key that says each one here.
    const WHY = {
        missing: 'prg.why.missing',
        limit: 'prg.why.limit',
        'no-events': 'prg.why.noEvents',
        'no-path': 'prg.why.noPath',
    };

    // Each source, and its name for a person.
    const KIND = {
        app: 'prg.kind.app',
        deb: 'prg.kind.deb',
        snap: 'prg.kind.snap',
        flatpak: 'prg.kind.flatpak',
        win: 'prg.kind.win',
        msix: 'prg.kind.msix',
    };

    // Each source as the summary counts it.
    const PART = {
        app: 'prg.part.app',
        deb: 'prg.part.deb',
        snap: 'prg.part.snap',
        flatpak: 'prg.part.flatpak',
        win: 'prg.part.win',
        msix: 'prg.part.msix',
    };

    // The sources an answer from an older server names none of: Linux's.
    const LINUX = ['app', 'deb', 'snap', 'flatpak'];

    // The sources worth showing: the server's (this computer's, then any
    // a remembered program came from).
    function kinds(body) {
        const k = body && Array.isArray(body.kinds) ? body.kinds.filter((x) => KIND[x]) : [];
        return k.length ? k : LINUX;
    }

    // The choices of the "Which programs" list: each source shown, then
    // the removed ones.
    function choices(body) {
        return kinds(body).concat(['removed']);
    }

    function when(at) {
        const d = new Date(at);
        if (!at || Number.isNaN(d.getTime()) || d.getUTCFullYear() < 2000) return '';

        const two = (n) => String(n).padStart(2, '0');
        const time = two(d.getHours()) + ':' + two(d.getMinutes());

        if (bg()) return two(d.getDate()) + '.' + two(d.getMonth() + 1) + '.' + d.getFullYear() + ', ' + time;

        return d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' }) + ', ' + time;
    }

    // A count as the page's language writes it (2,855 or 2855; 12 345 or
    // 12,345).
    function count(n) {
        try {
            return new Intl.NumberFormat(bg() ? 'bg-BG' : 'en-GB').format(n);
        } catch (err) {
            return String(n);
        }
    }

    function kindWord(kind) {
        return KIND[kind] ? T(KIND[kind]) : kind;
    }

    // A reason by its code; the server's English for one this page does
    // not know yet.
    function why(polled) {
        return WHY[polled.reason] ? T(WHY[polled.reason]) : polled.why;
    }

    // One sentence for the whole inventory and how it is kept current.
    function summary(body) {
        const all = (body && body.programs) || [];
        const here = all.filter((p) => p.present);
        const of = (kind) => count(here.filter((p) => p.kind === kind).length);

        if (!all.length) {
            return (body && body.first_looks && Object.keys(body.first_looks).length)
                ? T('prg.noneFound') : T('prg.notYet');
        }

        const parts = kinds(body).map((kind) => T(PART[kind], { n: of(kind) }));

        return T('prg.summary', { n: count(here.length), parts: parts.join(', ') });
    }

    // How each source is kept current: watched folders; folders that are
    // not there yet, counted (normal: most computers have no Flatpak); and
    // each folder polled for another reason, by name.
    function keeping(body) {
        const sources = (body && body.sources) || [];

        if (!sources.length) return [T('prg.keepNotStarted')];

        const watched = sources.reduce((n, s) => n + (s.watched || 0), 0);
        const lines = [T('prg.keepWatched', { n: count(watched) })];
        let missing = 0;

        for (const s of sources) {
            for (const p of s.polled || []) {
                if (p.reason === 'missing') {
                    missing++;
                    continue;
                }

                lines.push(p.dir
                    ? T('prg.keepPolled', { kind: kindWord(s.kind), dir: p.dir, why: why(p) })
                    : T('prg.keepPolledSource', { kind: kindWord(s.kind), why: why(p) }));
            }
        }

        if (missing) lines.push(T('prg.keepMissing', { n: count(missing) }));

        return lines;
    }

    // What happened to a program, in a few words.
    function happened(p) {
        switch (p.last_change) {
        case 'added':
            return T('prg.did.added', { when: when(p.changed_at) });
        case 'removed':
            return T('prg.did.removed', { when: when(p.removed_at || p.changed_at) });
        case 'returned':
            return T('prg.did.returned', { when: when(p.changed_at) });
        case 'updated':
            return p.previous_version
                ? T('prg.did.updated', { from: p.previous_version, to: p.version || '?', when: when(p.changed_at) })
                : T('prg.did.updatedPlain', { when: when(p.changed_at) });
        default:
            return '';
        }
    }

    // What changed in the last RECENT_DAYS days, newest first.
    function changes(body, now) {
        const since = (now || Date.now()) - RECENT_DAYS * 86400000;
        const all = ((body && body.programs) || []).filter((p) => p.last_change &&
            new Date(p.changed_at).getTime() >= since);

        all.sort((a, b) => new Date(b.changed_at) - new Date(a.changed_at));

        return {
            total: all.length,
            rows: all.slice(0, MOST_CHANGES).map((p) => ({ program: p, name: p.name || p.id, said: kindWord(p.kind) + ' · ' + happened(p) })),
            more: Math.max(0, all.length - MOST_CHANGES),
        };
    }

    // The list for one choice (a source, or 'removed') and a search, by
    // name.
    function list(body, choice, query) {
        const q = (query || '').trim().toLowerCase();
        const all = ((body && body.programs) || []).filter((p) => (choice === 'removed'
            ? !p.present
            : p.present && p.kind === choice));
        const found = q ? all.filter((p) => [p.name, p.id, p.exec].some((s) => s && s.toLowerCase().includes(q))) : all;

        found.sort((a, b) => (a.name || a.id).localeCompare(b.name || b.id));

        const rows = found.slice(0, MOST_ROWS).map((p) => ({
            program: p,
            name: p.name || p.id,
            said: choice === 'removed'
                ? kindWord(p.kind) + ' · ' + T('prg.did.removed', { when: when(p.removed_at || p.changed_at) })
                // A menu entry has no version; what it starts tells two
                // entries with one name apart.
                : (p.kind === 'app' ? (p.exec || '') : (p.version || '')),
        }));

        let note = '';
        if (!found.length) note = q ? T('prg.noMatch', { q: query.trim() }) : T('prg.emptyChoice');
        else if (found.length > MOST_ROWS) note = T('prg.showing', { shown: count(MOST_ROWS), n: count(found.length) });

        return { rows, total: found.length, note };
    }

    // The first-contact card: what was found about one program, as
    // [label, value] rows; nothing empty is shown.
    function found(p, body) {
        const rows = [[T('prg.f.kind'), kindWord(p.kind)], [T('prg.f.id'), p.id]];

        if (p.version) rows.push([T('prg.f.version'), p.version]);
        if (p.path) rows.push([T('prg.f.path'), p.path]);
        if (p.exec) rows.push([T('prg.f.exec'), p.exec]);

        const looked = body && body.first_looks ? body.first_looks[p.kind] : '';

        rows.push([T('prg.f.since'), p.initial
            ? (looked ? T('prg.f.before', { when: when(looked) }) : T('prg.f.beforePlain'))
            : when(p.first_seen)]);

        const last = happened(p);
        if (last && !(p.last_change === 'added' && p.changed_at === p.first_seen)) rows.push([T('prg.f.last'), last]);

        return rows;
    }

    // What the assistant does with it, today.
    function canDo(p) {
        return p.present ? T('prg.canDo') : T('prg.canDoGone', { when: when(p.removed_at || p.changed_at) });
    }

    // The kinds of use a card decides, in the server's order (an older
    // server sends none: these three).
    const VERBS = ['start', 'control-input', 'change'];

    // Each kind of use, and its name for a person.
    const VERB = {
        start: 'prg.verb.start',
        'control-input': 'prg.verb.control-input',
        change: 'prg.verb.change',
    };

    // The answers, in the order the buttons stand, and their words.
    const ANSWERS = ['ask', 'allow', 'never'];
    const ANSWER = { ask: 'prg.ans.ask', allow: 'prg.ans.allow', never: 'prg.ans.never' };

    /*
     * What the assistant may do with a program without asking: one row per
     * kind of use, with the answer chosen and what it means now. An
     * "always" given to an older version sleeps (the server says asleep),
     * and so does one on a menu entry, which has no version, once the
     * entry or the file it starts has changed;
     * on "ask every time" no "always" is acted on (honoured false); a
     * removed program's choices are kept for when it comes back, and said
     * as kept, not as done.
     */
    function may(p, body) {
        const verbs = body && Array.isArray(body.verbs) && body.verbs.length ? body.verbs : VERBS;
        const decided = (body && body.permits && body.permits[p.kind + ':' + p.id]) || [];
        const honoured = !(body && body.honoured === false);

        const rows = verbs.map((verb) => {
            const d = decided.find((x) => x.verb === verb);
            const answer = d ? d.answer : 'ask';
            let state;

            if (!p.present && answer === 'never') state = T('prg.state.keptNever');
            else if (!p.present && answer === 'allow') state = T('prg.state.keptAllow');
            else if (answer === 'never') state = T('prg.state.never');
            else if (answer === 'allow' && d.asleep && !p.version) state = T('prg.state.asleepEntry');
            else if (answer === 'allow' && d.asleep) state = T('prg.state.asleep', { from: d.version || '?', to: p.version || '?' });
            else if (answer === 'allow' && !honoured) state = T('prg.state.notHonoured');
            else if (answer === 'allow') state = T('prg.state.allow', { when: when(d.at) });
            else if (verb === 'control-input') state = T('prg.state.askInput');
            else state = T('prg.state.general');

            return { verb, label: VERB[verb] ? T(VERB[verb]) : verb, answer, state };
        });

        const notes = [];
        if (!p.present) notes.push(T('prg.mayGone'));
        if (body && body.stopped_since) notes.push(T('prg.mayStopped'));

        return { rows, note: notes.join(' ') };
    }

    /*
     * "Stop using other programs": the sentence and the button for the
     * switch, from the server's stopped_since (null when not stopped).
     */
    function stopping(body) {
        const since = body && body.stopped_since;

        if (since) {
            return { stopped: true, said: T('prg.stop.stopped', { when: when(since) }), button: T('prg.stop.off') };
        }

        return { stopped: false, said: T('prg.stop.running'), button: T('prg.stop.on') };
    }

    /*
     * The answers a waiting question offers, from GET /api/approvals: a
     * question about one program offers "Always for <name>" (that program
     * only, the version asked about) when its Always would answer it; the
     * tool-wide Always is offered beside it as "Always, any program", or
     * alone as before for a question about no program, and never for a
     * question that is always asked. While other programs are stopped a
     * question that uses one can only be refused, and says why.
     */
    function approvalChoices(item, toolName) {
        const p = item && item.program;
        const alwaysAsked = /^Always asked/.test((item && item.summary) || '');
        const out = { approve: true, reject: true, forProgram: null, anyProgram: null, note: '', askedBy: null };

        /*
         * Seen on a device elsewhere, which may not answer it (another
         * program asked for it; the server marks the card for that reader
         * only): who asked, and a line where the buttons would be, in place
         * of buttons that can only be refused. Before anything else: other
         * programs being stopped changes nothing about where it is answered.
         */
        if (item && item.at_the_computer_only) {
            out.approve = false;
            out.reject = false;
            out.note = T('desk.atTheComputer');
            if (item.asked_by) {
                const who = askedBy(item);
                out.askedBy = { label: who.chip, title: who.title + ' ' + T(item.tool === PERMIT ? 'out.ap.permit' : 'out.ap.note') };
            }
            return out;
        }

        if (item && item.programs_stopped) {
            out.approve = false;
            out.note = T('prg.ap.stopped');
            return out;
        }

        /*
         * Another program asked for it: the card says who, and offers
         * yes or no for this once. Nothing lasting: an "always" given
         * here would be given to whatever program asks next.
         */
        if (item && item.asked_by) {
            const who = askedBy(item);

            // One line on the card, so the answers stay in view; the rest
            // is said when the line is pointed at. The question itself
            // already starts "Always asked (another program asked for it)".
            // A program's own permission question: the yes lets that
            // program act, once; nothing is done here.
            out.askedBy = { label: who.chip, title: who.title + ' ' + T(item.tool === PERMIT ? 'out.ap.permit' : 'out.ap.note') };

            return out;
        }

        if (p && p.always) {
            const verb = VERB[p.verb] ? T(VERB[p.verb]).toLowerCase() : p.verb;
            const vars = { name: p.name, verb, version: p.version };

            out.forProgram = {
                label: T('prg.ap.always', vars),
                // A menu entry has no version: its Always follows the entry
                // and the file it starts instead.
                title: T(p.version ? 'prg.ap.alwaysTitle' : (p.kind === 'app' ? 'prg.ap.alwaysTitleEntry' : 'prg.ap.alwaysTitlePlain'), vars),
            };
        }

        if (!alwaysAsked) {
            out.anyProgram = p
                ? { label: T('prg.ap.any'), title: T('prg.ap.anyTitle', { tool: toolName }), secondary: true }
                : { label: T('cc.always'), title: T('cc.alwaysTitle', { tool: toolName }), secondary: false };
        }

        return out;
    }

    /*
     * Other programs asking (GET /api/outside, package otherprograms on the
     * server). A program that speaks the Model Context Protocol asks
     * through "pn-scripts-assistant mcp"; each question arrives with the
     * name that program gave itself. The switches say what they may ask;
     * the record and the cards say who did.
     */

    // Each switch, and each question, in words (literal keys: a test reads
    // them from this file).
    const SUBJECT = { programs: 'out.programs', ask: 'out.ask', memory: 'out.memory', permissions: 'out.permits' };
    const QUESTION = {
        assistant_status: 'out.q.status', list_programs: 'out.q.programs', ask_assistant: 'out.q.ask', search_memory: 'out.q.memory',
        permission_prompt: 'out.q.permit',
    };

    // The tool another program puts its own permission questions through
    // (otherprograms.PermissionTool on the server).
    const PERMIT = 'permission_prompt';

    // What turning a switch on means, said under it before it is turned:
    // none for the list of programs, whose name says it all.
    const MEANS = { ask: 'out.askMeans', memory: 'out.memoryMeans', permissions: 'out.permitsMeans' };

    // A question refused because its switch is off, by its tool.
    const REFUSED = {
        list_programs: 'out.rec.refused', ask_assistant: 'out.rec.refusedAsk', search_memory: 'out.rec.refusedMemory',
        permission_prompt: 'out.rec.refusedPermit',
    };

    // How one of the assistant's own tools ended (the record's words).
    const ENDED = { done: 'outcome.done', failed: 'outcome.failed', waiting: 'outcome.waiting', refused: 'outcome.refused' };

    // The name a program gave, or the words for one that gave none
    // (outside.Unnamed on the server).
    function caller(by, lower) {
        if (by && by !== 'unnamed') return by;
        return T(lower ? 'out.unnamedLower' : 'out.unnamed');
    }

    /*
     * The switches and the last questions as the page says them: the one
     * thing always answered, each switch with its state, and who asked
     * what lately and whether it was answered.
     */
    function outside(state) {
        const subjects = state && Array.isArray(state.subjects) ? state.subjects : [];
        const asked = state && Array.isArray(state.recent) ? state.recent : [];

        const rows = subjects.filter((s) => SUBJECT[s.id]).map((s) => ({
            id: s.id,
            label: T(SUBJECT[s.id]),
            allowed: !!s.allowed,
            state: s.allowed ? T('out.on', { when: when(s.since) }) : T('out.off'),
            means: MEANS[s.id] ? T(MEANS[s.id]) : '',
        }));

        // One of the questions, answered or not; or one of the assistant's
        // own tools another program asked for, which is named as that tool
        // and ends as any tool does.
        const recent = asked.map((a) => {
            const question = !!QUESTION[a.tool];
            const tool = window.pnToolName ? window.pnToolName(a.tool) : a.tool;

            return {
                when: when(a.at),
                who: caller(a.by),
                what: question ? T(QUESTION[a.tool]) : T('out.q.tool', { tool }),
                // A permission question is not "answered" by me: its
                // user allowed it, or it was not allowed.
                outcome: a.tool === PERMIT
                    ? (a.outcome === 'done' ? T('out.permitYes') : T('out.permitNo'))
                    : question
                        ? (a.outcome === 'done' ? T('out.answered') : T('out.notAnswered'))
                        : T(ENDED[a.outcome] || 'outcome.failed'),
                answered: a.outcome === 'done' || a.outcome === 'waiting',
            };
        });

        return {
            fixed: { label: T('out.status'), state: T('out.always') },
            rows,
            recent,
            none: recent.length ? '' : T('out.none'),
        };
    }

    /*
     * Who asked, for a deed or a waiting question another program asked
     * for (asked_by): the short line, what it means, and what kind of
     * record it is. Nothing for anything asked at the desk.
     */
    function askedBy(item) {
        const by = item && item.asked_by;
        if (!by) return null;

        // One of the questions other programs ask, as against one of the
        // assistant's own tools run because another program asked.
        const question = !!QUESTION[item.tool] && !item.changes_something;

        return {
            chip: T('out.askedBy', { who: caller(by, true) }),
            // Inside the record: the name and what a name is worth. A
            // program that gave none is said once, not twice.
            said: by === 'unnamed' ? caller(by) + '.' : caller(by) + '. ' + T('out.askedByTitle'),
            title: by === 'unnamed' ? T('out.unnamedTitle') : T('out.askedByTitle'),
            trait: question ? T('out.trait') : '',
            // An answer given to another program is in that program's
            // hands: the record does not say it stayed on this machine.
            // The answer to a question put to the assistant is made first
            // and fetched by the program after: the record cannot know
            // that it was, so it says what it knows.
            went: question && item.outcome === 'done' ? T(item.tool === 'ask_assistant' ? 'out.ready' : 'out.went') : '',
            // A permission question is decided by its user, or by nobody.
            decided: item.tool === PERMIT ? permitBy(item) : '',
        };
    }

    /*
     * What a question from another program came to, in the page's
     * language: the record keeps it in English. Empty for a deed from the
     * desk, whose result is the tool's own.
     */
    function outsideResult(d) {
        if (!d || !d.asked_by || !QUESTION[d.tool]) return '';

        const result = String(d.result || '');

        // A permission question: how it ended, whoever ended it. (One
        // refused because its switch is off is said with the rest, below.)
        if (d.tool === PERMIT) {
            for (const [like, key] of PERMIT_ENDED) {
                if (like.test(result)) return T(key);
            }
        }

        if (d.outcome === 'refused') {
            // Refused for being one of too many in a minute, whatever was
            // asked; or because its switch is off.
            const many = result.match(/more than (\d+) questions in a minute/);
            if (many) return T('out.rec.tooMany', { n: many[1] });

            return REFUSED[d.tool] ? T(REFUSED[d.tool]) : '';
        }

        if (d.tool === 'list_programs' || d.tool === 'search_memory') {
            const n = result.replace(/\D/g, '');
            if (!n) return '';

            return T(d.tool === 'list_programs' ? 'out.rec.told' : 'out.rec.found', { n: count(Number(n)) });
        }

        if (d.tool === 'ask_assistant') {
            // Given up for the person's own question, taken back with its
            // switch, or the model gave nothing.
            if (d.outcome !== 'done' && /my user asked me something/.test(result)) return T('out.rec.gaveWay');
            if (d.outcome !== 'done' && /switched off while I was answering/.test(result)) return T('out.rec.takenBack');
            if (d.outcome !== 'done') return T('out.rec.noAnswer');

            // The model's own words, under the page's word for them.
            return T('out.rec.answer', { text: result.replace(/^Answer: /, '') });
        }

        return '';
    }

    /*
     * What another program gave with its question (the question itself,
     * what it searched for), with the page's word before it: the record
     * keeps an English one. Empty when there is nothing to reword, and
     * the record's own line is then shown.
     */
    function outsideAsked(d) {
        if (!d || !d.asked_by || !QUESTION[d.tool]) return '';

        const asked = String(d.asked || '');

        if (d.tool === 'ask_assistant' && asked.startsWith('Question: ')) return T('out.rec.question', { text: asked.slice(10) });
        if (d.tool === 'search_memory' && asked.startsWith('Search: ')) return T('out.rec.search', { text: asked.slice(8) });
        if (d.tool === PERMIT && asked.startsWith('Permission: ')) return T('out.rec.permitAsked', { text: asked.slice(12) });

        return '';
    }

    /*
     * How a permission question of another program ended, by the words
     * the record keeps (brain/permit.go): a regular expression each, never
     * a quoted string.
     */
    const PERMIT_ENDED = [
        [/^Allowed on its card/, 'out.rec.permitYes'],
        [/told it may not/, 'out.rec.permitNo'],
        [/nobody answered in time/, 'out.rec.permitUnanswered'],
        [/stopped waiting/, 'out.rec.permitGone'],
        [/taken back/, 'out.rec.permitTakenBack'],
        [/permission questions only/, 'out.rec.permitNotOne'],
        [/were waiting already/, 'out.rec.permitFull'],
    ];

    // Where a permission question was decided, for its line on the
    // record: on its card at this computer, or by nobody. Not "by you":
    // the record cannot tell the window from a program posting as it.
    function permitBy(d) {
        const result = String((d && d.result) || '');

        if (/^Allowed on its card/.test(result) || /told it may not/.test(result)) return T('out.permitOnCard');
        if (/nobody answered|stopped waiting|taken back/.test(result)) return T('out.permitByNobody');

        return '';
    }

    /*
     * What the page says after a permission card was answered: the
     * invocation the server sent back, whether yes was pressed, and the
     * fault if there was one. Empty for any other card.
     */
    function permitDecided(invocation, approving, error) {
        if (!invocation || invocation.tool !== PERMIT) return '';

        /*
         * By what became of the question, not by which button was
         * pressed or whether the press failed: a second press on a card
         * already answered is refused, and the page must still say what
         * the first one did.
         */
        const result = String(invocation.result || '');

        if (/^Allowed on its card/.test(result)) return T('out.permit.yes');
        if (/told it may not/.test(result)) return T('out.permit.no');
        // A yes that could not be written down was handed over as a no.
        if (/could not be written/.test(result)) return T('out.permit.unrecorded');
        // Anything else is a question nobody waits for any more.
        if (error || result) return T('out.permit.gone');

        return T(approving ? 'out.permit.yes' : 'out.permit.no');
    }

    /*
     * Where a switch for what other programs may ask was changed, for its
     * line on the record. Not "who": the assistant's own page and a
     * program on this computer posting to the same address look the same,
     * so the record says where and nothing it cannot know. Empty for any
     * other deed.
     */
    function turnedWhere(d) {
        return d && d.tool === 'other_programs_switch' ? T('out.atDesk') : '';
    }

    window.pnPrograms = { summary, choices, keeping, changes, list, found, canDo, may, why, stopping, approvalChoices,
        outside, askedBy, outsideResult, outsideAsked, turnedWhere, permitDecided,
        reasons: Object.keys(WHY), answers: ANSWERS };

    // ---- drawing ----

    const el = (id) => document.getElementById(id);
    let last = null;
    let chosen = null;

    function say(id, text) {
        const node = el(id);
        if (node) node.textContent = text || '';
    }

    function row(item) {
        const b = document.createElement('button');
        b.type = 'button';
        b.className = 'program-row';

        const name = document.createElement('span');
        name.className = 'program-name';
        name.textContent = item.name;
        b.appendChild(name);

        const what = document.createElement('span');
        what.className = 'program-said';
        what.textContent = item.said;
        b.appendChild(what);

        b.addEventListener('click', () => open(item.program));

        return b;
    }

    function fill(id, items) {
        const box = el(id);
        if (!box) return;

        box.textContent = '';
        for (const item of items) box.appendChild(row(item));
    }

    function open(p) {
        chosen = p;
        saying = '';
        drawCard();

        const card = el('programs-card');
        if (card && card.scrollIntoView) card.scrollIntoView({ block: 'nearest' });
    }

    function drawCard() {
        const card = el('programs-card');
        if (!card) return;

        card.hidden = !chosen;
        if (!chosen) return;

        // The newest copy of the chosen program, after a reload.
        const now = ((last && last.programs) || []).find((p) => p.kind === chosen.kind && p.id === chosen.id) || chosen;

        say('programs-card-name', now.name || now.id);

        const box = el('programs-card-found');
        if (box) {
            box.textContent = '';

            for (const [label, value] of found(now, last)) {
                const dt = document.createElement('dt');
                dt.textContent = label;

                const dd = document.createElement('dd');
                dd.textContent = value;

                box.appendChild(dt);
                box.appendChild(dd);
            }
        }

        say('programs-card-can', canDo(now));
        drawMay(now);
    }

    // The card's choices: three buttons per kind of use, the chosen one
    // pressed; a press is saved at once and the card drawn again from the
    // server's answer.
    let saying = '';

    function drawMay(p) {
        const box = el('programs-card-may');
        if (!box) return;

        const m = may(p, last);
        box.textContent = '';

        for (const r of m.rows) {
            const item = document.createElement('div');
            item.className = 'permit program-permit';
            item.dataset.decision = r.answer === 'never' ? 'refuse' : r.answer;

            const text = document.createElement('span');
            text.className = 'permit-text';

            const name = document.createElement('span');
            name.className = 'permit-name as-typed';
            name.textContent = r.label;
            text.appendChild(name);

            const what = document.createElement('span');
            what.className = 'program-permit-state';
            what.textContent = r.state;
            text.appendChild(what);

            item.appendChild(text);

            const group = document.createElement('div');
            group.className = 'program-permit-choices';
            group.setAttribute('role', 'group');
            group.setAttribute('aria-label', r.label);

            for (const answer of ANSWERS) {
                const b = document.createElement('button');
                b.type = 'button';
                b.className = 'model-action';
                b.dataset.answer = answer;
                b.textContent = T(ANSWER[answer]);
                b.setAttribute('aria-pressed', String(answer === r.answer));
                b.addEventListener('click', () => decide(p, r.verb, answer));
                group.appendChild(b);
            }

            item.appendChild(group);
            box.appendChild(item);
        }

        say('programs-card-may-note', [m.note, saying].filter(Boolean).join(' '));
    }

    async function decide(p, verb, answer) {
        saying = '';

        try {
            const res = await fetch('/api/programs/permit', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
                body: JSON.stringify({ kind: p.kind, id: p.id, verb, answer }),
            });

            if (!res.ok) {
                // The server's reason is English; the page says it in
                // its own words, with the code for a phone (403).
                saying = T(res.status === 403 ? 'prg.mayDesk' : 'prg.mayFailed');
                drawCard();
                return;
            }
        } catch (err) {
            saying = T('prg.mayFailed');
            drawCard();
            return;
        }

        await load();
    }

    // The switch: its sentence, its button, and why a press failed.
    let stopSaying = '';

    function drawStop() {
        const box = el('programs-stop');
        if (!box) return;

        const s = stopping(last);
        box.dataset.stopped = String(s.stopped);
        say('programs-stop-said', s.said);

        const b = el('programs-stop-button');
        if (b) {
            b.textContent = s.button;
            b.classList.toggle('danger', !s.stopped);
        }

        say('programs-stop-note', stopSaying);
    }

    async function pressStop() {
        const resume = !!(last && last.stopped_since);
        stopSaying = '';

        try {
            const res = await fetch(resume ? '/api/programs/resume' : '/api/programs/stop', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
                body: '{}',
            });

            if (!res.ok) {
                stopSaying = T(res.status === 403 ? 'prg.stop.desk' : 'prg.stop.failed');
                drawStop();
                return;
            }
        } catch (err) {
            stopSaying = T('prg.stop.failed');
            drawStop();
            return;
        }

        await load();
    }

    /*
     * "What other programs may ask": the switches, and the last questions
     * with who asked. Its own answer (GET /api/outside), so a fault there
     * says so on this card and leaves the rest of the page as it is.
     */
    let lastOutside = null;
    let outsideFailed = false;
    let outsideSaying = '';

    // What was last drawn, so the page's own refresh every half minute
    // does not rebuild a switch somebody has their hand on.
    let outsideDrawn = '';

    function drawOutside() {
        const card = el('outside-card');
        if (!card) return;

        const now = JSON.stringify([lastOutside, outsideFailed, outsideSaying, bg()]);
        if (now === outsideDrawn) return;

        outsideDrawn = now;

        if (outsideFailed || !lastOutside) {
            say('outside-note', outsideFailed ? T('out.failed') : '');
            card.dataset.ready = 'false';
            return;
        }

        card.dataset.ready = 'true';

        const o = outside(lastOutside);

        say('outside-fixed-label', o.fixed.label);
        say('outside-fixed-state', o.fixed.state);

        const list = el('outside-switches');
        if (list) {
            list.textContent = '';

            for (const r of o.rows) {
                const item = document.createElement('li');
                item.className = 'outside-row';
                item.dataset.allowed = String(r.allowed);

                const label = document.createElement('label');
                label.className = 'outside-switch';

                const box = document.createElement('input');
                box.type = 'checkbox';
                box.id = 'outside-' + r.id;
                box.checked = r.allowed;
                box.addEventListener('change', () => setOutside(r.id, box.checked));

                const name = document.createElement('span');
                name.textContent = r.label;

                label.append(box, name);

                const state = document.createElement('span');
                state.className = 'outside-state';
                state.textContent = r.state;

                item.append(label, state);

                // What allowing it means, read before the box is ticked.
                if (r.means) {
                    const means = document.createElement('span');
                    means.className = 'outside-means';
                    means.textContent = r.means;
                    item.appendChild(means);
                }

                list.appendChild(item);
            }
        }

        const recent = el('outside-recent');
        if (recent) {
            recent.textContent = '';

            for (const a of o.recent) {
                const line = document.createElement('div');
                line.className = 'outside-asked';
                line.dataset.answered = String(a.answered);

                for (const [cls, text] of [['outside-when', a.when], ['outside-who', a.who], ['outside-what', a.what], ['outside-outcome', a.outcome]]) {
                    const part = document.createElement('span');
                    part.className = cls;
                    part.textContent = text;
                    line.appendChild(part);
                }

                recent.appendChild(line);
            }
        }

        say('outside-recent-note', o.none || T('out.recentNote'));
        say('outside-note', outsideSaying);
    }

    async function loadOutside() {
        try {
            const res = await fetch('/api/outside', { headers: { Accept: 'application/json' } });

            if (!res.ok) throw new Error(String(res.status));

            const body = await res.json();
            lastOutside = body && body.data ? body.data : body;
            outsideFailed = false;
        } catch (err) {
            outsideFailed = true;
        }

        drawOutside();
    }

    async function setOutside(subject, allowed) {
        outsideSaying = '';

        // Drawn again whatever comes back: the box was just ticked by hand,
        // and must end showing what the assistant keeps, also when the
        // answer is the same refusal as the last time.
        outsideDrawn = '';

        try {
            const res = await fetch('/api/outside/allow', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
                body: JSON.stringify({ subject, allowed }),
            });

            if (!res.ok) {
                // Two refusals with the same number: this page is not the
                // assistant's own window (desk.js), or it is another device.
                let why = null;

                try {
                    why = await res.json();
                } catch (err) {
                    why = null;
                }

                if (window.pnDesk && window.pnDesk.refused(res.status, why)) {
                    outsideSaying = T('desk.notHere');
                } else {
                    outsideSaying = T(res.status === 403 ? 'out.desk' : 'out.notKept');
                }
            } else {
                const body = await res.json();
                lastOutside = body && body.data ? body.data : body;
            }
        } catch (err) {
            outsideSaying = T('out.notKept');
        }

        // Drawn from what the assistant keeps, so a switch that was not
        // kept goes back to where it was.
        drawOutside();
    }

    function draw() {
        if (!last) return;

        say('programs-summary', summary(last));
        drawStop();
        drawOutside();

        const keep = el('programs-keeping');
        if (keep) {
            keep.textContent = '';

            for (const line of keeping(last)) {
                const li = document.createElement('li');
                li.textContent = line;
                keep.appendChild(li);
            }
        }

        const c = changes(last);
        fill('programs-changes', c.rows);
        say('programs-changes-note', c.total ? (c.more ? T('prg.moreChanges', { n: count(c.more) }) : '') : T('prg.noChanges', { days: RECENT_DAYS }));

        // Only the sources this computer has (and any a remembered
        // program came from) are offered.
        const select = el('programs-choice');
        const offered = choices(last);

        if (select) {
            for (const option of select.options) {
                const off = !offered.includes(option.value);
                option.hidden = off;
                option.disabled = off;
            }

            if (!offered.includes(select.value)) select.value = offered[0];
        }

        const l = list(last, select?.value || 'app', el('programs-search')?.value || '');
        fill('programs-list', l.rows);
        say('programs-list-note', l.note);

        drawCard();
    }

    async function load() {
        try {
            const res = await fetch('/api/programs');
            if (!res.ok) {
                say('programs-summary', T('prg.failed'));
                return;
            }

            const body = await res.json();
            last = body && body.data ? body.data : body;
            draw();
        } catch (err) {
            say('programs-summary', T('prg.failed'));
        }

        await loadOutside();
    }

    function wire() {
        document.querySelectorAll('[data-view="programs"]').forEach((b) => {
            if (b.tagName === 'BUTTON') b.addEventListener('click', load);
        });

        el('programs-choice')?.addEventListener('change', draw);
        el('programs-stop-button')?.addEventListener('click', pressStop);
        el('programs-search')?.addEventListener('input', draw);
        el('programs-card-close')?.addEventListener('click', () => {
            chosen = null;
            saying = '';
            drawCard();
        });

        window.addEventListener('pn-language', draw);

        // A program installed while the page is open shows up within a
        // minute; nothing is asked of the server while the page is closed.
        setInterval(() => {
            const view = document.querySelector('section.view[data-view="programs"]');
            if (view && !view.hidden) load();
        }, 30000);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
})();
