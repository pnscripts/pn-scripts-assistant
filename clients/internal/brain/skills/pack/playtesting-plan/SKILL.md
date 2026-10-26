---
name: playtesting-plan
description: Plan, run and analyse playtests for a game - test goals and questions, choosing testers, the session script, what to observe, think-aloud and surveys, simple metrics, consent and privacy, and turning findings into a prioritised fix list. Use when the user says "I want to playtest my game", "how do I get feedback on my prototype", "write a playtest survey", "testers said it is confusing, what now", or prepares a demo, jam build, early access or store release.
---

# Playtesting plan

Playtests answer specific questions by watching real players. Watch what they do more than what they say, and never explain the game during the test.

## Steps
1. **Set 2-4 test questions**, e.g. "Do new players understand the jump-dash without text?", "Where do players quit in level 2?", "Is the session length about 10 minutes?"
2. **Pick the build and scope:** the smallest build that answers the questions; known bugs listed so testers are not blocked by them.
3. **Choose testers:** 5-8 per round usually reveal most major problems. Mix the target audience with some newcomers; friends and family are kind, not neutral. Get age-appropriate consent (parents for minors).
4. **Prepare the session script:**
   - Intro: "We are testing the game, not you. Please think aloud. I cannot help during play."
   - Task: "Play until you reach the end or 20 minutes pass."
   - Observe silently; take timestamped notes.
   - Debrief questions (below), then a short survey.
5. **Observe and log:** where they hesitate, misread, die repeatedly, smile, get bored, ask questions, quit. Record screen and voice only with consent, and store recordings safely.
6. **Collect simple metrics** if the build supports it: time per level, deaths per section, completion rate, feature use. Follow the privacy rules of the platform and region (e.g. GDPR) for any data collected.
7. **Analyse:** group observations into issues; count how many testers hit each. Severity: blocker (cannot progress), major (frustration, quitting), minor (polish).
8. **Decide fixes:** for each issue, the change, owner and how the next test will check it. Retest with new players.

## Debrief questions
- What was the game about, in your own words?
- What was the most fun moment? The most frustrating?
- Was anything confusing? Where?
- What would you do next if you kept playing?
- On a scale of 1-10, how likely are you to play again? Why?

## Findings table
| Issue | Seen by | Severity | Evidence | Fix | Check next round |
|---|---|---|---|---|---|
| Dash not discovered | 5/7 | Major | stuck at gap 2-4 min | glowing tutorial gap + button prompt | 6/7 dash within 1 min |

## Observation sheet
- Columns: time, tester, location in game, what happened, quote, severity guess.

## Pitfalls
- Helping or explaining during play; it hides the real problems.
- Asking "did you like it?" - people are polite; observe behaviour.
- Fixing every comment; look for patterns across testers and the test questions.
- Testing too late; test the prototype as soon as the core loop exists.

## Output
The test plan (questions, testers, build, script, survey), an observation sheet template, and after a test the findings table with severity and prioritised fixes.
