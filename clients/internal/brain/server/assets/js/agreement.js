/*
 * The terms of use, accepted once before anything else.
 *
 * The words are the server's (/api/terms), drawn as text in the language the
 * page is in, and drawn again when the language changes. One button accepts;
 * the version shown goes with it, so terms changed by an update while this
 * page was open are not accepted unread. Accepted, the page loads again and
 * the server hands over the console.
 */
(function () {
    const T = (key) => (window.pnT ? window.pnT(key) : key);
    const lang = () => (window.pnLang ? window.pnLang() : 'en');

    const text = document.getElementById('agreement-text');
    const lead = document.getElementById('agreement-lead');
    const version = document.getElementById('agreement-version');
    const accept = document.getElementById('agreement-accept');
    const language = document.getElementById('agreement-language');
    const docs = document.getElementById('agreement-docs');
    const error = document.getElementById('agreement-error');

    let terms = null;

    function say(message) {
        error.textContent = message || '';
        error.hidden = !message;
    }

    function draw() {
        // The button names the other language, in that language.
        language.textContent = lang() === 'bg' ? 'English' : 'Български';
        language.lang = lang() === 'bg' ? 'en' : 'bg';

        if (!terms) return;

        const shown = lang() === 'bg' ? terms.bg : terms.en;

        lead.textContent = shown.lead;
        version.textContent = shown.version;
        text.replaceChildren();

        for (const part of shown.sections) {
            const heading = document.createElement('h3');
            heading.textContent = part.heading;

            const body = document.createElement('p');
            body.textContent = part.body;

            text.append(heading, body);
        }

        accept.disabled = false;
    }

    async function load() {
        try {
            const res = await fetch('/api/terms', { headers: { Accept: 'application/json' } });
            if (!res.ok) throw new Error(String(res.status));

            terms = await res.json();

            // Accepted in another window meanwhile: nothing to ask.
            if (terms.accepted) {
                location.reload();

                return;
            }

            say('');
            draw();
        } catch (err) {
            say(T('terms.unreadable'));
        }
    }

    language.addEventListener('click', () => {
        if (window.pnSetLanguage) window.pnSetLanguage(lang() === 'bg' ? 'en' : 'bg');
    });

    // The documentation opens in the browser: the window cannot follow links.
    docs.addEventListener('click', () => {
        if (!terms || !terms.online) return;

        fetch('/api/open-source', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ where: lang() === 'bg' ? terms.online.bg : terms.online.en }),
        }).catch(() => {});
    });

    accept.addEventListener('click', async () => {
        if (!terms) return;

        accept.disabled = true;
        say('');

        try {
            const res = await fetch('/api/terms', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ version: terms.version, language: lang() }),
            });

            if (res.status === 409) {
                say(T('terms.changed'));
                await load();

                return;
            }

            if (!res.ok) {
                const body = await res.json().catch(() => ({}));
                say(T('terms.notSaved') + (body.error ? ' ' + body.error : ''));
                accept.disabled = false;

                return;
            }

            location.reload();
        } catch (err) {
            say(T('terms.notSaved'));
            accept.disabled = false;
        }
    });

    window.addEventListener('pn-language', draw);

    draw();
    load();
})();
