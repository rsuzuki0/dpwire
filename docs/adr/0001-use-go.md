# ADR 0001: Use Go

- Status: Accepted
- Date: 2026-08-06

## Context

The client must remain maintainable without Python, Ruby, or system-library
runtime environments.

## Decision

Use Go 1.24 as the module language version and test newer supported releases in
CI. Prefer pure Go and permit only narrowly justified production dependencies.

## Consequences

The project can ship small cross-platform binaries. Platform integrations and
external renderers remain adapters rather than core dependencies.
