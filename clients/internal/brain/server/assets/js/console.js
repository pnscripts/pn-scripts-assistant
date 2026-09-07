/*
 * PN Brain console.
 *
 * Vanilla, no framework and no build step: this ships inside a single
 * self-contained binary, and the whole surface is one chat plus a few polled
 * panels. A bundler would add a build stage to the static image for very
 * little.
 *
 * Everything here talks to the same JSON API the CLI and desktop app use, so
 * the interface stays one client among several rather than a privileged path.
 */

/*
 * The reason, not the number.
 *
 * Every failure here used to read "/api/turn -> 400", which names the endpoint
 * that failed and nothing about why — and the server had put a sentence in the
 * body explaining it. Petar got that string in red in the middle of his
 * conversation, twice, with no way to tell a busy microphone from a broken
 * one. The other panels in this program already read the body; the console,
 * which is the one people look at, did not.
 */
async function why(res, path) {
    let said = '';

    try {
        const data = await res.json();

        said = (data && data.error) || '';
    } catch {
        said = '';
    }

    return new Error(said || `that did not work (${path} — ${res.status})`);
}

const api = {
    async get(path) {
        const res = await fetch(path, { headers: { Accept: 'application/json' } });
        if (!res.ok) throw await why(res, path);
        return res.json();
    },
    async post(path, body, signal) {
        const res = await fetch(path, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
            body: body === undefined ? undefined : JSON.stringify(body),
            // Passed through so a turn can be abandoned. Cancelling the request
            // cancels the context on the other side, which is what actually
            // stops the model rather than just stopping the voice.
            signal,
        });
        if (!res.ok) throw await why(res, path);
        return res.json();
    },
};

const el = (id) => document.getElementById(id);

const state = {
    conversationId: null,
    brainName: 'PN Brain',
    // What has to be said before it answers, or empty to answer everything.
    wakeWord: '',
    // Whether the name is needed on every sentence rather than once.
    alwaysName: true,
    /*
     * Nobody has told it what to call itself yet.
     *
     * True only while the naming card is up on a brand new brain. Everything
     * that would reach the room — the opening line and the microphone — waits
     * for that card, because a program that starts talking and listening over
     * a question it is still asking has not asked anything.
     */
    firstRun: false,
    busy: false,

    /*
     * The turn in flight, so it can be abandoned.
     *
     * Stopping used to mean stopping the voice: the model carried on
     * generating an answer nobody wanted, holding the only processor on the
     * machine, and the next thing anybody said queued behind it. On hardware
     * where an answer runs to a minute that is the difference between changing
     * your mind and waiting to be allowed to.
     */
    inFlight: null,
};

/* ---------- transcript ---------- */

function clearBoot() {
    el('boot')?.remove();
}

function addMessage(who, text, { cssClass = '', meta = '', actions = [] } = {}) {
    clearBoot();

    const wrap = document.createElement('div');
    wrap.className = `msg ${cssClass}`;

    const label = document.createElement('div');
    label.className = 'who';
    label.textContent = who;
    wrap.appendChild(label);

    const body = document.createElement('div');
    body.className = 'body';
    // textContent, never innerHTML: replies can contain fetched web content,
    // and rendering that as markup would turn a page the model read into
    // script running in this console.
    body.textContent = text;
    wrap.appendChild(body);

    if (actions.length) {
        const row = document.createElement('div');
        row.className = 'actions';
        actions.forEach((a) => {
            const chip = document.createElement('span');
            chip.className = 'action-chip';
            chip.textContent = a;
            row.appendChild(chip);
        });
        wrap.appendChild(row);
    }

    if (meta) {
        const m = document.createElement('div');
        m.className = 'meta';
        m.textContent = meta;
        wrap.appendChild(m);
    }

    const transcript = el('transcript');
    transcript.appendChild(wrap);
    transcript.scrollTop = transcript.scrollHeight;
    return wrap;
}

function showThinking() {
    clearBoot();
    const wrap = document.createElement('div');
    wrap.className = 'msg brain thinking';
    wrap.innerHTML = '<div class="who"></div><div class="thinking-line">thinking…</div>';
    wrap.querySelector('.who').textContent = state.brainName;
    el('transcript').appendChild(wrap);
    el('transcript').scrollTop = el('transcript').scrollHeight;
    el('orb').classList.add('thinking');
    return wrap;
}

/*
 * Which conversation this is, said on the screen.
 *
 * Opening the program dropped you back into the last conversation whatever its
 * age, and said nothing about it — so a sentence typed this morning joined a
 * thread from Friday night, and the only way to find out was to go looking in
 * the history afterwards.
 *
 * A marker rather than a message: it is not something the brain said, it is a
 * note about where you are, and dressing it up as speech would put words in
 * its mouth that it never uttered.
 */
function markThread(text) {
    if (!text) return;

    clearBoot();

    const mark = document.createElement('div');
    mark.className = 'thread-mark';
    mark.textContent = text;

    el('transcript').appendChild(mark);
}

// When something happened, in the words somebody would use out loud.
function whenSaid(iso) {
    const at = new Date(iso);

    if (Number.isNaN(at.getTime())) return '';

    const minutes = (Date.now() - at.getTime()) / 60000;

    if (minutes < 60) return `${Math.max(1, Math.round(minutes))} minutes ago`;

    const sameDay = at.toDateString() === new Date().toDateString();

    return sameDay
        ? `at ${at.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`
        : at.toLocaleString([], {
            day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit',
        });
}

/* ---------- sending ---------- */

