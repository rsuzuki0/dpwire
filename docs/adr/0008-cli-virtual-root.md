# ADR 0008: Hide the protocol root from the CLI

## Decision

The CLI accepts only root-relative device paths such as `Documents/paper.pdf`.
`.` represents the device root. The protocol-internal `Document/...` form is
rejected instead of retained as a second syntax.

The public Go library continues to use canonical `RemotePath` values beginning
with `Document`, because those values model the wire protocol. The CLI alone
performs the translation. HTTP endpoint names remain private implementation
details. ADR 0011 defines optional DPWire object references independently of
the hidden protocol root.

## Consequences

Users familiar with Unix file commands and FTP transfer commands can use
`ls`, `file`, `stat`, `mkdir`, `cp`, `mv`, `put`, and `get` without learning
Sony's internal root name or HTTP interface. There is one accepted path syntax,
so old scripts containing `Document/` must be updated rather than silently
preserving two forms.
