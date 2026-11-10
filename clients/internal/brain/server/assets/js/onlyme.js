/*
 * What "answer only me" says when it does not let something through.
 *
 * Two places say it: the voice line under the Talk button, when a turn was
 * heard and not acted on, and the Privacy page, while the switch shows on and
 * no voice can be recognised. Both used to show the server's English sentence
 * as it was, so a Bulgarian page explained itself in English. The server now
 * sends a key and its values beside the sentence (refused_key and
 * refused_vars on a turn, only_me_blocked_why on the voiceprint state), and
 * this says them in the page's language; the sentence is the fallback.
 *
 * Kept apart from console.js and voiceprint.js so the wording can be run on
 * the server's own answers in a test (TestAnswerOnlyMeIsSaidInBothLanguages).
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

    const bg = () => (window.pnLang ? window.pnLang() === 'bg' : false);

    // Why a voice could not be recognised, by the server's reason code.
    const WHY = {
        notInstalled: 'the voiceprint model is not installed',
        stale: 'the part that tells voices apart is from an older version and has not been updated yet',
        notTaught: 'no voice has been taught to this brain yet',
        nothingRecorded: 'nothing was recorded',
        unreadable: 'the recording could not be measured',
        withdrawn: 'telling voices apart is not offered in this version',
    };

    function why(code, english) {
        if (WHY[code]) return T('onlyme.why.' + code, null, WHY[code]);

        return english || '';
    }

    // Two decimals, with a decimal comma in Bulgarian.
    function number(value) {
        const n = Number(value);

        if (!Number.isFinite(n)) return '';

        const text = n.toFixed(2);

        return bg() ? text.replace('.', ',') : text;
    }

    // refused is why a turn was heard and not acted on.
    function refused(answer) {
        const vars = (answer && answer.refused_vars) || {};

        switch (answer && answer.refused_key) {
        case 'onlyme.notYou':
            return T('onlyme.notYou', { alike: number(vars.alike), match: number(vars.match) },
                'that is not your voice ({alike} alike, {match} needed)');
        case 'onlyme.unrecognised':
            // A reason this page has no words for: the server's sentence.
            if (!WHY[vars.why]) return answer.refused || '';

            return T('onlyme.unrecognised', { why: why(vars.why, '') },
                'answer only me is on, and your voice cannot be recognised now: {why}');
        default:
            return (answer && answer.refused) || '';
        }
    }

    // notActedOn is the voice line under the Talk button for such a turn.
    function notActedOn(answer) {
        return T('onlyme.notActedOn', { why: refused(answer) }, 'Not acted on: {why}');
    }

    // blocked is the Privacy page's row while the switch is on and no voice
    // can be recognised: its label and its value.
    function blocked(state) {
        return [
            T('onlyme.label', null, 'answer only me'),
            T('onlyme.blocked', { why: why(state.only_me_blocked_why, state.only_me_blocked) },
                'on, but your voice cannot be recognised now ({why}), so nothing it hears is acted on. '
                    + 'Type instead, or fetch what it needs again.'),
        ];
    }

    window.pnOnlyMe = { refused, notActedOn, blocked, reasons: Object.keys(WHY) };
})();
