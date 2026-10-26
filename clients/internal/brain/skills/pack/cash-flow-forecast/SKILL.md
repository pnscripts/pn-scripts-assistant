---
name: cash-flow-forecast
description: Build a simple 13-week or 12-month cash flow forecast for a small business or freelancer - opening balance, expected receipts and payments, taxes, the lowest point and actions to avoid a shortfall. Use when the user asks "will I have enough money", "cash flow forecast", "when will I run out of cash", "can I afford to hire / buy this", "plan for a slow season", or shares invoices, bills and a bank balance.
---

# Cash flow forecast

Forecast cash, not profit: when money actually enters and leaves the bank. Keep it simple, transparent and easy to update weekly. For tax amounts and financing decisions, tell the user to consult an accountant.

## Steps
1. **Choose the horizon.** 13 weeks for tight cash or day-to-day control; 12 months for planning a hire, a purchase or seasonality.
2. **Start from the real bank balance** on a stated date (all business accounts, minus amounts already committed).
3. **List receipts by expected payment date,** not invoice date: open customer invoices (adjusted for how late each customer usually pays), expected new sales, other income. Be conservative.
4. **List payments by due date:** rent, salaries, contributions, suppliers, subscriptions, loan repayments, VAT and income tax payments, one-offs (equipment, insurance, annual fees).
5. **Compute each period:** `closing = opening + receipts - payments`; the next opening equals this closing.
6. **Find the lowest point** and compare it with a minimum buffer the user chooses (e.g. one month of fixed costs).
7. **Run two scenarios:** base and cautious (largest customer pays 30 days late, sales 20% lower). Show both lows.
8. **Propose actions** if the low goes below the buffer: invoice and chase sooner, ask for deposits, spread a purchase, renegotiate a due date, cut or pause costs, arrange financing early (with an accountant or bank).
9. **Update weekly:** replace forecasts with actuals, roll forward one period.

## Template (13 weeks, one row per week)
| Week | Opening | Customer receipts | Other in | Salaries | Suppliers | Taxes | Other out | Closing |
|---|---|---|---|---|---|---|---|---|
| 1 | 8 000 | 2 500 | 0 | 0 | 1 200 | 0 | 300 | 9 000 |
| 2 | 9 000 | 0 | 0 | 3 000 | 400 | 1 100 | 300 | 4 200 |

Spreadsheet formula for closing in row 2: `=B2+C2+D2-E2-F2-G2-H2`; opening in row 3: `=I2`.

## Example insight
> Lowest point: week 6 at 1 300 in the cautious case, below your 3 000 buffer. Cause: the annual software renewal and quarterly tax fall in the same week as salaries. Options: ask the client with the 4 000 invoice for payment on due date, or move the renewal to monthly billing.

## Pitfalls
- Using invoice dates instead of when the money really arrives.
- Forgetting VAT collected is not your money, and tax payments are lumpy.
- Optimistic new sales with no signed order behind them.
- Annual and quarterly costs that hide between months.
- Never comparing the forecast with what actually happened.

## Questions to ask the user
- What is the exact bank balance today, across all business accounts?
- Which customers usually pay late, and by how many days?
- Which sales are signed, and which are only hoped for?
- What large or annual payments are coming (insurance, licences, equipment, taxes)?
- What minimum balance lets you sleep well?
- Is there an overdraft or credit line, and what does it cost?

## Output
Hand back: the forecast table (or a spreadsheet layout with formulas), the lowest balance and its week or month in each scenario, the main drivers, a short list of actions, and a note to confirm tax amounts with an accountant.