async function send(text) {
    if (state.busy || !text.trim()) return;

    state.busy = true;
    el('send').disabled = true;
    addMessage('you', text, { cssClass: 'you' });

    const placeholder = showThinking();

    // A typed message lights the reactor too, so the picture reflects what the
    // brain is doing whether or not anybody is talking to it.
    if (window.brainMapState) window.brainMapState('thinking');

    /*
     * Held so it can be abandoned mid-answer.
     *
     * Cancelling the request cancels the context on the other side, which
     * stops the model rather than only stopping the voice — the difference
     * between changing your mind and waiting to be allowed to.
     */
    const abandon = new AbortController();

    state.inFlight = abandon;

    // And the microphone stays open on its name while it works, so cutting in
    // does not require finding the keyboard.
    watchForYou(abandon);

    /*
     * Which is worth saying, in the place that otherwise contradicts it.
     *
     * The line under the microphone read "Say “PN Brain” to start" all the way
     * through an answer — an invitation to begin, printed over a brain that
     * was already halfway through something, while the word above it said
     * Thinking. Two states at once, and neither of them the useful one: what
     * somebody wants to know mid-answer is that they can cut in.
     */
    if (talking.on) {
        setVoiceStatus('Say anything to cut in');
    }

    try {
        const data = await api.post('/api/chat', {
            conversation_id: state.conversationId,
            message: text,
            provider: el('provider').value || null,
        }, abandon.signal);

        state.conversationId = data.conversation_id;
        placeholder.remove();

        addMessage(state.brainName, data.reply, {
            cssClass: 'brain',
            meta: `${data.provider} · ${data.model}`,
            actions: data.actions_taken || [],
        });

        // A pause for approval is the interesting case, so surface it
        // immediately instead of waiting for the next poll.
        // Show which memories the brain actually reached for.
        if (data.recalled?.length && window.brainMapRecall) {
            window.brainMapRecall(data.recalled);
        }

        speak(data.reply);

        if (window.brainMapState) {
            window.brainMapState(talking.on ? 'listening' : 'idle');
        }

        if (data.pending_approvals?.length) refreshApprovals();
        refreshStatus();
        refreshActivity();
    } catch (err) {
        placeholder.remove();

        /*
         * Being stopped is not an error, and must not read as one.
         *
         * An abandoned turn arriving in the transcript as a red failure would
         * make deliberately changing your mind look like something went wrong
         * — and the next thing in the transcript is whatever was said instead,
         * which needs a line above it saying why the answer is missing.
         */
        if (err.name === 'AbortError') {
            markThread('Stopped — that answer was not finished');
        } else {
            addMessage('error', String(err.message || err), { cssClass: 'error' });
        }
    } finally {
        state.inFlight = null;
        state.busy = false;
        el('send').disabled = false;
        el('orb').classList.remove('thinking');
        el('talk').onclick = toggleTalking;
    el('input').focus();
    }
}

/* ---------- panels ---------- */

async function refreshStatus() {
    try {
        const s = await api.get('/api/status');
        state.brainName = s.name || state.brainName;
        state.wakeWord = s.wake_word || '';
        state.alwaysName = s.always_name !== false;
        state.firstRun = s.first_run === true;
        el('brain-name').textContent = s.name;
        el('engine-meta').textContent = `${s.provider} · ${s.model}`;
        renderPrivacy(s.privacy);
        renderStorage(s.storage);

        // The gauges around the reactor show these, so they move for a reason.
        if (window.brainMapGauges) {
            const used = s.storage && s.storage.total_bytes
                ? 1 - s.storage.free_bytes / s.storage.total_bytes
                : 0;

            window.brainMapGauges({
                storage: used,
                // A full queue is ten waiting; more than that is still full.
                queue: Math.min(1, (s.memory.pending_lessons || 0) / 10),
            });
        }
        el('conn-dot').className = 'dot online';
        el('conn-text').textContent = `online · ${s.capabilities.length} capabilities`;
        // Only offer to speak when there is something that can. A control for
        // a capability the machine lacks is a promise the app cannot keep.
        // Talking needs both halves: hearing you and answering aloud.
        const canTalk = s.capabilities.includes('listening')
            && s.capabilities.includes('speech');

        el('talk').hidden = !canTalk;
        el('microphone-field').hidden = !canTalk;

        el('voice-field').hidden = !s.capabilities.includes('speech');
        if (s.capabilities.includes('speech')) loadVoices();

        if (canTalk) {
            loadMicrophones();

            /*
             * On by default: somebody who has a microphone and a voice
             * installed wants to talk to it, and having to switch that on
             * every time is a small tax on the thing they came for. Started
             * once per session, and only if they have not already stopped it.
             *
             * Not while the naming card is up. Opening the microphone there
             * meant a brand new brain sat listening to the room, ready to
             * answer to a name its owner had not chosen yet, behind a dialog
             * covering the screen — and the first thing it would ever learn
             * would have been overheard rather than said to it.
             */
            if (!talkingStartedOnce && !state.firstRun) {
                talkingStartedOnce = true;
                talking.on = true;
                talkLoop();
            }
        }
        document.title = s.name;
    } catch {
        el('conn-dot').className = 'dot offline';
        el('conn-text').textContent = 'offline';
    }
}

function renderPrivacy(privacy) {
    if (!privacy) return;

    const summary = el('privacy-summary');

    // The panels live in views that can be switched away from, so anything
    // that renders into one has to cope with it not being there. Throwing here
    // would abort the rest of refreshStatus and quietly stop the whole status
    // panel updating.
    if (!summary) return;

    summary.textContent = privacy.summary;
    summary.className = 'privacy-summary ' + privacy.mode;
    el('privacy-detail').textContent = privacy.detail;
}

const GB = 1024 ** 3;

function renderStorage(storage) {
    if (!storage) return;

    const usedPct = storage.total_bytes
        ? Math.round((storage.used_bytes / storage.total_bytes) * 100)
        : 0;

    const fill = el('storage-fill');
    fill.style.width = usedPct + '%';
    fill.className = storage.level === 'ok' ? '' : storage.level;

    el('storage-text').textContent =
        `${(storage.free_bytes / GB).toFixed(0)}GB free · brain using ` +
        `${(storage.database_bytes / (1024 ** 2)).toFixed(0)}MB`;

    const warning = el('storage-warning');
    warning.hidden = !storage.advice;
    if (storage.advice) el('storage-advice').textContent = storage.advice;
}

/*
 * Lessons the brain proposes to remember.
 *
 * These are inferences a model made about what Petar meant, and nothing can
 * check them — which is why they wait here instead of promoting themselves.
 * Accepting one runs the same duplicate check as an automatic promotion, so
 * the one path a person touches is not the one path that creates duplicates.
 */
/*
 * Reading a reply aloud.
 *
 * Local engines only — a cloud voice would post every reply, including whatever
 * the brain recalled from this machine, to a third party to be turned into
 * audio. Failure is deliberately quiet: not hearing an answer that is already
 * on screen is a small thing, and an error box about it would be a larger one.
 */
/*
 * Listening.
 *
 * What comes back goes into the input box, not straight to the brain. A
 * recogniser that mishears "delete the backups" should not have that reach
 * something with tools before a person has read it.
 */
/*
 * Conversation mode.
 *
 * Listen until the speaker stops, send, speak the reply, listen again. The loop
 * is what makes it a conversation rather than a series of dictations, and every
 * step of it is visible in the transcript so it is never unclear whether the
 * brain is listening, thinking, or waiting.
 *
 * Transcripts are sent without confirmation here, which the push-to-talk button
 * deliberately does not do. That is safe for the same reason it is safe to let
 * the model choose tools at all: anything that changes something still stops for
 * approval. A misheard sentence can waste a reply; it cannot delete a file.
 */
const talking = { on: false, running: false };

// Whether the loop has been started automatically this session. Without it,
// every status refresh — every five seconds — would restart a conversation the
// user had just switched off.
let talkingStartedOnce = false;

/*
 * How long the brain stays in the conversation after the last exchange.
 *
 * Long enough to ask a follow-up without saying the name again, short enough
 * that the room does not walk back in five minutes later.
 *
 * From the end of the exchange, not the start of it, and that distinction is
 * the whole feature on this machine. A reply here takes minutes: measured from
 * the moment somebody spoke, the window had always expired by the time the
 * brain stopped talking, so every single follow-up needed the name again and
 * the conversation never actually stayed open.
 */
