---
name: software-testing
description: Plan and write automated tests - unit, integration and end-to-end - choose what to test, design good test cases with edge cases, structure them with arrange-act-assert, and fix flaky or slow tests. Use when the user asks "how do I test this", "write tests for this function", "what should I test", wants better coverage, sets up pytest, Jest, JUnit or a similar framework, or has tests that fail randomly or take too long.
---

# Software testing

A good test fails for exactly one reason and tells you what broke. Test behaviour that matters to users, not private details of the implementation.

## Steps
1. **Learn the context.** Language, framework, existing test tool and how tests are run (`pytest`, `npm test`, `mvn test`, `go test ./...`). Follow the project's existing conventions.
2. **Pick the level.**
   - Unit: one function or class, no network or disk; many of these, milliseconds each.
   - Integration: real database, file system or several modules together; fewer.
   - End-to-end: the whole app through its UI or API; a handful for the critical paths (sign-up, checkout).
3. **List the cases before coding.** For each behaviour: the normal case, boundaries (0, 1, max, max+1), empty and missing input, invalid types or formats, error paths, and anything that broke before.
4. **Write each test as arrange - act - assert.** One behaviour per test, a name that reads as a sentence, no logic (loops, ifs) inside the test.
5. **Isolate what you do not control.** Replace time, randomness, network and external services with fakes or stubs; seed random generators; freeze the clock.
6. **Run and watch it fail first.** A test that has never failed may be testing nothing. Break the code on purpose once to see the test catch it.
7. **Run in CI on every change** and keep the suite fast enough that people actually run it locally.

## Example (pytest)
```python
import pytest
from shop.pricing import apply_discount

def test_ten_percent_discount_reduces_price():
    assert apply_discount(price_cents=2000, percent=10) == 1800

def test_zero_percent_keeps_price():
    assert apply_discount(price_cents=2000, percent=0) == 2000

@pytest.mark.parametrize("percent", [-1, 101])
def test_out_of_range_percent_is_rejected(percent):
    with pytest.raises(ValueError):
        apply_discount(price_cents=2000, percent=percent)
```

## Fixing flaky tests
- Order dependence: run tests in random order; remove shared global state; reset fixtures.
- Timing: replace `sleep` with waiting for a condition; use fake clocks.
- External services: stub them; never depend on the internet in unit tests.
- Concurrency: make the test deterministic or test the pieces separately.
- Quarantine a flaky test only with a ticket and a deadline; do not just retry forever.

## Pitfalls
- Chasing a coverage number; 100 % coverage with weak assertions proves little.
- Asserting on implementation details (private calls, exact log text) so every refactor breaks tests.
- Mocking so much that the test only checks the mocks.
- Huge shared fixtures nobody understands; build only the data the test needs.
- Tests that pass on the developer's machine but depend on time zone, locale or file paths.

## Output
Return the test plan (behaviours and cases), the test code ready to paste in the project's framework, the command to run it, and any gaps you deliberately left untested with the reason.
