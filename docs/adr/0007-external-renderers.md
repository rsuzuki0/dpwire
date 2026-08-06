# ADR 0007: Keep document renderers external

- Status: Accepted
- Date: 2026-08-06

## Context

Markdown and LaTeX conversion ecosystems are larger and less stable than the
device protocol client.

## Decision

PDF is passed through. Markdown and LaTeX use isolated adapters for programs
such as Pandoc, latexmk, or Tectonic. Commands are invoked without a shell.

## Consequences

The core remains small and production runtime dependencies stay explicit.
