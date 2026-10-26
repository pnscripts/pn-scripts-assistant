---
name: bg-tax-deadlines-calendar
description: Build a personal tax and reporting calendar for a Bulgarian freelancer, sole trader or small company (НАП filings, VAT, social security, annual returns, financial statements) with reminders, as orientation to be verified on nra.bg and with an accountant. Use when the user asks "what are my tax deadlines", "when is the annual return due", "make me a НАП calendar", "remind me before tax dates", "I missed a deadline", or starts a new business or activity.
---

# Bulgarian tax deadlines calendar

Turn the user's situation into a dated list of obligations with reminders. Deadlines move (laws change, a date falling on a weekend shifts to the next working day, one-off extensions happen): mark every date "verify on nra.bg" and advise the user to consult an accountant.

## Steps
1. **Profile the taxpayer.** Ask: legal form (individual with freelance income, sole trader ЕТ, ЕООД / ООД), VAT-registered or not, employees or not, self-insured or not, rental or foreign income, whether an accountant already files.
2. **Pick the obligation groups that apply** from the list below. Do not include groups that do not apply; a short accurate calendar beats a long generic one.
3. **Put dates on the calendar** for the next 12 months. For each item record: what, period covered, usual deadline, who files, how (НАП e-services with a qualified electronic signature or personal code, or through the accountant).
4. **Add reminders** 7 days and 1 day before each date, and a monthly "gather documents" reminder on the 1st.
5. **Mark uncertainty.** Every row ends with "verify on nra.bg" and the user confirms with their accountant once a year.
6. **If a deadline was missed,** do not guess penalties. Advise filing or paying as soon as possible and contacting the accountant; late interest usually grows with time.

## Obligation groups (usual pattern, verify each year)
- **VAT-registered:** monthly VAT return, ledgers and VIES declaration, plus payment, around mid-month for the previous month.
- **Employer:** monthly payroll declarations and social and health contributions, usually in the second half of the following month.
- **Self-insured person:** monthly social and health contributions, usually in the second half of the following month.
- **Freelancer or sole trader:** quarterly advance income tax (if required), annual income tax return and payment in spring for the previous year.
- **Company:** annual corporate tax return and payment, annual financial statements and their publication in the Commercial Register, usually by mid-year; quarterly advance tax if applicable.
- **Everyone with local property or vehicles:** municipal property tax and vehicle tax, often with an early-payment discount in spring.

## Template
| Date | Obligation | Period | Who files | Status |
|---|---|---|---|---|
| (day) Feb | VAT return + payment | January | accountant | verify on nra.bg |
| (day) Feb | Social contributions | January | self | verify on nra.bg |
| (day) Apr | Annual income tax return | previous year | self | verify on nra.bg |
| (day) Jun | Annual financial statements | previous year | accountant | verify on nra.bg |

Fill the day only after the user or the assistant has checked the current official calendar; otherwise leave "(day)" and say so.

## Pitfalls
- Copying last year's calendar: dates and forms change.
- Confusing "file" and "pay" deadlines when they differ.
- Forgetting zero filings: a registered business usually files even with no sales.
- Weekend and public-holiday shifts.
- Assuming the accountant files everything; confirm in writing who does what.

## Where to check
- The НАП tax and social security calendar and announcements on nra.bg
- The municipality's website for local property and vehicle taxes
- The Commercial Register for filing annual financial statements
- Your accountant, for anything specific to your activity

## Example reminder text
> In 7 days: VAT return and payment for March. Gather sales and purchase invoices and send them to the accountant today. Verify the date on nra.bg.

## Output
Hand back: the filled calendar table for the next 12 months, the reminder schedule (or calendar entries in the user's format, e.g. `.ics` lines), the open points to confirm, and a clear note that dates are orientation to be verified on nra.bg and with an accountant.
