---
name: level-design
description: Plan and review game levels - purpose of each level, teaching new mechanics without text, layout and flow, pacing of tension and rest, landmarks and readability, secrets and optional paths, difficulty placement, and a level-by-level sheet - for platformers, puzzles, shooters, adventure and other genres. Use when the user asks "help me design a level", "players get lost / stuck in my level", "how do I introduce this mechanic", "plan the levels for my game", or shares a map sketch, screenshot or level description.
---

# Level design

Every level has a job: teach, test, combine, surprise or rest. Design the job first, then the layout.

## Steps
1. **Define the level's purpose** in one line: "Teach wall-jump safely, then test it over a pit." Note the mechanics, enemies and items available at this point.
2. **Teach without text where possible** using the four beats:
   - Introduce in a safe place (no penalty for failing).
   - Develop with a simple real challenge.
   - Twist the idea (timing, combination, different angle).
   - Conclude with a test that uses what was learned.
3. **Plan the flow.** Sketch the critical path from start to goal; mark choke points, branches, and where the player must look next. Use light, colour, shape and motion to lead the eye.
4. **Pace tension and rest:** after each hard section, a safe area with a reward or view. Checkpoints before difficult parts, never right after a long easy stretch.
5. **Landmarks and readability:** a few unique, visible landmarks; consistent visual language (what is climbable, breakable, dangerous). The player should always be able to answer "where am I and where can I go?"
6. **Optional content:** secrets and side paths rewarded with something meaningful, visible enough to tempt (a glimpse of a chest behind a wall).
7. **Difficulty placement:** compare with neighbouring levels; follow the game's overall curve. Spike only at deliberate tests (bosses, chapter ends).
8. **Greybox first:** build with simple blocks, playtest, then add art. Watch where testers hesitate, die or get lost.
9. **Fill the level sheet** (below) for each level.

## Level sheet template
| Field | Content |
|---|---|
| Level | 1-4 "Old Mill" |
| Purpose | Teach rotating platforms; combine with wind gusts |
| New element | Rotating platform |
| Beats | safe intro room -> 3 platforms over shallow water -> wind + platforms -> exit test |
| Length / target time | 3-4 min first play |
| Checkpoints | after intro, before final test |
| Secrets | coin room behind waterwheel (visible gap) |
| Difficulty (1-10) | 4 |
| Known issues from tests | players miss the second jump timing - add visual cue |

## Pitfalls
- Introducing a mechanic in a dangerous spot; the first failure feels unfair.
- Mazes of identical corridors; players get lost and quit.
- Hidden instant-death traps (no warning); readable danger is fair danger.
- Designing only for the designer's skill level; testers will be much worse at first.

## Output
A level sheet per level (or for the reviewed level), a text sketch of the flow with beats and checkpoints, specific fixes for problems found, and what to watch for in the next playtest.
