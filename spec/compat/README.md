# Compatibility catalogs

`operations.json` and `error_codes.json` are deterministic derivatives of the
immutable Polaris snapshot plus the reviewed status overlay in
`implementation.json`. Regenerate them with:

```sh
go run ./tools/spec-check -update
```

Derived status values do not establish physical-device compatibility. Only a
record with explicit model, firmware, test date, and operation result may be
promoted to `device-verified`.

`quirks.json` records places where a preserved reference, a translation, and
physical firmware differ. Each entry identifies the affected operation and
firmware, the specification behavior, the observed behavior, the chosen
compatibility behavior, and its regression tests. Prefer behavior that is both
specification-conformant and accepted by hardware. If those genuinely conflict,
keep the divergence in a model/firmware compatibility layer rather than
weakening the default protocol implementation.
