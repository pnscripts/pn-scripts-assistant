---
name: meeting-transcript-summary
description: Turn a meeting transcript, call recording text, chat log or rough notes into a clear summary of decisions, action items with owners and dates, open questions and key discussion points. Use when the user pastes or attaches a transcript (TXT, VTT, SRT, DOCX), says "summarise this meeting", "what did we decide", "who has to do what", or needs minutes for people who missed the call.
---

# Meeting transcript summary

Readers of meeting summaries want three things: what was decided, who does what by when, and what is still open. Put those first. Report only what was actually said.

## Steps
1. **Get the context.** Meeting title, date, participants and purpose. Take them from the transcript header or ask briefly if missing.
2. **Clean the input mentally.** Ignore small talk, filler, cross-talk and recognition errors. Map speaker labels ("Speaker 2") to names only when the transcript makes it clear.
3. **Find decisions.** Look for phrases like "let's go with", "agreed", "we'll", "final answer". A decision needs agreement, not just a suggestion.
4. **Find action items.** For each: task, owner, due date. If owner or date was not stated, write "owner: not set" rather than guessing.
5. **Find open questions and risks.** Anything deferred, disputed or waiting on information.
6. **Summarise the discussion** by topic, not by timeline, in 1-3 sentences each, including the main positions when people disagreed.
7. **Check numbers, names and dates** against the transcript.

## Template
```
Meeting: <title> - <date> - <participants>
Purpose: <one line>

Decisions
1. ...

Action items
| Task | Owner | Due |
|---|---|---|
| ... | ... | ... |

Open questions
- ...

Discussion by topic
- <Topic>: ...

Next meeting: <date or "not set">
```

## Example
Transcript fragment: "Anna: I can send the revised budget by Friday. Mark: OK, and we drop the print campaign, agreed? Anna: Agreed."
- Decision: print campaign dropped.
- Action: Anna - send revised budget - Friday (give the actual date if the meeting date is known).

## Checklist
- [ ] Every action has an owner or is marked "not set"
- [ ] Relative dates ("Friday", "next week") converted to real dates when possible
- [ ] Decisions separated from ideas and suggestions
- [ ] Disagreements reported fairly, without taking sides
- [ ] No sensitive personal remarks copied without need

## Pitfalls
- Recording a suggestion as a decision.
- Assigning tasks to whoever spoke last about the topic.
- Automatic transcripts mishear names, numbers and product terms; flag doubtful ones with [?].
- Very long transcripts: summarise in parts, then merge; do not skip the end, where decisions often happen.
- Keep confidential content in the summary only if the user's audience may see it.

## Output
Hand back the summary in the template, decisions and action items first, with uncertain items marked [?] and a short list of questions for the organiser to confirm.
