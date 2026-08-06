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
