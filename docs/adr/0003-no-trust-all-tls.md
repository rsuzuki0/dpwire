# ADR 0003: Never trust all TLS certificates by default

- Status: Accepted
- Date: 2026-08-06

## Context

Reference clients commonly bypass certificate validation.

## Decision

Production connections verify a stored device CA or pinned certificate.
Explicit insecure operation, if later implemented, cannot be a persistent
default. Simulator tests trust only their ephemeral simulator certificate.

## Consequences

Credential import and first contact need deliberate trust handling, but normal
traffic is protected from silent interception.
