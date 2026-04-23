# Logging Strategy

## 1. Which option maintains Clean Architecture?

Use **Option C**.

Why:
- It keeps logging outside the usecase and domain.
- It treats logging as a decorator concern instead of a core concern.
- It lets log only allowlisted request and response fields, without leaking full business objects into logs.

## 2. What’s the difference between business logs and operational logs?

Business logs:
- Record domain-significant outcomes such as `order_placed`, `payment_authorized`, or `refund_rejected`.
- Belong at the application boundary or in domain-event handlers.

Operational logs:
- Record system behavior such as request start/finish, dependency failures, retries, and trace IDs.
- Belong in middleware, decorators, or adapters.

## 3. How do you avoid logging sensitive data?

Use redaction and a strict allowlist.

Rules:
- Never log raw request, response, or configuration objects blindly.
- Redact PII, tokens, passwords, secrets, and identifiers that are not needed for debugging.
- Log only the fields that are safe and useful.
- Use types that implement `slog.LogValuer` for sensitive values.
