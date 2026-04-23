# Design metrics for order processing

## Design metrics for order processing

The metrics are design for the order checkout usecase and cover following layers:

- Business logic
- Dependency behavior
- Asynchronous processing

## 1. Where do you instrument?

- **Usecase/application layer** for business metrics like order success/failure and end-to-end checkout duration.
- **Infrastructure adapters** for dependency metrics like latency, errors, API failures etc.
- **Worker/infrastructure wrappers** for queue lag, retries, throughput, and fulfillment latency.

## 2. Why does this cause metric explosion, and how do you fix it?

This explodes because `customerID`, `productID`, and `orderID` are high-cardinality labels - each unique combination creates a new time series.

Fix:

- Using only bounded labels such as `status` or `customer_tier`.
- Keeping ids in logs or traces.
- Aggregating by business category instead of individual entities.

## 3. How do you correlate metrics with traces?

- Use exemplars on histograms and counters that represent important business or dependency events.
- Keeping trace IDs in logs for every request.
