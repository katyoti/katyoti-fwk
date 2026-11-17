# ktw-test

Virtual terminal test harness sidecar — programmatic agent runs +
session management for integration testing.

## Composes modules

| Module | Role |
|---|---|
| [testharness](../../modules/testharness/) | Domain logic |
| [framework](../../modules/framework/) | Bootstrap |

## Bootstrap

See [cmd/bootstrap.go](cmd/bootstrap.go).

## Use

Wired by integration tests + smoke harnesses. Not deployed to end-user
universes by default.
