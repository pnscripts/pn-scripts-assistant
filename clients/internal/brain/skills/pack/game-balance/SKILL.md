---
name: game-balance
description: Balance a game's numbers and choices - damage, health, costs, rewards, drop rates, economy sources and sinks, difficulty, character or unit options and matchmaking fairness - using spreadsheets, simple formulas, simulations and playtest data. Use when the user says "this weapon / character is overpowered", "my economy breaks after an hour", "the game is too easy / too hard", "how much should this upgrade cost", or shares a stats table, spreadsheet or player complaints about fairness.
---

# Game balance

Balance means every meaningful option is worth considering and the challenge matches the intended experience. Model it in a spreadsheet, then confirm with play.

## Steps
1. **State the intent:** what should feel strong, risky, rare or fast? Who is the target player (casual, competitive)? Balance serves the design, not perfect equality.
2. **Collect the numbers** into one table: units/items with cost, damage, health, speed, cooldown, range, unlock time; economy sources (rewards) and sinks (costs, upgrades, repairs).
3. **Pick a value measure** and compare options on it:
   - Damage per second = damage x hits per second (include accuracy, crit chance x crit bonus).
   - Time to kill = target health / DPS.
   - Efficiency = value / cost (DPS per coin, health per coin).
   Options with similar cost should land within roughly 10-20% of each other unless they trade off something clear (range vs damage, safety vs speed).
4. **Check the economy:** income per minute vs cost of the next meaningful upgrade over time. Plot it; a growing gap feels grindy, a shrinking one makes everything trivial. Every source needs a sink.
5. **Set growth curves deliberately:**
   - Linear (cost = a + b x level) - easy to read.
   - Exponential (cost = base x rate^level, e.g. rate 1.07-1.15) - common in idle games; check the late game.
   - Make enemy power and player power curves grow at related rates.
6. **Simulate** where possible: a few spreadsheet rows or a small script that plays out a session or a battle 1,000 times; look at win rates and time to goal.
7. **Validate with play and data:** pick rates, win rates, completion and quit points. In competitive games, an option chosen or winning far more than others needs a look, but also consider player skill.
8. **Change one thing at a time,** in small steps (often 5-15%), and record each change with the reason.

## Example
Tower A: damage 10, 1 shot/s, cost 50 -> 0.20 DPS per coin. Tower B: damage 35, 0.5 shots/s, cost 100 -> 0.175 DPS per coin, but splash hits 3 enemies -> effective 0.52. B dominates groups; fix by reducing splash to 2 targets or raising cost to 150, then test wave 8 and wave 15.

## Change log template
| Version | Change | Reason | Expected effect | Measured effect |
|---|---|---|---|---|
| 0.4.2 | Tower B splash 3 -> 2 | B used in 80% of wins | B pick rate 40-50% | B pick 47%, win rate even |

## Checklist
- Every option has a clear role or trade-off.
- Income and costs are plotted over the whole game, not only the start.
- Changes are small, one at a time, and recorded.
- Spreadsheet results were confirmed by real play.

## Pitfalls
- Balancing only on spreadsheets; feel and player behaviour matter.
- Huge swings that flip the strongest option every patch.
- Ignoring the late game, where exponential numbers break.
- In multiplayer, nerfing popular fun options instead of buffing weak ones; prefer raising weak options when possible.

## Output
The balance table with value measures, the list of outliers with explanations, proposed number changes with reasoning, a simple test or simulation plan, and a change log template.
