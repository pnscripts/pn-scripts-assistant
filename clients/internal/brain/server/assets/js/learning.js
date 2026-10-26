/*
 * Learning: a lesson at the learner's level, practice questions one at a
 * time, and the topics with what is due again (server/study.go).
 *
 * A question arrives without its answer. The answer is sent, marked on the
 * server — by a rule, by the model as a judge, or by the learner when
 * neither can tell — and only then is the right answer shown. Lessons and
 * questions are a model's words, so everything is drawn with textContent;
 * the small Markdown reader below makes elements, never HTML.
 */
(function () {
    const el = (id) => document.getElementById(id);
    const T = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);

    let lastList = null;
    let lastLesson = null;
    let quiz = null; // { title, review, questions, at, right, answered, topic }

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

        if (!res.ok) {
            const err = new Error(data.error || `${res.status}`);
            err.status = res.status;
            throw err;
        }

        return data.data !== undefined ? data.data : data;
    }

    function say(text, bad) {
        const p = el('study-said');
        p.textContent = text || '';
        p.classList.toggle('is-bad', !!bad);
    }

    function failed(err) {
        say(err.status === 503 ? T('study.noModel') : T('writing.failed', { why: err.message }), true);
    }

    function busy(on) {
        for (const id of ['study-lesson', 'study-quiz', 'study-lesson-quiz']) {
            const b = el(id);
            if (b) b.disabled = on;
        }
    }

    const levelName = (name) => T(`study.level.${name}`);

    /* ---------- A lesson, as elements ---------- */

    // inline puts **bold** and `code` into a parent as elements.
    function inline(parent, text) {
        const parts = String(text).split(/(\*\*[^*]+\*\*|`[^`]+`)/g);

        for (const part of parts) {
            if (!part) continue;

            if (part.startsWith('**') && part.endsWith('**') && part.length > 4) {
                const b = document.createElement('strong');
                b.textContent = part.slice(2, -2);
                parent.append(b);
            } else if (part.startsWith('`') && part.endsWith('`') && part.length > 2) {
                const c = document.createElement('code');
                c.textContent = part.slice(1, -1);
                parent.append(c);
            } else {
                parent.append(document.createTextNode(part.replace(/\*([^*]+)\*/g, '$1')));
            }
        }
    }

    const ANSWERS = /^(answers?|отговори|отговорите)$/i;

    // drawLesson reads the lesson's Markdown into box and gives back its title.
    function drawLesson(box, text) {
        box.replaceChildren();

        let title = '';
        let target = box;
        let list = null;
        let code = null;
        let para = null;

        const close = () => { list = null; para = null; };

        for (const raw of String(text).split('\n')) {
            const line = raw.replace(/\s+$/, '');

            if (line.startsWith('```')) {
                if (code) {
                    code = null;
                } else {
                    close();
                    const pre = document.createElement('pre');
                    code = document.createElement('code');
                    pre.append(code);
                    target.append(pre);
                }
                continue;
            }

            if (code) {
                code.textContent += (code.textContent ? '\n' : '') + raw;
                continue;
            }

            if (!line.trim()) { close(); continue; }

            const heading = /^(#{1,4})\s+(.*)$/.exec(line);
            if (heading) {
                close();
                const words = heading[2].replace(/[*#:]+$/g, '').trim();

                if (heading[1].length === 1 && !title) { title = words; continue; }

                // The answers to "check yourself" stay folded until asked for.
                if (ANSWERS.test(words)) {
                    const details = document.createElement('details');
                    details.className = 'study-answers';
                    const summary = document.createElement('summary');
                    summary.textContent = T('study.answers');
                    details.append(summary);
                    box.append(details);
                    target = details;
                    continue;
                }

                target = box;
                const h = document.createElement('h3');
                h.textContent = words;
                box.append(h);
                continue;
            }

            const item = /^\s*(?:[-*•]|(\d+)[.)])\s+(.*)$/.exec(line);
            if (item) {
                para = null;
                const kind = item[1] ? 'OL' : 'UL';

                if (!list || list.tagName !== kind) {
                    list = document.createElement(kind.toLowerCase());
                    if (item[1] && item[1] !== '1') list.start = Number(item[1]);
                    target.append(list);
                }

                const li = document.createElement('li');
                inline(li, item[2]);
                list.append(li);
                continue;
            }

            // A line under a list item belongs to it.
            if (list && /^\s{2,}/.test(raw) && list.lastElementChild) {
                list.lastElementChild.append(document.createElement('br'));
                inline(list.lastElementChild, line.trim());
                continue;
            }

            list = null;

            if (!para) {
                para = document.createElement('p');
                target.append(para);
            } else {
                para.append(' ');
            }

            inline(para, line.trim());
        }

        return title;
    }

    function showLesson(topic, model) {
        lastLesson = { topic, model };

        const card = el('study-lesson-card');
        const title = drawLesson(el('study-lesson-text'), topic.lesson || '');

        el('study-lesson-title').textContent = title || topic.topic;

        const meta = [levelName(topic.level)];
        if (model) meta.push(T('study.by', { model }));
        el('study-lesson-meta').textContent = meta.join(' · ');

        card.hidden = false;
        card.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    /* ---------- The form ---------- */

    function form() {
        return {
            topic: el('study-topic').value.trim(),
            level: el('study-level').value,
            language: el('study-language').value,
            minutes: Number(el('study-minutes').value),
            count: Number(el('study-count').value),
        };
    }

    async function teach() {
        const f = form();
        if (!f.topic) { say(T('study.needTopic'), true); el('study-topic').focus(); return; }

        busy(true);
        say(T('study.writingLesson'));

        try {
            const done = await send('/api/study/lesson', f);
            say(T('study.lessonReady'));
            showLesson(done.topic, done.model);
            await loadList();
        } catch (err) {
            failed(err);
        } finally {
            busy(false);
        }
    }

    async function practise(over) {
        const f = Object.assign(form(), over || {});
        if (!f.review && !f.topic) { say(T('study.needTopic'), true); el('study-topic').focus(); return; }

        busy(true);
        if (!f.review) say(T('study.writingQuiz'));

        try {
            const made = await send('/api/study/quiz', f);

            if (!made.questions.length) {
                say(T('study.nothingDue'));
                return;
            }

            let line = T('study.quizReady', { n: made.questions.length });
            if (made.dropped) line += ' ' + T('study.dropped', { n: made.dropped });
            say(made.review ? '' : line);

            startQuiz(made);
            await loadList();
        } catch (err) {
            if (f.review) say(T('writing.failed', { why: err.message }), true);
            else failed(err);
        } finally {
            busy(false);
        }
    }

    /* ---------- One question at a time ---------- */

    function startQuiz(made) {
        quiz = {
            review: made.review,
            topic: made.topic && made.topic.topic,
            questions: made.questions,
            at: 0,
            right: 0,
            answered: false,
        };

        el('study-quiz-card').hidden = false;
        drawQuestion();
        el('study-quiz-card').scrollIntoView({ behavior: 'smooth', block: 'start' });
    }

    function drawQuestion() {
        const q = quiz.questions[quiz.at];
        const n = quiz.questions.length;

        el('study-quiz-title').textContent = quiz.review
            ? T('study.reviewTitle') + (quiz.topic ? ` · ${quiz.topic}` : '')
            : `${T('study.quizTitle')} · ${quiz.topic || ''}`;
        el('study-quiz-count').textContent = T('study.progress', { i: quiz.at + 1, n });
        el('study-bar').style.width = `${Math.round((quiz.at / n) * 100)}%`;
        el('study-question').textContent = q.question;

        const options = el('study-options');
        options.replaceChildren();

        const choice = q.kind === 'choice' && Array.isArray(q.options);
        el('study-answer-row').hidden = choice;
        options.hidden = !choice;

        if (choice) {
            q.options.forEach((text, i) => {
                const b = document.createElement('button');
                b.type = 'button';
                b.className = 'study-option';
                b.dataset.letter = 'ABCDEF'[i];

                const letter = document.createElement('span');
                letter.className = 'study-letter';
                letter.textContent = 'ABCDEF'[i];

                const words = document.createElement('span');
                words.textContent = text;

                b.append(letter, words);
                b.addEventListener('click', () => answer(text, b));
                options.append(b);
            });
        }

        const input = el('study-answer');
        input.value = '';
        input.disabled = false;
        input.inputMode = q.kind === 'number' ? 'decimal' : 'text';
        el('study-check').disabled = false;

        el('study-verdict').hidden = true;
        el('study-self').hidden = true;
        el('study-next').hidden = true;
        quiz.answered = false;

        if (!choice) setTimeout(() => input.focus({ preventScroll: true }), 50);
    }

    function whenText(due) {
        const ms = new Date(due) - new Date();

        if (ms < 3600e3) return T('study.soon');
        if (ms < 36 * 3600e3) return T('study.tomorrow');

        return T('study.inDays', { n: Math.round(ms / 86400e3) });
    }

    function showVerdict(m) {
        const head = el('study-verdict-head');
        const box = el('study-verdict');

        box.hidden = false;
        box.classList.remove('is-right', 'is-wrong', 'is-unsure');

        if (!m.sure) {
            box.classList.add('is-unsure');
            head.textContent = T('study.unsure', { answer: m.expected });
            el('study-verdict-why').textContent = m.explain || '';
            el('study-self').hidden = false;
            return;
        }

        box.classList.add(m.right ? 'is-right' : 'is-wrong');
        head.textContent = m.right ? T('study.right') : T('study.wrong', { answer: m.expected });

        const who = m.by === 'rule' ? T('study.byRule')
            : m.by === 'learner' ? T('study.byLearner') : T('study.byModel', { model: m.by });
        const when = m.learnt ? T('study.learnt') : T('study.again', { when: whenText(m.question.due) });

        el('study-verdict-why').textContent = [m.explain, when, T('study.markedBy', { who })].filter(Boolean).join(' ');
        el('study-self').hidden = true;

        if (m.right) quiz.right++;
        quiz.answered = true;

        const last = quiz.at === quiz.questions.length - 1;
        const next = el('study-next');
        next.textContent = last ? T('study.finish') : T('study.next');
        next.hidden = false;
        next.focus({ preventScroll: true });
    }

    async function answer(given, button, self) {
        if (quiz.answered) return;

        const q = quiz.questions[quiz.at];

        el('study-check').disabled = true;
        el('study-answer').disabled = true;

        for (const b of el('study-options').querySelectorAll('button')) b.disabled = true;
        if (button) button.classList.add('is-picked');

        try {
            const body = { card: q.id, answer: given };
            if (self !== undefined) body.self = self;

            const m = await send('/api/study/answer', body);

            if (q.kind === 'choice') {
                for (const b of el('study-options').querySelectorAll('button')) {
                    const text = b.lastElementChild.textContent;

                    if (m.sure && m.expected.endsWith(text) && m.expected.length <= text.length + 3) b.classList.add('is-right');
                    else if (b.classList.contains('is-picked') && m.sure && !m.right) b.classList.add('is-wrong');
                }
            }

            showVerdict(m);
        } catch (err) {
            say(T('writing.failed', { why: err.message }), true);
            el('study-check').disabled = false;
            el('study-answer').disabled = false;
            for (const b of el('study-options').querySelectorAll('button')) b.disabled = false;
        }
    }

    function next() {
        if (quiz.at < quiz.questions.length - 1) {
            quiz.at++;
            drawQuestion();
            return;
        }

        const n = quiz.questions.length;

        el('study-bar').style.width = '100%';
        el('study-question').textContent = T('study.done', { right: quiz.right, n });
        el('study-options').replaceChildren();
        el('study-options').hidden = true;
        el('study-answer-row').hidden = true;
        el('study-next').hidden = true;

        const box = el('study-verdict');
        box.hidden = false;
        box.classList.remove('is-right', 'is-wrong', 'is-unsure');
        box.classList.add(quiz.right === n ? 'is-right' : 'is-unsure');
        el('study-verdict-head').textContent = quiz.right === n ? T('study.doneAll') : T('study.doneMissed');
        el('study-verdict-why').textContent = '';
        el('study-self').hidden = true;

        loadList();
    }

    function stop() {
        quiz = null;
        el('study-quiz-card').hidden = true;
        loadList();
    }

    /* ---------- The topics ---------- */

    function drawList(data) {
        lastList = data;

        const ul = el('study-list');
        ul.replaceChildren();

        const topics = data.topics || [];
        const datalist = el('study-topics');
        datalist.replaceChildren();

        let cards = 0;
        let learnt = 0;

        for (const t of topics) {
            cards += t.cards;
            learnt += t.learnt;

            const option = document.createElement('option');
            option.value = t.topic;
            datalist.append(option);

            const li = document.createElement('li');
            li.className = 'writing-item study-item';
            if (t.due) li.classList.add('is-due');

            const main = document.createElement('div');
            main.className = 'study-item-main';

            const name = document.createElement('strong');
            name.textContent = t.topic;

            const level = document.createElement('span');
            level.className = 'plan-chip';
            level.textContent = levelName(t.level);

            const answered = t.right + t.wrong;
            const score = answered ? T('study.score', { pct: Math.round((t.right * 100) / answered) }) : T('study.notYet');

            const stats = document.createElement('small');
            stats.className = 'study-stats';
            stats.textContent = T('study.stats', { cards: t.cards, learnt: t.learnt, score })
                + (t.due ? ` · ${T('study.due', { n: t.due })}` : '');

            if (t.cards) {
                const meter = document.createElement('span');
                meter.className = 'study-meter';
                meter.setAttribute('aria-hidden', 'true');
                const fill = document.createElement('span');
                fill.style.width = `${Math.round((t.learnt / t.cards) * 100)}%`;
                meter.append(fill);
                main.append(name, level, stats, meter);
            } else {
                main.append(name, level, stats);
            }

            const actions = document.createElement('div');
            actions.className = 'study-item-actions';

            const button = (label, fn, cls) => {
                const b = document.createElement('button');
                b.type = 'button';
                b.className = `model-action ${cls || 'quiet'}`;
                b.textContent = label;
                b.addEventListener('click', () => fn(b));
                actions.append(b);
                return b;
            };

            if (t.due) {
                button(T('study.review', { n: t.due }), () => practise({ review: true, topic: t.topic, level: t.level, count: 10 }), ' ');
            }

            button(T('study.more'), () => practise({ topic: t.topic, level: t.level, language: t.lang }));
            button(T('study.lesson'), () => openLesson(t));
            button(T('study.remove'), (b) => removeTopic(t, b));

            li.append(main, actions);
            ul.append(li);
        }

        el('study-empty').hidden = topics.length > 0;
        el('study-counts').textContent = topics.length
            ? T('study.counts', { topics: topics.length, cards, learnt })
            : '';

        const all = el('study-review-all');
        all.hidden = !data.due;
        all.textContent = T('study.reviewAll', { n: data.due });

        const badge = el('nav-learning');
        if (badge) {
            badge.hidden = !data.due;
            badge.textContent = String(data.due || 0);
        }
    }

    async function loadList() {
        try {
            drawList(await get('/api/study'));
        } catch (err) {
            say(T('writing.failed', { why: err.message }), true);
        }
    }

    async function openLesson(t) {
        try {
            const data = await get(`/api/study/${t.id}`);

            el('study-topic').value = t.topic;
            el('study-level').value = t.level;

            if (!data.topic.lesson) {
                say(T('study.noLesson'));
                return;
            }

            showLesson(data.topic, '');
        } catch (err) {
            say(T('writing.failed', { why: err.message }), true);
        }
    }

    async function removeTopic(t, button) {
        if (!button.classList.contains('danger')) {
            button.classList.add('danger');
            button.textContent = T('gl.sure');
            setTimeout(() => { button.classList.remove('danger'); button.textContent = T('study.remove'); }, 4000);
            return;
        }

        try {
            await send(`/api/study/${t.id}/remove`);
            say(T('study.removed'));

            if (lastLesson && lastLesson.topic.id === t.id) el('study-lesson-card').hidden = true;

            await loadList();
        } catch (err) {
            say(T('writing.failed', { why: err.message }), true);
        }
    }

    function wire() {
        if (!el('study-list')) return;

        el('study-lesson').addEventListener('click', teach);
        el('study-quiz').addEventListener('click', () => practise());
        el('study-lesson-quiz').addEventListener('click', () => {
            if (lastLesson) {
                el('study-topic').value = lastLesson.topic.topic;
                el('study-level').value = lastLesson.topic.level;
            }
            practise();
        });
        el('study-lesson-close').addEventListener('click', () => { el('study-lesson-card').hidden = true; });
        el('study-check').addEventListener('click', () => {
            const given = el('study-answer').value.trim();
            if (given) answer(given);
            else el('study-answer').focus();
        });
        el('study-answer').addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                el('study-check').click();
            }
        });
        el('study-self-right').addEventListener('click', () => answer(el('study-answer').value.trim() || '-', null, true));
        el('study-self-wrong').addEventListener('click', () => answer(el('study-answer').value.trim() || '-', null, false));
        el('study-next').addEventListener('click', next);
        el('study-stop').addEventListener('click', stop);
        el('study-review-all').addEventListener('click', () => practise({ review: true, topic: '', count: 10 }));
        el('study-topic').addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                teach();
            }
        });

        // Keys 1–6 or A–F pick an option.
        document.addEventListener('keydown', (e) => {
            const card = el('study-quiz-card');
            if (!quiz || quiz.answered || card.hidden || !card.offsetParent) return;
            if (e.ctrlKey || e.metaKey || e.altKey || /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName)) return;

            const key = e.key.length === 1 ? e.key.toUpperCase() : '';
            const i = key && '123456'.includes(key) ? Number(key) - 1 : (key ? 'ABCDEF'.indexOf(key) : -1);
            const b = el('study-options').children[i];

            if (i >= 0 && b && !b.disabled) b.click();
        });

        for (const item of document.querySelectorAll('.nav-item[data-view="learning"]')) {
            item.addEventListener('click', loadList);
        }

        window.addEventListener('pn-language', () => {
            if (lastList) drawList(lastList);
            if (lastLesson && !el('study-lesson-card').hidden) showLesson(lastLesson.topic, lastLesson.model);
            if (quiz && !quiz.answered && quiz.at < quiz.questions.length) {
                el('study-quiz-count').textContent = T('study.progress', { i: quiz.at + 1, n: quiz.questions.length });
            }
        });

        loadList();
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', wire);
    } else {
        wire();
    }
}());
