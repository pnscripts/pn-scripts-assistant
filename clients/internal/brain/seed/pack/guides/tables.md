# Tables: budgets and sales, with every total checked

Open **Tables** in the menu, or ask: "total my expenses by category in
budget.xlsx", "which product sold most last quarter?", "estimate the next
three months of sales".

## Looked over first
Give the full path of a table — .csv, .tsv, .xlsx or .ods. Before anything
is worked out, it is looked over: every column with its sum, smallest,
largest and average; cells that should be numbers and are not, by row; rows
that are the same in every cell (an expense entered twice); and every row
that calls itself a total ("Total", "Общо"), compared with what the rows
above it really add up to. Numbers are read however they are written —
1 234,56 or 1,234.56, with "лв.", "€" or "%", a negative in brackets — and
each column decides which mark is its decimal one; a number such as 1,250
that could be a thousand or one and a quarter is never guessed.

## Worked out exactly
Ask a question in your own words. The language model does not calculate:
it only turns the question into a recipe — which rows, a new column from a
formula, how to group, what to add up, the order, a forecast — which you can
read under **How it was calculated**. The program carries the recipe out
with exact arithmetic (no rounding errors), and rows that are the sheet's own
totals are left out, so nothing is counted twice.

## Checked a second way
Every total in the result is added up again straight from the source rows
and must match to the last cent; the rows counted into groups must be every
row that passed the filter; cells that could not be read are listed by row.
A forecast is an estimate from the past periods — the average of the last
three, or the straight-line trend — and is labelled as one.

## Saved, never overwritten
**Save as .xlsx** writes the result to a new workbook, with the forecast and
the checks on sheets of their own. It never writes over a file that is
there, and your original table is never changed. Nothing leaves this
computer.
