# Changelog

What changed, in the terms somebody using it would notice. The full history is
in the commits; this is the part worth reading before updating.

Versions follow [semantic versioning](https://semver.org). From 1.0.0 on, the
first number changes only when something you rely on stops working the way it
did (a setting, a command, where the data lives); the second when something
new arrives; the third for fixes. Before 1.0.0 the second number changed
whenever something visible changed.

## 1.0.0 (2026-11-10)

The first version we call finished. Everything the 0.x releases added is
here. Before it is published, every release is updated in our tests from the
version before it (an old brain opened by the new program, nothing lost) and
installed, updated and removed on Windows. Once it is published, its files
are installed the way a stranger meets them: on a clean Ubuntu 24.04 and a
clean Windows, by the written instructions alone, and removed again. That
second check can only run after the release is out, so a problem it finds is
fixed in the next version. On Ubuntu new versions arrive through Software
Updater; on Windows the assistant says when one is out and installs it with
one button. What it does not do yet, or has not yet been
seen doing, is written down in
[docs/guide/known-limits.md](docs/guide/known-limits.md)
([на български](docs/guide/known-limits.bg.md)).

Nothing you have needs to change: a brain from 0.13.0 or earlier opens as it
is, and your settings stay. The one thing tidied on the way is the review
queue 0.13.0 filled with its own guides (below).

### Security

- **Both parts of voice recognition are checked before they are used.** The
  library that runs it and the model that tells voices apart are downloaded
  the first time you turn the feature on. Each is now refused unless it is
  byte for byte the published file: the library as Microsoft released ONNX
  Runtime 1.31.0, the model as it is in one fixed commit of its Hugging Face
  repository (until now it was taken from whatever that repository's main
  branch held). Both SHA-256 sums are fixed in the program and both
  downloads have a size limit, so a changed, damaged or endless download
  never ends up on your computer. A model fetched by an earlier version is
  checked the same way the first time voice recognition is used after the
  update; if it is not the published file it is not used, and the Privacy
  page offers to fetch the right one.

- **The voice and listening parts are fixed files too.** The neural voice
  (Piper 2023.11.14-2 for 64-bit Intel and ARM computers), its three voices,
  the speech model and whisper.cpp were all fetched from whatever their
  projects held on the day; the Piper program, which runs on your computer,
  had no check at all. Now each comes from one release or commit with its
  SHA-256 fixed in the program, and whisper.cpp is built only from release
  v1.9.5 after checking that its commit is the expected one. A copy installed
  by an earlier version is brought to that release, keeping the speech model
  you already downloaded. A file that is not the published one is never
  written under its real name, every download has a size limit, and the
  archive is fetched into a temporary file of its own instead of a fixed name
  others could create first.

- **Ollama is a fixed release too.** Setup fetched the newest Ollama with no
  check, and Ollama is a program that runs on your computer as a service.
  Now it installs Ollama 0.40.11 for 64-bit Intel and ARM Linux, each archive
  with its SHA-256 fixed in the program (the sums Ollama publishes with the
  release) and an exact size limit, checked before anything in your home
  folder is replaced. An Ollama you already have that is as new or newer is
  kept and used as it is, never downgraded; an older one is replaced only
  when you press Install for it.

- **"Answer only me" no longer lets everybody through when it cannot
  recognise your voice.** If the part that tells voices apart was missing,
  or was the previous version's runtime right after an update, no voice was
  checked and every voice was answered while the switch still showed on.
  Now, with the switch on, what it hears is not acted on until your voice
  can be recognised again; the command centre says why, the Privacy page
  shows it beside the switch (in English or Bulgarian, as the page is), and
  typing works as always.

- **Imported skills are not taken into memory unreviewed.** Copies of agent
  skills imported from your disk are kept in the brain's knowledge folder so
  they travel with it, and were read into memory there as if they were the
  program's own guides. They are skills, used when a message is about them,
  and are no longer read as memories. Only the guides and your own files in
  that folder are taken in without asking, and a path that climbs out of the
  folder is not trusted because of where it starts.

### Voices

- **New voices, free to use for anything.** The three voices 0.13 installed
  (Jenny (Dioco), alba and northern_english_male) were each fine-tuned from a
  voice whose recordings are licensed for research only. They are replaced by
  three trained from scratch on recordings in the public domain: the robot,
  still the default and still a woman's voice spoken as a machine, is now
  made from Cori, a British woman; the woman is LJ Speech and the man is
  Norman, both American. After the update the new voices arrive by
  themselves the first time the program starts (while *May look things up
  about itself* is on; otherwise *Voice (speaking)* shows an Update button),
  a voice you had chosen from the old three becomes the robot, and the old
  files are no longer used. Bulgarian is spoken by the system voice as
  before.

### Credits

- **The voices and parts are credited.** A new card in What it runs on,
  *Credits and licences* (English and Bulgarian), names who made each voice
  and from what (Cori, LJ Speech and Norman, trained by Bryce Beattie from
  LibriVox recordings in the public domain), and credits Piper, whisper.cpp,
  OpenAI's Whisper model and Ollama. THIRD-PARTY-NOTICES.md lists the same
  with the pinned releases.

### Fixed

- **A question no longer waits behind background learning.** Reading folders
  and learning from the last conversation now step aside while the assistant
  answers or while you speak, and a long piece of learning that is already
  running is stopped and started again afterwards. A question that begins
  while learning is already waiting is waited for as well, rather than
  costing it one of its tries. Stopped a fourth time in a row, that piece is
  left for later: what a folder said is filed the next
  time the program starts, and the conversation is looked at again after its
  next answer. On a four-core computer without a graphics card a question
  asked while a new brain read its own guides used to wait about fourteen
  minutes.

- **A brain from 0.13.0 reads again after the update.** 0.13.0 stored every
  reading of its own guides again on each pass, so its review queue passed
  the two hundred at which reading stops, on every drive. When this version
  first opens such a brain it removes the repeats and the readings of
  skills, files one copy of each guide's reading the way this version does,
  and leaves everything else in the queue (what your documents and
  conversations taught) waiting for you as before. This happens once.

- **A new brain reads its own guides once.** The knowledge folder inside the
  brain was never marked as read, so its first sixty guides were read again
  every ninety seconds for as long as the program ran. It is now read to the
  end and left alone; what the guides say is known at once instead of waiting
  for you in the review queue, and the built-in skills stay skills rather
  than being copied into memory line by line. What a document says is now
  counted under documents in What it knows, not under conversation.

- **Every document in a folder is read whole, and you hear when the folder is
  done.** Folders are read a bite at a time, and a bite could stop in the
  middle of a document: its first sentences were remembered and the rest was
  never read. Each bite now ends between documents. What is left to read is
  counted in documents again, so "I have finished reading" is said once when
  a folder is done, and a folder is no longer reported finished while most
  of it is still unread.

- **The answer on screen stays while background work finishes.** A folder
  bite, or the voice recognition download, that ended during an answer used
  to clear the "Thinking" line or write its own note over it.

- **The status shows when the model does not answer.** The top bar said
  "online" and named the model whenever the program itself was running, even
  with Ollama stopped and every question failing. It now says the model is
  not answering (or is not allowed in your privacy mode), with a "Why?"
  button to the Models view.

- **Telling voices apart keeps working after an update.** The program now
  uses ONNX Runtime 1.31.0 (with onnxruntime_go 1.38.0). If you turned the
  feature on in an earlier version, the old runtime is recognised when the
  program starts and replaced with the matching one in the background, about
  11 MB, without asking again and without teaching it your voice again. With
  looking things up online switched off, the Privacy page shows the part as
  missing and its button fetches it. The refresh and the button never
  download at the same time: pressed while a download runs, the button says
  so instead of starting another.

- **An update on Windows that cannot finish gives you your assistant back.**
  When something else still holds the program file (a virus scanner, a copy
  opened by hand while the update runs), the installer waits a minute,
  changes nothing if the file is still held, and opens the copy that is
  installed, instead of leaving the program closed.

## 0.13.0 (2026-11-08)

### Your rules

- **Say it once and it becomes a rule, if you say yes.** "From now on,
  always ask me before you send anything", „никога не трий без да ме питаш",
  "it's pointless to write who is writing at the top of an email": a card
  under the answer proposes it in your own words, with the message it came
  from. Keep it, change it first, or say it is not a rule; nothing becomes a
  rule by itself, and a declined sentence is not proposed again.
- **Rules are followed, not only remembered.** They are read before every
  answer and every step of a goal, and checked before anything goes out or
  stays (sending, posting, commits, changing files, paying, deleting), and
  again afterwards. A rule that plainly forbids an action stops it and says
  which rule, whatever else is allowed; one whose words go further than can
  be checked ("never email clients") asks you instead, with the rule beside
  the question.
- **A Rules page** lists them with where and when each came from and what it
  does to an action, with a switch, an editor and a delete each.

### The last look before anything goes out

- **Every email the assistant sends is polished before you are asked**: what
  the reader already sees (who is writing, the subject again), sentences that
  say nothing, anything said twice and sure spelling slips are taken out, and
  placeholders such as "[Name]", a wrong weekday or a machine talking about
  itself are pointed at. The question shows the words as they will go and
  what was changed; a message with a placeholder left in is not sent; after
  sending it is shown again with where it went, from which address and when.
  The Writing page has the same **Last look before sending**.

### Claude Code, Codex and Cursor's agent as helpers

- **Ask a coding agent on this computer**, signed in with your own account,
  to do one task in a folder you choose: only read (the default), change
  files in that folder without running commands, or full access, which is
  asked about every time even when everything else is allowed. Your rules go
  with the task, the answer comes back with every file the folder shows it
  added, changed or removed and the commands it ran, and the question says
  how its allowance is being spent. No sign-in is read or kept, and no API
  key is passed to it.

### Looking after this computer

- **"Can I restart?"**: answered from the machine: a newer kernel waiting and
  whether its drivers built by DKMS (a USB Wi-Fi adapter's, a graphics
  card's) are ready for it, software being installed, the assistant's own
  work still running, work not committed or not pushed. On the System page
  and in conversation. Linux.
- **Leftovers**: a headless browser, a virtual display, a waiting command, a
  log follower or a small web server whose job is over, started by the
  assistant or orphaned (never your own programs or services), listed and
  stopped only with your yes.
- **After a power cut** the assistant says so and when it was last seen
  working; a task it was running is parked with that reason.

### Fixed

- **Offline, the Updates card no longer says "Everything is current".** When
  a check could not be made (no network, or the daily check switched off) it
  says that nothing it could check is out of date and points to the row that
  explains why.

## 0.12.0 (2026-11-08)

### Installing and updating, with the least effort

- **Ubuntu: one line, then updates come by themselves.** The assistant has its
  own apt repository, signed with its own key and published with every
  release (`https://pnscripts.github.io/pn-scripts-assistant/apt`). One line
  adds it, `sudo apt install pn-scripts-assistant` installs, and from then on
  **Software Updater** brings each new version with your other updates
  (`sudo apt upgrade` does the same). The `.deb` adds the same repository when
  installed by hand, as Chrome and VS Code do; delete
  `/etc/apt/sources.list.d/pn-scripts-assistant.sources` to stop it, and
  `apt purge` removes it.
- **A new version is announced in the window.** A line across the top and the
  *What it runs on* page say when one is out, with both version numbers. The
  check is one request a day to GitHub's public releases page with nothing
  about you in it; it can be switched off there, and it says so plainly when
  there is no network.
- **Windows: one click updates it.** *Update now* downloads the new installer
  from the release, accepts it only if its SHA-256 matches the release's
  `SHA256SUMS`, closes the assistant, installs over it and opens it again.
  Nothing it learned is touched. The portable zip is never replaced by itself:
  its button opens the release page.
- **`pn-scripts-assistant update`** does the same from a terminal on Linux,
  and `update --check` only asks.
- Installing and updating are explained first in the guides, in English and
  Bulgarian.

### Proved like a user

- A clean Ubuntu 24.04 adds the repository with the documented line, installs
  the previous version from it and updates with `apt upgrade`, after every
  release.
- On a Windows runner an older installed copy is told a new version is out,
  and the same request the button sends downloads, checks and installs it; the
  program closes, the new version opens, the brain stays.
- winget manifests are written from every published release, checked with
  `winget validate`, and the previous and newest versions are installed the
  way winget installs them (its download, hash check and switches); the
  manifests are attached to the release, ready for the winget community
  repository.

## 0.11.0 (2026-11-08)

### Changed

- **An idle window no longer keeps the processor busy.** A minute after the
  last key or mouse movement the page rests: every poller and animation is
  held and one quiet connection (`/api/pulse`) stays open, which wakes it the
  moment the assistant starts answering, speaks, hears somebody or wants a
  decision. Minimised or hidden, even that connection closes. Measured on
  four cores without a graphics card, the untouched window went from 45 % of
  a core (and up to 86 % more in the display server) to about 2 % (and 1 %);
  hidden, about 0.5 %. With *reduce motion* on, it rests after ten seconds.
  CI now fails if the resting or hidden window costs more than its budget.
- **Background reading stays out of the way.** PDFs read while learning are
  read at low priority (nice 10 and idle I/O on Linux, below-normal on
  Windows).
- **The microphone level is asked for once a second, not eleven times, while
  no microphone is open.** One script that was loaded twice (two copies of
  every poller on the Devices page) is loaded once.

### Windows

- **Moving the window to a screen with different scaling** resizes it to what
  Windows suggests, and **minimising it** tells the page it is not seen, so
  the page rests. Both are proved on a Windows runner.

### Documents

- **Installing, setting up and troubleshooting are now in Bulgarian too**, as
  is the list of known limits.
- **A stranger's install is proved on the published files**: on every release
  a clean Ubuntu 24.04 runner installs the published `.deb` with `sudo apt
  install`, starts it, opens its window and removes it; a clean Windows runner
  installs, opens and removes the published installer. Following the guide to
  the letter found two faults, both fixed: `sha256sum -c SHA256SUMS` failed
  for anyone who downloaded one file (now `--ignore-missing`), and the guide
  named the wrong folder for the brain on Windows.

### Fixed

- **The memory map can be reached again, and reads at a glance.** With the
  panels standing in their world, the memory map had no way in; *What it
  knows* now has a button for it. Memories are drawn larger (and at full
  size on high-density screens), the lines brighter, the network fills the
  panel, and the projects and best-connected memories carry their names.
- **A resting window keeps its numbers current** while the assistant learns
  in the background: they are refreshed once a minute without waking the
  page.
- **"Nothing needs a decision" no longer sits above things waiting for a
  decision**, and the system monitor's button and note are no longer cut off
  at the panel's edge on 1366×768 and 1440×900 screens.
- **Proofreading leaves correct Bulgarian alone in more places.** It no
  longer asks for a comma inside „който и да е“, „в случай че“ or
  „включително когато“; keeps the full article on the subject of a „да“
  clause („без кандидатът да знае“); reads „по точно една причина“ as the
  preposition; knows „цит.“ as an abbreviation; lets questions listed inside
  brackets run on in small letters; does not take a spreadsheet error such as
  `#REF!` for the end of a sentence; does not judge „в“ and „във“ across code;
  pairs quotation marks across paragraphs, so a quoted letter is not
  reported as unclosed; and pairs brackets across the lines of one
  paragraph, so text wrapped by an editor is not either.
- **The built-in skills' Bulgarian reads clean.** One real slip was corrected
  („с `NULL`“), examples that show mistakes on purpose are marked as such, and
  a test now keeps every Bulgarian skill free of findings.

## 0.10.0 (2026-11-01)

### Security

An independent review of everything added since 0.4 found these; all are
fixed, each with a test.

- **Setup could be driven by another web page.** The setup screen's own small
  server had no check of where a request came from, so a page open in your
  browser could have switched every approval off or pointed the assistant at
  another server. Setup now answers only its own page on this computer, and
  everything that changes something must be sent as a change (POST).
- **Goal mode no longer reads protected files without asking.** Keys,
  passwords and the other protected places are asked about in goal mode too.
  Commands handed to a shell as text (`bash -c`, `cmd /c`, PowerShell
  `-Command`) are judged by what they run, so deleting with them is asked
  about like `rm`; an encoded PowerShell command is always asked about. On
  Windows, cmd and PowerShell commands are never treated as confined.
- **A folder name cannot carry a second mail command.** A mailbox name with a
  line break in it is refused instead of being sent to the mail server.
- **Paired phones read, the computer decides.** A device paired for talking
  can no longer change the mailbox, write or import files by path, start goal
  work, change skills, providers, devices or the tunnel.
- **A saved mail password stays with its server.** Changing the account or a
  mail server now needs the password typed again.
- **The yes sends the words it showed.** A draft edited after you were asked
  to send it is not sent; you are asked again.
- **Web pages are checked at the moment of connecting**, so a name that
  changes its address after the check (DNS rebinding) cannot reach this
  computer or the local network. A proxy set in the environment is not used
  while that check is on.
- **Translations are written as new files only**, never through a link, and
  where they are written is held to the same protection as what is read.
- **Spreadsheet cells in originality reports cannot run as formulas.**
- **Answers of the assistant's API are not kept in the window's cache**, which
  is not encrypted.
- **On Windows the window stays on the assistant's own pages**: anything else
  opens in your browser, and the developer tools are off.

### Changed

- **Its own icon on Windows.** The window, the taskbar, the Start menu and
  desktop shortcuts, the installer and the entry under *Settings → Apps* show
  the assistant's icon instead of the blank default one.
- **Linux and Windows only.** Releases no longer carry macOS files; the
  downloads are the Ubuntu/Debian package and the Windows installer and zip.
  The source still compiles for macOS, unsupported.

### Checked

- **Updating from 0.8 keeps everything.** A brain written by 0.8.1
  (conversations with their sources, memories, the record of what it did, mail
  drafts, to-dos, lessons and questions, research reports, customer answers,
  invoices, goals and reminders) opens in this version with every record
  readable and nothing stored in the clear. `scripts/upgrade-check.sh` repeats
  the check for any earlier release.

### Documentation

- **Known limits**, written down: [docs/guide/known-limits.md](docs/guide/known-limits.md)
  says what this version does not do or has not been seen doing, per system,
  for models and providers, safety, mail, devices and languages.
- The roadmap and the developer guide say where things stand on 31 October
  2026 (two systems, the Windows window, current size of the code).

### Fixed

- The Learning page no longer says "1 topics · 1 questions".

## 0.9.0 (2026-10-31)

### Added

- **Its own window on Windows.** The assistant no longer opens as a browser
  tab on Windows: it has a window of its own, drawn by Microsoft Edge
  WebView2, which Windows 11 includes and almost every Windows 10 has. Links
  to sources and downloads open in your usual browser. Where WebView2 is
  missing it opens in the browser as before. Both Windows downloads carry
  Microsoft's `WebView2Loader.dll` beside the program, with its licence.

### Fixed

- **The proofreader leaves code alone.** Fenced code blocks and `inline
  code` in Markdown are no longer checked, on the Writing page and in the
  proofread tool: a selector, a query or a shell line is not a sentence.
  On the built-in skill pack this took the Bulgarian findings from 470 to 13.
- **Unticked checklist boxes are not slips.** "- [ ]" was reported as a space
  before a bracket, and applying the sure findings turned it into "[]".
- A task test that could fail now and again on a busy machine, because the
  task it started was still writing when its folder was removed, now stops
  and waits for the task first.

## 0.8.1 (2026-10-27)

### Added

- **The whole built-in skill pack: 137 skills in 32 areas, in English and
  Bulgarian.** Besides the areas of 0.8.0: writing and editing, email,
  e-commerce, project management, productivity, small-business finance and
  bookkeeping, legal-document basics, health and fitness information,
  cooking and home, travel, personal finance, game design, career and CV,
  family organisation, cars and gardening. Skills on money, law and health
  give general orientation and say when a professional is needed; the
  Bulgarian VAT skill follows the VAT Act in force from 1 January 2026
  (registration when the turnover of a calendar year exceeds the threshold).

## 0.8.0 (2026-10-27)

### Added

- **Skills.** A new Skills page lists every skill the assistant can follow,
  with a badge for where it came from: built in (in Bulgarian too when the
  screen is in Bulgarian), read from the skill folders of other agents on
  the computer (SKILL.md folders as Claude Code keeps them, and plain
  playbook files), from the assistant's own `my-skills` folder, or written
  by its owner, whose own win over any of the same name. Search, a filter,
  a switch for each skill (a skill switched off is not offered at all), the
  folders skills are read from with how many each holds, folders of your
  own, and Reload, which reads every folder again; start and every learning
  pass do the same, so a skill added or changed anywhere is picked up. The
  program can carry a built-in skill pack (each skill with its Bulgarian
  version, references and scripts, written into the brain on start). A
  skill is instructions, not permission: its scripts run only as commands
  you approve.
- **43 built-in skills in English and Bulgarian**, in 16 areas: research
  and fact-checking, summarising, translation, software development, data
  and spreadsheets, design and UI, marketing and SEO, social media, sales
  and customer support, HR and hiring, education, languages, security and
  privacy, 3D and Blender, audio and video, photography. More areas follow
  as each skill passes its review.

### Changed

- With a hundred skills on hand, the built-in pack's skills and the ones
  read from skill folders are offered only when a request shares two words
  of what a skill is for (or names it), three at most per turn, so they never
  crowd out the tools that do the work; a request in Bulgarian finds them
  by their Bulgarian description.

## 0.7.0 (2026-10-26)

### Added

- **Learning, with the assistant as a tutor.** A new Learning page, and the
  same in the chat ("teach me fractions for a ten-year-old", "quiz me",
  "what should I revise?"). Name any topic and a level (child, beginner,
  intermediate or advanced) for a lesson in one clear shape: the goal, what
  to know first, small steps each with a worked example, common mistakes,
  questions to check yourself with their answers folded away, and what to
  learn next. Practice questions (choices, numbers, short answers) come one
  at a time without their answers. Answers are marked by rules wherever a
  rule can tell (a letter or the option's words, a number however it is
  written (12,5, 1/3, "120 cm"), a short answer whatever its capitals) and
  otherwise judged by the model against the expected answer, or by you when
  there is no model. Missed questions come back in minutes, right ones
  tomorrow, then in three days, a week and a month, until they count as
  learnt; the page lists every topic with its progress and what is due. A
  tutor specialist teaches with only these tools and reading, never writing.
- **Research with sources.** A new Research page, and the same in the chat
  ("research … with sources"). The assistant plans the searches, reads the
  best sources it finds (official and scholarly first, forums last, never
  more than two pages from one site) and writes a report from them only: a
  short answer, what the sources say, where they disagree and what they do
  not settle, with a number after every claim and a list of sources saying
  what kind of place each is, its date and when it was read. Then every
  sentence is checked against its own source: citations to sources that do
  not exist are removed, and a claim with no source, a figure that is in
  none of its sources, or a sentence that shares almost no words with its
  source is marked and put to a fact checker that sees only the source's
  passage. Reports are kept, sealed, and download as Markdown. **Check a
  text against its sources** does the same for any text that cites [1],
  [2] … with web addresses or files. A fact-checker specialist joins the
  researcher. Research reads the web, so it is off while privacy keeps the
  machine to itself.
- **Customer replies for a small business.** A new Customers page, and the
  same in the chat ("answer this customer"). Replies are written only from
  the business's own answer book and the documents in its folder (prices,
  terms, hours; text, PDF, Word, OpenDocument, spreadsheets), in the
  customer's language, in a friendly or formal tone, with its signature.
  Each message is told apart (question, order, complaint, refund,
  cancellation) and each reply checked before the owner sees it: what the
  business's words do not answer is left as [[ASK: …]] for the owner, a
  promise of money or time nobody has made and a figure the business never
  gave are marked, and so is anything on the owner's "always leave to me"
  list. A reply is kept as a mail draft that waits for a yes like any
  other, and a draft with a gap still in it cannot be sent. A corrected
  reply can be saved in the answer book. A customer-care specialist writes
  drafts only.
- **Tables, with every total checked.** A new Tables page, and the same in
  the chat ("total my expenses by category"). A .csv, .tsv, .xlsx or .ods
  table is looked over first: each column's sum, range and average, cells
  that are not numbers (by row), rows entered twice, and every "Total" row
  compared with what its rows really add up to. Numbers are read however
  they are written (1 234,56 or 1,234.56, лв., €, %, a negative in
  brackets), each column deciding its decimal mark, and 1,250 is never
  guessed. A question becomes a recipe the model writes but never
  calculates; the program carries it out exactly (filters, a new column
  from a formula, groups, sums, averages, the largest first, a forecast by
  average or trend), leaves the sheet's own totals out so nothing counts
  twice, and checks every total a second way from the source rows. The
  result saves as a new workbook with its checks on a sheet of their own,
  never over an existing file. The finance and accounting specialists use
  it instead of adding up themselves. The .xlsx writer now writes several
  sheets, and .ods tables are read cell by cell.
- **Invoices and VAT: information, not advice.** A new Invoices & VAT page,
  and the same in the chat. An invoice register of sales and purchases,
  typed in, read from a PDF, Word file or scan, or imported from a table,
  every invoice checked as it is added: VAT exactly the rate of the net to
  the cent, the total the net and the VAT, a rate the country has, the basis
  for a zero rate, no VAT under reverse charge, the country's invoice
  number form, a plausible VAT number, no date in the future, no issued
  number twice, and gaps in the numbering shown. A month's VAT by rate, the
  amount to pay or deduct and its due date, and the year's sales against
  the registration threshold. Bulgaria's rules come from the VAT Act in
  force from 1 January 2026, each with its article, a link to the source and
  the date it was checked (20 % and 9 %, €51,130, the 14th of the next
  month, invoices within 5 days, ten-digit numbers); for another country
  the owner gives the rates and nothing is said about its law. Nothing is
  ever filed. The accountant specialist uses these tools.
- **Filling in PDF forms.** A new PDF forms page, and the same in the chat
  ("fill in this application"). A form's fields are read (text,
  checkboxes, radio buttons, lists, with their choices and whether they are
  required or read-only) and the form is filled into a new file beside the
  original, which is never changed and never written over. Every value is
  read back from the new file before it is saved, and a value a field
  cannot take is said while the rest is filled. Every filled value is drawn
  into the form so it shows the same in every reader: Helvetica for plain
  Latin text, otherwise a font from this computer that has the letters
  (Cyrillic included), embedded with only the letters used. Works with old
  and compressed (PDF 1.5+) files alike, and mends a damaged cross-reference
  table; encrypted and signed PDFs are left alone. No new library: the
  program reads and writes the PDF itself, as an incremental update.