const ENGAGED_FOR = 45000;

let spokeAt = 0;

function engaged() {
    // Set to be called every time, nothing stays open: each sentence has to
    // carry the name. In a room with a television in it that is the difference
    // between an assistant and a participant.
    if (state.alwaysName) return false;

    return Date.now() - spokeAt < ENGAGED_FOR;
}

// Called at both ends of a turn: hearing the name opens the window so a long
// think does not close it, and finishing the reply reopens it so the follow-up
// is measured from when there was actually a chance to speak.
function stayEngaged() {
    spokeAt = Date.now();
}

function disengage() {
    spokeAt = 0;
}

function toggleTalking() {
    if (talking.running) {
        // Stops after the turn in flight; cutting a reply off mid-sentence
        // would be worse than a moment's wait.
        talking.on = false;
        setTalkButton('stopping');
        setVoiceStatus('Finishing this turn…');

        return;
    }

    talking.on = true;
    talkLoop();
}

/*
 * The two things the command centre's quick commands need.
 *
 * Exposed here rather than reimplemented there: starting a conversation means
 * forgetting the current one's identity so the next message opens a new one,
 * and only this file knows that.
 */
// Whether the conversation is being held by voice.
//
// Read by the progress line, which announces steps aloud only when somebody is
// talking rather than typing: a person at the keyboard can see the screen and
// has not asked to be talked at.
window.brainIsTalking = () => talking.on === true;

window.brainCommands = {
    talk: toggleTalking,
    newConversation() {
        state.conversationId = null;

        const transcript = el('transcript');

        if (transcript) transcript.innerHTML = '';

        addMessage(state.brainName, 'New conversation. What would you like to do?',
            { cssClass: 'brain' });
    },
};

function setTalkButton(state) {
    // The reactor shows this too, so the centre of the picture says what the
    // brain is doing without anybody reading the button.
    if (window.brainMapState) window.brainMapState(state);

    const button = el('talk');
    const label = el('talk-label');

    if (!button || !label) return;

    button.dataset.state = state;

    /*
     * And the shared status, which is what colours it.
     *
     * The state names here are this page's own — "waiting", "stopping" — and
     * they are not the words the brain uses. Mapping both onto one set of
     * statuses is what lets the button, the feed and the core agree.
     */
    const status = window.brainStatusOf ? window.brainStatusOf(state) : 'idle';

    button.dataset.status = status;
    label.dataset.status = status;

    button.setAttribute('aria-pressed', state === 'idle' ? 'false' : 'true');

    /*
     * The name comes from the same place as the colour.
     *
     * The two have to agree, and keeping a second list here is how they came
     * to disagree: the panel said "Making out the words" while this said
     * "Thinking" about the very same moment.
     */
    label.textContent = state === 'stopping'
        ? 'Stopping'
        : window.brainStatusLabel(status);
}

async function talkLoop() {
    if (talking.running) return;

    talking.running = true;

    /*
     * Consecutive failures, not failures.
     *
     * One failed listen used to put a red line in the transcript and close the
     * microphone for good, and the commonest cause is the least serious: the
     * device is held for a moment by its own voice, or by the turn already
     * running. Petar typed and spoke the same sentence at once and got
     * "/api/turn -> 400" in his conversation with listening switched off
     * behind it, so the second half of what he said went nowhere.
     *
     * It gives up only when it is really not working, and says so once.
     */
    let trouble = 0;

    try {
        while (talking.on) {
            /*
             * Two different states that both have the microphone open.
             *
             * Waiting is hearing the room and discarding it. Listening is being
             * in a conversation. Telling them apart is the difference between
             * a brain that looks like it is hanging on every word in the room
             * and one that visibly is not.
             */
            const open = engaged() || !state.wakeWord;

            setTalkButton(open ? 'listening' : 'waiting');
            setVoiceStatus(open
                ? 'Speak when ready'
                : `Say \u201c${state.wakeWord}\u201d to start`);

            let heard;
            try {
                heard = await api.post('/api/turn', {
                    device: el('microphone').value || '',
                    engaged: engaged(),
                });

                trouble = 0;
            } catch (err) {
                if (!talking.on) break;

                trouble += 1;

                // Five in a row is a microphone that is not coming back, and
                // worth a line in the transcript. One is a moment.
                if (trouble >= 5) {
                    addMessage('error',
                        `I cannot open the microphone: ${String(err.message || err)}`,
                        { cssClass: 'error' });

                    break;
                }

                setTalkButton('waiting');
                setVoiceStatus('the microphone was busy — trying again');

                await new Promise((again) => setTimeout(again, 1200));

                continue;
            }

            if (!talking.on) break;

            const said = (heard.transcript || '').trim();

            /*
             * A name nothing here has met is read back before it is used.
             *
             * The recogniser does not hesitate over a word it has never seen;
             * it returns the nearest thing it knows, confidently. "pnscripts
             * .com" arrived as "pncryptz.com" and a considered opinion
             * followed about a website that does not exist — and nothing in
             * the exchange marked it as a guess.
             *
             * Only for names, and only for ones nothing on this machine has
             * ever seen. Anything already in memory passes silently, or the
             * brain would query its owner about their own most-visited site.
             */
            if (said && (heard.unfamiliar || []).length) {
                addMessage(state.brainName,
                    `I heard \u201c${heard.unfamiliar.join('\u201d and \u201c')}\u201d ` +
                    'but I do not know that name — is that right? ' +
                    'Say it again or type it, and I will remember the spelling.',
                    { cssClass: 'note' });

                speak(`I heard ${heard.unfamiliar.join(', and ')}, but I do not know ` +
                    'that name. Is that right?');
            }

            /*
             * Nothing said to the room is a turn.
             *
             * Until the brain has been addressed by name it hears everything —
             * a television, somebody else talking, its own voice off the
             * speakers — and every one of those used to become a question it
             * answered. Now they pass, and nothing about them reaches the
             * transcript.
             */
            if (heard.ends) {
                // Somebody said thank you. That is how a person leaves a
                // conversation, and staying engaged past it means answering
                // whatever they say to somebody else next.
                disengage();
                setVoiceStatus(`Say “${state.wakeWord}” to start`);
                continue;
            }

            if (!heard.addressed) {
                /*
                 * Shown, not merely dropped.
                 *
                 * A name the microphone writes down differently is the reason
                 * this was switched off once before, and the failure is
                 * invisible from outside: the brain never answers and nobody
                 * can see why. Putting what it heard on screen is how its owner
                 * finds out that whisper writes their brain's name as something
                 * else — and the setting takes a list, so they can add it.
                 *
                 * None of this is written down or sent anywhere. It is on
                 * screen until the next thing is said, and then it is gone.
                 */
                const ignored = (heard.heard || '').trim();

                setVoiceStatus(ignored
                    ? `Heard “${short(ignored)}” · say “${state.wakeWord}” to start`
                    : `Say “${state.wakeWord || 'the name'}” to start`);
                continue;
            }

            // Addressed, so the conversation is open until it goes quiet.
            stayEngaged();

            if (!said) {
                // The name on its own: somebody getting its attention before
                // saying what they want.
                setVoiceStatus('Listening…');
                continue;
            }

            addMessage('you', said);
            setTalkButton('thinking');
            setVoiceStatus('Thinking…');

            let reply;
            try {
                reply = await api.post('/api/chat', {
                    conversation_id: state.conversationId,
                    message: said,
                    provider: el('provider').value || '',
                    spoken: true,
                });
            } catch (err) {
                addMessage('error', String(err.message || err), { cssClass: 'error' });
                break;
            }

            state.conversationId = reply.conversation_id;

            // The draft goes as the real thing arrives, or the answer appears
            // twice — once as it was written and once finished.
            showDraft('');
            showActivity([]);
            addMessage(state.brainName, reply.reply, { cssClass: 'brain' });

            if (window.brainMapRecall && reply.recalled) window.brainMapRecall(reply.recalled);
        if (window.brainMapGauges) {
            window.brainMapGauges({ recall: Math.min(1, (reply.recalled || []).length / 12) });
        }

            if (!talking.on) break;

            // Written and spoken, always — the transcript is the record and
            // the voice is how you hear it without looking.
            setTalkButton('speaking');
            setVoiceStatus('Speaking…');

            try {
                /*
                 * Already said, most of the time.
                 *
                 * A spoken turn is now read out sentence by sentence while it
                 * is still being written, so by the time the reply arrives here
                 * it has usually already been heard. Saying it again would
                 * repeat the whole answer to somebody who just listened to it.
                 *
                 * The fall-back matters: a turn that used a tool cannot be
                 * streamed, so that one still arrives unsaid.
                 */
                if (!reply.already_spoken) {
                    // wait: true holds until the voice has actually stopped. On
                    // speakers the microphone hears the brain, and estimating
                    // the duration from the word count — which this replaced —
                    // was wrong in both directions: too short and it
                    // transcribed itself, too long and every exchange dragged.
                    await api.post('/api/speak', { text: reply.reply, wait: true });
                }
            } catch {
                // Not being heard is not a reason to end the conversation.
            }

            // A short settle for the tail of the audio and the room's echo.
            await new Promise((r) => setTimeout(r, ECHO_SETTLE_MS));

            // Now there is a chance to reply, so the window starts here.
            stayEngaged();
        }
    } finally {
        talking.on = false;
        talking.running = false;
        setTalkButton('idle');
        setVoiceStatus('');
    }
}

