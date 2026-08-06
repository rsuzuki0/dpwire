# ADR 0004: Separate sync planning from application

- Status: Accepted
- Date: 2026-08-06

## Context

Annotations and divergent local/device edits make automatic overwrite unsafe.

## Decision

Synchronization first produces a reviewable plan and later applies it only
after revalidating revisions and hashes. Deletion and conflict overwrite are
disabled by default.

## Consequences

Sync requires two steps but provides an auditable recovery boundary.
