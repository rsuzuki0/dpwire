# ADR 0002: Start with one module

- Status: Accepted
- Date: 2026-08-06

## Context

Splitting the library, CLI, and simulator across modules would multiply release
and compatibility work before their boundaries are proven.

## Decision

Use the module `github.com/rsuzuki0/digitalpaper`. Enforce package dependency
direction mechanically.

## Consequences

Library and tools share revisions while remaining structurally separated. A
future module split requires a separate ADR and demonstrated need.