## 0.6.0 (2026-10-26)

### Added

- **Writing and proofreading, in English and Bulgarian.** A new Writing page,
  and the same in the chat ("proofread this", "make it more formal"). The
  proofreading uses rules rather than a model (instant, offline, nothing
  leaves the computer), and every correction says why: doubled words,
  spacing, Latin letters hidden in Cyrillic words, в/във and с/със, ѝ, „не“
  apart from a verb, по-/най-, the full article after a preposition, the comma
  before че, защото, но and който, „…“ quotes; a/an, could of, more then, I.
  Sure corrections are ticked and made in one press; the rest are yours to
  decide. Choose a tone (formal, friendly, concise, persuasive, simple,
  polite) and a word limit to have them checked too, or have the text
  rewritten in that tone and see the changes before keeping them. Compare
  two versions word by word, on the page or with "what changed?".
- **Translation with your own glossary.** Translate a text, or a file into a
  new file beside it with its layout kept: Word and OpenDocument (styles,
  tables, headers, pictures), subtitles (timings kept), Markdown (code left
  alone) and plain text; the original is never replaced. Keep a glossary of
  agreed translations (add terms one by one, paste them from a spreadsheet,
  or save them as one), and every translation follows it and is checked: a
  term not translated as agreed, or a number, address or link that went
  missing, is listed with the result. A new Translation page, in English and
  Bulgarian, and a translator specialist for longer jobs.
