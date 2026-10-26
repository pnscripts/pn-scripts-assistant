---
name: data-cleaning
description: Clean a messy table or dataset before analysis - duplicates, blanks, inconsistent spellings, mixed date and number formats, stray spaces, wrong encodings and outliers - in a spreadsheet, CSV, Excel file or with Python/pandas. Use when the user says the data is "a mess", totals do not add up, a CSV opens with broken characters or merged columns, an import fails, or before any report, chart or merge of two lists.
---

# Data cleaning

Make the data trustworthy without silently losing or inventing anything. Always work on a copy and keep a log of every change.

## Steps
1. **Keep the original.** Save the raw file untouched; clean in a copy or a new sheet/script so every step can be repeated.
2. **Profile first.** For each column record: type it should be, count of blanks, number of distinct values, min/max, 5 sample values. In pandas: `df.info()`, `df.describe(include="all")`, `df[col].value_counts(dropna=False).head(20)`.
3. **Fix structure.**
   - One header row, one record per row, one kind of value per column.
   - Unmerge cells, remove subtotal and blank spacer rows, split combined fields ("Sofia, 1000" into city and postcode).
   - Encoding: if letters show as `Ð¡` or `�`, re-import as UTF-8 (or try Windows-1251 for older Cyrillic files).
4. **Fix values.**
   - Trim spaces and non-breaking spaces; normalise case where it carries no meaning.
   - Map spelling variants to one value with an explicit lookup table ("Sofia", "sofia ", "София" -> "Sofia").
   - Convert numbers stored as text; watch decimal commas vs points and thousand separators.
   - Parse dates with an explicit format (`%d.%m.%Y`), never by guessing; check that day and month did not swap.
5. **Handle duplicates.** Define what a duplicate is (same ID? same name + date?). Review a sample before deleting; keep the most complete or latest row and log what was removed.
6. **Handle missing values.** Decide per column: leave blank, mark as "unknown", or fill from a reliable source. Do not fill with averages or zeros without telling the user.
7. **Check outliers.** Flag values outside a plausible range (negative quantities, ages over 120, prices 100x the median). Ask before changing; many are real.
8. **Validate.** Row counts before and after, totals of key amounts, no blank IDs, all categories in the allowed list.

## Example (pandas)
```python
df = pd.read_csv("orders.csv", encoding="utf-8", dtype=str)
df.columns = df.columns.str.strip().str.lower()
df["city"] = df["city"].str.strip().replace({"sofia": "Sofia", "софия": "Sofia"})
df["amount"] = pd.to_numeric(df["amount"].str.replace(" ", "").str.replace(",", "."), errors="coerce")
df["date"] = pd.to_datetime(df["date"], format="%d.%m.%Y", errors="coerce")
bad = df[df["amount"].isna() | df["date"].isna()]   # review, do not drop blindly
df = df.drop_duplicates(subset=["order_id"], keep="last")
```

## Cleaning log template
| Step | Column | Rule | Rows affected |
|------|--------|------|---------------|
| 1 | city | trimmed, merged 4 spellings into "Sofia" | 213 |
| 2 | order_id | removed exact duplicates, kept latest | 17 |

## Pitfalls
- Spreadsheets auto-convert: long IDs lose digits, codes like `00123` lose zeros, `1-2` becomes a date. Import such columns as text.
- `errors="coerce"` turns bad values into blanks silently; always count and inspect them.
- Removing rows with any blank cell can throw away most of the data.
- Cleaning one export by hand does not fix the next one; suggest fixing the source form or a reusable script.
- Personal data: remove columns that are not needed for the analysis.

## Output
Return the cleaned file or script, the cleaning log (what changed and how many rows), rows set aside for review, and remaining issues the user must decide on.
