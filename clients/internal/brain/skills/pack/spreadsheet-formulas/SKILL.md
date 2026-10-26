---
name: spreadsheet-formulas
description: Write, explain and fix spreadsheet formulas in Excel, Google Sheets or LibreOffice Calc - lookups, conditional sums and counts, dates, text, dynamic arrays and error handling. Use when the user asks "what formula do I need", pastes a formula that returns #N/A, #VALUE!, #REF! or a wrong total, wants to combine data from two sheets, or describes a calculation in words and needs it as a formula.
---

# Spreadsheet formulas

Turn a described calculation into a correct, readable formula, and explain why a broken one fails. Ask which app and version the user has, because some functions exist only in newer versions.

## Steps
1. **Pin down the data.** Ask for the sheet layout: header row, column letters, sample rows, the expected result for one row. Check the locale: many European locales use `;` instead of `,` as the argument separator and `,` as the decimal mark.
2. **State the logic in words first.** "Sum column D where column B is 'North' and the date in A is in March 2026." Confirm it with the user before writing anything.
3. **Pick the simplest function that fits.**
   - Lookup: `XLOOKUP` (Excel 365/2021, Sheets) or `INDEX`/`MATCH` for older versions; avoid `VLOOKUP` with a hard-coded column number.
   - Conditional totals: `SUMIFS`, `COUNTIFS`, `AVERAGEIFS`.
   - Lists that change size: `FILTER`, `UNIQUE`, `SORT` (dynamic arrays).
   - Text: `TEXTJOIN`, `LEFT`/`MID`/`RIGHT`, `TRIM`, `SUBSTITUTE`, `TEXTSPLIT` where available.
   - Dates: `EOMONTH`, `EDATE`, `NETWORKDAYS`, `DATEDIF`; build dates with `DATE(y,m,d)`, never from text.
4. **Fix references.** Use `$` for anchors that must not move when copied (`$B$2`), or turn the range into a table and use structured names (`Sales[Amount]`).
5. **Handle errors deliberately.** Wrap only the expected failure: `IFERROR(XLOOKUP(...), "not found")`, or use the lookup's own "if not found" argument. Never hide every error with a blanket `IFERROR(..., 0)`.
6. **Test.** Check the formula on 2-3 rows by hand, including an empty cell, a missing key and an edge date.

## Examples
- Total sales for "North" in March 2026:
  `=SUMIFS(D:D, B:B, "North", A:A, ">="&DATE(2026,3,1), A:A, "<"&DATE(2026,4,1))`
- Price from a product sheet, with a clear message when missing:
  `=XLOOKUP(A2, Products!A:A, Products!C:C, "no price")`
- Older Excel equivalent:
  `=IFERROR(INDEX(Products!C:C, MATCH(A2, Products!A:A, 0)), "no price")`
- Unique customers in a list: `=COUNTA(UNIQUE(C2:C500))`

## Error diagnosis
- `#N/A` - the lookup key was not found: extra spaces, numbers stored as text, or a different spelling. Test with `=A2=Products!A5` and `=ISTEXT(A2)`.
- `#VALUE!` - wrong data type, usually text in a number or date column.
- `#REF!` - a referenced row, column or sheet was deleted.
- `#DIV/0!` - divide only when the denominator is non-zero: `=IF(B2=0, "", A2/B2)`.
- `#SPILL!` - a dynamic array has no empty room below or to the right.
- Wrong total but no error - numbers stored as text are silently ignored by `SUM`.

## Pitfalls
- `VLOOKUP` without `FALSE`/`0` as the last argument does an approximate match and returns wrong rows.
- Whole-column references are fine in `SUMIFS` but slow inside array formulas on large sheets.
- Merged cells break sorting, filtering and ranges; suggest "Centre across selection" instead.
- Rounding: display formatting does not change the stored value; use `ROUND` when totals must match an invoice.
- Function names are translated in some localised versions; give the English name and mention this.

## Output
Give the final formula ready to paste (with the right separator for the user's locale), a one-line explanation of each part, where to put it and how to copy it down, and how to check it on one row.
