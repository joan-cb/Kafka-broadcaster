# Writing Tests

## Unit Testing

- Unit tests must be added for any function that can be meaningfully tested.
- Use the built-in `go test` framework. Name test functions with the `Test` prefix.
- Use table-driven tests to cover multiple cases in a concise manner.
- Aim for test coverage of at least 80% on business logic packages.
- Keep each test focused on a single behaviour; avoid testing multiple concerns in one test function.
- Use `t.Helper()` in shared test utilities so failure lines point to the caller, not the helper.
- Use `t.Parallel()` where tests are independent to speed up the suite.

## Integration Testing

- Write integration tests for Kafka producer/consumer interactions using an embedded or dockerised Kafka broker (e.g. via `testcontainers-go`).
- Separate integration tests from unit tests using build tags (e.g. `//go:build integration`) so they are not run by default.
- Integration tests should cover the full message lifecycle: produce → transform → route → consume.

## Error and Edge Case Testing

- Explicitly test error paths — malformed payloads, missing headers, unknown routing keys.
- Test that transformation rules handle unexpected or missing JSON fields gracefully.
- Test enrichment logic with both valid and invalid input transaction IDs.

## Benchmarking

- Write benchmark functions with the `Benchmark` prefix to measure performance.
- Use the `testing` package's built-in profiling tools to identify performance bottlenecks.

## Mocking and Stubbing

- Use interfaces to create mocks and stubs for testing purposes.
- Inject dependencies via interfaces to facilitate testing.

## Naming Conventions

- Name tests descriptively: `Test<Unit>_<Scenario>_<ExpectedOutcome>` (e.g. `TestRouter_UnknownHeader_ReturnsError`).
- Name table-driven test cases clearly in the `name` field so failures are easy to identify.

## General

- Before committing, ensure all tests pass and can run locally.
- Do not use `init()` or global state in tests; each test should be self-contained.
- Avoid `time.Sleep` in tests; use channels or `sync` primitives to coordinate goroutines.
- Use `github.com/stretchr/testify` for assertions if it improves readability, but prefer the standard library where it suffices.
