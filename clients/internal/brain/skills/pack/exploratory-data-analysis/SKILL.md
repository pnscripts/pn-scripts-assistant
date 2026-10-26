---
name: exploratory-data-analysis
description: Explore a new dataset to understand what it contains and find the patterns, trends, gaps and surprises worth reporting - summary statistics, distributions, segments, time trends and correlations - in a spreadsheet, pivot table or Python/pandas. Use when the user shares a CSV or Excel file and asks "what does this data tell me", "find insights", "why did sales drop", wants a first look before a report, or needs to know which questions the data can answer.
---

# Exploratory data analysis

Go from "here is a file" to a short list of well-supported findings. Every claim must point to a number the user can reproduce.

## Steps
1. **Agree on the question.** Ask what decision the analysis should help with (e.g. "where to cut costs", "which product to promote"). Without a question, explore broadly but rank findings by usefulness.
2. **Know the grain.** What is one row: an order, a customer, a day? Count rows, date range, distinct IDs. Check whether data is complete for the whole period (a missing week looks like a drop).
3. **Check quality quickly.** Blanks, duplicates, impossible values. If problems are serious, clean first (see the data-cleaning skill) and say so.
4. **Describe each key column.**
   - Numbers: count, mean, median, min, max, percentiles; compare mean and median to detect skew.
   - Categories: top values with counts and shares; lump the long tail into "Other".
   - Dates: rows per day/week/month to spot gaps and seasonality.
5. **Segment.** Break the main metric down by the obvious dimensions (region, product, channel, customer type). Use a pivot table or `df.groupby(...).agg(...)`. Look for the few segments that drive most of the total.
6. **Look at change over time.** Compare like with like: same period last year, same weekday, per-day averages for months of different length.
7. **Relationships.** Scatter or correlation for numeric pairs; cross-tables for categories. Treat correlation as a lead, not a cause.
8. **Test each finding.** Is it large enough to matter? Is the sample big enough (a 50 % rise from 2 to 3 sales is noise)? Does it survive removing one outlier or one big customer?
9. **Write up.** 3-7 findings, each with the number, the comparison and what it may mean.

## Example (pandas)
```python
df.shape, df["date"].min(), df["date"].max()
df.describe(include="all").T
df.groupby("region")["revenue"].agg(["count", "sum", "median"]).sort_values("sum", ascending=False)
df.set_index("date").resample("W")["revenue"].sum()
df[["price", "quantity", "discount"]].corr()
```

## Finding template
> **Revenue in the North fell 18 % in Q3 vs Q3 last year (EUR 41k vs 50k).** Two customers account for 80 % of the drop; order counts from other customers are flat. Next step - check whether those two accounts moved to a competitor.

## Pitfalls
- Averages hide skew; report the median for incomes, prices and durations.
- Percentages without base counts mislead; always show both.
- Simpson's paradox: a trend in every segment can reverse in the total when the mix changes.
- Survivorship: data that only includes active customers cannot explain churn.
- Do not dredge: testing 50 splits will produce a few "significant" ones by chance.
- Keep the user's personal or customer data inside the analysis; aggregate before sharing.

## Output
Hand back: a data overview (rows, period, grain, quality notes), the ranked findings with numbers, charts or tables that support them, caveats, and the next questions or data that would confirm them.
