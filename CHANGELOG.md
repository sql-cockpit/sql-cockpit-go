# Changelog

## 0.1.0 - 2026-11-21

First release.

- `sqlcockpit.NewClient` for the portal API v1: account, usage, API keys (list), tenants, team members, Agents, instances and support tickets.
- Typed structs for every request and response, taken from the OpenAPI document. Nullable fields are pointers; times are `time.Time`.
- `*sqlcockpit.Error` with `Code`, `Status`, `Message`, `Fields` and `RetryAfter`, matched by `errors.Is` against `ErrUnauthenticated`, `ErrForbidden`, `ErrNotFound`, `ErrValidation`, `ErrRateLimited` and `ErrServer`, with helpers such as `IsNotFound`.
- GET requests retry on 429 and 5xx (up to 2 times, honouring Retry-After up to 30 seconds). Writes are never retried.
- https only (http allowed for localhost), redirects refused, responses capped at 2 MiB, ids checked before sending, the API key kept out of errors, formatted output and logs.
- Standard library only.
