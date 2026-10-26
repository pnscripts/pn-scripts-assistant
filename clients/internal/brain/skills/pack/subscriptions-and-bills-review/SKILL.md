---
name: subscriptions-and-bills-review
description: Find and trim recurring costs - streaming, apps, software, gym, phone, internet, insurance, utilities - from bank statements or a list, flag duplicates, forgotten trials and price rises, and plan safe cancellations or renegotiations, as general guidance, not financial advice. Use when the user says "what am I subscribed to", "cut my monthly bills", "find recurring payments in my statement", "how do I cancel this", "my bill went up", or pastes a bank or card export (CSV).
---

# Subscriptions and bills review

Find every recurring payment, decide keep / downgrade / cancel / renegotiate, and make a safe plan. This is general guidance, not investment advice; for insurance or loan products the user may want a licensed financial adviser.

## Steps
1. **Collect 3 months of transactions** (12 months to catch annual renewals) from all cards and accounts, plus app-store subscription lists.
2. **Detect recurring payments:** same merchant at regular intervals, similar amounts, words like SUB, MONTHLY, RENEW. Group by merchant and show the yearly cost (`monthly * 12`).
3. **Flag:**
   - duplicates (two music or cloud services, family and individual plan both paid)
   - trials that turned into paid plans
   - price rises versus earlier months
   - services not used in the last 30-60 days (ask the user)
   - charges the user does not recognise (possible fraud: contact the bank through its official number)
4. **Decide per item** with the user: keep, downgrade, switch to annual only if it will surely be used, pause, cancel, renegotiate.
5. **Renegotiate bills** (phone, internet, insurance, energy where the market allows): compare current offers on official sites, then ask the provider for a retention or loyalty price; note contract end dates and early-termination fees.
6. **Cancel safely:** use the official account page or app (typed by hand, not a link from an email), take a screenshot of the confirmation, check the next statement. Note that deleting an app usually does not cancel the subscription.
7. **Prevent creep:** a list of all subscriptions with renewal dates, reminders 3 days before trials end, one card for subscriptions.

## Template
| Service | Amount | Cycle | Yearly | Last used | Decision | Renewal / end date |
|---|---|---|---|---|---|---|
| Video streaming A | 12.99 | monthly | 155.88 | yesterday | keep | 5th monthly |
| Video streaming B | 9.99 | monthly | 119.88 | 3 months ago | cancel | 18th |
| Cloud storage | 2.99 | monthly | 35.88 | daily | keep | 2nd |

## Example message to a provider
> Hello, my contract ends on 30 April. My bill rose from 25 to 31 per month. Other offers for similar service are lower. Can you offer a better price to keep me as a customer? Thank you.

## Pitfalls
- Cancelling something tied to backups or photos before exporting the data.
- Missing annual renewals that appear once a year.
- Calling a "cancellation number" found in an ad or unsolicited email (scam risk).
- Early-termination fees that cost more than the saving.

## Checklist
- [ ] All cards, accounts and app stores checked
- [ ] Annual renewals found (12 months of history)
- [ ] Unrecognised charges reported to the bank
- [ ] Data exported before cancelling storage or photo services
- [ ] Cancellation confirmations saved
- [ ] Next statement checked for stopped charges

## Output
Hand back: the table of recurring payments with yearly totals, flagged items, the decision per item, the total monthly and yearly saving, cancellation and renegotiation steps with dates, and a reminder list for renewals.
