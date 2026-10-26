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
        // Learning.
        'nav.learning': ['Learning', 'Учене'],
        'study.title': ['Learn something', 'Научете нещо'],
        'study.intro': [
            'Name any topic and choose a level: I write a lesson with worked examples, or practice questions '
                + 'answered one at a time. Answers are marked by rules wherever a rule can tell, and the questions '
                + 'you miss come back until you know them. Lessons and questions are written by a language '
                + 'model, which can be wrong, so check anything important against a textbook or a teacher.',
            'Напишете тема и изберете ниво: пиша урок с решени примери или въпроси за упражнение, по един '
                + 'наведнъж. Отговорите се проверяват по правила, където правило може да реши, а въпросите, '
                + 'на които сгрешите, се връщат, докато ги научите. Уроците и въпросите се пишат от езиков '
                + 'модел, който може да греши, затова проверявайте важното в учебник или с учител.',
        ],
        'study.topic': ['What to learn', 'Какво да научите'],
        'study.topicHint': ['e.g. fractions, the water cycle, Python loops', 'напр. дроби, кръговратът на водата, цикли в Python'],
        'study.level': ['Level', 'Ниво'],
        'study.level.child': ['Child (8–12)', 'Дете (8–12 г.)'],
        'study.level.beginner': ['Beginner', 'Начинаещ'],
        'study.level.intermediate': ['Intermediate', 'Напреднал'],
        'study.level.advanced': ['Advanced', 'Експерт'],
        'study.language': ['Language', 'Език'],
        'study.auto': ['Same as the topic', 'Като темата'],
        'study.minutes': ['Lesson length', 'Дължина на урока'],
        'study.min10': ['About 10 minutes', 'Около 10 минути'],
        'study.min15': ['About 15 minutes', 'Около 15 минути'],
        'study.min30': ['About 30 minutes', 'Около 30 минути'],
        'study.min45': ['About 45 minutes', 'Около 45 минути'],
        'study.count': ['Questions', 'Въпроси'],
        'study.teach': ['Teach me', 'Научи ме'],
        'study.practise': ['Practise', 'Упражнение'],
        'study.practiseThis': ['Practise this topic', 'Упражнение по тази тема'],
        'study.close': ['Close', 'Затвори'],
        'study.needTopic': ['Write what you want to learn first.', 'Първо напишете какво искате да научите.'],
        'study.writingLesson': ['Writing the lesson… this can take a minute or two.', 'Пиша урока… може да отнеме минута-две.'],
        'study.writingQuiz': ['Writing the questions… this can take a minute.', 'Пиша въпросите… може да отнеме около минута.'],
        'study.lessonReady': ['The lesson is ready and kept with the topic.', 'Урокът е готов и е запазен към темата.'],
        'study.quizReady': ['{n} questions are ready.', 'Готови са {n} въпроса.'],
        'study.dropped': ['{n} could not be marked and were left out.', '{n} не можеха да се проверят и са пропуснати.'],
        'study.answers': ['Show the answers', 'Покажи отговорите'],
        'study.by': ['written by {model}', 'написано от {model}'],
        'study.quizTitle': ['Practice', 'Упражнение'],
        'study.reviewTitle': ['Review', 'Преговор'],
        'study.progress': ['Question {i} of {n}', 'Въпрос {i} от {n}'],
        'study.yourAnswer': ['Your answer', 'Вашият отговор'],
        'study.check': ['Check', 'Провери'],
        'study.checking': ['Checking…', 'Проверявам…'],
        'study.right': ['Right.', 'Вярно.'],
        'study.wrong': ['Not quite. The answer is: {answer}', 'Не съвсем. Отговорът е: {answer}'],
        'study.unsure': ['I cannot tell for certain. The expected answer is: {answer}. Was yours the same?',
            'Не мога да преценя със сигурност. Очакваният отговор е: {answer}. Вашият същият ли беше?'],
        'study.selfRight': ['I was right', 'Бях прав(а)'],
        'study.selfWrong': ['I was wrong', 'Сгреших'],
        'study.learnt': ['Learnt — this question now rests for a month.', 'Научено — този въпрос почива един месец.'],
        'study.again': ['It comes back {when}.', 'Ще се върне {when}.'],
        'study.soon': ['in a few minutes', 'след няколко минути'],
        'study.tomorrow': ['tomorrow', 'утре'],
        'study.inDays': ['in {n} days', 'след {n} дни'],
        'study.markedBy': ['Marked by {who}.', 'Проверено от {who}.'],
        'study.byRule': ['a rule', 'правило'],
        'study.byLearner': ['you', 'вас'],
        'study.byModel': ['the model {model}, against the expected answer', 'модела {model} спрямо очаквания отговор'],
        'study.next': ['Next question', 'Следващ въпрос'],
        'study.finish': ['See the result', 'Виж резултата'],
        'study.stop': ['Stop', 'Спри'],
        'study.done': ['Done: {right} of {n} right.', 'Готово: {right} от {n} верни.'],
        'study.doneMissed': ['The ones you missed come back in a few minutes; review them from the list below.',
            'Сгрешените се връщат след няколко минути; преговорете ги от списъка по-долу.'],
        'study.doneAll': ['All right — well done.', 'Всичко е вярно — браво.'],
        'study.nothingDue': ['Nothing is due for review now.', 'В момента няма нищо за преговор.'],
        'study.topicsTitle': ['What you are learning', 'Какво учите'],
        'study.counts': ['{topics} topics · {cards} questions · {learnt} learnt', 'Теми: {topics} · въпроси: {cards} · научени: {learnt}'],
        'study.reviewAll': ['Review everything due ({n})', 'Преговор на всичко ({n})'],
        'study.review': ['Review ({n})', 'Преговор ({n})'],
        'study.more': ['More questions', 'Още въпроси'],
        'study.lesson': ['Lesson', 'Урок'],
        'study.noLesson': ['There is no lesson for this topic yet: press Teach me.', 'Към тази тема още няма урок: натиснете „Научи ме“.'],
        'study.remove': ['Remove', 'Премахни'],
        'study.removed': ['Removed.', 'Премахнато.'],
        'study.stats': ['{cards} questions · {learnt} learnt · {score}', 'въпроси: {cards} · научени: {learnt} · {score}'],
        'study.score': ['{pct}% right', '{pct}% верни'],
        'study.notYet': ['not practised yet', 'още без упражнение'],
        'study.due': ['{n} due', 'за преговор: {n}'],
        'study.empty': ['Nothing yet. Name a topic above and press Teach me or Practise.',
            'Още няма нищо. Напишете тема горе и натиснете „Научи ме“ или „Упражнение“.'],
        'study.noModel': ['No language model is available to write lessons and questions. Questions already kept can still be practised.',
            'Няма езиков модел, който да пише уроци и въпроси. Вече запазените въпроси могат да се упражняват.'],

        // Customers.
        'nav.customers': ['Customers', 'Клиенти'],
        'care.title': ['Answer a customer', 'Отговор на клиент'],
        'care.intro': [
            'Paste a customer\'s message and I write a reply from your own answer book and the documents in your '
                + 'business folder — prices, terms, opening hours — in the customer\'s language. Before you see it, '
                + 'the reply is checked: a question your words cannot answer is left for you as [[ASK: …]], and a '
                + 'promise of money or time, or a figure you never gave, is marked. Nothing is sent: a reply waits on '
                + 'the Mail page for your yes, and one with a question still in it cannot be sent.',
            'Поставете съобщението на клиента и аз пиша отговор от вашата книга с отговори и документите в папката '
                + 'на фирмата — цени, условия, работно време — на езика на клиента. Преди да го видите, отговорът се '
                + 'проверява: въпрос, на който думите ви не отговарят, остава за вас като [[ASK: …]], а обещание за '
                + 'пари или срок или число, което никога не сте давали, се отбелязва. Нищо не се изпраща: отговорът '
                + 'чака одобрението ви в „Поща“, а отговор с неотговорен въпрос не може да бъде изпратен.',
        ],
        'care.message': ['The customer\'s message', 'Съобщението на клиента'],
        'care.messageHint': ['Hello, do you deliver to Plovdiv, and how much does it cost?', 'Здравейте, доставяте ли до Пловдив и колко струва?'],
        'care.language': ['Reply in', 'Отговор на'],
        'care.auto': ['The customer\'s language', 'Езика на клиента'],
        'care.write': ['Write a reply', 'Напиши отговор'],
        'care.writing': ['Reading your answers and documents and writing…', 'Чета отговорите и документите ви и пиша…'],
        'care.readyShort': ['The reply is below; nothing in it needs your decision.', 'Отговорът е по-долу; в него няма нищо за ваше решение.'],
        'care.look': ['The reply is below. Look at the marked points first.', 'Отговорът е по-долу. Първо прегледайте отбелязаното.'],
        'care.noModel': ['No language model is available to write the reply.', 'Няма езиков модел, който да напише отговора.'],
        'care.replyTitle': ['The reply', 'Отговорът'],
        'care.replyText': ['Change anything before keeping it', 'Променете каквото трябва, преди да го запазите'],
        'care.gap': ['Only you can answer', 'Само вие можете да отговорите'],
        'care.promise': ['A promise nobody has made', 'Обещание, което никой не е давал'],
        'care.unsupported': ['Not in your words', 'Не е от вашите думи'],
        'care.alwaysPoint': ['You decide these yourself', 'Това решавате сами'],
        'care.okMark': ['Ready', 'Готово'],
        'care.ready': ['Everything in it comes from your own answers and documents.', 'Всичко в него е от вашите отговори и документи.'],
        'care.basedOn': ['Written from: {what}', 'Написан от: {what}'],
        'care.basedOnNothing': ['Nothing in your answer book or documents matches this message, so the reply leaves the answers to you.',
            'Нищо в книгата с отговори или документите ви не отговаря на това съобщение, затова отговорите са оставени на вас.'],
        'care.to': ['Customer\'s email', 'Имейл на клиента'],
        'care.subject': ['Subject', 'Тема'],
        'care.keep': ['Keep as a mail draft', 'Запази като чернова'],
        'care.copy': ['Copy', 'Копирай'],
        'care.learn': ['Save in the answer book', 'Запази в книгата с отговори'],
        'care.learnHint': ['Check the question and the answer below, then add them.', 'Проверете въпроса и отговора по-долу и ги добавете.'],
        'care.needTo': ['Give the customer\'s email to keep the reply as a draft.', 'Въведете имейла на клиента, за да запазите отговора като чернова.'],
        'care.kept': ['Kept as draft {n}. It waits on the Mail page for your yes.', 'Запазен като чернова {n}. Чака одобрението ви в „Поща“.'],
        'care.keptGaps': ['Kept as draft {n}. Fill in the [[ASK: …]] gaps on the Mail page before sending it.',
            'Запазен като чернова {n}. Попълнете празните места [[ASK: …]] в „Поща“, преди да го изпратите.'],
        'care.copied': ['Copied.', 'Копирано.'],
        'care.copyByHand': ['Selected — press Ctrl+C to copy.', 'Маркирано — натиснете Ctrl+C, за да копирате.'],
        'care.bookTitle': ['Answer book', 'Книга с отговори'],
        'care.bookIntro': [
            'The questions customers ask, answered the way you want them answered. Replies use these first, word for word where they fit.',
            'Въпросите, които клиентите задават, с отговорите, които искате да получат. Отговорите използват първо тях, дословно, където пасват.',
        ],
        'care.q': ['Question', 'Въпрос'],
        'care.qHint': ['e.g. Do you deliver on Saturdays?', 'напр. Доставяте ли в събота?'],
        'care.a': ['Answer, as you want it given', 'Отговор, както искате да бъде даден'],
        'care.add': ['Add to the answer book', 'Добави в книгата'],
        'care.change': ['Save the change', 'Запази промяната'],
        'care.edit': ['Change', 'Промени'],
        'care.added': ['Kept in the answer book.', 'Запазено в книгата с отговори.'],
        'care.empty': ['No answers yet. Add the questions your customers ask most.', 'Още няма отговори. Добавете въпросите, които клиентите ви задават най-често.'],
        'care.profileTitle': ['Your business', 'Вашата фирма'],
        'care.profileIntro': [
            'How replies sound and what they may draw on. Documents are read on this computer: text, Markdown, PDF, Word, OpenDocument, spreadsheets.',
            'Как звучат отговорите и от какво могат да черпят. Документите се четат на този компютър: текст, Markdown, PDF, Word, OpenDocument, таблици.',
        ],
        'care.name': ['Business name', 'Име на фирмата'],
        'care.tone': ['Tone', 'Тон'],
        'care.friendly': ['Friendly', 'Приятелски'],
        'care.formal': ['Formal', 'Официален'],
        'care.signature': ['Signature', 'Подпис'],
        'care.folder': ['Folder with your prices, terms and hours', 'Папка с цени, условия и работно време'],
        'care.folderHint': ['/home/you/Documents/Shop', '/home/you/Документи/Магазин'],
        'care.always': ['Always leave to me messages about', 'Винаги оставяй на мен съобщения за'],
        'care.alwaysHint': ['e.g. lawyer, press, wholesale', 'напр. адвокат, медии, на едро'],
        'care.save': ['Save', 'Запази'],
        'care.saved': ['Saved.', 'Запазено.'],
        'care.files': ['{n} documents found in the folder.', 'Намерени документи в папката: {n}.'],

        // Tables.
        'nav.tables': ['Tables', 'Таблици'],
        'tab.title': ['Work with a table', 'Работа с таблица'],
        'tab.intro': [
            'Give the path of a table — a budget, sales, expenses — as .csv, .xlsx or .ods. I look it over first: '
                + 'every column\'s sums, cells that are not numbers, repeated rows, and whether the sheet\'s own totals '
                + 'are right. Then ask a question and the program works it out exactly — the language model only turns '
                + 'your question into a recipe, it never adds anything up — and checks every total a second way from '
                + 'the rows. Nothing in your file is changed, and nothing leaves this computer.',
            'Въведете пътя до таблица — бюджет, продажби, разходи — във формат .csv, .xlsx или .ods. Първо я '
                + 'преглеждам: сумите на всяка колона, клетките, които не са числа, повтарящите се редове и дали '
                + 'собствените суми на таблицата са верни. След това задайте въпрос и програмата го изчислява точно — '
                + 'езиковият модел само превръща въпроса в рецепта и никога не събира сам — и проверява всяка сума по '
                + 'втори начин от редовете. Файлът ви не се променя и нищо не излиза от този компютър.',
        ],
        'tab.path': ['Table (.csv, .xlsx, .ods)', 'Таблица (.csv, .xlsx, .ods)'],
        'tab.pathHint': ['/home/you/Documents/budget.xlsx', '/home/you/Документи/бюджет.xlsx'],
        'tab.sheet': ['Sheet', 'Лист'],
        'tab.look': ['Look it over', 'Прегледай'],
        'tab.reading': ['Reading the table…', 'Чета таблицата…'],
        'tab.rows': ['{n} rows', 'редове: {n}'],
        'tab.col': ['Column', 'Колона'],
        'tab.kind': ['Kind', 'Вид'],
        'tab.kind.number': ['numbers', 'числа'],
        'tab.kind.text': ['text', 'текст'],
        'tab.kind.empty': ['empty', 'празна'],
        'tab.filled': ['Filled', 'Попълнени'],
        'tab.blank': ['Blank', 'Празни'],
        'tab.sum': ['Sum', 'Сума'],
        'tab.min': ['Smallest', 'Най-малко'],
        'tab.max': ['Largest', 'Най-голямо'],
        'tab.mean': ['Average', 'Средно'],
        'tab.checks': ['Checks', 'Проверки'],
        'tab.ok': ['OK', 'Наред'],
        'tab.lookAt': ['Look at this', 'Прегледайте'],
        'tab.askTitle': ['Work something out', 'Изчислете нещо'],
        'tab.question': ['Question', 'Въпрос'],
        'tab.questionHint': ['e.g. totals by category, without rent, largest first', 'напр. суми по категория, без наема, най-голямата първа'],
        'tab.run': ['Work it out', 'Изчисли'],
        'tab.working': ['Turning the question into a calculation and checking it…', 'Превръщам въпроса в изчисление и го проверявам…'],
        'tab.allChecked': ['Done. Every total was checked a second way.', 'Готово. Всяка сума е проверена по втори начин.'],
        'tab.someToLook': ['Done, but {n} checks need a look — see below.', 'Готово, но {n} проверки изискват внимание — вижте по-долу.'],
        'tab.noModel': ['No language model is available to turn the question into a calculation. Looking the table over still works.',
            'Няма езиков модел, който да превърне въпроса в изчисление. Прегледът на таблицата работи и без него.'],
        'tab.forecast': ['Estimate', 'Прогноза'],
        'tab.forecastNote': ['An estimate from the past periods, not a promise: the average of the last three, or the straight-line trend.',
            'Оценка по минали периоди, а не обещание: средното от последните три или правата линия на тенденцията.'],
        'tab.recipe': ['How it was calculated', 'Как е изчислено'],
        'tab.out': ['Save as a new workbook', 'Запазване като нова работна книга'],
        'tab.resultName': ['result', 'резултат'],
        'tab.save': ['Save as .xlsx', 'Запази като .xlsx'],
        'tab.saved': ['Saved to {path}, with the checks on their own sheet.', 'Запазено в {path}, с проверките на отделен лист.'],
        'tab.exists': ['There is already a file with that name; choose another.', 'Вече има файл с това име; изберете друго.'],
        'tab.f.unread': ['{col}: {n} cells are not numbers and are left out of its sums (rows {rows})',
            '{col}: {n} клетки не са числа и не влизат в сумите (редове {rows})'],
        'tab.f.ambiguous': ['{col}: numbers like 1,250 could be a thousand or one and a quarter, and nothing in the column says which; they are not counted until written unambiguously',
            '{col}: числа като 1,250 може да са хиляда или едно цяло и четвърт и нищо в колоната не показва кое; не се броят, докато не се запишат еднозначно'],
        'tab.f.duplicate': ['rows {rows} are the same in every cell — counted twice?', 'редове {rows} съвпадат във всяка клетка — броени два пъти?'],
        'tab.f.clean': ['Every number could be read; no repeated rows; no total rows to compare.',
            'Всички числа се прочетоха; няма повтарящи се редове; няма редове със суми за сравнение.'],
        'tab.f.totalOK': ['row {row}, {col}: the sheet\'s total {value} matches its rows', 'ред {row}, {col}: сумата в таблицата {value} съвпада с редовете ѝ'],
        'tab.f.totalWrong': ['row {row}, {col}: the sheet says {stated} but rows {from}–{to} add up to {sum} (a difference of {diff})',
            'ред {row}, {col}: в таблицата пише {stated}, а редове {from}–{to} дават {sum} (разлика {diff})'],
        'tab.f.skipped': ['rows {rows} are the sheet\'s own totals and were left out, so nothing is counted twice',
            'редове {rows} са собствени суми на таблицата и са изключени, за да не се брои нищо два пъти'],
        'tab.f.filterMismatch': ['the filter kept {kept} rows, but {again} rows match it when checked again',
            'филтърът остави {kept} реда, а при повторна проверка му отговарят {again}'],
        'tab.f.filterOK': ['{n} of {of} rows match the filter, checked twice', '{n} от {of} реда отговарят на филтъра, проверено два пъти'],
        'tab.f.sumLimited': ['{col}: the groups shown add up to {shown} of {total} in all (only the first {limit} groups are shown)',
            '{col}: показаните групи дават {shown} от общо {total} (показани са само първите {limit} групи)'],
        'tab.f.sumOK': ['{col}: the groups add up to {total}, the same as the rows added one by one',
            '{col}: групите дават {total}, колкото и редовете, събрани един по един'],
        'tab.f.sumMismatch': ['{col}: the groups add up to {shown} but the rows add up to {total}', '{col}: групите дават {shown}, а редовете — {total}'],
        'tab.f.sumUnread': ['{col}: {n} cells are not numbers and are not in the sum (rows {rows})',
            '{col}: {n} клетки не са числа и не са в сумата (редове {rows})'],
        'tab.f.groupsOK': ['all {n} rows are in exactly one of {groups} groups', 'всички {n} реда са в точно една от {groups} групи'],
        'tab.f.groupsMismatch': ['{n} rows in {groups} groups, but {passed} rows passed the filter', '{n} реда в {groups} групи, а през филтъра минаха {passed}'],
        'tab.f.nothing': ['{n} rows used; nothing needed checking a second way', 'използвани редове: {n}; нямаше нужда от втора проверка'],

        // Invoices and VAT.
        'nav.invoices': ['Invoices & VAT', 'Фактури и ДДС'],
        'books.title': ['Invoices and VAT', 'Фактури и ДДС'],
        'books.intro': [
            'Keep a register of the invoices you issue and receive. Each one is checked as it is added — the VAT is the '
                + 'rate of the net to the cent, the total adds up, the rate is one the country has, an issued invoice has '
                + 'the right kind of number and none is used twice — and a month\'s VAT is worked out from them, with the '
                + 'date the return is due and how near your sales are to the registration threshold. Everything stays on this computer.',
            'Водете регистър на издадените и получените фактури. Всяка се проверява при добавяне — ДДС е точно '
                + 'ставката от основата до стотинка, общата сума е вярна, ставката съществува в страната, издадената '
                + 'фактура има правилен номер и никой номер не се повтаря, — а от тях се изчислява ДДС за месеца, '
                + 'срокът за справка-декларацията и колко близо са продажбите ви до прага за регистрация. Всичко остава на този компютър.',
        ],
        'books.notAdvice': [
            'This is information, not tax advice. The rules below are the law as it read on the date shown; it changes. '
                + 'Check with your accountant or the tax authority before you file anything.',
            'Това е информация, а не данъчен съвет. Правилата по-долу са законът към показаната дата, а той се променя. '
                + 'Проверете със счетоводителя си или с НАП, преди да подадете каквото и да е.',
        ],
        'books.period': ['Month', 'Месец'],
        'books.sum': ['Work out the VAT', 'Изчисли ДДС'],
        'books.line': ['', ''],
        'books.count': ['Invoices', 'Фактури'],
        'books.salesAt': ['Sales at {rate} %', 'Продажби с {rate} %'],
        'books.purchasesAt': ['Purchases at {rate} %', 'Покупки с {rate} %'],
        'books.reverseLine': ['Reverse charge (no VAT charged)', 'Обратно начисляване (без начислен ДДС)'],
        'books.toPay': ['VAT on sales {output}, on purchases {input}: {result} {currency} to pay.',
            'ДДС от продажби {output}, от покупки {input}: {result} {currency} за внасяне.'],
        'books.toDeduct': ['VAT on sales {output}, on purchases {input}: {result} {currency} to deduct or carry forward.',
            'ДДС от продажби {output}, от покупки {input}: {result} {currency} за приспадане или възстановяване.'],
        'books.due': ['Due by {date}. {rule}', 'Срок: {date}. {rule}'],
        'books.noDueForSpan': ['The tax period is a month: each month of this span has its own return and due date.',
            'Данъчният период е месец: всеки месец от този период има своя справка-декларация и срок.'],
        'books.addTitle': ['Add an invoice', 'Добавяне на фактура'],
        'books.kind': ['Kind', 'Вид'],
        'books.sale': ['Sale (issued)', 'Продажба (издадена)'],
        'books.purchase': ['Purchase (received)', 'Покупка (получена)'],
        'books.saleShort': ['Sale', 'Продажба'],
        'books.purchaseShort': ['Purchase', 'Покупка'],
        'books.readPath': ['Read it from a document (PDF, Word, scan)', 'Прочитане от документ (PDF, Word, сканиран)'],
        'books.readHint': ['/home/you/Documents/invoice.pdf', '/home/you/Документи/фактура.pdf'],
        'books.read': ['Read', 'Прочети'],
        'books.reading': ['Reading the invoice…', 'Чета фактурата…'],
        'books.readDone': ['Read. Check every field against the document before adding it.', 'Прочетено. Сверете всяко поле с документа, преди да я добавите.'],
        'books.noModel': ['No language model is available to read the invoice; type its fields in instead.',
            'Няма езиков модел, който да прочете фактурата; въведете полетата ръчно.'],
        'books.number': ['Number', 'Номер'],
        'books.date': ['Date', 'Дата'],
        'books.party': ['Customer or supplier', 'Клиент или доставчик'],
        'books.partyVAT': ['Their VAT number', 'Техният ДДС номер'],
        'books.net': ['Net (without VAT)', 'Основа (без ДДС)'],
        'books.rate': ['VAT rate', 'Ставка на ДДС'],
        'books.vat': ['VAT', 'ДДС'],
        'books.gross': ['Total', 'Общо'],
        'books.auto': ['worked out when empty', 'изчислява се, ако е празно'],
        'books.note': ['Note (e.g. the basis for a zero rate)', 'Бележка (напр. основание за нулева ставка)'],
        'books.reverse': ['Reverse charge', 'Обратно начисляване'],
        'books.reverseShort': ['reverse', 'обратно'],
        'books.add': ['Add to the register', 'Добави в регистъра'],
        'books.check': ['Only check', 'Само провери'],
        'books.added': ['Invoice {n} is in the register.', 'Фактура {n} е в регистъра.'],
        'books.checkedOnly': ['Checked; not added.', 'Проверена; не е добавена.'],
        'books.twice': ['An issued invoice with that number is already in the register.', 'Вече има издадена фактура с този номер в регистъра.'],
        'books.importTitle': ['Import a table of invoices', 'Внасяне на таблица с фактури'],
        'books.importIntro': ['One invoice a row, with columns such as date, number, customer or supplier, net, rate, VAT and total. Every row is checked; rows that are totals or have no date are left out.',
            'По една фактура на ред, с колони като дата, номер, клиент или доставчик, основа, ставка, ДДС и общо. Всеки ред се проверява; редовете със суми или без дата се пропускат.'],
        'books.import': ['Import', 'Внеси'],
        'books.imported': ['{n} invoices added to the register.', 'Добавени фактури в регистъра: {n}.'],
        'books.skipped': ['Rows left out: {rows}.', 'Пропуснати редове: {rows}.'],
        'books.registerTitle': ['Invoice register', 'Регистър на фактурите'],
        'books.empty': ['No invoices yet.', 'Още няма фактури.'],
        'books.rulesTitle': ['The rules used', 'Използвани правила'],
        'books.country': ['Country', 'Държава'],
        'books.ownRates': ['Your VAT rates, in %', 'Вашите ставки на ДДС, в %'],
        'books.currency': ['Currency', 'Валута'],
        'books.ownRules': ['Your own rates are used; nothing is said about the law of your country. Check them with your accountant.',
            'Използват се вашите ставки; нищо не се твърди за закона на вашата държава. Проверете ги със счетоводителя си.'],
        'books.checked': ['Checked against the law as published on {date}.', 'Сверено с публикувания закон на {date}.'],
        'books.fields': ['What an invoice must contain', 'Какво трябва да съдържа фактурата'],
        'books.sources': ['Sources', 'Източници'],
        'books.sourceChecked': ['checked {date}', 'проверено на {date}'],
        'books.inForce': ['in force from {date}', 'в сила от {date}'],
        'books.f.vatMath': ['invoice {number}: {rate} % of {net} is {want}, but the invoice says {vat}', 'фактура {number}: {rate} % от {net} е {want}, а във фактурата пише {vat}'],
        'books.f.grossMath': ['invoice {number}: {net} + {vat} is {want}, but the invoice says {gross}', 'фактура {number}: {net} + {vat} е {want}, а във фактурата пише {gross}'],
        'books.f.rateUnknown': ['invoice {number}: {rate} % is not one of the rates, which are {rates}', 'фактура {number}: {rate} % не е сред ставките, които са {rates}'],
        'books.f.zeroBasis': ['invoice {number}: a zero rate needs the legal basis written on the invoice', 'фактура {number}: при нулева ставка във фактурата трябва да е посочено основанието'],
        'books.f.numberForm': ['invoice {number}: an issued invoice\'s number does not have the form the law requires', 'фактура {number}: номерът на издадената фактура не е във формата, който изисква законът'],
        'books.f.vatID': ['invoice {number}: {id} does not look like a VAT number of the country', 'фактура {number}: {id} не прилича на ДДС номер от страната'],
        'books.f.noNumber': ['an invoice has no number', 'фактура без номер'],
        'books.f.noDate': ['invoice {number} has no date, or one that is not a date', 'фактура {number} няма дата или датата не е вярна'],
        'books.f.future': ['invoice {number} is dated {date}, in the future', 'фактура {number} е с дата {date}, в бъдещето'],
        'books.f.noParty': ['invoice {number} does not say who it is with', 'фактура {number} не посочва другата страна'],
        'books.f.duplicate': ['invoice number {number} is in the register {n} times', 'номер {number} е в регистъра {n} пъти'],
        'books.f.gap': ['issued invoice numbers are missing between {from} and {to}: {missing}', 'липсват номера на издадени фактури между {from} и {to}: {missing}'],
        'books.f.reverseVAT': ['invoice {number} is reverse charge but shows VAT of {vat}; under reverse charge the invoice shows no rate and no amount',
            'фактура {number} е с обратно начисляване, но показва ДДС {vat}; при обратно начисляване не се посочват ставка и размер на данъка'],
        'books.f.amount': ['invoice {number}: {value} is not an amount', 'фактура {number}: {value} не е сума'],
        'books.f.invoiceOK': ['invoice {number}: the amounts add up and the rate is allowed', 'фактура {number}: сумите са верни и ставката е допустима'],
        'books.f.periodOK': ['{n} invoices in {period}; every amount adds up', 'фактури за {period}: {n}; всички суми са верни'],
        'books.f.threshold': ['sales in {year} so far: {sales} of the {threshold} registration threshold ({pct} %)',
            'продажби през {year} досега: {sales} от прага за регистрация {threshold} ({pct} %)'],
        'books.f.overThreshold': ['sales in {year} passed the {threshold} registration threshold on {date}, with invoice {number}',
            'продажбите през {year} надминаха прага за регистрация {threshold} на {date}, с фактура {number}'],

        // Research with sources.
        'nav.research': ['Research', 'Проучване'],
        'research.title': ['Research a question', 'Проучете въпрос'],
        'research.intro': [
            'Ask a question and I look it up on the web: I choose what to search for, read the best sources I '
                + 'find — official bodies and scholarly sources first, never one site over and over — and write a '
                + 'report from them only, with a number after every claim. Then every sentence is checked against '
                + 'the source it cites, and what the sources do not say is marked. Your question is sent to a search '
                + 'engine and each page is requested from its site; the report is kept here. A language model writes '
                + 'the report and can be wrong, so read the marked claims in their source before relying on them.',
            'Задайте въпрос и аз го проучвам в интернет: избирам какво да търся, чета най-добрите намерени източници '
                + '— първо официални органи и научни източници, никога един и същ сайт отново и отново — и пиша '
                + 'доклад само от тях, с номер след всяко твърдение. След това всяко изречение се проверява спрямо '
                + 'източника, който посочва, и се отбелязва какво източниците не казват. Въпросът ви се изпраща на '
                + 'търсачка и всяка страница се изисква от сайта ѝ; докладът се пази тук. Докладът се пише от езиков '
                + 'модел, който може да греши, затова прочетете отбелязаните твърдения в източника, преди да разчитате на тях.',
        ],
        'research.noWeb': ['Research reads the web, and the privacy setting keeps this machine to itself. Allow the web in Control to use it.',
            'Проучването чете интернет, а настройката за поверителност не позволява излизане от този компютър. Разрешете интернет в „Контрол“, за да го използвате.'],
        'research.noModel': ['No language model is available to write the report.', 'Няма езиков модел, който да напише доклада.'],
        'research.nothing': ['No source could be found or read for that question. Try other words.',
            'Не беше намерен или прочетен нито един източник за този въпрос. Опитайте с други думи.'],
        'research.question': ['Question', 'Въпрос'],
        'research.questionHint': ['e.g. What changed in EU rules on online returns in 2025?', 'напр. Какво се промени в правилата на ЕС за връщане на онлайн покупки през 2025 г.?'],
        'research.depth': ['Depth', 'Дълбочина'],
        'research.depth.quick': ['Quick — 4 sources', 'Бързо — 4 източника'],
        'research.depth.thorough': ['Thorough — 8 sources', 'Задълбочено — 8 източника'],
        'research.depthName.quick': ['Quick', 'Бързо'],
        'research.depthName.thorough': ['Thorough', 'Задълбочено'],
        'research.language': ['Report in', 'Доклад на'],
        'research.auto': ['Same as the question', 'Езика на въпроса'],
        'research.run': ['Research', 'Проучи'],
        'research.working': ['Searching, reading and checking… {s} s. This usually takes one to three minutes.',
            'Търся, чета и проверявам… {s} сек. Обикновено отнема от една до три минути.'],
        'research.done': ['Done: {n} sources read. The report is below and kept in Your research.',
            'Готово: прочетени източници — {n}. Докладът е по-долу и е запазен в „Вашите проучвания“.'],
        'research.nSources': ['{n} sources', 'източници: {n}'],
        'research.supported': ['{n} of {of} claims supported', 'подкрепени твърдения: {n} от {of}'],
        'research.flagged': ['{n} to check', 'за проверка: {n}'],
        'research.sourceN': ['Source {n}', 'Източник {n}'],
        'research.download': ['Download (.md)', 'Изтегляне (.md)'],
        'research.open': ['Open', 'Отвори'],
        'research.removedReport': ['The report was removed.', 'Докладът е премахнат.'],
        'research.listTitle': ['Your research', 'Вашите проучвания'],
        'research.empty': ['Nothing yet. Ask a question above and press Research.',
            'Още няма нищо. Задайте въпрос горе и натиснете „Проучи“.'],
        'research.checkTitle': ['Check a text against its sources', 'Проверка на текст спрямо източниците му'],
        'research.checkIntro': [
            'Paste a text that puts [1], [2] … after its claims — an essay, an article, an answer — and list its '
                + 'sources in the same order, as web addresses or full paths of files on this computer. Every sentence '
                + 'is checked against the source it cites. Nothing is kept.',
            'Поставете текст, в който след твърденията има [1], [2] … — есе, статия, отговор — и изброете '
                + 'източниците му в същия ред като уеб адреси или пълни пътища до файлове на този компютър. Всяко '
                + 'изречение се проверява спрямо източника, който посочва. Нищо не се запазва.',
        ],
        'research.checkText': ['Text with [1], [2] … after its claims', 'Текст с [1], [2] … след твърденията'],
        'research.checkSources': ['Sources, one per line, in the order of their numbers', 'Източници, по един на ред, в реда на номерата им'],
        'research.checkSourcesHint': ['https://example.org/page\n/home/you/Documents/notes.pdf', 'https://example.org/page\n/home/you/Документи/бележки.pdf'],
        'research.checkRun': ['Check the claims', 'Провери твърденията'],
        'research.checking': ['Reading the sources and checking…', 'Чета източниците и проверявам…'],
        'research.checked': ['{n} claims: {ok} supported, {bad} to check.', 'Твърдения: {n} — подкрепени: {ok}, за проверка: {bad}.'],
        'research.removed': ['{n} citations pointed to no source and were removed.', 'Премахнати препратки към несъществуващ източник: {n}.'],
        'research.unread': ['Could not read {what}', 'Не можа да се прочете {what}'],
        'research.markOk': ['Supported', 'Подкрепено'],
        'research.markCheck': ['Check', 'Проверете'],
        'research.status.unsourced': ['It cites no source.', 'Не посочва източник.'],
        'research.status.number': ['A figure in it is not in the cited source.', 'Число в него го няма в посочения източник.'],
        'research.status.weak': ['Few of its words are in the cited source.', 'Малко от думите му са в посочения източник.'],
        'research.status.unread': ['The cited source could not be read.', 'Посоченият източник не можа да бъде прочетен.'],
        'research.verdict.confirmed': ['Confirmed by the fact check', 'Потвърдено при проверката'],
        'research.verdict.partly': ['Only partly supported', 'Само частично подкрепено'],
        'research.verdict.not_supported': ['Not supported by the cited source', 'Не се подкрепя от посочения източник'],

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
        // Plan: what is next, to-do lists, the online calendar.
        'nav.plan': ['Plan', 'План'],
        'plan.title': ['What is next', 'Какво предстои'],
        'plan.intro': [
            'Everything coming up in one time order: the diary, your online calendar, reminders and to-dos '
                + 'that are due. Ask in the conversation too: "what is my week like?", "put milk on the shopping list".',
            'Всичко предстоящо, подредено по време: дневникът, онлайн календарът ви, напомнянията и задачите '
                + 'със срок. Можете да питате и в разговора: „какво ми предстои тази седмица?“, „добави мляко в списъка за пазаруване“.',
        ],
        'plan.ahead': ['Ahead', 'Напред'],
        'plan.two': ['Today and tomorrow', 'Днес и утре'],
        'plan.week': ['The next 7 days', 'Следващите 7 дни'],
        'plan.fortnight': ['The next 14 days', 'Следващите 14 дни'],
        'plan.today': ['Today', 'Днес'],
        'plan.tomorrow': ['Tomorrow', 'Утре'],
        'plan.allDay': ['all day', 'цял ден'],
        'plan.overdue': ['Overdue', 'Просрочени'],
        'plan.undated': ['To-dos with no date', 'Задачи без срок'],
        'plan.more': ['and {n} more', 'и още {n}'],
        'plan.nothing': ['Nothing in the next {days} days.', 'Няма нищо през следващите {days} дни.'],
        'plan.missing': ['Could not look in {what}.', 'Не можах да погледна в {what}.'],
        'plan.kind.event': ['diary', 'дневник'],
        'plan.kind.online': ['online calendar', 'онлайн календар'],
        'plan.kind.reminder': ['reminder', 'напомняне'],
        'plan.kind.todo': ['due', 'срок'],
        'todo.title': ['To-do lists', 'Списъци със задачи'],
        'todo.what': ['What to do', 'Какво да се направи'],
        'todo.list': ['List', 'Списък'],
        'todo.listHint': ['shopping, home, work…', 'пазар, дом, работа…'],
        'todo.due': ['Due (optional)', 'Срок (по избор)'],
        'todo.allLists': ['All lists', 'Всички списъци'],
        'todo.general': ['general', 'общ'],
        'todo.empty': ['Nothing to do on this list.', 'Няма задачи в този списък.'],
        'todo.doneTitle': ['Done lately', 'Свършени наскоро'],
        'todo.needText': ['Write what is to be done.', 'Напишете какво трябва да се направи.'],
        'todo.added': ['Added to {list}: {what}.', 'Добавено в „{list}“: {what}.'],
        'todo.tick': ['Mark "{what}" as done', 'Отбележи „{what}“ като свършено'],
        'todo.undo': ['Mark "{what}" as not done', 'Отбележи „{what}“ като несвършено'],
        'todo.removeOne': ['Remove "{what}"', 'Премахни „{what}“'],
        'todo.overdue': ['overdue since {date}', 'просрочено от {date}'],
        'todo.dueToday': ['due today', 'срок днес'],
        'todo.dueTomorrow': ['due tomorrow', 'срок утре'],
        'todo.dueOn': ['due {date}', 'срок {date}'],
        'cal.title': ['Online calendar', 'Онлайн календар'],
        'cal.intro': [
            'Connect the calendar you keep on a server — Nextcloud, Fastmail, iCloud, a hosting company — through '
                + 'its CalDAV address and an app password. Its appointments then appear under What is next, and you '
                + 'can ask me to add one; I never change or delete anything there. The password goes only to that address.',
            'Свържете календара, който поддържате на сървър — Nextcloud, Fastmail, iCloud, хостинг компания — чрез '
                + 'неговия CalDAV адрес и парола за приложение. Срещите от него ще се показват в „Какво предстои“ и '
                + 'можете да ме помолите да добавя среща; никога не променям и не изтривам нищо там. Паролата отива само на този адрес.',
        ],
        'cal.url': ['CalDAV address of the calendar', 'CalDAV адрес на календара'],
        'cal.user': ['User name', 'Потребителско име'],
        'cal.password': ['App password', 'Парола за приложение'],
        'cal.save': ['Check and save', 'Провери и запази'],
        'cal.remove': ['Remove', 'Премахни'],
        'cal.on': ['Connected: {url}', 'Свързан: {url}'],
        'cal.off': ['No online calendar is connected.', 'Няма свързан онлайн календар.'],
        'cal.keep': ['saved — leave empty to keep it', 'запазена — оставете празно, за да остане'],
        'cal.checking': ['Asking the calendar server…', 'Проверка при сървъра на календара…'],
        'cal.saved': ['Connected to the calendar "{name}".', 'Свързан с календара „{name}“.'],
        'cal.removed': ['The online calendar is no longer connected.', 'Онлайн календарът вече не е свързан.'],
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
        'your calendar server': 'вашия сървър за календар',
        'your login, to your own calendar server': 'данните за вход — към вашия сървър за календар',
        'the appointment, to your own calendar server': 'срещата — към вашия сървър за календар',
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
        business_answers: ['Show the answer book', 'Книгата с отговори'],
        calculate_table: ['Work something out from a table', 'Изчисление от таблица'],
        change_a_setting: ['Change a setting', 'Промяна на настройка'],
        change_business_answers: ['Change the answer book', 'Промяна в книгата с отговори'],
        change_the_diary: ['Change the diary', 'Промяна в дневника'],
        change_this_conversation: ['Rename or tidy this conversation', 'Промяна на този разговор'],
        check_for_updates: ['Check for updates', 'Проверка за обновления'],
        check_answer: ['Mark an answer', 'Проверка на отговор'],
        check_claims: ['Check claims against their sources', 'Проверка на твърдения спрямо източниците'],
        check_name: ['Check a name is free', 'Проверка дали име е свободно'],
        check_requirements: ['Check what a task needs', 'Проверка на изискванията'],
        click: ['Click on the screen', 'Щракване по екрана'],
        closest_titles: ['Find the closest titles', 'Най-близки заглавия'],
        compare_melody: ['Compare a melody', 'Сравняване на мелодия'],
        compare_pictures: ['Compare pictures', 'Сравняване на изображения'],
        compare_versions: ['Compare two versions', 'Сравняване на версии'],
        change_glossary: ['Change the glossary', 'Промяна в речника'],
        confirm_hire: ['Confirm a new specialist', 'Потвърждаване на специалист'],
        customer_reply: ['Write a reply to a customer', 'Отговор на клиент'],
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
        import_invoices: ['Import a table of invoices', 'Внасяне на таблица с фактури'],
        how_fast_can_you_answer: ['Measure answer speed', 'Измерване на скоростта на отговор'],
        inspect_project: ['Look at a project', 'Преглед на проект'],
        install_a_model: ['Install a model', 'Инсталиране на модел'],
        install_a_part: ['Install a part', 'Инсталиране на компонент'],
        install_requirement: ['Install what a task needs', 'Инсталиране на необходимото'],
        learn_from_folder: ['Learn from a folder', 'Учене от папка'],
        lesson: ['Write a lesson', 'Урок'],
        list_background: ['List background work', 'Фонова работа'],
        list_devices: ['List devices in the house', 'Устройства в дома'],
        list_directory: ['List a folder', 'Съдържание на папка'],
        list_drives: ['List drives', 'Дискове'],
        list_integrations: ['List integrations', 'Интеграции'],
        list_models: ['List models', 'Модели'],
        list_reminders: ['List reminders', 'Напомняния'],
        list_todos: ['List the to-dos', 'Списъци със задачи'],
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
        quiz: ['Practice questions', 'Въпроси за упражнение'],
        read_a_page: ['Read a web page', 'Четене на уеб страница'],
        read_calendar: ['Read the online calendar', 'Четене на онлайн календара'],
        read_document: ['Read a document', 'Четене на документ'],
        read_email: ['Read email', 'Четене на имейл'],
        read_file: ['Read a file', 'Четене на файл'],
        read_integration: ['Read from an integration', 'Четене от интеграция'],
        read_mailbox: ['Read the mailbox', 'Четене на пощенската кутия'],
        record_invoice: ['Check and record an invoice', 'Проверка и запис на фактура'],
        remember_how_to_do_this: ['Learn a procedure', 'Научаване на процедура'],
        remind_me: ['Set a reminder', 'Напомняне'],
        remove_a_part: ['Remove a part', 'Премахване на компонент'],
        research: ['Research a question with sources', 'Проучване на въпрос с източници'],
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
        study_progress: ['Learning progress', 'Напредък в ученето'],
        table_summary: ['Look a table over', 'Преглед на таблица'],
        stop_hearing_this_machine: ['Stop hearing this machine', 'Изключване на звука от компютъра'],
        todo: ['Keep the to-do lists', 'Задачи в списъците'],
        translate: ['Translate a text', 'Превод на текст'],
        translate_document: ['Translate a file', 'Превод на файл'],
        type_text: ['Type text', 'Въвеждане на текст'],
        vat_summary: ['Work out a month\'s VAT', 'Изчисляване на ДДС за месеца'],
        web_search: ['Search the web', 'Търсене в интернет'],
        what_am_i_hearing: ['What it is hearing', 'Какво чува'],
        what_can_you_do: ['What it can do', 'Какво може'],
        what_is_next: ['What is next', 'Какво предстои'],
        what_is_on: ['What is on the diary', 'Какво има в дневника'],
        what_ive_been_taught: ['What it has been taught', 'Какво е научило'],
        what_this_machine_needs: ['What this machine needs', 'Какво е нужно на компютъра'],
        what_you_know: ['What it knows', 'Какво знае'],
        write_calendar: ['Add to the online calendar', 'Запис в онлайн календара'],
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
