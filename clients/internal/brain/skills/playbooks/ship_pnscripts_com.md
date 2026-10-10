name: ship_pnscripts_com
version: 1
when: I ask to ship, deploy, release or put live a change on pnscripts.com, games.pnscripts.com or the site, run php artisan ship, the full test suite, a migration, ftp:sync or db:sync
tools: run_command, read_file, search_files, do_in_background, list_background, fetch_url, drive_browser
---

The site repository is DEV/Projects/pnscripts/pnscripts.com (DEV is the owner's workspace folder; find it with find /media /mnt -maxdepth 3 -type d -name DEV). It has its own AI rules in .ai/, and they win over anything here. Read .ai/standards/ship-every-change.md, git-dev-hub.md and no-database-wipes.md before shipping, and the profile DEV/ai-brain/projects/pnscripts/pnscripts.com.md for the current branch state.

The owner's rule (ship-every-change): a change is finished only when it is committed, pushed to dev, pushed to staging and main (fast-forward only), deployed with php artisan ftp:sync (code first), synced to the live database, and verified on production in both locales on every affected host.

## Before you start

1. Prepare work on a branch in a git worktree in your own scratch directory. Never edit, switch or reset the main checkout: other sessions use it. Another session may also remove a worktree it does not recognise; if a run fails en masse, first check that the directory still exists.
2. Content that lives in the database ships as a migration, not as a hand edit. Match on the shipped wording so an editor's later change is never overwritten, and write through the query builder for models with HasSlug.
3. Make sure no factory builders or other heavy jobs are running (see machine_throttling). The full suite must run alone.
4. Ask the owner for his yes to deploy, naming the branch and what changes.

## Steps

1. In the worktree: ./vendor/bin/pint --test and the affected tests (php artisan test --filter=...). A red tree never ships.
2. Merge the branch into the local dev branch of the main repository only when the owner has agreed, and with a fast-forward or a clean merge commit.
3. Run the full suite once on the integration head. It takes about 80 to 90 minutes, longer than the 5-minute command limit, so start it in the background: run_command ["bash","-lc","cd <repo> && setsid nohup nice -n 19 php artisan test > <scratch>/suite.log 2>&1 &"], or use do_in_background. Poll by reading the log every few minutes, never with a tight loop.
4. When the suite is green, ship from dev without re-running it: php artisan ship --skip-tests (Pint still runs). Add --message="..." only if something is still uncommitted. Use --dry-run first to see the plan. ship refuses to run on the live server and from any branch other than dev.
5. Rows: ship asks before replacing live rows. With no terminal it leaves them alone; pass --yes only when the owner has confirmed the row replacement in this conversation. db:sync is a bulk replace: compare first (php artisan db:sync --compare) and stop if the server holds rows that local does not, because that is content edited in the live admin.
6. After a refspec push, update the local lanes: git fetch, then git branch --force staging origin/staging and the same for main.
7. After a commit that includes a deletion, check git show --stat HEAD for the staging trap described in ship-every-change.md.

## Verify

1. Never trust the success message. Fetch the changed pages on production in English and Bulgarian (/bg) on every affected host: pnscripts.com, games.pnscripts.com, blog.pnscripts.com, hosting.pnscripts.com. Check the status code and the changed text with fetch_url or drive_browser.
2. For data changes, confirm the expected rows on the live database as the ship-every-change rule describes, and count what was supposed to change.
3. Report the commit, the time of deployment, the suite result (tests passed and skipped) and the pages you checked.
4. Record the shipped state in the pnscripts.com profile in DEV/ai-brain and in business/pipeline.md.

## Never

- Never touch the main checkout from an agent worktree, never force-push, and never rewrite pushed commits.
- Never run the full suite beside builders, and never run two suites at once.
- Never run migrate:fresh, db:wipe or any wipe on the live database, and never replace live rows without the owner's yes in this conversation.
- Never deploy without the owner's yes.
- Never type passwords or FTP credentials, never read .env values, and never put secrets into commits or logs.
- Never move the local site off *.pnscripts.local.
- Never delete branches, backups or old migrations; reverse a change with a new migration.
