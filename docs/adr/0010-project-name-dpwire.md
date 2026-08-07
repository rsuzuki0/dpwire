# ADR 0010: Name the project DPWire

- Status: Accepted
- Date: 2026-08-06

## Decision

The project name is **DPWire**. The repository, Go module, and public package
use `dpwire`:

```text
github.com/rsuzuki0/dpwire
```

The user command remains `dp`, and the simulator remains `dp-sim`.

“Wire” denotes the protocol link between a host application and a digital-paper
device. The link can run over USB Ethernet or Wi-Fi.

## Configuration compatibility

New installations use the platform configuration directory named `dpwire`.
When an existing `digitalpaper` configuration directory is present, DPWire
continues using it in place. Private keys and profiles are not moved or copied
implicitly. If both directories exist, `dpwire` is selected.

## Consequences

Project-owned identifiers and release archives use DPWire. Sony Digital Paper,
Digital Paper App, Fujitsu QUADERNO, protocol hostnames such as
`digitalpaper.local`, and recorded source titles retain their proper names.
