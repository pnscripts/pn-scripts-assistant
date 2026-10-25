/*
 * The pages in English and Bulgarian.
 *
 * Which one: what the assistant has been told its owner speaks, asked of the
 * server once; until that answer arrives, and when it has nothing to say, the
 * browser's own language. A page written in one language and switched to the
 * other once the answer arrives is redrawn: every element carrying
 * data-i18n is filled again and a "pn-language" event tells the scripts that
 * draw their own rows to draw them again.
 *
 * Every string a person reads in the parts that use this is here, in both
 * languages, side by side — so a sentence changed in one is seen to need
 * changing in the other.
 */
(function () {
    const STRINGS = {
        // Sources under an answer.
        'sources.title': ['Sources', 'Източници'],
        'sources.cited': ['cited', 'цитиран'],
        'sources.read': ['read, not cited', 'прочетен, без цитат'],
        'sources.open': ['Open', 'Отвори'],
        'sources.folder': ['Show in folder', 'Покажи в папката'],
        'sources.failed': ['It could not be opened.', 'Не можа да се отвори.'],
        'kind.memory': ['Remembered', 'Запомнено'],
        'kind.document': ['File', 'Файл'],
        'kind.project': ['Project', 'Проект'],
        'kind.page': ['Web page', 'Уеб страница'],
        'kind.result': ['Search result', 'Резултат от търсене'],
        'kind.mail': ['Email', 'Имейл'],
        'kind.conversation': ['Earlier conversation', 'Предишен разговор'],
        'kind.site': ['A site you visit', 'Сайт, който посещавате'],

        // What I did and why.
        'nav.deeds': ['What I did', 'Какво направих'],
        'deeds.title': ['What I did and why', 'Какво направих и защо'],
        'deeds.intro': [
            'Every action I took, put to you or refused — with what was asked, who did it, '
                + 'which model and why, and whether anything left this machine. The program keeps '
                + 'this record itself; it is not the model\'s account of what it did.',
            'Всяко действие, което извърших, предложих за одобрение или отказах — с това какво '
                + 'беше поискано, кой специалист го изпълни, с кой модел и защо, и дали нещо е '
                + 'напуснало този компютър. Записът се води от самата програма, а не от модела.',
        ],
        'deeds.period': ['Period', 'Период'],
        'deeds.day': ['Last 24 hours', 'Последните 24 часа'],
        'deeds.week': ['Last 7 days', 'Последните 7 дни'],
        'deeds.month': ['Last 31 days', 'Последните 31 дни'],
        'deeds.all': ['Everything', 'Всичко'],
        'deeds.onlyChanges': ['Only actions that change something', 'Само действия, които променят нещо'],
        'deeds.onlyLeaves': ['Only what left this machine', 'Само изпратеното извън компютъра'],
        'deeds.refresh': ['Refresh', 'Обнови'],
        'deeds.empty': ['Nothing was done in this period.', 'За този период няма действия.'],
        'deeds.error': ['The record could not be read.', 'Записът не можа да бъде прочетен.'],
        'deeds.count': ['{n} actions', '{n} действия'],
        'deeds.countChanges': ['{n} changed something', '{n} с промяна'],
        'deeds.countLeaves': ['{n} left this machine', '{n} извън компютъра'],
        'deeds.countWaiting': ['{n} waiting for you', '{n} чакат вас'],
        'deeds.countRefused': ['{n} refused', '{n} отказани'],
        'deeds.countFailed': ['{n} failed', '{n} неуспешни'],
        'outcome.done': ['Done', 'Изпълнено'],
        'outcome.failed': ['Failed', 'Неуспешно'],
        'outcome.waiting': ['Waiting for you', 'Чака вашето решение'],
        'outcome.refused': ['Refused', 'Отказано'],
        'deeds.why': ['Asked for', 'Поискано'],
        'deeds.who': ['Who', 'Кой'],
        'deeds.you': ['Your assistant', 'Вашият асистент'],
        'deeds.model': ['Model', 'Модел'],
        'deeds.modelWhy': ['Chosen because', 'Избран, защото'],
        'deeds.asked': ['Given', 'Подадени данни'],
        'deeds.result': ['Result', 'Резултат'],
        'deeds.task': ['Task', 'Задача'],
        'deeds.risk': ['Risk', 'Риск'],
        'deeds.left': ['Left this machine', 'Излезе извън компютъра'],
        'deeds.approved': ['You approved it', 'Вие го одобрихте'],
        'deeds.took': ['took {t}', 'отне {t}'],
        'deeds.changes': ['changes something', 'променя нещо'],
        'deeds.looks': ['only looks', 'само чете'],
        'risk.low': ['low', 'нисък'],
        'risk.medium': ['medium', 'среден'],
        'risk.high': ['high', 'висок'],
        'risk.critical': ['critical', 'критичен'],

        // Control.
        'nav.control': ['Control', 'Контрол'],
        'control.asks': ['How much it asks', 'Колко често пита'],
        'control.before': ['Before it does something on this machine', 'Преди да направи нещо на този компютър'],
        'freedom.ask': ['Ask me every time', 'Питай ме всеки път'],
        'freedom.granted': ['Do what I have allowed, ask about the rest', 'Прави позволеното, питай за останалото'],
        'freedom.everything': ['Act freely, and record everything', 'Действай свободно и записвай всичко'],
        'means.ask': [
            'It asks before anything that changes something. Every action is recorded.',
            'Пита преди всяко действие, което променя нещо. Всяко действие се записва.',
        ],
        'means.granted': [
            'It does what you have already allowed and asks about the rest. Every action is recorded.',
            'Прави това, което вече сте позволили, и пита за останалото. Всяко действие се записва.',
        ],
        'means.everything': [
            'It does anything it can without asking. Every action is recorded, and files it '
                + 'overwrites are kept so they can be put back.',
            'Прави всичко, което може, без да пита. Всяко действие се записва, а файловете, '
                + 'които презаписва, се пазят, за да могат да бъдат върнати.',
        ],
        'leaves.ask': ['Nothing changes until you say yes.', 'Нищо не се променя, докато не потвърдите.'],
        'leaves.granted': ['Anything you have not allowed waits for you.', 'Всичко непозволено изчаква вашето решение.'],
        'leaves.everything': [
            'Overwritten files are kept for undo: the last 60, for up to 14 days.',
            'Презаписаните файлове се пазят за връщане: последните 60, до 14 дни.',
        ],
        'control.lookOnline': ['May look things up about itself', 'Може да проверява информация за себе си'],
        'control.lookOnlineNote': [
            'Updates, the list of models it could install, documentation. Sends nothing about you — '
                + 'this is not your privacy setting.',
            'Обновления, списъкът с модели за инсталиране, документация. Не изпраща нищо за вас — '
                + 'това не е настройката за поверителност.',
        ],
        'control.privacyNote': [
            'What may leave this machine is a separate setting, currently “{p}”. It is changed under '
                + 'Privacy in System, and nothing on this page changes it.',
            'Какво може да напуска този компютър е отделна настройка, в момента „{p}“. Тя се променя '
                + 'от „Поверителност“ в „Система“ и нищо на тази страница не я променя.',
        ],
        'control.answers': ['Answers', 'Отговори'],
        'control.cite': ['Say where answers come from', 'Посочвай откъде идват отговорите'],
        'control.citeNote': [
            'Each answer lists what it was built from — your files, pages it read, things it '
                + 'remembers — and ends a sentence with the number of its source. Spoken answers are '
                + 'listed without numbers. The list is kept either way.',
            'Всеки отговор изрежда на какво се основава — ваши файлове, прочетени страници, '
                + 'запомнени неща — и завършва изречението с номера на източника. Изговорените '
                + 'отговори се изброяват без номера. Списъкът се пази и при изключена настройка.',
        ],
        'control.every': ['Every capability', 'Всички възможности'],
        'control.everyNote': [
            'Reading and looking never ask. The rest is your decision, and you can change any of it '
                + 'here at any time. The switch takes a capability away completely.',
            'Четенето и разглеждането никога не питат. Останалото е ваше решение и можете да го '
                + 'промените тук по всяко време. Превключвателят изключва възможността напълно.',
        ],
        'control.show': ['Show', 'Покажи'],
        'control.showAll': ['All', 'Всички'],
        'control.showLeaves': ['Can send something off this machine', 'Могат да изпратят нещо извън компютъра'],
        'control.showChanges': ['Change something', 'Променят нещо'],
        'control.showOff': ['Switched off', 'Изключени'],
        'control.summary': [
            '{n} capabilities · {l} can send something off this machine · {o} switched off',
            '{n} възможности · {l} могат да изпратят нещо извън компютъра · {o} изключени',
        ],
        'control.empty': ['Nothing registered.', 'Няма регистрирани възможности.'],
        'control.nothingHere': ['None match.', 'Няма съвпадения.'],
        'control.onlyLooks': ['only looks', 'само чете'],
        'control.askMe': ['Ask me', 'Питай ме'],
        'control.allow': ['Always allow', 'Винаги позволявай'],
        'control.never': ['Never', 'Никога'],
        'control.thisRun': ['allowed until the program stops', 'позволено до спиране на програмата'],
        'control.on': ['On', 'Вкл.'],
        'control.off': ['Off', 'Изкл.'],
        'control.switch': ['Switch {name} on or off', 'Включване или изключване на „{name}“'],
        'control.canSend': ['Can send: {what}', 'Може да изпрати: {what}'],
        'control.stays': ['Stays on this machine', 'Остава на този компютър'],
        'hidden.off': ['switched off by you', 'изключено от вас'],
        'hidden.needs': ['needs something you switched off', 'нуждае се от нещо, което сте изключили'],
        'hidden.privacy': ['out of reach at this privacy setting', 'недостъпно при тази настройка за поверителност'],
        'control.failed': ['That could not be saved.', 'Промяната не можа да бъде запазена.'],
        'privacy.private': ['private', 'частно'],
        'privacy.research': ['research', 'проучване'],
        'privacy.open': ['open', 'отворено'],
        // Writing and proofreading.
        'nav.writing': ['Writing', 'Писане'],
        'writing.title': ['Writing and proofreading', 'Писане и коректура'],
        'writing.intro': [
            'Paste or type a text in English or Bulgarian. Proofreading uses rules, not a model, so it is '
                + 'instant and nothing leaves this computer. Each correction says why; only the sure ones are '
                + 'ticked, and clicking one shows it in the text. A rewrite in a tone uses the language model and '
                + 'is shown as changes for you to keep or discard.',
            'Поставете или въведете текст на английски или български. Коректурата използва правила, а не '
                + 'модел, затова е мигновена и нищо не напуска компютъра. Всяка поправка казва защо; отметнати '
                + 'са само сигурните, а с щракване се вижда къде е в текста. Пренаписването в избран тон '
                + 'използва езиковия модел и се показва като промени, които можете да приемете или откажете.',
        ],
        'writing.language': ['Language', 'Език'],
        'writing.auto': ['Detect', 'Автоматично'],
        'writing.en': ['English', 'Английски'],
        'writing.bg': ['Bulgarian', 'Български'],
        'writing.tone': ['Tone', 'Тон'],
        'writing.noTone': ['No tone check', 'Без проверка на тона'],
        'writing.maxWords': ['Most words', 'Най-много думи'],
        'writing.text': ['Text', 'Текст'],
        'writing.placeholder': ['Paste or type the text here. Ctrl+Enter proofreads it.',
            'Поставете или въведете текста тук. Ctrl+Enter го проверява.'],
        'writing.check': ['Proofread', 'Провери'],
        'writing.rewrite': ['Rewrite in this tone', 'Пренапиши в този тон'],
        'writing.copy': ['Copy text', 'Копирай текста'],
        'writing.copied': ['Copied.', 'Копирано.'],
        'writing.needTone': ['Choose a tone first.', 'Първо изберете тон.'],
        'writing.empty': ['There is no text yet.', 'Все още няма текст.'],
        'writing.stats': [
            '{words} words · {sentences} sentences · longest {longest} words · about {minutes} min to read',
            '{words} думи · {sentences} изречения · най-дългото — {longest} думи · около {minutes} мин. четене',
        ],
        'writing.slips': ['Corrections', 'Поправки'],
        'writing.toneNotes': ['Tone and length', 'Тон и дължина'],
        'writing.none': ['No slips found by the rules. Meaning, style and facts are for you to judge.',
            'Правилата не откриха грешки. Смисълът, стилът и фактите остават за ваша преценка.'],
        'writing.apply': ['Make the ticked corrections', 'Нанеси отметнатите поправки'],
        'writing.use': ['Use this correction', 'Използвай тази поправка'],
        'writing.remove': ['take out', 'премахни'],
        'writing.checkIt': ['check it', 'проверете'],
        'writing.applied': ['Corrections made: {n}.', 'Нанесени поправки: {n}.'],
        'writing.working': ['Rewriting… on a computer without a graphics card this can take a minute.',
            'Пренаписване… на компютър без графична карта това може да отнеме минута.'],
        'writing.rewritten': ['Rewritten by {model}: {removed} words taken out, {added} put in.',
            'Пренаписано от {model}: премахнати думи — {removed}, добавени — {added}.'],
        'writing.keep': ['Use this version', 'Използвай тази версия'],
        'writing.discard': ['Discard', 'Откажи'],
        'writing.kept': ['The new version is in the text box.', 'Новата версия е в полето за текст.'],
        'writing.compareTitle': ['Compare two versions', 'Сравняване на две версии'],
        'writing.before': ['Earlier version', 'По-ранна версия'],
        'writing.after': ['Later version', 'По-късна версия'],
        'writing.compare': ['Compare', 'Сравни'],
        'writing.same': ['The two versions are the same.', 'Двете версии са еднакви.'],
        'writing.diffCounts': ['{removed} words taken out, {added} put in, {same} unchanged.',
            'Премахнати думи: {removed}, добавени: {added}, без промяна: {same}.'],
        'writing.legendRemoved': ['Taken out', 'Премахнато'],
        'writing.legendAdded': ['Put in', 'Добавено'],
        'writing.failed': ['That did not work: {why}', 'Не се получи: {why}'],
        // Translation and the glossary.
        'nav.translation': ['Translation', 'Превод'],
        'tr.title': ['Translation', 'Превод'],
        'tr.intro': [
            'Translate a text, or a whole file into a new file with the same layout. Every translation follows '
                + 'your glossary and is checked afterwards: a term not translated as agreed, or a number, address or '
                + 'link that did not survive, is listed for you to look at. Translation uses the language model.',
            'Преведете текст или цял файл в нов файл със същото оформление. Всеки превод спазва речника ви и '
                + 'се проверява след това: термин, преведен не както е уговорено, или число, адрес или връзка, '
                + 'които са се изгубили, се изреждат, за да ги прегледате. Преводът използва езиковия модел.',
        ],
        'tr.from': ['From', 'От'],
        'tr.to': ['Into', 'На'],
        'tr.detect': ['Detect', 'Автоматично'],
        'tr.source': ['Text', 'Текст'],
        'tr.result': ['Translation', 'Превод'],
        'tr.placeholder': ['Paste or type the text. Ctrl+Enter translates it.',
            'Поставете или въведете текста. Ctrl+Enter го превежда.'],
        'tr.go': ['Translate', 'Преведи'],
        'tr.working': ['Translating… on a computer without a graphics card this can take a minute.',
            'Превеждане… на компютър без графична карта това може да отнеме минута.'],
        'tr.done': ['Translated. The glossary was followed and nothing was lost.',
            'Преведено. Речникът е спазен и нищо не е изгубено.'],
        'tr.checkThese': ['Translated — check the lines below.', 'Преведено — проверете редовете по-долу.'],
        'tr.used': ['Glossary terms used: {terms}', 'Използвани термини от речника: {terms}'],
        'tr.missTerm': ['"{source}" should be translated as "{want}", and that is not in the translation.',
            '„{source}“ трябва да се преведе като „{want}“, а това го няма в превода.'],
        'tr.missKept': ['"{source}" is in the original and not in the translation.',
            '„{source}“ го има в оригинала, но не и в превода.'],
        'tr.fileTitle': ['Translate a file', 'Превод на файл'],
        'tr.formats': ['Formats: {formats}. The translation is written beside the original, as report.bg.docx for example; nothing is replaced.',
            'Формати: {formats}. Преводът се записва до оригинала, например report.bg.docx; нищо не се заменя.'],
        'tr.path': ['Full path of the file', 'Пълен път до файла'],
        'tr.fileGo': ['Translate the file', 'Преведи файла'],
        'tr.needPath': ['Give the full path of the file.', 'Въведете пълния път до файла.'],
        'tr.fileWorking': ['Translating the file… a long document takes several minutes.',
            'Превеждане на файла… дълъг документ отнема няколко минути.'],
        'tr.fileDone': ['Translated {n} parts and wrote {path}', 'Преведени части: {n}. Записан файл: {path}'],
        'gl.title': ['Glossary', 'Речник'],
        'gl.intro': [
            'Your agreed translations: a word or phrase and how it is always translated into a language. A term '
                + 'translated as itself is never translated — a name or a brand. Kept with your knowledge base.',
            'Вашите уговорени преводи: дума или израз и как винаги се превежда на даден език. Термин, '
                + 'преведен като себе си, никога не се превежда — име или марка. Пази се в базата ви знания.',
        ],
        'gl.source': ['Term', 'Термин'],
        'gl.target': ['Always translated as', 'Винаги се превежда като'],
        'gl.any': ['Any language', 'Всеки език'],
        'gl.anyShort': ['any', 'всеки'],
        'gl.note': ['Note (optional)', 'Бележка (по избор)'],
        'gl.noteShort': ['Note', 'Бележка'],
        'gl.langs': ['Languages', 'Езици'],
        'gl.add': ['Add', 'Добави'],
        'gl.added': ['Added: {term} → {target}.', 'Добавено: {term} → {target}.'],
        'gl.changed': ['Changed: {term} → {target}.', 'Променено: {term} → {target}.'],
        'gl.show': ['Show', 'Покажи'],
        'gl.all': ['All languages', 'Всички езици'],
        'gl.export': ['Save as a spreadsheet', 'Запази като таблица'],
        'gl.count': ['{n} of {all} terms', '{n} от {all} термина'],
        'gl.empty': ['No terms yet. Add the words you want translated the same way every time.',
            'Все още няма термини. Добавете думите, които искате да се превеждат винаги еднакво.'],
        'gl.remove': ['Remove', 'Премахни'],
        'gl.removeOne': ['Remove {term} from the glossary', 'Премахване на „{term}“ от речника'],
        'gl.sure': ['Press again to remove', 'Натиснете отново'],
        'gl.removed': ['Removed {term}.', 'Премахнато: {term}.'],
        'gl.importTitle': ['Add many at once', 'Добавяне на много наведнъж'],
        'gl.importHelp': [
            'One term per line: the term, then its translation, separated by a comma, a semicolon or a tab — '
                + 'as a spreadsheet saves them. Optional third to fifth columns: from, into, note. Lines without '
                + 'languages use the ones chosen above.',
            'По един термин на ред: терминът и преводът му, разделени със запетая, точка и запетая или табулация — '
                + 'както ги записва програма за таблици. По избор колони три до пет: от, на, бележка. Редовете без '
                + 'езици използват избраните по-горе.',
        ],
        'gl.import': ['Add these', 'Добави ги'],
        'gl.imported': ['Terms added: {n}.', 'Добавени термини: {n}.'],
        'gl.problems': ['Not added — {list}', 'Не са добавени — {list}'],
        // The mail desk.
        'nav.mail': ['Mail', 'Поща'],
        'mail.title': ['New mail', 'Нова поща'],
        'mail.intro': [
            'Sorted from the headers alone, without opening anything: what looks suspicious, what needs a reply, '
                + 'what is from people, and newsletters and notifications by sender. Read from your mail server, '
                + 'or from Thunderbird on this computer.',
            'Подредена само по заглавната информация, без да се отваря нищо: какво изглежда подозрително, какво '
                + 'чака отговор, кое е от хора, а бюлетините и известията — по подател. Чете се от пощенския ви '
                + 'сървър или от Thunderbird на този компютър.',
        ],
        'mail.period': ['Since', 'От'],
        'mail.day': ['The last 24 hours', 'Последните 24 часа'],
        'mail.three': ['The last 3 days', 'Последните 3 дни'],
        'mail.week': ['The last 7 days', 'Последните 7 дни'],
        'mail.reading': ['Reading the mail…', 'Четене на пощата…'],
        'mail.counts': ['{total} messages · {unseen} not opened · from {source}',
            '{total} писма · {unseen} неотворени · от {source}'],
        'mail.morning': ['A summary of new mail every morning, in the conversation',
            'Обобщение на новата поща всяка сутрин, в разговора'],
        'mail.morningOn': ['From tomorrow at eight, a summary of new mail every morning.',
            'От утре в осем — обобщение на новата поща всяка сутрин.'],
        'mail.morningOff': ['No morning summary.', 'Без сутрешно обобщение.'],
        'mail.fromServer': ['your mail server', 'пощенския ви сървър'],
        'mail.fromLocal': ['Thunderbird on this computer', 'Thunderbird на този компютър'],
        'mail.nothing': ['Nothing new in this period.', 'Няма нищо ново за този период.'],
        'mail.none': ['There is no mailbox to read. Set up your mail account under Settings, or use Thunderbird on this computer.',
            'Няма пощенска кутия за четене. Настройте пощенския си акаунт в „Настройки“ или използвайте Thunderbird на този компютър.'],
        'mail.suspicious': ['Suspicious — do not act on these without checking', 'Подозрителни — не действайте по тях без проверка'],
        'mail.reply': ['Needs a reply', 'Чака отговор'],
        'mail.people': ['From people', 'От хора'],
        'mail.bulk': ['Newsletters and notifications', 'Бюлетини и известия'],
        'mail.unread': ['new', 'ново'],
        'mail.noSubject': ['(no subject)', '(без тема)'],
        'mail.draftIt': ['Draft a reply', 'Подготви отговор'],
        'mail.draftAsk': ['Read the message {ref} from {who} and draft a reply to it.',
            'Прочети писмото {ref} от {who} и подготви чернова на отговор.'],
        'mail.draftsTitle': ['Waiting for your yes', 'Чакат вашето „да“'],
        'mail.draftsIntro': [
            'Replies I have drafted. Nothing is sent until you press Send on that message — once, as it stands '
                + 'here. Change the text first if you like; it is saved when you send.',
            'Отговори, които съм подготвил. Нищо не се изпраща, докато не натиснете „Изпрати“ за съответното '
                + 'писмо — веднъж и точно както е тук. Можете първо да промените текста; той се запазва при изпращане.',
        ],
        'mail.noSend': ['Sending needs your mail account under Settings. Until then, copy the text into your mail program.',
            'За изпращане е нужен пощенският ви акаунт в „Настройки“. Дотогава копирайте текста в пощенската си програма.'],
        'mail.noDrafts': ['No drafts are waiting.', 'Няма чакащи чернови.'],
        'mail.draftHead': ['Draft {id} · {at}', 'Чернова {id} · {at}'],
        'mail.answers': ['answers {what}', 'отговор на {what}'],
        'mail.warning': ['Careful — the message this answers: {what}', 'Внимание — писмото, на което отговаря: {what}'],
        'mail.failedNote': ['The last try did not send: {why}', 'Последният опит не беше изпратен: {why}'],
        'mail.to': ['To', 'До'],
        'mail.subject': ['Subject', 'Тема'],
        'mail.body': ['Message', 'Съобщение'],
        'mail.save': ['Save changes', 'Запази промените'],
        'mail.saved': ['Saved.', 'Запазено.'],
        'mail.send': ['Send', 'Изпрати'],
        'mail.sendSure': ['Send to {to}?', 'Изпращане до {to}?'],
        'mail.sent': ['Sent to {to}.', 'Изпратено до {to}.'],
        'mail.discard': ['Discard', 'Откажи'],
        'mail.done': ['Sent and discarded', 'Изпратени и отказани'],
        'mail.wasSent': ['sent', 'изпратено'],
        'mail.wasDiscarded': ['discarded', 'отказано'],
        'deeds.stayedTrait': ['stayed on this machine', 'остана на този компютър'],
    };

    // What tools.Leaves says leaves, in both languages.
    const LEAVES = {
        'the address it is asked to read': 'адреса, който трябва да прочете',
        'the search words, to a search engine': 'думите за търсене — към търсачка',
        'the message, to the people it is addressed to': 'съобщението — към получателите му',
        'what the page you open is sent': 'това, което се изпраща към отворената страница',
        'your login, to your own mail server': 'данните за вход — към вашия пощенски сървър',
        'a request for the download — nothing about you': 'заявка за изтегляне — нищо за вас',
        'what the integration is given': 'това, което се подава на интеграцията',
        'web search': 'търсене в интернет',
        'mail': 'поща',
        'your mail server': 'вашия пощенски сървър',
        'an integration off this machine': 'интеграция извън компютъра',
    };

    // Each capability's name, for people rather than for the model.
    const TOOLS = {
        acquire_integration: ['Find and add an integration', 'Намиране и добавяне на интеграция'],
        activate_integration: ['Switch an integration on', 'Включване на интеграция'],
        ask_first: ['Ask you a question', 'Въпрос към вас'],
        audit_originality: ['Check a catalogue for originality', 'Проверка на каталог за оригиналност'],
        brain_copies: ['Copies of its knowledge', 'Копия на знанията'],
        change_a_setting: ['Change a setting', 'Промяна на настройка'],
        change_the_diary: ['Change the diary', 'Промяна в дневника'],
        change_this_conversation: ['Rename or tidy this conversation', 'Промяна на този разговор'],
        check_for_updates: ['Check for updates', 'Проверка за обновления'],
        check_name: ['Check a name is free', 'Проверка дали име е свободно'],
        check_requirements: ['Check what a task needs', 'Проверка на изискванията'],
        click: ['Click on the screen', 'Щракване по екрана'],
        closest_titles: ['Find the closest titles', 'Най-близки заглавия'],
        compare_melody: ['Compare a melody', 'Сравняване на мелодия'],
        compare_pictures: ['Compare pictures', 'Сравняване на изображения'],
        compare_versions: ['Compare two versions', 'Сравняване на версии'],
        change_glossary: ['Change the glossary', 'Промяна в речника'],
        confirm_hire: ['Confirm a new specialist', 'Потвърждаване на специалист'],
        decide_waiting: ['Decide what is waiting', 'Решение по чакащи действия'],
        draft_reply: ['Draft a reply', 'Чернова на отговор'],
        do_in_background: ['Work in the background', 'Работа във фонов режим'],
        drive_browser: ['Use its own browser', 'Работа със собствен браузър'],
        edit_document: ['Edit a document', 'Редактиране на документ'],
        edit_file: ['Edit a file', 'Редактиране на файл'],
        fetch_url: ['Fetch a web address', 'Изтегляне на уеб адрес'],
        films_without_subtitles: ['Find films without subtitles', 'Филми без субтитри'],
        forget_how_to_do_this: ['Forget a learned procedure', 'Забравяне на научена процедура'],
        forget_reminder: ['Remove a reminder', 'Премахване на напомняне'],
        game_build: ['Build a game', 'Компилиране на игра'],
        game_check: ['Check a game', 'Проверка на игра'],
        game_docs: ['Read game engine documentation', 'Документация на игрови двигател'],
        game_engines: ['Game engines on this machine', 'Игрови двигатели на компютъра'],
        glossary: ['Glossary', 'Речник'],
        godot_build: ['Build with Godot', 'Компилиране с Godot'],
        godot_docs: ['Read the Godot documentation', 'Документация на Godot'],
        godot_status: ['Godot on this machine', 'Godot на компютъра'],
        hire_agent: ['Propose a new specialist', 'Предложение за нов специалист'],
        how_fast_can_you_answer: ['Measure answer speed', 'Измерване на скоростта на отговор'],
        inspect_project: ['Look at a project', 'Преглед на проект'],
        install_a_model: ['Install a model', 'Инсталиране на модел'],
        install_a_part: ['Install a part', 'Инсталиране на компонент'],
        install_requirement: ['Install what a task needs', 'Инсталиране на необходимото'],
        learn_from_folder: ['Learn from a folder', 'Учене от папка'],
        list_background: ['List background work', 'Фонова работа'],
        list_devices: ['List devices in the house', 'Устройства в дома'],
        list_directory: ['List a folder', 'Съдържание на папка'],
        list_drives: ['List drives', 'Дискове'],
        list_integrations: ['List integrations', 'Интеграции'],
        list_models: ['List models', 'Модели'],
        list_reminders: ['List reminders', 'Напомняния'],
        list_waiting: ['List what is waiting', 'Чакащи действия'],
        list_windows: ['List open windows', 'Отворени прозорци'],
        look_at_screen: ['Look at the screen', 'Поглед към екрана'],
        mail_digest: ['Sum up new mail', 'Обобщение на новата поща'],
        make_a_picture: ['Make a picture', 'Създаване на изображение'],
        make_a_video: ['Make a video', 'Създаване на видео'],
        make_subtitles: ['Make subtitles', 'Създаване на субтитри'],
        open_app: ['Open an application', 'Отваряне на приложение'],
        places_it_learns_from: ['Places it learns from', 'Места, от които се учи'],
        plan_project: ['Plan a project', 'Планиране на проект'],
        proofread: ['Proofread', 'Коректура'],
        put_in_the_diary: ['Add to the diary', 'Запис в дневника'],
        put_it_back: ['Put a file back', 'Връщане на файл'],
        read_a_page: ['Read a web page', 'Четене на уеб страница'],
        read_document: ['Read a document', 'Четене на документ'],
        read_email: ['Read email', 'Четене на имейл'],
        read_file: ['Read a file', 'Четене на файл'],
        read_integration: ['Read from an integration', 'Четене от интеграция'],
        read_mailbox: ['Read the mailbox', 'Четене на пощенската кутия'],
        remember_how_to_do_this: ['Learn a procedure', 'Научаване на процедура'],
        remind_me: ['Set a reminder', 'Напомняне'],
        remove_a_part: ['Remove a part', 'Премахване на компонент'],
        run_as_admin: ['Run as administrator', 'Изпълнение като администратор'],
        run_command: ['Run a command', 'Изпълнение на команда'],
        scroll: ['Scroll', 'Превъртане'],
        search_files: ['Search files', 'Търсене във файлове'],
        send_draft: ['Send a draft', 'Изпращане на чернова'],
        send_email: ['Send email', 'Изпращане на имейл'],
        set_appearance: ['Change how it looks', 'Промяна на външния вид'],
        set_device: ['Control a device', 'Управление на устройство'],
        set_wake_word: ['Change the wake word', 'Промяна на думата за активиране'],
        start_project: ['Start a project', 'Започване на проект'],
        stop_background: ['Stop background work', 'Спиране на фонова работа'],
        stop_hearing_this_machine: ['Stop hearing this machine', 'Изключване на звука от компютъра'],
        translate: ['Translate a text', 'Превод на текст'],
        translate_document: ['Translate a file', 'Превод на файл'],
        type_text: ['Type text', 'Въвеждане на текст'],
        web_search: ['Search the web', 'Търсене в интернет'],
        what_am_i_hearing: ['What it is hearing', 'Какво чува'],
        what_can_you_do: ['What it can do', 'Какво може'],
        what_is_on: ['What is on the diary', 'Какво има в дневника'],
        what_ive_been_taught: ['What it has been taught', 'Какво е научило'],
        what_this_machine_needs: ['What this machine needs', 'Какво е нужно на компютъра'],
        what_you_know: ['What it knows', 'Какво знае'],
        write_document: ['Write a document', 'Писане на документ'],
        write_file: ['Write a file', 'Запис на файл'],
        write_originality_report: ['Write an originality report', 'Доклад за оригиналност'],
    };

    function guess() {
        const said = (navigator.language || '').toLowerCase();

        return said.startsWith('bg') ? 'bg' : 'en';
    }

    let lang = guess();

    function T(key, vars) {
        const pair = STRINGS[key];
        let text = pair ? pair[lang === 'bg' ? 1 : 0] : key;

        if (vars) {
            for (const [name, value] of Object.entries(vars)) {
                text = text.split('{' + name + '}').join(String(value));
            }
        }

        return text;
    }

    function toolName(name) {
        const pair = TOOLS[name];

        if (pair) return pair[lang === 'bg' ? 1 : 0];

        return String(name || '').replace(/_/g, ' ');
    }

    function leavesWhat(what) {
        if (lang !== 'bg' || !what) return what || '';

        return LEAVES[what] || what;
    }

    // Fills every element that names its string, in the language now chosen.
    function apply(root) {
        const scope = root || document;

        scope.querySelectorAll('[data-i18n]').forEach((node) => {
            node.textContent = T(node.dataset.i18n);
        });

        scope.querySelectorAll('[data-i18n-title]').forEach((node) => {
            node.title = T(node.dataset.i18nTitle);
        });

        scope.querySelectorAll('[data-i18n-placeholder]').forEach((node) => {
            node.placeholder = T(node.dataset.i18nPlaceholder);
        });

        scope.querySelectorAll('[data-i18n-label]').forEach((node) => {
            node.setAttribute('aria-label', T(node.dataset.i18nLabel));
        });

        document.documentElement.lang = lang;
    }

    function set(next) {
        if (next !== 'bg' && next !== 'en') return;

        const changed = next !== lang;

        lang = next;
        apply();

        if (changed) window.dispatchEvent(new CustomEvent('pn-language', { detail: lang }));
    }

    window.pnLang = () => lang;
    window.pnT = T;
    window.pnToolName = toolName;
    window.pnLeavesWhat = leavesWhat;
    window.pnApplyLanguage = apply;
    window.pnSetLanguage = set;

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => apply());
    } else {
        apply();
    }

    fetch('/api/ui-language', { headers: { Accept: 'application/json' } })
        .then((res) => (res.ok ? res.json() : null))
        .then((body) => {
            const data = body && (body.data !== undefined ? body.data : body);

            if (data && data.language) set(data.language);
        })
        .catch(() => { /* the browser's language stands */ });
})();