// Long enough for the tail of the audio and a room's echo to die away, short
// enough not to be felt as a pause.
const ECHO_SETTLE_MS = 350;

// Enough of an overheard line to recognise it, not enough to put somebody
// else's conversation on the screen.
function short(text) {
    return text.length > 38 ? `${text.slice(0, 38).trimEnd()}\u2026` : text;
}

function setVoiceStatus(text) {
    const hint = el('voice-status');
    if (hint) hint.textContent = text;
}

let voicesLoaded = false;

/*
 * Which voice reads answers aloud.
 *
 * Loaded once and remembered on the server, so a voice somebody picked is
 * still theirs after a restart. Changing it speaks a sample immediately —
 * choosing a voice from a list of names without hearing any of them is
 * choosing blind.
 */
async function loadVoices() {
    if (voicesLoaded) return;

    let data;
    try {
        data = await api.get('/api/voices');
    } catch {
        return;
    }

    const select = el('voice');
    const voices = data.voices || [];

    select.textContent = '';

    voices.forEach((v) => {
        const option = document.createElement('option');
        option.value = v.id;
        option.textContent = v.name;
        select.appendChild(option);
    });

    if (data.current) select.value = data.current;

    // Which language is being spoken, which whisper needs told. Left to guess
    // it sometimes translates instead of transcribing, so Bulgarian speech
    // comes back as English prose with no error to explain it.
    const language = el('language');
    language.value = data.language || '';
    el('language-field').hidden = false;

    language.onchange = async () => {
        try {
            await api.post('/api/language', { code: language.value });
        } catch (err) {
            addMessage('error', String(err.message || err), { cssClass: 'error' });
        }
    };

    el('voice-field').hidden = voices.length < 2;
    voicesLoaded = true;

    select.onchange = async () => {
        try {
            await api.post('/api/voice', { id: select.value });
            await api.post('/api/speak', { text: 'This is how I sound.' });
        } catch (err) {
            addMessage('error', String(err.message || err), { cssClass: 'error' });
        }
    };
}

/*
 * Which input to listen through.
 *
 * The list is a choice, but the entry selected by default comes from the
 * server rather than being guessed here, and that distinction cost a morning
 * of silence.
 *
 * Guessing here meant taking the first entry whose name contained "mic", which
 * is how "Microphone (echo cancelled)" won — a name that says nothing at all
 * about which physical microphone feeds it. It was being fed by an analog jack
 * with nothing plugged into it. Worse, choosing here sends that device
 * explicitly with every request and so overrides the server's own checks: it
 * had already worked out the right answer and was being told to ignore it.
 *
 * The server can see what this page cannot — which microphone the echo
 * canceller actually captures from, and whether an input carries any signal —
 * so it decides, and this asks.
 */
async function loadMicrophones() {
    let reply;
    try {
        reply = await api.get('/api/microphones');
    } catch {
        return;
    }

    const mics = reply.microphones || [];
    const select = el('microphone');

    /*
     * Rebuilt only when the devices themselves changed.
     *
     * Not cached once and left alone, which is what it used to do: microphones
     * are plugged in and unplugged while the program is running, and a list
     * fetched at startup goes on offering a headset that left an hour ago.
     * Rebuilding unconditionally is the opposite failure — it would discard a
     * choice already made on every refresh.
     */
    const signature = mics.map((m) => m.id).join('|');

    if (signature !== select.dataset.devices) {
        const had = select.value;

        select.textContent = '';

        mics.forEach((m) => {
            const option = document.createElement('option');
            option.value = m.id;
            option.textContent = m.name + (m.id === reply.inUse ? ' (in use)' : '');
            select.appendChild(option);
        });

        select.dataset.devices = signature;

        // The server's answer, unless a choice was already made and the device
        // it names is still attached.
        select.value = mics.some((m) => m.id === had) ? had : (reply.inUse || '');
    }

    el('microphone-field').hidden = mics.length === 0;
}


/*
 * Reading a reply aloud.
 *
 * Not a separate preference any more. If you are talking to it, it talks back —
 * having to switch that on separately was a setting nobody wants to think
 * about. Typed messages stay silent, which is what typing means.
 *
 * Failure is deliberately quiet: not hearing an answer that is already on
 * screen is a small thing, and an error box about it would be a larger one.
 */
