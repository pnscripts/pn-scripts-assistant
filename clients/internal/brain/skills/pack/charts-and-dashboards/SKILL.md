---
name: charts-and-dashboards
description: Choose, build and review charts and simple dashboards that answer a clear question - the right chart type, honest axes, readable labels, accessible colours and a sensible layout - in Excel, Google Sheets, Looker Studio, Power BI or code such as matplotlib and plotly. Use when the user asks "which chart should I use", "make a graph of this", "build a dashboard", "this chart looks confusing", or needs visuals for a report or presentation.
---

# Charts and dashboards

A chart exists to answer one question at a glance. Start from the question and the message, then choose the form.

## Steps
1. **Write the message as a sentence.** "Online sales overtook shop sales in May." The chart title can be that sentence.
2. **Pick the chart by the comparison.**
   | Comparison | Use |
   |------------|-----|
   | change over time | line chart (column chart for few periods) |
   | ranking or comparing categories | horizontal bar chart, sorted |
   | parts of a whole (2-5 parts) | stacked bar or 100 % bar; pie only for 2-3 slices |
   | distribution | histogram or box plot |
   | relationship between two numbers | scatter plot |
   | one key number | big number with comparison ("EUR 12.4k, +8 % vs last month") |
   | many items x many periods | heat map or small multiples |
3. **Prepare the data.** Aggregate to the level shown, sort categories by value, group small categories into "Other", use consistent units.
4. **Make it honest.**
   - Bar and column charts start at zero. Lines may zoom in, but say so in the axis.
   - Same scale when charts are compared side by side.
   - No 3D, no dual axes unless unavoidable and clearly labelled.
5. **Make it readable.** Direct labels instead of legends where possible, short axis labels with units, no more than 5-7 colours, grey for context and one accent colour for the point. Remove gridline clutter and borders.
6. **Make it accessible.** Do not rely on red/green alone; add labels or patterns. Check text contrast. Provide a short text summary or data table next to the chart.
7. **Dashboard layout.**
   - Top row: 3-5 key numbers with comparison to target or previous period.
   - Middle: trend charts for those numbers.
   - Bottom: breakdowns and detail tables.
   - One filter bar for date range and main dimensions; same colours mean the same thing everywhere.
   - Note the data source and "last updated" time.
8. **Review with fresh eyes.** Can someone state the message in 5 seconds? If not, cut.

## Example (matplotlib)
```python
ax = monthly.plot(kind="line", color=["#999999", "#1f77b4"])
ax.set_title("Online sales overtook shop sales in May")
ax.set_ylabel("Revenue, EUR thousand"); ax.set_ylim(bottom=0)
ax.spines[["top", "right"]].set_visible(False)
```

## Pitfalls
- Pie charts with many slices, and donuts used for precise comparison.
- Cumulative charts that always rise and hide a slowdown.
- Too many KPIs: if everything is highlighted, nothing is.
- Mixing totals and averages, or calendar and fiscal periods, on one dashboard.
- Truncated bars that exaggerate small differences.

## Output
Give the recommended chart type with a one-line reason, the prepared data or the steps to build it in the user's tool (or the code), a title that states the message, and a short checklist of fixes if reviewing an existing chart or dashboard.
