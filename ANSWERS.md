# Answers

## 1: How do you trace domain operations without passing context? Describe two approaches.

1. Emit domain events from the pure domain, then trace them in the event handlers.
2. Trace around the domain call from an outer score.

Both approaches preserve domain purity. The first is better when the operation is business-significant and can be modeled as an event. The second is better when execution timing or boundary tracing is enough.

## 2: Give specific examples

1. When mocking is the RIGHT choice
   - Testing pure business logic in a usecase.
   - Testing error handling, retries, recovery, or timeout without depending on infrastructure.
   - Example: a usecase that should send a notification only when payment succeeds.

2. When mocking HIDES bugs
   - When the mock does not enforce the infrastructure contract e.g. SQL query, API contract.
   - Example bug: a repository mock returns success even though the real SQL is invalid, the constraint is not hit.
   - Example bug: a payment client mock accepts a request shape that differs from the real API or the validation fails.

3. When you need both mock tests and integration tests
   - When the code has a real adapter contract.
   - Example: test the usecase with mocks, then test the repository against a real database and the HTTP client against a real stub server.
   - The mock test proves business logic coverage; the integration test proves the adapter contract.

## 3: Customer reports: "Order was charged but shows as failed." - Design debugging workflow

Logs:
- Correlation/trace ID for the full request path.
- Order ID, payment transaction ID, customer ID, and state transition logs.
- Logs from infrastructure - DB query success/failure, payment API response, retries, and any async processing.
- A timeline of events such as `order_created`, `payment_authorized`, `order_marked_failed`, `compensation_started`.

Metrics:
- Count of charged-but-failed outcomes if available.
- Payment success rate vs order finalization success rate.
- Latency and error rate of payment calls and DB writes.

Alerts:
- Alert when payment success stays high but order success drops.
- Alert on mismatch between payment-authorized and order-completed counts.
- Alert on backlog growth if final state depends on async processing.

## 4: The outbox pattern - How do you test this end-to-end WITHOUT flaky timing issues?

By using polling with a bounded timeout:

- Commit transaction with outbox row.
- Run the background worker in the backgroup routine or as a container if necessary.
- Poll the database and the downstream stub until the outbox entry is marked processed and the expected outcome assertion completes e.g. testify `eventually` method.
- Fail with a deadline if the condition never becomes true.

## 5: Your test passes locally but fails in CI. The test does:

The test is comparing two time instances. `time.Now()` is not stable enough to use as the expected value.

Fixes:
- Inject a clock interface and use a fake clock in tests.
- Capture `now := time.Now()` once and compare against that exact value if the test needs time.
- Compare with a tolerance when exact equality is not required e.g. testify `WithinDuration`.
- Use time arguments in the domain logic - accept `time.Time` as an argument into the time related domain methods.
