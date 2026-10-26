---
name: household-budget
description: Build a simple monthly household budget from the user's income and spending - fixed and variable costs, irregular annual bills, a savings line, and a plan to close a gap - using a spreadsheet or plain table, as general guidance, not financial advice. Use when the user says "help me make a budget", "where does my money go", "I am always short at the end of the month", "plan our family spending", or pastes bank transactions, a CSV export or a list of expenses.
---

# Household budget

Help the user see where money goes and plan the next month. This is general guidance, not investment advice; for investments, pensions or large debts refer them to a licensed financial adviser. Never judge spending choices; show numbers and options.

## Steps
1. **Net income.** Monthly take-home pay for everyone in the household, plus regular other income. For irregular income, use a cautious average of the last 6-12 months.
2. **Collect spending.** Ideally the last 2-3 months of bank and card transactions. Group them; ask about cash.
3. **Sort into categories:**
   - Fixed needs: rent or mortgage, utilities, insurance, transport passes, childcare, minimum debt payments
   - Variable needs: groceries, fuel, medicine, household items
   - Wants: eating out, entertainment, hobbies, shopping, subscriptions
   - Irregular: annual insurance, taxes, car service, gifts, school costs, holidays (divide the yearly total by 12)
   - Savings and extra debt payments
4. **Compare** income with total planned spending. Show the monthly surplus or gap.
5. **Close a gap** with options ranked by impact: the largest categories first, then subscriptions (see `subscriptions-and-bills-review`), then renegotiating bills, then income ideas.
6. **Set the plan for next month:** a number per category, savings paid first on payday, irregular costs moved to a separate "sinking fund".
7. **Review** once a week for 10 minutes and once a month in full; adjust numbers, not the habit.

## Template
| Category | Planned | Actual | Difference |
|---|---|---|---|
| Net income | 2 400 | | |
| Rent | 700 | | |
| Utilities | 150 | | |
| Groceries | 450 | | |
| Transport | 120 | | |
| Irregular (yearly / 12) | 110 | | |
| Wants | 300 | | |
| Savings | 250 | | |
| Left to assign | 320 | | |

Formula for "Left to assign": `=B2-SUM(B3:B9)`. Aim for zero by giving every amount a job.

## Example insight
> Groceries averaged 610 in the last three months, 160 above plan. Eating out added 240. Moving 100 from eating out to groceries and planning meals for the week brings you within budget without cutting savings.

## Pitfalls
- Budgeting with gross pay instead of take-home pay.
- Forgetting irregular bills, which then feel like emergencies.
- Too many categories; 8-12 are enough.
- A plan so strict that it is abandoned in week two; leave a small buffer.
- Treating budgeting tips as investment or tax advice.

## Output
Hand back: the filled budget table, the monthly surplus or gap, the top three changes with their effect in money, a short weekly review routine, and a reminder that this is not investment advice and that a financial adviser can help with investments or complex debts.