- **Scans, photos and spreadsheets.** Scans and photos (a scanned contract,
  a photo of a receipt, a PDF with no text in it) are read on this computer,
  by Tesseract or by a local model that can see, and say they came from a
  scan. Spreadsheets are written as real .xlsx files (bold frozen header,
  numbers as numbers) and read back as rows with their numbers, every sheet
  by name; OpenDocument files are read without LibreOffice. Tesseract and the
  PDF tools can be installed from the list of what a task needs.
- **Mail, sorted, with replies that wait for your yes.** A new Mail page sorts
  new mail from its headers (suspicious first, then what needs a reply, then
  people, then newsletters and notifications by sender) from your mail
  server or from Thunderbird. Replies are drafted in the message's language
  and tone and kept waiting; each is sent only when you press Send on it, as
  it stands, and is recorded in What I did. Sending a draft from the chat
  always asks, even when everything else is allowed. Optionally, a one-line
  summary of new mail every morning.
- **Plan: what is next, to-do lists and your online calendar.** A new Plan
  page puts the diary, your online calendar, reminders and due to-dos in one
  time order, overdue first. Keep to-do lists by name, with an optional due
  day, ticked off in one click. Connect a CalDAV calendar (Nextcloud,
  Fastmail, iCloud and others) with an app password: the server is checked
  before anything is kept, its appointments appear under What is next, and
  new ones can be added, never changed or deleted. Also in the chat: "what
  is next?", "put milk on the shopping list".
