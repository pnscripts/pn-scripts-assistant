/*
 * The conversation already on the page follows the page's language.
 *
 * A message was drawn once, in the language of that moment. Switching the
 * page to Bulgarian turned every panel around it and left the conversation
 * as it was: "you" above what was typed, "Read notes.txt" and "1.5s" under
 * an answer, until a reload drew it again. What the two of you said is
 * never touched; who said it, what was done under an answer and how long
 * each thing took are the page's own words, and those are said again.
 *
 * Kept apart from console.js so it can be run without a browser: the test
 * draws a message, switches the language and reads what the page says.
 */
(function () {
    const words = (key, vars) => (window.pnT ? window.pnT(key, vars) : key);
    const inBulgarian = () => Boolean(window.pnLang && window.pnLang() === 'bg');

    // The three speakers that are not the assistant, named in the page's language.
    const WHO = { you: 'cc.who.you', error: 'cc.who.error', system: 'cc.who.system' };

    // Who said a message: one of the three above, or the name as given.
    function who(name) {
        return WHO[name] ? words(WHO[name]) : name;
    }

    // How long one thing took, with the decimal mark of the page's language.
    function took(millis) {
        if (millis >= 1000) {
            const figure = (millis / 1000).toFixed(1);

            return words('cc.seconds', { n: inBulgarian() ? figure.replace('.', ',') : figure });
        }

        return words('cc.millis', { n: millis || 0 });
    }

    /*
     * steps draws what the assistant did, one openable line each.
     *
     * The chips it replaces said "Search the web" and stopped there, which
     * is a category of action rather than an action: a tool called with the
     * wrong argument and a tool that is merely slow produced the same chip.
     * Each line here opens onto what was actually asked for and what came
     * back, so watching it work does not mean trusting a summary.
     *
     * Closed by default. Somebody reading an answer has not asked for four
     * screens of tool output, and the whole point of the summary is that it
     * is usually enough.
     */
    function steps(list) {
        const box = document.createElement('div');
        box.className = 'steps';

        list.forEach((s) => {
            const row = document.createElement('details');
            row.className = 'step' + (s.failed ? ' failed' : '');

            const head = document.createElement('summary');

            const name = document.createElement('span');
            name.className = 'step-what';
            name.textContent = (window.pnDidLine ? window.pnDidLine(s) : s.summary || s.tool)
                || words('cc.didSomething');
            head.appendChild(name);

            // How long it took, beside what it was. The number is the
            // difference between "it is thinking" and "it is stuck", and it
            // is never visible anywhere else once the answer has arrived.
            const time = document.createElement('span');
            time.className = 'step-time';
            time.textContent = took(s.millis);
            head.appendChild(time);

            row.appendChild(head);

            const detail = document.createElement('div');
            detail.className = 'step-detail';

            if (s.tool) {
                const t = document.createElement('div');
                t.className = 'step-tool';
                t.textContent = s.tool;
                detail.appendChild(t);

                // On a Bulgarian page the name above is the tool's own words
                // in Bulgarian when the step carries them. When it does not,
                // the name is the tool's, and its English summary is kept here.
                if (inBulgarian() && s.summary && !s.summary_bg) {
                    const said = document.createElement('div');
                    said.className = 'step-tool';
                    said.textContent = s.summary;
                    detail.appendChild(said);
                }
            }

            if (s.asked) {
                const a = document.createElement('pre');
                a.className = 'step-asked';
                // textContent throughout: a tool result can hold a web page
                // it just fetched, and rendering that as markup would turn
                // something the model read into script running in this console.
                a.textContent = s.asked;
                detail.appendChild(a);
            }

            if (s.result) {
                const r = document.createElement('pre');
                r.className = 'step-result';
                r.textContent = s.result;
                detail.appendChild(r);
            }

            row.appendChild(detail);
            box.appendChild(row);
        });

        return box;
    }

    // Every message on the page that has words of the page's own in it.
    let drawn = [];

    /*
     * keep remembers a drawn message so a change of language can say it
     * again: label is where who said it is written, who the speaker as
     * given, steps what was done under it and box where steps() drew them,
     * also whatever else the message redraws for itself.
     */
    function keep(wrap, parts) {
        drawn.push({ wrap, ...parts });
    }

    /*
     * redraw says every kept message in the page's language.
     *
     * A line somebody opened stays open: the language was changed, not the
     * place they were reading. Messages no longer on the page (a new
     * conversation cleared them) are forgotten rather than drawn.
     */
    function redraw() {
        drawn = drawn.filter((message) => message.wrap.isConnected !== false);

        drawn.forEach((message) => {
            if (message.label) message.label.textContent = who(message.who);

            if (message.box && message.steps && message.steps.length) {
                const fresh = steps(message.steps);

                Array.from(message.box.children).forEach((row, i) => { fresh.children[i].open = row.open; });

                message.box.replaceWith(fresh);
                message.box = fresh;
            }

            if (message.also) message.also();
        });
    }

    window.addEventListener('pn-language', redraw);

    window.pnTranscript = { who, took, steps, keep, redraw };
})();
