# Architecture

The public Go library is the only protocol surface consumed by workflows and
applications. Dependency direction is enforced by `tools/arch-check`.

```text
internal/wire and low-level helpers
              ↓
      public digitalpaper API
              ↓
       workflow packages
              ↓
        dp and OS adapters
```

The library must not import CLI, rendering, workflow, command, or simulator
packages. Workflow packages must use the public API rather than importing
`internal/wire` directly. Renderer adapters do not communicate with devices.

Future source locations are reserved with documentation, not functions that
return false success.
