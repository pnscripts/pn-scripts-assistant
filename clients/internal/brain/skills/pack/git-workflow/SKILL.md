---
name: git-workflow
description: Guide everyday Git work safely - branches, commits, merging and rebasing, resolving conflicts, undoing mistakes and working with a remote such as GitHub or GitLab. Use when the user asks how to commit, branch, merge, rebase, push or pull, sees "merge conflict", "detached HEAD", "rejected - non-fast-forward", committed to the wrong branch, wants to undo a commit, or needs a clean history for a pull request.
---

# Git workflow

Keep work recoverable. Before any command that rewrites history or discards changes, check `git status` and say what will be lost.

## Steps
1. **Look before acting.** Run `git status`, `git branch -vv` and `git log --oneline -10`. Know the current branch, uncommitted changes and how far you are from the remote.
2. **Start work on a branch.**
   ```
   git switch main
   git pull --ff-only
   git switch -c feature/login-form
   ```
3. **Commit small, coherent changes.** Review with `git diff`, stage on purpose (`git add -p` for parts of a file), then commit with a clear message:
   ```
   Add password strength meter to sign-up form

   Shows weak/medium/strong while typing; blocks submit when weak.
   ```
   First line up to about 50 characters, imperative mood; body says why.
4. **Stay up to date.** `git fetch`, then either `git rebase origin/main` (linear history, only for branches nobody else uses) or `git merge origin/main` (safe on shared branches).
5. **Resolve conflicts.** Open each file listed by `git status`, keep the right parts between `<<<<<<<`, `=======` and `>>>>>>>`, remove the markers, run the tests, then `git add` the file and `git rebase --continue` or `git commit`. To give up: `git rebase --abort` or `git merge --abort`.
6. **Push and open a pull request.** `git push -u origin feature/login-form`. After a rebase of your own branch use `git push --force-with-lease`, never plain `--force`.
7. **Clean up after merge.** `git switch main && git pull --ff-only && git branch -d feature/login-form`.

## Undo recipes
| Situation | Command |
|---|---|
| Discard changes in one file | `git restore file.txt` |
| Unstage a file | `git restore --staged file.txt` |
| Fix the last commit message or add a forgotten file (not pushed) | `git commit --amend` |
| Undo the last commit, keep the changes | `git reset --soft HEAD~1` |
| Undo a pushed commit safely | `git revert <hash>` |
| Committed on main instead of a branch | `git branch new-branch`, then `git reset --hard origin/main` on main (after checking nothing else is lost) |
| "Lost" a commit after reset or rebase | `git reflog`, then `git switch -c rescue <hash>` |
| Put work aside for a moment | `git stash push -m "wip"`, later `git stash pop` |

## Pitfalls
- Never rewrite history (`rebase`, `reset`, `amend`, force push) on a branch others have pulled.
- `git reset --hard` and `git clean -fd` delete uncommitted work permanently; confirm with the user first.
- Never commit secrets (`.env`, keys, passwords). If one was pushed, rotate it immediately; deleting the file does not remove it from history.
- Add build output, dependencies and editor files to `.gitignore` before the first commit.
- A detached HEAD is harmless until you commit; create a branch (`git switch -c name`) to keep that work.
- Commands and defaults differ slightly between Git versions; check `git --version` and the official documentation if something behaves unexpectedly.

## Output
Return the exact commands in order with a one-line explanation each, a warning before any destructive step, and how to verify the result (`git status`, `git log --oneline --graph`).