function speak(text) {
    if (!talking.on || !text) return;

    api.post('/api/speak', { text }).catch(() => {});
}

async function refreshLessons() {
    let pending = [];
    try {
        pending = await api.get('/api/lessons');
    } catch {
        return;
    }

    const panel = el('lessons-panel');
    const list = el('lessons');
    list.textContent = '';
    panel.hidden = pending.length === 0;
    el('lesson-count').textContent = pending.length;

    pending.forEach((lesson) => {
        const card = document.createElement('div');
        card.className = 'approval';

        const content = document.createElement('p');
        // textContent, never innerHTML: a language model wrote this string and
        // it may contain anything at all.
        content.textContent = lesson.content;

        if (lesson.confidence) {
            const badge = document.createElement('span');
            badge.className = 'tool';
            badge.textContent = lesson.confidence;
            content.prepend(badge);
        }

        const actions = document.createElement('div');
        actions.className = 'approval-actions';

        const accept = document.createElement('button');
        accept.className = 'approve';
        accept.textContent = 'Remember';
        accept.onclick = () => decideLesson(lesson.id, 'accept');

        const reject = document.createElement('button');
        reject.className = 'reject';
        reject.textContent = 'Discard';
        reject.onclick = () => decideLesson(lesson.id, 'reject');

        actions.append(accept, reject);
        card.append(content, actions);
        list.appendChild(card);
    });
}

async function decideLesson(id, decision) {
    try {
        const result = await api.post(`/api/lessons/${id}/${decision}`);
        addMessage('system', result.message || 'Done.', { cssClass: 'brain' });
    } catch (err) {
        addMessage('error', String(err.message || err), { cssClass: 'error' });
    }

    refreshLessons();
    refreshDrives();
    refreshStatus();

    // Remembering something adds a node, so the map is no longer current.
    if (window.brainMapReload) window.brainMapReload();
}

/*
 * Where the brain could live.
 *
 * Shown always rather than only when the disk is filling: somebody deciding
 * whether to plug a drive in wants to know what it would gain them before the
 * warning appears, not after.
 */
async function refreshDrives() {
    let data;

    try {
        data = await api.get('/api/drives');
    } catch {
        return;
    }

    /*
     * The drive going away is said at the top of the screen, not in this card.
     *
     * Somebody talking to the brain is not looking at the storage panel, and
     * this is the one state where everything else carries on looking correct
     * while nothing is being kept.
     */
    const lost = el('drive-lost');

    if (lost) {
        lost.hidden = !data.drive_gone;

        const where = el('drive-lost-where');

        if (where) where.textContent = data.drive_gone ? data.current : '';
    }

    const list = el('drives');
    list.textContent = '';

    (data.drives || []).forEach((d) => {
        const row = document.createElement('div');
        row.className = 'drive' + (d.current ? ' current' : '');

        const where = document.createElement('span');
        where.className = 'drive-path';
        where.textContent = d.mount_point;

        const free = document.createElement('span');
        free.className = 'drive-free';
        free.textContent = (d.free_bytes / GB).toFixed(0) + 'GB free';

        row.append(where, free);

        const marks = [];
        if (d.current) marks.push('in use');
        if (d.removable) marks.push('removable');
        if (!d.writable) marks.push('not writable');

        if (marks.length) {
            const note = document.createElement('span');
            note.className = 'drive-note';
            note.textContent = marks.join(' · ');
            row.appendChild(note);
        }

        list.appendChild(row);
    });

    if (data.how && (data.drives || []).length > 1) {
        const how = document.createElement('p');
        how.className = 'storage-hint';
        how.textContent = data.how;
        list.appendChild(how);
    }
}

async function refreshApprovals() {
    let pending = [];
    try {
        pending = await api.get('/api/approvals');
    } catch {
        return;
    }

    const panel = el('approvals-panel');
    const list = el('approvals');
    list.textContent = '';
    panel.hidden = pending.length === 0;
    el('approval-count').textContent = pending.length;

    pending.forEach((item) => {
        const card = document.createElement('div');
        card.className = 'approval';

        const tool = document.createElement('span');
        tool.className = 'tool';
        tool.textContent = item.tool;

        const summary = document.createElement('p');
        // The summary is the whole point of the prompt: it is what a person
        // actually judges, so it is rendered as text and never as markup.
        summary.textContent = item.summary;
        summary.prepend(tool);

        const actions = document.createElement('div');
        actions.className = 'approval-actions';

        const approve = document.createElement('button');
        approve.className = 'approve';
        approve.textContent = 'Approve';
        approve.onclick = () => decide(item.id, 'approve');

        const reject = document.createElement('button');
        reject.className = 'reject';
        reject.textContent = 'Reject';
        // "deny" is what the server calls it. This said "reject", so the button
    // answered 400 and rejected nothing — the action stayed in the queue.
    reject.onclick = () => decide(item.id, 'deny');

        actions.append(approve, reject);
        card.append(summary, actions);
        list.appendChild(card);
    });
}

async function decide(id, decision) {
    try {
        const answer = await api.post(`/api/approvals/${id}/${decision}`);

        /*
         * What the action actually did, from where the server actually puts it.
         *
         * This read `answer.result`, and the server sends the whole invocation
         * under `invocation` — so every approved action reported "Done —
         * undefined". The one moment somebody most needs to be told what
         * happened is the moment after they have agreed to it, and it said
         * nothing at all.
         */
        const done = answer.invocation || {};
        const said = (done.result || '').trim();

        const note = decision === 'approve'
            ? (answer.error
                ? `Failed: ${answer.error}`
                : (said ? `Done — ${said}` : 'Done.'))
            : 'Rejected. Nothing was changed.';

        addMessage('system', note, { cssClass: 'brain' });
    } catch (err) {
        addMessage('error', String(err.message || err), { cssClass: 'error' });
    }
    refreshApprovals();
    refreshLessons();
    refreshDrives();
    refreshActivity();
    refreshStatus();
}

const MARKS = { executed: '✓', pending: '•', rejected: '×', failed: '!' };

