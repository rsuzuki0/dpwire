# ADR 0005: Do not reserve features with successful stubs

- Status: Accepted
- Date: 2026-08-06

## Context

Empty functions returning success hide missing safety and protocol work.

## Decision

Reserve directories and capability identifiers only. Unsupported operations
return typed errors and unpublished commands stay out of help. The P0 simulator
returns HTTP 501 for reserved registration endpoints.

## Consequences

Callers can distinguish unavailable work from successful work, and later
features have stable locations without misleading behavior.
