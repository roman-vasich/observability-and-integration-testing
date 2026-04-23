# Tracing

## 1. Which option for usecases? Why?
Which option?

- **Option A** - accept `context.Context`.

Why?
- It matches the `Go` and `OTEL` models.
- It carries deadlines, cancellation, and request-scoped data (requires transport to implement the tracing extraction/propagation).

## 2. Which option for domain methods? Why?
Which option?

- **None of the options**.

Why:
- The domain must stay pure.
- Domain methods should not know about `context.Context` or any instrumentation.

## 3. Which option for repository methods?
Which option?

- **Option A** - accept `context.Context`.

Why:
- Repositories perform I/O, so they need cancellation and timeout propagation.
- The repository should remain focused on persistence, not on instrumentation.

## 4. How do you trace a domain method without passing context to it?

There are two approaches to preserve domain purity:

1. Wrap the domain call in a traced **decorator**/**domain service**.
2. Emit a domain event and trace the event handling outside the domain.

