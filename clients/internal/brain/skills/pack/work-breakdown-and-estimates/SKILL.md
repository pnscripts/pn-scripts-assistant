---
name: work-breakdown-and-estimates
description: Break a project or deliverable into a work breakdown of small, checkable tasks, estimate effort with ranges, find dependencies and the critical path, and produce a realistic timeline with buffer. Use when the user asks "how long will this take", "break this down into tasks", "estimate this project", "make a timeline / Gantt", or needs to give a deadline or quote to a client or manager.
---

# Work breakdown and estimates

Big tasks hide work; small tasks reveal it. Estimate in ranges, add up honestly, and say what the estimate assumes.

## Steps
1. **Start from deliverables**, not activities. Write each deliverable as a noun ("Checkout page", "Training manual").
2. **Decompose** each deliverable into tasks until each task is:
   - done by one person or team,
   - about 0.5-3 days of effort (smaller for uncertain work),
   - finished with a visible result ("draft sent for review").
   Include the forgotten work: setup, reviews, testing, fixes, documentation, handover, meetings, approvals.
3. **Estimate each task with three numbers:** optimistic (O), most likely (M), pessimistic (P). Expected = (O + 4M + P) / 6. Use past similar work if available; ask the person who will do it.
4. **Mark dependencies:** which tasks must finish before others start; note external waits (approvals, deliveries).
5. **Find the critical path:** the longest chain of dependent tasks. Delays there delay the whole project.
6. **Convert effort to calendar time.** Effort days / (people x realistic focus share). A person rarely gets more than 5-6 focused hours a day; account for holidays and other duties.
7. **Add buffer** at the end of the project or before key milestones (not hidden inside each task), sized to the uncertainty - for example 15-30%.
8. **State assumptions and exclusions.** Every estimate is conditional; write the conditions down.

## Template
| ID | Deliverable | Task | Owner | O | M | P | Expected (days) | Depends on |
|---|---|---|---|---|---|---|---|---|
| 1.1 | Checkout page | Design mock-up | Ana | 1 | 2 | 4 | 2.2 | - |
| 1.2 | Checkout page | Build and connect payment | Ivo | 2 | 3 | 6 | 3.3 | 1.1 |
| 1.3 | Checkout page | Test and fix | Ivo | 1 | 1.5 | 3 | 1.7 | 1.2 |

## Example answer to "how long?"
"About 7 working days of effort (range 5-11). With one developer at ~70% focus and one approval step, plan for 3 calendar weeks; the main risk is the payment provider's account verification."

## Checklist
- Every deliverable has tasks for review, testing and handover.
- No task is longer than about 3 days of effort.
- Each estimate was checked by the person doing the work.
- The buffer is visible, not hidden inside tasks.

## Pitfalls
- Giving one number when the work is uncertain; give a range and a confidence.
- Estimating someone else's work without asking them.
- Forgetting review, testing and rework cycles - often a large share of the total.
- Treating an estimate as a promise; re-estimate when scope changes and say so early.

## Output
The task breakdown table, total effort as a range, the critical path, a calendar timeline with buffer and milestones, and the list of assumptions and exclusions.