- **Answers say where they came from.** Each answer lists its sources (things
  it remembers with the file they came from, files and web pages it read,
  search results), numbered, and a written answer ends each sentence that
  relies on one with its number, like [2]. A number that points nowhere is
  taken out before the answer is shown. Open a web source in your browser or
  a file's folder from the list; the list stays with the conversation.
- **What I did and why.** A new page lists every action, every action put to
  you and every refusal, with the request behind it, the specialist, the
  model and why it was chosen, what it was given and returned, and whether
  anything left the computer. Filter by period, by changes, or by what left.
- **Control.** The permissions page is now Control: every capability says
  whether it can send anything off this computer and what, has an on/off
  switch, and can be filtered; citing sources is switched here too. These
  pages, and the sources, are in English and Bulgarian, following the
  language you speak to it in.

- **Each step gets the model its work needs.** A job's steps are thought with
  the model their expert's work calls for (code, hard thinking, speed,
  pictures), chosen from what is installed at that moment, with the reason
  shown; a model that keeps failing gives way to the next.
- **One question at a time.** When it needs to ask, it asks one question with
  two to four answers to click, the recommended one first; reply with a
  number, a word or your own sentence.
- **Starting knowledge inside the program.** A new brain gets short guides in
  its knowledge folder (getting started, portable copy and USB, the scam
  guard, experts and models, trusted sources). Updates refresh the guides you
  have not changed and never touch the ones you have.

