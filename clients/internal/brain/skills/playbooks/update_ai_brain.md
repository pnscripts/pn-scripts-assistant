name: update_ai_brain
version: 1
when: I ask to update the AI brain, record what was done, write a lesson, update a project profile, the pipeline or accounts notes, run check-brain or sync the skill adapters in DEV/ai-brain
tools: run_command, read_file, write_file, edit_file, search_files, list_directory
---

The shared AI Brain is DEV/ai-brain (DEV is the owner's workspace folder; find it with find /media /mnt -maxdepth 3 -type d -name DEV). Its router is AGENTS.md; the index of where things live is brain/index.md. It is a git repository. Repositories with their own brain (pnscripts.com keeps one in .ai/) record their own lessons there; the repository's rules win.

## Before you start

1. Read DEV/ai-brain/AGENTS.md, section "Editing the brain", and .ai/workflows/evolve-brain.md.
2. Search before writing: one fact lives in one place. Look in projects/, business/, .ai/memory/lessons/, .skills/playbooks/, checklists/ and troubleshooting/ for an existing owner of the fact, and update it instead of restating it.
3. Run git status in DEV/ai-brain: other sessions edit it too. Do not sweep their changes into your commit.

## Where things go

- A project's state (paths, commands, status, branch state, known issues): projects/<group>/<repo>.md, with last_verified updated. Families of small repos share one profile (for example projects/marketplace/games-batch-10-11.md).
- Business state (pipeline stages, accounts, owner decisions): business/pipeline.md and business/accounts.md. Status only: never logins, IBANs, VAT numbers, addresses or other private values.
- A new learning (a trap, a missing step, a wrong doc): a lesson in .ai/memory/lessons/YYYYMMDD-<slug>.md from .ai/templates/lesson-template.md, with status: proposed and an honest confidence (low, medium, high). Lessons are never born validated, and new knowledge never skips straight into .skills/, standards or workflows.
- A lesson about pnscripts.com goes into that repository's .ai/memory/lessons/, not here.
- A new or renamed playbook or pack: .skills/; then run the adapter sync (below).

## Steps

1. Write or edit the canonical file. Keep the front matter (id, type, confidence, last_verified, sources) accurate. British English, full sentences, facts with dates and evidence (commit hashes, URLs, test counts).
2. If you added or renamed a playbook or pack: run_command ["python3",".ai/hooks/sync-skill-adapters.py"] with dir DEV/ai-brain. Never hand-edit .agents/skills/.
3. Run the check: run_command ["python3","scripts/check-brain.py"] with dir DEV/ai-brain. Fix what it reports and run it again until it passes.
4. Commit by explicit paths: git add <each file you changed>, then git commit with a message that says what changed. Never git add -A.
5. Push only if the owner wants the brain pushed.
6. Changes to brain law (constitution, L1 standards, routing) need the owner's go-ahead first.

## Verify

- check-brain.py exits with status 0, and you quote its last line.
- git show --stat HEAD lists only your files.
- The fact you recorded can be found from brain/index.md or the profile that owns it.

## Never

- Never read or quote DEV/ai-brain/local/, .env files, keys or database dumps.
- Never write secrets, passwords, tokens, account numbers, IBANs, VAT numbers, salaries, loan amounts, personal addresses, phone numbers or private email addresses into the brain.
- Never modify DEV/Projects/skillwork/, DEV/Archive/ or DEV/Assets/ (except new release zips where the platform-release standard says so).
- Never delete lessons; archive them into .ai/memory/lessons/archive/ with a status.
- Never commit another session's work, and never force-push.
- Never type passwords.
