# ADR 0006: Support explicit addresses before discovery

- Status: Accepted
- Date: 2026-08-06

## Context

mDNS behavior and dependencies vary across operating systems and device
generations.

## Decision

Initial real-device phases accept an explicit address. Discovery is introduced
later behind a small interface.

## Consequences

Core protocol work is testable without mDNS. Discovery can be added without
changing client operations.
