# ADR 0001: Use Go

- Status: Accepted
- Date: 2026-08-06
- Amended: 2026-08-07

## Context

The client must remain maintainable without Python, Ruby, or system-library
runtime environments.

## Decision

Use Go 1.25 as the module language version and test newer supported releases in
CI. Prefer pure Go and permit only narrowly justified production dependencies.
The minimum version was raised from Go 1.24 because the first upstream
`golang.org/x/text` release containing the fix for GO-2026-5970 requires Go
1.25.

## Consequences

The project can ship small cross-platform binaries. Platform integrations and
external renderers remain adapters rather than core dependencies.