### Changed

- **Specialists for every field, and only when needed.** Business strategist,
  finance analyst, accountant, legal specialist, marketing specialist and
  music producer join the developers, writer, designer and researcher, each
  with only the tools its work needs.
- **Original and meaningful before finished.** Whatever a job produces (a
  game, a logo, a jingle, a story, a strategy, a contract draft) is checked
  by the specialists of the fields it touches (trademarks, look, sound, game
  rules, text and meaning, money, tax, law, business), side by side, and only
  for the sides it has; why each check was or was not used is in the task,
  and the findings have their own "Originality and meaning" section in the
  final report. New guides: "How I work" and
  "Originality and licences", in English and Bulgarian.
- **Works on goals on its own.** One setting, agreed to once: a goal is
  planned, staffed, provisioned and finished end to end. When no model here
  fits a step, the largest one that fits the computer's memory and disk is
  downloaded and its checksum verified, falling back to the next on failure;
  user-level tools are installed from their official sources with their
  published checksums; each goal that makes something works in a folder of
  its own; the report lists what was put on the computer. Only four things
  are still asked: a payment or a paid service, sending or publishing in your
  name, deleting files outside the goal's folder, and a change to the whole
  system. Only what it downloaded itself is ever offered for removal.
- **The goal-mode consent says what privacy does not cover.** Models, tools
  and integrations are downloaded from the internet even when privacy is
  "nothing leaves this machine"; the consent says so, and that those
  downloads send nothing about you. Settings show the same words the program
  enforces.
