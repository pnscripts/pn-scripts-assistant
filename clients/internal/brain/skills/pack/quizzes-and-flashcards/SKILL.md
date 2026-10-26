---
name: quizzes-and-flashcards
description: Turn notes, a chapter, a lecture transcript or a topic into flashcards and quizzes that test real understanding - one fact per card, clear answers, mixed question types and an import-ready format for Anki or Quizlet. Use when the user says "make flashcards from this", "quiz me", "create a test for my class", pastes study notes or a PDF chapter, or wants CSV or tab-separated cards to import.
---

# Quizzes and flashcards

Good cards are small, unambiguous and ask the learner to retrieve, not recognise. Good quizzes check understanding, not only memory.

## Steps
1. **Clarify the use.** Self-study cards, a class quiz or a practice exam? Learner level, number of items, language, and target format (plain list, CSV, tab-separated for Anki, Quizlet text import).
2. **Extract the key ideas.** From the source, list definitions, facts, processes, cause-effect links and typical problems. Skip trivia that will not be tested or used.
3. **Write flashcards with the minimum-information rule.** One idea per card; the question must have a single correct answer.
4. **Write quiz items in mixed types.**
   - Recall (short answer) for key facts.
   - Multiple choice with 3–4 plausible distractors based on real misconceptions.
   - Apply: a short scenario or a problem with new numbers.
   - Explain: "Why does…" or "What would happen if…".
5. **Add an answer key with a one-line explanation** for every item, so wrong answers teach something.
6. **Check quality.** Every answer is supported by the source; no item gives away another; no "all of the above"; difficulty runs from easy to hard.
7. **Format for import** when asked, and tell the user how to import it.

## Good and bad cards
| Bad | Better |
|---|---|
| Q: Photosynthesis? | Q: Which gas do plants take in for photosynthesis? A: Carbon dioxide |
| Q: List all causes of WWI | Four cards, one per cause, each with a cue |
| Q: The capital is ___ (which country?) | Q: Capital of Portugal? A: Lisbon |

## Import formats
```
# Anki: one card per line, front<TAB>back, save as .txt (UTF-8)
Which organelle makes ATP?	Mitochondrion
What does "mitosis" produce?	Two genetically identical cells

# CSV (Quizlet or spreadsheets), quote fields that contain commas
"Capital of Portugal","Lisbon"
```
For cloze cards in Anki use `{{c1::hidden text}}` inside the front field with the Cloze note type.

## Quiz template
```
1. (MC) Which change increases reaction rate? a) lower temperature b) larger particles c) a catalyst d) less concentration
   Answer: c - a catalyst lowers activation energy.
2. (Apply) A car travels 150 km in 2.5 h. Average speed?  Answer: 60 km/h (150 / 2.5).
```

## Pitfalls
- Cards that copy whole paragraphs; they become reading, not recall.
- Distractors that are obviously silly; they test nothing.
- Answers that depend on wording the learner never saw.
- Content not in the source presented as fact; mark any addition clearly.
- Forgetting spaced repetition: suggest reviewing new cards the next day and then at growing intervals.

## Output
Hand back: the flashcards (in the requested import format), the quiz with numbered items and an answer key with brief explanations, and a one-line note on how to import or use them for spaced review.
