# ADR 0009: Complete and soak the PDF core before adding renderers

## Decision

The first daily-use release is PDF-only. It includes profile onboarding,
fresh pairing, the familiar file-operation CLI, guarded deletion, installable
binaries, checksums, and recovery documentation. It excludes Markdown, LaTeX,
Pandoc, latexmk, Tectonic, HTML conversion, CUPS, and other rendering workflows.

After release, the PDF core will be used without major feature expansion for a soak
period. Renderer work begins only after physical-device behavior, reconnects,
conflicts, partial failures, CLI semantics, and the public Go API have proved
stable in ordinary use.

## Consequences

The first usable release has a smaller dependency and failure surface. Problems
found during daily use can be attributed to the device protocol or core client
rather than to external document toolchains. Renderer directories and ADR 0007
remain valid architectural reservations, but they do not determine the release scope or
schedule.