- **Integrations a goal needs are found and added on their own.** From the
  built-in catalogue first, then the official MCP registry, only from
  trusted publishers (add your own in integrations/trusted.json), pinned to
  one version from npm or PyPI, running on this computer and needing no
  account or secret, and only while privacy allows it. Each is reviewed
  before anything is downloaded, given to the one specialist that needs it,
  and the tools the server says destroy are not offered. The registry is
  searched with a short word for the kind of tool, never your words.
- **Unused downloads are removed on their own.** Once a day, models, tools
  and integrations that goal mode downloaded and nobody has used for 30 days
  are removed and the removal is proven; never anything you installed or set
  by name, and nothing while a job is under way. A failed removal is tried
  again the next day.
- **A goal is tested from the request to the report.** "Make me a space
  strategy game", with goal mode on and privacy set to this computer only,
  runs on a brain of its own against a fake model library and tool source:
  the plan, the specialists, a model and a tool downloaded and checked, the
  work in the goal's own folder, the originality checks and one report,
  with nobody asked anything.

### Fixed

- The diary said appointment times in UTC rather than local time, so an
  appointment at two in Sofia was read as noon and a whole day as the evening
  before.
- On a computer without graphics acceleration goal mode no longer downloads
  the largest model that fits in memory (minutes per answer); it chooses one
  that answers in reasonable time.
- When nothing here could write code, goal mode now downloads a model that
  can, instead of stopping the goal.
- The general assistant is no longer limited to a personal assistant's mail
  and diary tools, and a specialist is offered only the tools it may use, so
  steps no longer go round between specialists until they fail.
- A "strategy game" no longer brings a business strategist; music a plan
  makes brings the sound check even when the request never said "sound".
- Installs in goal mode wait for the result, are recorded and can be removed
  again; the report says work was done without asking because of goal mode.
- A data race when steps of one job ran side by side.
- **Originality checks with real tools.** Names are searched in trademark
  registers, app and game stores and the web; a game's one sentence is
  compared with the closest titles; pictures are compared with reference
  pictures you give (resized, recoloured and mirrored copies are found);
  melodies in MIDI files and code are compared, in any key, with well-known
  public-domain themes. Findings go into a report (item, aspect, finding,
  risk, action), saved as a table and a CSV sheet to download. "Audit the
  originality of <folder>" checks a whole catalogue, each item only for what
  it has, side by side; your own works are reported, never changed.
- **Fixed by itself.** When a check finds something in what the assistant
  made, the one who made it changes it and the same specialist checks again;
  the report says what was changed.
- **Plain wording on the screens.** The brain is now explained where its
  copies are kept: it is the assistant's knowledge base, everything it has
  learned, remembered and been told, in one folder. Team members are called
  specialists, learned ways of working are called procedures, and "ships
  with" became "included with".

- **A moved brain does not learn its own knowledge again.** Files inside the
  brain's folder are remembered relative to it (`@brain/knowledge/…`), and
  what was learned before is rewritten that way on the next start, so a brain
  on a USB stick or another computer keeps one copy of each fact.
