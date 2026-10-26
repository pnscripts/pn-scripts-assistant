---
name: core-loop-and-progression
description: Design or fix a game's core loop and progression - moment-to-moment actions, session loop, long-term goals, rewards, unlock pacing, difficulty curve and the reasons players come back - and diagnose why a game feels boring, grindy or shallow. Use when the user asks "what is the core loop of my game", "my game gets boring after 10 minutes", "how should players progress / unlock things", "design an upgrade system", or plans retention for a mobile, browser or desktop game.
---

# Core loop and progression

The core loop is what the player does again and again; progression is why doing it keeps feeling worthwhile. Both must serve the game's core fantasy.

## Steps
1. **Write the three loops:**
   - Moment (5-30 s): act -> feedback -> small reward (jump, shoot, match, place).
   - Session (5-30 min): goal -> challenge -> result -> reward -> new choice (a level, a run, a day in a farm).
   - Long term (days-weeks): mastery, collection, story, building, social status.
   Draw each as arrows; every step must be clear to the player.
2. **Check the moment loop first.** Is the basic action satisfying with placeholder art? Feedback should be immediate: sound, animation, numbers, screen effect. If the loop is not fun, progression will not save it.
3. **Choose progression types** that fit the fantasy:
   - Skill (the player gets better) - puzzles, action, sports.
   - Power (the character gets stronger) - RPGs, idle, roguelites.
   - Content (new levels, areas, story).
   - Options (new tools, builds, strategies).
   - Cosmetic or collection.
4. **Pace unlocks.** Something new every few minutes early on, then wider gaps. Introduce one new idea at a time: teach, test, twist, combine.
5. **Shape the difficulty curve:** a saw-tooth (rise, peak at a boss or test, small drop with a new tool, rise again), not a straight line.
6. **Balance rewards:** variable but fair; avoid rewards that make the core action unnecessary (e.g. auto-play that removes the game).
7. **Design return reasons** that respect the player: unfinished goals, daily puzzles, building projects, story chapters. Avoid punishing absence.
8. **Map it on a timeline** - minute 1, 5, 15, 60, day 1, day 7 - what the player learns, unlocks and feels.

## Example
Tower-defence browser game.
- Moment: place tower -> enemies hit -> coins pop.
- Session: 10-wave map -> stars 1-3 -> unlock next map.
- Long term: tower upgrades with stars, new tower every 3 maps, endless mode after map 12.
Diagnosis of "boring after map 5": no new tower between map 3 and 6; fix by adding a new enemy type at map 4 and a tower at map 5.

## Quick test
- Ask a tester to describe what they do and why they keep going after 5 minutes; if they cannot, the loop or its goal is unclear.

## Pitfalls
- Grind: repeating the same content only to raise numbers.
- Too many systems early; new players need one clear goal.
- Pay-to-skip pressure or loot boxes; many players and some countries reject them - check platform and local rules if monetisation is involved.
- Progression that makes the game easier so fast that challenge disappears.

## Output
The three loops written and diagrammed (in text arrows), a progression timeline table, the diagnosis of any problem with concrete fixes, and the next small test to validate the changes.
