# Architecture

The public Go library is the only protocol surface consumed by workflows and
applications. Dependency direction is enforced by `tools/arch-check`.

```text
internal/wire and low-level helpers
              ↓
         public dpwire API
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

Named profile management is a separate public support package. It may depend on
the core `dpwire` and `credentials` packages, while the core library must
not depend on profile storage. Profile files and copied private keys are written
through owner-private persistence primitives in `internal/atomicfile`.

Fresh registration is exposed by the public `pairing` package and implemented
by `internal/wire/registration`. The wire package owns the HTTP registration
state machine and cryptography but never writes credentials. `profiles` owns
atomic persistence and invokes `pairing`; the core authenticated client remains
independent from both.