- **Each expert runs only its own tools**, also when everything is allowed.
  A step that reaches for another expert's tool is handed, without asking
  you, to an expert who has it (a specialist first, the general assistant
  only when no specialist does).
- The built-in playbooks are only what every user needs: asking, planning a
  goal, mail, documents and the scam guard. Procedures specific to one
  business are no longer shipped in the program.

## 0.5.0 (2026-10-23)

It knows what your other agents know, carries its knowledge with it, and
looks twice at every message before you act on it.

### Added

- **Agent skills become playbooks.** On start and on every round of the
  learning, every `SKILL.md` under `~/.claude/skills` (and any folder listed
  in `BRAIN_SKILL_FOLDERS`) becomes a playbook: its description says when,
  its text (up to 6,000 characters) says how, with a pointer to the full
  file. A changed skill is refreshed; a skill you wrote or taught yourself is
  never overwritten: remove the `imported:` line from a playbook to make it
  yours.
- **A knowledge folder inside the brain.** `knowledge/` in the brain's folder
  is always learned, and is stored relative to the brain, so a brain on a USB
  stick or opened through Remote has the same documents on any machine.
  Imported skills are copied there with their references (never secrets,
  databases or caches), so their playbooks keep working where the agent is
  not installed.
- **A portable copy.** The Windows zip now runs without installing
  anything: the brain is kept in `PN-SCRIPTS-ASSISTANT-DATA` beside the
  program, so the folder works from a USB stick, and the computer it runs on
  does not remember it. On Linux, put a file named `PORTABLE` beside the
  program for the same. The installer and the .deb keep the brain in your
  account, as before.
- **A scam guard.** Every message `read_mailbox` and `read_email` read is
  checked (a sender domain that does not exist, look-alike brands and links,
  replies that go elsewhere, pressure combined with a request for passwords,
  money or access), and the verdict is written under it. Lists flag the
  obvious ones from the headers. A new `scam_guard` playbook says what to do
  next. Domains are looked up only when privacy allows the web. Mail read
  over IMAP brings its Reply-To along, and the addresses behind the links of
  an HTML message are listed under its text, so a link hidden behind
  "Verify your account" is checked too.

### Security

- Built with Go 1.26.10, which fixes GO-2026-6809 in `crypto/x509`
  (found by govulncheck in CI on 2026-10-23).

## 0.4.0 (2026-10-11)

Everything is allowed out of the box and still recorded, what it keeps about
you is encrypted at rest, it already knows how the work here is done, and the
first meeting is a conversation. See ADR 0007 and ADR 0008.

### Before you update, read this

- **The first start encrypts your brain, and shows a recovery key once.**
  Write it down or save it as a file before you press "I have kept it"; it is
  never shown again. Without it, the brain cannot be opened on another
  computer, or on this one if its keyring is lost.
- **Older versions cannot read an encrypted brain.** There is no way back to
  0.3.0 with the same data once 0.4.0 has started on it. The copy taken before
  sealing (`brain.sqlite.before-sealing.pnsealed`) is itself sealed, so if you
  might want to go back, copy the brain's folder somewhere safe before the
  first start.
- **Everything is allowed by default**, including on an existing install,
  which is switched over once. If you want the old limits, set
  `BRAIN_CONFINE=1` before you start it.

### What you will notice

- **Everything allowed, still recorded.** Privacy starts at `open`, nothing is
  asked before it acts, and nothing is confined: installing, sending mail,
  hiring, integrations, work outside a project, protected files, local
  addresses. Claude Code and Codex run with their own permission checks off.
  An existing install is switched over once, on upgrade. `BRAIN_CONFINE=1` puts
  every limit back. Every tool call is still written down, and a tool you
  refused by name stays refused.
- **Twelve built-in playbooks**, versioned, offered when their words come up:
  building and publishing HTML5 games on itch.io and Game Jolt, building
  products, shipping pnscripts.com, WordPress.org review, social profiles,
  naming, the sales hold, GitHub costs, machine load and the AI brain. A skill
  of your own with the same name wins.
- **It can drive a Chrome of its own** (`drive_browser`), signed in once by
  you, and **read Thunderbird's mail folders** (`read_mailbox`).
- **Encrypted at rest.** Conversations, memories, the record of every tool
  call, the profile, the voiceprint, credentials, your name and the log are
  sealed so only the program can read them; the key is in this computer's
  keyring. An existing brain is sealed in place on the first start, with a
  proved sealed copy first and safe against a power cut. You are shown a
  recovery key once; on another computer the brain opens with it.
- **More room to work.** Commands may run for three hours, a turn may take
  thirty steps, a task four hours; bigger downloads and models install
  without asking; root is asked for through the system's own password dialog
  (`run_as_admin`).
- **No naming form and no greeting formula.** The first meeting is a
  conversation; your name, its name and your language are picked up from what
  you say.
- **The copyright holder is the company**, ПН СКРИПТС ЕООД (PN Scripts),
  in the licence, the package and the Windows installer. Security reports and
  the package's maintainer address go to office@pnscripts.com. The licence is
  still MIT.

### Smaller fixes

- Built with Go 1.26.9, which closes twelve vulnerabilities govulncheck
  found in the standard library of 1.26.6 (`net/http`, `net/textproto`,
  `crypto/tls`, `html/template`). Nothing in this code changed for it.

## 0.3.0 (2026-10-06)

Two settings that had quietly become one are two again, the Models view says
something true about every model, and mail goes to the server that sends it.

### What you will notice

- **The permission level and privacy are independent.** Loading `brain.conf`
  used to raise the permission level to match privacy, so somebody who chose
  "ask every time" with privacy open was acting freely again after a restart.
  Setup coupled them the other way. Now each is read as written, and setup
  asks about privacy separately. The interface and the documentation say what
  each one actually does: in open mode the memory recalled for a turn goes to
  the hosted provider with the messages; the database itself stays here.
- **The Models view lists sizes, not "undefined".** Each model is one row with
  every size it is published in ("270m", "8x7b" and "e2b" included) and what
  each wants in memory and how it will run on this machine. A model opens on
  the largest size that runs comfortably here rather than the largest that
  merely fits. Model names are shown as typed, not capitalised.