async function refreshActivity() {
    let items = [];
    try {
        items = await api.get('/api/activity');
    } catch {
        return;
    }

    /*
     * Its own box, not the whole panel.
     *
     * This used to clear #activity, which feed.js also draws into — so every
     * three seconds it deleted whatever background job was running and feed.js
     * put it back a second and a half later. That flicker was visible on
     * screen, and the same collision is how a running job came to sit directly
     * above the words "Nothing yet."
     */
    const box = el('activity-items');

    if (!box) return;

    box.textContent = '';

    if (!items.length) {
        window.brainActivityChanged();

        return;
    }

    /*
     * Each entry is an icon, two lines and a tag.
     *
     * The tag is the entry's own kind and outcome — a tool that ran, a message,
     * something waiting on a decision — not a severity invented to give the
     * column some colour. When everything is ordinary, everything is the same
     * colour, and that is the useful state to be able to recognise.
     */
    const KINDS = {
        message: ['MSG', 'said'],
        tool: ['RUN', 'ran'],
        lesson: ['MEM', 'learned'],
        approval: ['ASK', 'waiting'],
    };

    items.forEach((item) => {
        const row = document.createElement('div');
        row.className = `feed-item ${item.status || ''}`;

        const icon = document.createElement('span');
        icon.className = 'row-icon';
        icon.textContent = (KINDS[item.kind] || ['·'])[0];

        const text = document.createElement('span');
        text.className = 'row-text';

        const title = document.createElement('span');
        title.className = 'row-name';
        title.textContent = item.summary || item.tool || '—';

        const when = document.createElement('span');
        when.className = 'row-state';
        when.textContent = [(KINDS[item.kind] || [null, item.kind])[1], ago(item.when)]
            .filter(Boolean).join(' · ');

        text.append(title, when);

        const tag = document.createElement('span');
        tag.className = `feed-tag ${item.status || ''}`;
        tag.textContent = (item.status || item.kind || '').toUpperCase();

        row.append(icon, text, tag);
        box.appendChild(row);
    });

    window.brainActivityChanged();
}

/*
 * "Nothing yet." belongs to neither half of the panel.
 *
 * It is the answer to a question about both — the background jobs feed.js
 * draws and the entries this file draws — so it is settled in one place that
 * both call, rather than by whichever of them happened to run last. Owning it
 * separately is what put a running job directly above the words "Nothing yet."
 */
window.brainActivityChanged = function () {
    const empty = el('activity-empty');

    if (!empty) return;

    const jobs = el('activity-jobs');
    const items = el('activity-items');

    const anything = (jobs && !jobs.hidden && jobs.children.length > 0)
        || (items && items.children.length > 0);

    empty.hidden = anything;
};

/*
 * How long ago, in words.
 *
 * Rounded on purpose. "Four minutes ago" is what somebody wants from a feed;
 * a timestamp to the second is a thing to decode.
 */
function ago(when) {
    if (!when) return '';

    const seconds = (Date.now() - new Date(when).getTime()) / 1000;

    if (!isFinite(seconds) || seconds < 0) return '';
    if (seconds < 60) return 'just now';
    if (seconds < 3600) return `${Math.round(seconds / 60)}m ago`;
    if (seconds < 86400) return `${Math.round(seconds / 3600)}h ago`;

    return `${Math.round(seconds / 86400)}d ago`;
}

// Returns whether anything was restored, so the caller knows whether a
// greeting would be an opening or an interruption.
async function restoreConversation() {
    try {
        const convo = await api.get('/api/conversations/latest');

        /*
         * What was said, and only that.
         *
         * A stored conversation holds the raw output of every tool the brain
         * called as well as the two people in it. Replaying all of it put
         * search results in the transcript as though the brain had read them
         * out — the same fault as showing the system prompt, which this
         * already guarded against.
         */
        const said = (convo.messages || []).filter(
            (m) => (m.role === 'user' || m.role === 'assistant') &&
                (m.content || '').trim() !== ''
        );

        if (!said.length) return false;

        // Only if there is one. A conversation restored without its identity
        // still reads correctly; carrying on from it would start a new one,
        // which is a smaller fault than showing an empty screen.
        if (convo.id) state.conversationId = convo.id;

        said.forEach((m) => {
            addMessage(
                m.role === 'user' ? 'you' : state.brainName,
                m.content,
                { cssClass: m.role === 'user' ? 'you' : 'brain' }
            );
        });

        return true;
    } catch {
        /* An empty console is a fine fallback; don't block startup on history. */
        return false;
    }
}

/*
 * Opening a conversation from the history beside it.
 *
 * Replaces what is on screen rather than adding to it, and takes the
 * conversation's identity with it, so carrying on from an old exchange
 * continues that one instead of quietly starting another.
 */
window.brainOpenConversation = async function (id) {
    let convo;

    try {
        convo = await api.get(`/api/conversations/${id}`);
    } catch (err) {
        addMessage('error', String(err.message || err), { cssClass: 'error' });

        return;
    }

    const transcript = el('transcript');

    if (transcript) transcript.innerHTML = '';

    state.conversationId = convo.id;

    /*
     * What was said, and only that.
     *
     * A stored conversation holds more than the two people in it: the system
     * prompt that set the brain up, and the raw output of every tool it
     * called. Replaying all of it put "You are PN Brain, Petar's personal AI
     * assistant…" at the top of the transcript as though the brain had opened
     * by reciting its own instructions, and buried the actual exchange under
     * tool output nobody asked to see twice.
     */
    (convo.messages || [])
        .filter((m) => m.role === 'user' || m.role === 'assistant')
        .filter((m) => (m.content || '').trim() !== '')
        .forEach((m) => {
            addMessage(
                m.role === 'user' ? 'you' : state.brainName,
                m.content,
                { cssClass: m.role === 'user' ? 'you' : 'brain' }
            );
        });
};

/* ---------- wiring ---------- */

el('composer').addEventListener('submit', (e) => {
    e.preventDefault();
    const input = el('input');
    const text = input.value;
    input.value = '';
    input.style.height = 'auto';

    /*
     * Typing while it is working interrupts it, the same as speaking over it.
     *
     * Cutting in by voice has worked for a while and typing did not, so the
     * only way to add something mid-answer was to talk — which is no use to
     * somebody at a keyboard, or in a room where they would rather not. The
     * message then queued behind a turn that might run for minutes, and by the
     * time it was read the moment had passed.
     *
     * The voice stops, what was already said is kept as what was said, and the
     * new message goes in as the next thing — so it reads as a person adding
     * to a conversation rather than as one being abandoned and restarted.
     */
    const step = window.brainWork ? window.brainWork() : null;

    if (text.trim() && step && step.busy && !step.background) {
        stopTalking();
    }

    send(text);
});

// Enter sends, Shift+Enter breaks the line — the convention everywhere else.
el('input').addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        el('composer').requestSubmit();
    }
});

el('input').addEventListener('input', (e) => {
    e.target.style.height = 'auto';
    e.target.style.height = `${Math.min(e.target.scrollHeight, 190)}px`;
});

/*
 * Opening the program: which conversation this is, and what it has to say.
 *
 * Both, in that order, because the second only makes sense once you know the
 * first. The greeting is composed on the server from what it already knows
 * rather than generated by the model, so it arrives immediately — a greeting
 * that takes a minute to appear is not a greeting.
 */
