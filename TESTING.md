# Testing

## Design testing strategy with clear boundaries

| Test Level | What to Test | What to Mock |
|------------|--------------|--------------|
| Unit | Pure order rules and usecase decisions such as validating input, rejecting zero-amount orders, choosing whether to call payment, and mapping dependency errors into domain/application errors. | Repositories, payment client, inventory client, clock, ID generator, message publisher. |
| Integration | Adapter contracts and infrastructure behavior such as SQL generation, schema constraints, serialization, HTTP request/response handling, timeout behavior, and retry behavior. | Only systems outside the boundary under test. For repository tests: mock nothing, use a real database. For payment client tests: mock the third-party API with a real HTTP stub such as WireMock. |
| E2E | Full order flow from inbound request to persisted state and emitted side effects, such as `POST /orders` creating an order, charging payment, reserving inventory, writing outbox records, and exposing the final status. | Only systems that are truly external to the service under test and cannot be run locally, or nothing if all dependencies can be started in containers. |