- **Mail goes through the outgoing server.** With the outgoing server left
  blank, mail went to the reading server and failed. An incoming server named
  `imap.<domain>` now sends through `smtp.<domain>`; anything else asks for the
  outgoing server in Settings. A server that accepts and then says nothing no
  longer holds a tool call forever, and every message carries a Message-ID.

### Smaller fixes

- The naming card no longer saves an empty owner under a placeholder name.
- A reminder with nobody named begins "A reminder:", not ", a reminder:".
- The README and installing guide no longer name a version that was never
  published.

## 0.2.2 (2026-09-27)

Every action in both workflows on its current major, and pinned to a commit.

These four were held back on purpose: they run only in the job that publishes
a release, which nothing but a real release can exercise, and merging an
untested change into the path that signs and publishes downloads is how a
release day goes wrong. They go in here because this release is the thing that
tests them: `download-artifact`, `action-gh-release` and
`attest-build-provenance` all ran to produce the files beside this note.

`main` also holds admins to its checks now. It did not, and the consequence
was not theoretical: a commit that failed CI reached `main` because the person
pushing it could bypass the rule. A protection with a hole in it for whoever
is most likely to be in a hurry is not much of a protection.

## 0.2.1 (2026-09-27)

Six jobs had been uploading nothing and passing.

The first release exposed it, which is the only thing that could have: the
publish job went looking for five sets of artifacts and found three. The CLI
and the doctor, for all five systems, and the Linux desktop binary, were built
with `working-directory: clients` and written to `../dist`, the repository
root, while the upload step looked in `clients/dist`. It found no files, said
so in a line nobody reads, and reported success.

`if-no-files-found: error` on every upload in the workflow now, which is the
half of this fix that matters. The path was one wrong word; a step that can
find nothing and call it a success is a shape that will be wrong again.

0.2.0's downloads were complete for anybody installing it: the package, the
disk images and the Windows installer were all there. What was missing is the
command-line tools beside them.

## 0.2.0 (2026-09-27)

The first release anybody outside this machine can download.

It is a large one, and the reason is in the version number below: 0.1.0 was
prepared (it has notes, it has a package) and was never published. There was
no tag and no release, so the README pointed at an empty page. Everything
built since then arrives here at once.

### It can do more

- **An organisation rather than a team.** Jobs, seats and agents who occupy
  them; work goes to a specialist, up to a manager, or past a reviewer. Hiring
  happens from a sentence, from a template, or because a task needed somebody.
  The job catalogues behind it are the real ones, ESCO and O\*NET, fetched by
  setup and read in by the brain itself.
- **Tasks that finish**, with goals, a diary, and jobs that run in the
  background; every task and step says how serious it is, and a critical one
  asks through a grant rather than a prompt.
- **It can see the machine it runs on**: one list of what is installed, what
  is missing, what needs signing in to, and says what it can do from that
  list rather than from a paragraph written months ago.
- **It hires, equips and confines**, and decides for itself how each piece of
  work is done: Claude Code or Codex on your subscription, or a model here.
- **A real browser, pictures, a phone paired in person**, and reaching the
  brain from outside the house when you deliberately open it.
- **A voice that knows when to speak**, how loud the room is, and who is
  speaking.
- **Three voices that make three sounds**, and every system voice choosable by
  name: a robot, a woman, a man, and each underlying variant behind them.
- **The interface stands in one scene** and goes round without a menu.

### It is more honest about itself

- **Everything it does is said once, by the assistant** rather than by a string
  written into the program. The introduction, the help, the first run and the
  greeting are all decided from what is actually installed.
- **Everything is allowed by default**, and the switch that used to quietly
  change privacy when you changed permissions no longer does. They were one
  setting pretending to be two.
- **The tools a turn is given are chosen**, not filtered; the model was being
  sent more schema than it could read.

### Faults fixed

- **Any web page you had open could drive this.** A page in your browser could
  reach the assistant and change things. Both the `Origin` and the `Host` are
  checked before a request is routed now.
- **The model was reading a third of what it was sent.** The context window
  was smaller than the prompt, silently, so the beginning of every long
  conversation was dropped.
- **An answer that took 28 minutes was thrown away at 15.** The work finished;
  the connection had been cut. The three requests that wait on a model set
  their own deadline now.
- **A newer build looked older to `apt`**, so upgrades were refused as
  downgrades. Versions are ordered by a timestamp rather than by a commit hash.
- **Turning the room down could leave a program quiet for good.**
- **Twenty-eight vulnerabilities in the Go toolchain**, fixed by raising it;
  and requests to a model now retry rather than failing on the first refusal.
- **A build without cgo chose a different game engine**: the same source
  behaving differently depending on how it was compiled.

### The product around the program

- **Installers for every system**: the `.deb`, a macOS `.dmg`, and a Windows
  installer, each built on the system it is for.
- **CI on every push and pull request**: formatting, vet, the suite under the
  race detector, a build for each system, `govulncheck`, and the package put
  through `lintian`.
- **The package is byte-for-byte reproducible**, checked between two machines:
  a GitHub runner and the maintainer's desktop build one commit into the same
  file. Three things had leaked into it: the clock, the Go patch version and
  the builder's umask. CI builds it twice on every push and fails if they ever
  stop matching.
- **Every released file carries a signed provenance attestation**, so a
  download can be checked against the workflow and commit that built it.
- **`SECURITY.md`, `CONTRIBUTING.md`, a code of conduct** and issue templates,
  because a project that reads people's files should say how to report a fault
  in it before somebody has to ask.

## 0.1.0 (prepared, never published)

The first version that could be installed rather than built: an Ubuntu
package, a native window, and a brain that remembers. It was built and
checked, and no tag was ever pushed, so it never reached a releases page.
Kept here because the work is real and 0.2.0 stands on it.

The full notes are in
[docs/RELEASE-NOTES-0.1.0.md](docs/RELEASE-NOTES-0.1.0.md).