async function openWhereWeLeftOff(canSpeak) {
    let greeting;

    try {
        greeting = await api.get('/api/greeting');
    } catch {
        return;
    }

    /*
     * Either the conversation you were in, or a new one — and the screen says
     * which before anything else appears on it.
     *
     * The brain decides, because the rule is one rule and it belongs in one
     * place: a restart in the middle of something is an interruption and
     * carrying on is what was meant; a day later is a different day.
     */
    if (greeting.carry_on) {
        const restored = await restoreConversation();

        if (restored && greeting.last) {
            markThread(`Carrying on — last said ${whenSaid(greeting.last.when)}`);
        }
    } else {
        state.conversationId = null;

        if (greeting.last) {
            const topic = (greeting.last.topic || '').trim();

            markThread(`New conversation. The last one was ${whenSaid(greeting.last.when)}`
                + (topic ? ` — ${topic.length > 60 ? `${topic.slice(0, 60).trimEnd()}…` : topic}` : ''));
        }
    }

    if (!greeting.text) return;

    /*
     * Written whenever it is said.
     *
     * It used to be spoken on every start and written only onto a fresh
     * screen, on the reasoning that reintroducing itself over a transcript
     * somebody is reading is noise. That produced something worse: a voice
     * saying a sentence that appears nowhere on the page, so the record and
     * the room disagreed about what had just been said — and the transcript
     * is meant to be the record of exactly that.
     *
     * If it is worth saying it is worth showing. If it is not worth showing it
     * should not be said.
     */
    addMessage(state.brainName, greeting.text, { cssClass: 'brain' });

    /*
     * And the introduction in full, in the conversation.
     *
     * It is a list — what it can do and the words to ask for each — and a list
     * is the wrong shape for something said out loud. So the voice gets one
     * sentence and the chat gets the whole of it, where it can be read,
     * scrolled back to, and left alone until it is wanted.
     *
     * Sent once, by the brain, and never offered again.
     */
    if (greeting.shown) {
        addMessage(state.brainName, greeting.shown, { cssClass: 'brain' });
    }

    // Under the room: a greeting is something it says on its own, and nobody
    // asked for it at the level of an answer they were waiting for.
    if (canSpeak) {
        api.post('/api/speak', { text: greeting.text, unprompted: true }).catch(() => {});
    }
}

(async function start() {
    await refreshStatus();

    let canSpeak = false;

    try {
        canSpeak = (await api.get('/api/status')).capabilities.includes('speech');
    } catch {
        // A greeting nobody hears is still worth showing.
    }

    /*
     * Spoken every time; written only on a fresh screen.
     *
     * Opening the program used to be silent whenever there was a conversation
     * to restore, which is nearly always — so the one moment somebody most
     * wants to be told where things stand was the one moment it said nothing
     * at all, and a restart in the middle of an hour of work looked
     * indistinguishable from nothing having happened.
     */
    /*
     * And nothing is said over the naming card either.
     *
     * The server refuses the greeting while the brain is new, which is the
     * guarantee; this is so the page does not ask for one it cannot have.
     * Begin reloads, and the greeting arrives then, by the chosen name.
     */
    if (!state.firstRun) await openWhereWeLeftOff(canSpeak);

    refreshApprovals();
    refreshLessons();
    refreshDrives();
    refreshActivity();
    el('talk').onclick = toggleTalking;
    el('input').focus();

    // Polling rather than websockets: approvals can be decided from the Filament
    // admin or another window, and a few seconds of staleness costs nothing here.
    setInterval(() => {
        refreshStatus();
        refreshApprovals();
        refreshLessons();
        refreshDrives();
        refreshActivity();
    }, 5000);
})();

/*
 * Stopping it mid-sentence.
 *
 * Cutting in by talking has worked for a while — the level detector watches for
 * it while the brain speaks — but that was the only way, and it requires being
 * somewhere you can talk. Somebody at the keyboard, or in a room where they
 * would rather not shout, had to sit through four paragraphs of an answer they
 * could already tell was wrong.
 *
 * Escape as well as the button, because Escape is what everything else on the
 * machine uses for "not that" and a hand is already on the keyboard.
 */
/*
 * Stop means stop.
 *
 * It used to mean "stop the voice", so pressing it during a minute of thinking
 * did nothing at all: the model carried on, holding the only processor on the
 * machine, and whatever was said next queued behind an answer already
 * abandoned. Three things now, which is what stopping an assistant means — the
 * voice, the work, and whatever it was about to do next.
 */
async function stopTalking() {
    try {
        await api.post('/api/interrupt', {});
    } catch {
        // Nothing to report. If the request failed the voice is still going,
        // which says so by itself.
    }

    if (state.inFlight) {
        state.inFlight.abort();
        state.inFlight = null;
    }
}

window.brainStop = stopTalking;

/*
 * Listening while it is working, and stopping for whatever is said.
 *
 * Cutting in used to work only while it was speaking, which is the smaller
 * half of the wait: on this machine most of a turn is thinking, with the
 * microphone shut. So the one moment somebody most wants to say "no, not that"
 * was the one moment nothing was listening.
 *
 * And it asked for the name. The name is what separates being spoken to from
 * being in the same room as a television, and it earns that everywhere except
 * here — this is the brain already working for the person who has just started
 * talking, so there is nothing to disambiguate. Being made to say a name
 * before you can interrupt is being made to wait your turn by the thing that
 * is supposed to be waiting for you.
 *
 * What still has to hold is that it was speech at all. The recording has
 * already passed the test that tells a voice from a fan, which is the one that
 * matters in a room with this machine in it.
 */
async function watchForYou(abandon) {
    if (!talking.on) return;

    while (!abandon.signal.aborted && state.busy) {
        let heard;

        try {
            heard = await api.post('/api/turn', {
                device: el('microphone').value || '',
                engaged: false,
                interrupting: true,
            }, abandon.signal);
        } catch (err) {
            // The turn finished and took the listen with it, which is the
            // ordinary way out of this loop.
            if (abandon.signal.aborted || !state.busy) return;

            /*
             * Otherwise the microphone was busy for a moment — its own voice
             * holds it while it speaks — so wait and ask again rather than
             * giving up. Giving up here would close the microphone for the
             * rest of the answer, which is most of the time somebody wants to
             * be able to cut in.
             */
            await new Promise((again) => setTimeout(again, 1500));

            continue;
        }

        if (!heard || !heard.addressed) continue;

        const said = (heard.transcript || '').trim();

        /*
         * Stop first, then say what was said.
         *
         * Abandoning the answer before sending the new sentence matters: the
         * two would otherwise be answered at once on a machine with one
         * processor, and the thing being interrupted would win the race.
         */
        await stopTalking();

        if (said) {
            // Given a moment for the abandoned turn to unwind, so this arrives
            // as the next thing rather than as a second thing at the same time.
            setTimeout(() => send(said), 150);
        }

        return;
    }
}

el('stop-now')?.addEventListener('click', stopTalking);

document.addEventListener('keydown', (e) => {
    if (e.key !== 'Escape') return;

    // Not while typing: Escape in a text box means "leave this field", and
    // hijacking it would make the composer feel broken.
    const typing = document.activeElement;

    if (typing && /^(INPUT|TEXTAREA)$/.test(typing.tagName)) return;

    stopTalking();
});

