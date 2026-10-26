---
name: systematic-debugging
description: Find the root cause of a bug methodically - reproduce it, read the real error, form one hypothesis at a time, test it with the smallest experiment, fix the cause and prove the fix. Use when the user says "it does not work", "I get this error", "it crashes", "works on my machine but not on the server", pastes a stack trace or log, or has been guessing at fixes without progress.
---

# Systematic debugging

Debugging is an experiment, not guesswork. Change one thing at a time and believe only what you can observe.

## Steps
1. **Collect the facts.** Exact error message and stack trace, the command or action that triggers it, expected vs actual result, environment (OS, language and library versions), and what changed recently (code, data, config, updates).
2. **Reproduce it reliably.** Write down the shortest steps that show the bug every time. If it is intermittent, note how often and under which conditions (load, time, specific data).
3. **Read the error properly.** Start from the first error, not the last; find the topmost stack frame in the user's own code. Look up unfamiliar messages in the official documentation.
4. **Shrink the problem.** Remove code, data and steps until the bug just barely remains. A minimal reproduction often reveals the cause on its own.
5. **Form one hypothesis.** "The list is empty because the filter runs before the data loads." Predict what you will see if it is true.
6. **Test it with the cheapest experiment.** Add a print or log line, set a breakpoint, inspect a variable, run one test. If the prediction fails, discard the hypothesis; do not stretch it.
7. **Bisect when lost.** Between a good and a bad version use `git bisect`; inside a long function, check the midpoint and halve again.
8. **Fix the cause, not the symptom.** Wrapping a crash in `try` or adding a sleep hides the bug. Ask why the bad value appeared at all.
9. **Prove the fix.** Write a test that fails before and passes after; rerun the original reproduction; remove debug output.

## Example
Symptom: `TypeError: Cannot read properties of undefined (reading 'name')` in `profile.js:18`.
- Fact: happens only for new users.
- Hypothesis: new users have no `settings` object, so `user.settings.name` fails.
- Experiment: log `user` for a new account; `settings` is missing. Confirmed.
- Fix: create default settings at sign-up, and guard the old accounts with a migration; add a test for a fresh user.

## Useful questions
- What is different between the case that works and the one that fails?
- Is the code that runs really the code you are reading (cache, old build, wrong branch, wrong environment)?
- Is the input what you think it is? Print it, do not assume it.

## Pitfalls
- Changing several things at once, then not knowing which one helped.
- Trusting a comment, a variable name or a memory instead of the actual value.
- Stopping at the first plausible story without testing it.
- Leaving debug prints, disabled checks or broad `catch` blocks in the final code.
- Ignoring warnings that appeared just before the error.

## Output
Return the root cause in one or two sentences, the evidence that confirms it, the fix (code or steps), the test that proves it, and any related places that may have the same bug.
