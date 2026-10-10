name: github_cost_control
version: 1
when: I ask about GitHub costs, GitHub Actions minutes or budget, CI workflows, creating, archiving or making a repository private, the pnscripts organisation, failing Actions emails or Packagist
tools: run_command, read_file, search_files, list_directory
---

GitHub facts for PN Scripts are in DEV/ai-brain/projects/pnscripts/pnscripts.com.md and DEV/ai-brain/business/accounts.md (DEV is the owner's workspace folder; find it with find /media /mnt -maxdepth 3 -type d -name DEV). Use the gh command line, which the owner has signed in; never sign in for him.

The rules:
- The pnscripts organisation has an Actions budget of $0 with stop-usage. Do not rely on CI for verification; run the tests locally.
- Private repositories do not run Actions. New private repositories get no .github/workflows at all.
- Public repositories that keep CI run it only on pull_request and workflow_dispatch, never on every push. Scheduled workflows (cron) need a good reason and a low frequency.
- Every archived repository must also be private. This is the owner's explicit rule.

## Before you start

1. List the organisation's repositories and their state: gh repo list pnscripts --limit 200 --json name,visibility,isArchived.
2. Read the repository's profile in DEV/ai-brain/projects/ and DEV/ai-brain/projects/registry.md.

## Steps

1. Auditing workflows: for each repository with .github/workflows, read the on: triggers. Propose changing push triggers to pull_request plus workflow_dispatch. Propose removing workflows from private repositories. Show the list to the owner before changing anything.
2. Editing a workflow: change it on a branch, commit by explicit path, and push only with the owner's yes.
3. Creating a repository: gh repo create pnscripts/<name> --private, without workflows, and only with the owner's yes.
4. Archiving a repository: first check Packagist (https://packagist.org/packages/pnscripts/) for a package that installs from it, because a private repository breaks composer require. If one exists, stop and ask the owner. Otherwise make it private first, then archive: gh repo edit pnscripts/<name> --visibility private --accept-visibility-change-consequences, then gh repo archive pnscripts/<name> --yes.
5. A repository that is already archived and still public: GitHub refuses visibility changes on archived repositories, so unarchive, make it private, and archive again: gh repo unarchive R --yes, then gh repo edit R --visibility private --accept-visibility-change-consequences, then gh repo archive R --yes.
6. Keep the local folder of an archived repository unless the owner asks to move it.
7. Update DEV/ai-brain/projects/registry.md and the repository's profile with the new state and date.

## Verify

- gh repo view <repo> --json visibility,isArchived shows PRIVATE and true for every archived repository.
- gh api orgs/pnscripts/settings/billing/actions (if available to the signed-in account) or the billing page in the browser shows no paid minutes.
- For changed workflows: quote the new on: block.

## Never

- Never raise the Actions budget, add a payment method or change billing; that is the owner's decision.
- Never make a private repository public, archive, rename or delete a repository without the owner's yes.
- Never delete a repository; archive it (private) instead.
- Never force-push or rewrite history on shared branches.
- Never type passwords or tokens, never print tokens, and never commit secrets; repository secrets are set by the owner.