/*
 * The button appears exactly while there is something to stop.
 *
 * Driven by the same reading everything else uses rather than a second poll of
 * its own; see signals.js.
 */
setInterval(() => {
    const button = el('stop-now');

    if (!button) return;

    const step = window.brainWork ? window.brainWork() : { busy: false };

    // Background work is the brain's own; nobody is waiting on it and stopping
    // it is not what this button is for.
    button.hidden = !(step.busy && !step.background);
}, 400);

/*
 * Keeping the button, the core and the feed telling the same story.
 *
 * There were two accounts of what the brain was doing and they disagreed. The
 * talk button and the core were driven by this page's own loop — which knows
 * when it asked for a turn and nothing else — while the feed came from the
 * brain itself. So the button read "Waiting" while the panel beside it showed
 * a recogniser part-way through a sentence, and the core was amber for work
 * the button said was not happening.
 *
 * The brain's account wins, because it is the one that knows. The page's loop
 * still sets the state at the moments it alone is aware of — the microphone
 * opening, a turn being sent — and anything the brain reports overrides it
 * while a turn is in flight.
 */
/*
 * No mapping of its own any more.
 *
 * This used to collapse the brain's kinds into the four the button knew —
 * transcribing and tool both became "thinking" — so mid-turn the button named
 * and coloured the same instant differently from the panel beside it. The
 * shared status is used unchanged; see status.js.
 */

setInterval(() => {
    // Only while the page is holding a voice conversation. Somebody typing has
    // not asked the button to narrate, and background work the brain gave
    // itself is not something to report as though they were waiting on it.
    if (!talking.on) return;

    const step = window.brainWork ? window.brainWork() : null;

    if (!step || !step.busy) return;

    // Listening is reported as background, since nobody waits on it, but it is
    // exactly the state the button exists to show.
    if (step.background && step.kind !== 'listening') return;

    const status = window.brainStatusOf(step);

    if (status && el('talk')?.dataset.status !== status) setTalkButton(status);
}, 500);

/*
 * The answer appearing as it is written.
 *
 * It used to arrive in one piece at the end, so a turn that takes a minute
 * showed nothing for a minute — and on a spoken turn the assistant talked the
 * whole way through while the transcript beside it stayed empty. From the
 * outside that is indistinguishable from a program that has stopped, which is
 * the doubt every other part of this interface exists to remove.
 *
 * A draft, replaced by the real message when the turn finishes. It carries a
 * class of its own so it can be told apart in the DOM and removed cleanly,
 * rather than being left behind as a duplicate of the answer beside it.
 */
function showDraft(text) {
    const transcript = el('transcript');

    if (!transcript) return;

    let draft = transcript.querySelector('.msg-draft');

    if (!text) {
        if (draft) draft.remove();

        return;
    }

    if (!draft) {
        draft = document.createElement('div');
        draft.className = 'msg brain msg-draft';

        const who = document.createElement('span');
        who.className = 'who';
        who.textContent = state.brainName;

        const body = document.createElement('div');
        body.className = 'body';

        draft.append(who, body);
        transcript.appendChild(draft);
    }

    const body = draft.querySelector('.body');

    if (body.textContent !== text) {
        body.textContent = text;

        // Only while already at the bottom, so reading back through the
        // conversation is not yanked forward every time a word arrives.
        const atBottom = transcript.scrollHeight - transcript.scrollTop
            - transcript.clientHeight < 80;

        if (atBottom) transcript.scrollTop = transcript.scrollHeight;
    }
}

setInterval(() => {
    const step = window.brainWork ? window.brainWork() : null;

    showDraft(step && step.busy ? (step.so_far || '') : '');
}, 400);

/*
 * What it is doing, inside the conversation rather than beside it.
 *
 * The panel on the left has always carried this, and it is the wrong place for
 * it during a turn: somebody reading a conversation is looking at the
 * conversation, and a tool that ran between their question and the answer is
 * part of that exchange, not a separate stream to cross-reference by
 * timestamp.
 *
 * So the steps of the turn in flight appear under the question that caused
 * them — what ran, what it was asked, what came back — and settle into a quiet
 * record once the answer arrives. The same information either way; the
 * difference is whether it reads as part of the conversation or as telemetry.
 */
const SHOWN_IN_CHAT = new Set(['tool', 'thinking', 'transcribing', 'learning']);

function showActivity(steps) {
    const transcript = el('transcript');

    if (!transcript) return;

    let box = transcript.querySelector('.turn-activity');

    if (!steps.length) {
        if (box) box.remove();

        return;
    }

    if (!box) {
        box = document.createElement('div');
        box.className = 'turn-activity';
        transcript.appendChild(box);
    }

    const signature = steps
        .map((s) => `${s.kind}|${s.note}|${(s.detail || []).join('~')}`)
        .join('\n');

    if (box.dataset.shown === signature) {
        // Only the clock on the last line moves.
        const last = box.lastElementChild?.querySelector('.act-took');
        const step = steps[steps.length - 1];

        if (last && step) last.textContent = feedTime(step.took);

        return;
    }

    box.textContent = '';
    box.dataset.shown = signature;

    steps.forEach((step, i) => {
        const row = document.createElement('div');
        row.className = 'act';
        row.dataset.status = window.brainStatusOf
            ? window.brainStatusOf({ ...step, busy: true })
            : 'thinking';
        row.dataset.running = i === steps.length - 1 ? 'yes' : 'no';

        const dot = document.createElement('span');
        dot.className = 'act-dot';

        const body = document.createElement('span');
        body.className = 'act-body';

        const what = document.createElement('span');
        what.className = 'act-what';
        what.textContent = step.note || step.kind;
        body.appendChild(what);

        (step.detail || []).forEach((line) => {
            const d = document.createElement('span');
            d.className = 'act-detail';
            d.textContent = line;
            body.appendChild(d);
        });

        const took = document.createElement('span');
        took.className = 'act-took';
        took.textContent = feedTime(step.took);

        row.append(dot, body, took);
        box.appendChild(row);
    });

    const atBottom = transcript.scrollHeight - transcript.scrollTop
        - transcript.clientHeight < 120;

    if (atBottom) transcript.scrollTop = transcript.scrollHeight;
}

/*
 * Only the steps of the turn being answered.
 *
 * The history holds listening and learning as well, and those belong in the
 * panel rather than in the conversation: nobody asked for them and they happen
 * whether or not anybody is talking. What goes here is the work that this
 * question caused.
 */
async function pollActivity() {
    const step = window.brainWork ? window.brainWork() : null;

    if (!step || !step.busy || step.background) {
        showActivity([]);

        return;
    }

    try {
        const reply = await api.get('/api/steps');
        const steps = (reply.steps || []).filter(
            (s) => SHOWN_IN_CHAT.has(s.kind) && !s.background
        );

        // The current turn only: everything since the last time it was idle.
        showActivity(steps.slice(-6));
    } catch {
        /* The conversation still reads without it. */
    }
}

setInterval(pollActivity, 2000);
