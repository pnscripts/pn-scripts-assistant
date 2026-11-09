/*
 * What the status pill says about the connection.
 *
 * It said "online" and named the configured model whenever the page could
 * reach this program, which is a different question from whether anything
 * can answer: with Ollama stopped the pill was green and every question
 * failed. The answer is in /api/status already (providers[].reachable of the
 * default provider), so the pill follows it and offers the way to the
 * explanation (the Models view, which says what is not answering and why).
 *
 * Kept apart from console.js so the decision can be run on the server's own
 * answer in a test (TestTheStatusPillFollowsTheModel).
 */
(function () {
    const T = (key, vars, english) => {
        const said = window.pnT ? window.pnT(key, vars) : key;

        if (said !== key) return said;

        // Before i18n.js has loaded, or in a test without it.
        let text = english;

        for (const [name, value] of Object.entries(vars || {})) {
            text = text.split('{' + name + '}').join(String(value));
        }

        return text;
    };

    function connection(status) {
        const count = (status.capabilities || []).length;
        const providers = status.providers || [];
        const inUse = providers.find((p) => p.default)
            || providers.find((p) => p.name === status.provider);

        if (inUse && !inUse.permitted) {
            return {
                tone: 'warn',
                text: T('conn.forbidden', null, 'online · the model in use is not allowed in this privacy mode'),
                title: T('conn.forbiddenWhy', { name: inUse.name },
                    '{name} is not allowed by the privacy setting, so questions cannot be answered. Models shows what can be used.'),
                why: true,
            };
        }

        if (inUse && !inUse.reachable) {
            return {
                tone: 'warn',
                text: T('conn.silent', null, 'online · the model is not answering'),
                title: T('conn.silentWhy', { name: inUse.name },
                    '{name} does not answer, so questions cannot be answered. Models shows what is wrong and what to do.'),
                why: true,
            };
        }

        return {
            tone: 'online',
            text: T('conn.online', { n: count }, 'online · {n} capabilities'),
            title: '',
            why: false,
        };
    }

    window.pnConnection = connection;
})();
