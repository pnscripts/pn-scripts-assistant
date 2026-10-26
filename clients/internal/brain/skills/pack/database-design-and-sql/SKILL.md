---
name: database-design-and-sql
description: Design relational database schemas and write, explain or speed up SQL queries for PostgreSQL, MySQL/MariaDB, SQLite or SQL Server - tables, keys, relationships, normalisation, indexes, joins, aggregates, migrations and safe data changes. Use when the user asks how to structure data for an app, needs a query or report, gets wrong or duplicated rows from a join, has a slow query, sees an SQL error, or must change a live database without losing data.
---

# Database design and SQL

Model the real things and their relationships first, then write queries against that model. Treat every change to live data as risky: back up, test on a copy, use transactions.

## Design steps
1. **List the entities and facts.** Nouns become tables (customer, order, product), facts become columns, verbs between nouns become relationships.
2. **Choose keys.** Every table gets a primary key (`id` integer identity or UUID). Foreign keys enforce links (`orders.customer_id → customers.id`).
3. **Map relationships.** One-to-many: foreign key on the "many" side. Many-to-many: a join table (`order_items(order_id, product_id, quantity)`).
4. **Normalise to remove duplication.** One fact in one place: no `phone1, phone2, phone3` columns (use a child table), no customer address copied into each order unless it is a historical snapshot on purpose.
5. **Pick types and constraints.** `NOT NULL` where required, `UNIQUE` for emails or codes, `CHECK (quantity > 0)`, exact `DECIMAL(12,2)` for money (never float), timestamps with time zone in UTC.
6. **Plan indexes.** Primary and unique keys are indexed; add indexes for foreign keys and for columns used often in `WHERE`, `JOIN` and `ORDER BY`. Composite index order matters: equality columns first, then range.

## Example schema
```sql
CREATE TABLE customers (
  id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  email      VARCHAR(255) NOT NULL UNIQUE,
  name       VARCHAR(200) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE orders (
  id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  customer_id BIGINT NOT NULL REFERENCES customers(id),
  total       DECIMAL(12,2) NOT NULL CHECK (total >= 0),
  placed_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX orders_customer_placed ON orders (customer_id, placed_at);
```
(PostgreSQL syntax; adjust identity and timestamp types for other engines.)

## Query example
Top 10 customers by spend since 1 January 2026:
```sql
SELECT c.name, COUNT(o.id) AS orders, SUM(o.total) AS spent
FROM customers c
JOIN orders o ON o.customer_id = c.id
WHERE o.placed_at >= '2026-01-01'
GROUP BY c.id, c.name
ORDER BY spent DESC
LIMIT 10;
```

## Debugging queries
1. Ask for the engine and version, the table definitions and a few sample rows.
2. Too many rows: a join on a non-unique key multiplies rows; check the join conditions, aggregate in a subquery first.
3. Missing rows: `INNER JOIN` drops rows without a match, or a `WHERE` on the right table turns a `LEFT JOIN` into an inner one; move that condition into `ON`.
4. `NULL` surprises: `= NULL` is never true, use `IS NULL`; `NOT IN` with a `NULL` in the list returns nothing, use `NOT EXISTS`.
5. Slow: run `EXPLAIN` (or `EXPLAIN ANALYZE` on a copy); look for full scans on large tables, missing indexes, functions on indexed columns (`WHERE LOWER(email) = ...` needs an expression index), `SELECT *` over many columns.

## Safe changes to live data
- Back up first and know how to restore.
- Run the `SELECT` with the same `WHERE` before an `UPDATE` or `DELETE` and check the row count.
- Wrap in a transaction: `BEGIN; UPDATE ...; -- check; COMMIT;` or `ROLLBACK;`.
- Use migrations under version control; add columns as nullable or with a default, backfill, then add constraints.

## Pitfalls
- Building SQL by string concatenation with user input opens SQL injection; always use parameterised queries.
- Storing comma-separated lists in one column makes searching and integrity impossible.
- Too many indexes slow writes; index for real queries, not "just in case".
- Engines differ (identity, upsert, date functions, `LIMIT` vs `TOP`); state which one the SQL is for.

## Output
Hand back the schema or query as runnable SQL for the user's engine, a short explanation of how it works, expected results or row counts to check, and any index or safety step needed before running it on real data.
