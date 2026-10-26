---
name: code-review
description: Review a code change, pull request, diff or single file for correctness bugs, security holes, readability and test coverage, and give ranked, actionable findings. Use when the user pastes a diff or file and asks "review this", "is this code OK", "what is wrong here", "check my PR before I merge", or wants a second opinion on a refactor, a new function or a script.
---

# Code review

Find the problems that would hurt users or maintainers first; style comes last. Every finding must point to a line and say what goes wrong.

## Steps
1. **Understand the intent.** Ask or infer what the change should do, the language and framework, and how it is run. Read the description, then the tests, then the code.
2. **Read the whole change, then the context.** Look at callers of changed functions and at code the diff removes. A bug often hides in what is no longer there.
3. **Check correctness first.** For each changed block ask:
   - What happens with empty, null, zero, negative, very large, Unicode or duplicate input?
   - Are loops and ranges off by one? Are conditions inverted or missing an `else`?
   - Are errors caught, logged and returned, or silently swallowed?
   - Are resources (files, connections, locks) closed on every path, including errors?
   - Is shared state touched from several threads or requests without protection?
   - Do time zones, rounding, floating-point money or locale formats matter here?
4. **Check security.** Untrusted input reaching SQL, shell, HTML, file paths or `eval`; secrets in code or logs; missing authorisation checks; unsafe deserialisation; outdated dependencies with known issues.
5. **Check tests.** Does a test fail without the change and pass with it? Are edge cases and the error path covered?
6. **Check maintainability.** Names that lie, duplicated logic, functions doing three things, dead code, comments that contradict the code.
7. **Rank and write findings.** Severity: blocker (wrong result, data loss, security), major (likely bug, missing test), minor (clarity), nit (style). Drop anything you cannot justify.

## Finding template
```
[major] orders.py:42 - total uses float, so 0.1 + 0.2 gives 0.30000000000000004.
Scenario: an order of three 0.10 items shows 0.30000000000000004 on the invoice.
Fix: store cents as int or use Decimal; add a test with 0.10 x 3.
```

## Example
Diff: `if user.role == "admin" or "editor":`
Finding: [blocker] the second operand is a non-empty string, so the condition is always true and every user gets editor rights. Fix: `if user.role in ("admin", "editor"):`.

## Checklist
- [ ] Intent understood and matched
- [ ] Edge cases and error paths traced
- [ ] Untrusted input handled safely
- [ ] Tests cover the change
- [ ] Each finding has a location, scenario and fix

## Pitfalls
- Do not flood the review with nits; ten style remarks hide the one real bug.
- Do not claim a bug you have not traced; say "possible" and explain the doubt.
- Do not rewrite the whole design unless asked; suggest the smallest safe fix.
- Linters and formatters catch style; spend human attention on logic.
- Be kind and specific: critique the code, not the author.

## Output
Return a short verdict (approve / approve with changes / request changes), then findings ranked most severe first, each with location, scenario and suggested fix, and finally what you could not check (for example code not shown).
