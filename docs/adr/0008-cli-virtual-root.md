# ADR 0008: Hide the protocol root from the CLI

## Decision

The CLI has a fixed device root `/` and retains no directory position between
commands. It accepts absolute paths such as `/Documents/paper.pdf` and the
root-relative shorthand `Documents/paper.pdf`. Public output uses the absolute
form. The protocol-internal `Document/...` form is rejected.

The public Go library continues to use canonical `RemotePath` values beginning
with `Document`, because those values model the wire protocol. The CLI alone
performs the translation. HTTP endpoint names remain private implementation
details. ADR 0011 defines optional DPWire object references independently of
the hidden protocol root.

## Consequences

Users familiar with Unix file commands and FTP transfer commands can use
`ls`, `file`, `stat`, `mkdir`, `cp`, `mv`, `put`, and `get` without learning
Sony's internal root name or HTTP interface. Absolute and root-relative forms
resolve identically, while old scripts containing `Document/` must be updated.
