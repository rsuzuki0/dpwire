# Registration wire protocol

This internal package implements complete P3 client registration: PIN, RFC 3526
group 14 DH, PBKDF2-HMAC-SHA256, transcript HMACs, the protocol's AES-CBC
wrapping format, RSA identity generation, device-certificate validation, and
cleanup. It preserves the device's raw 256/257-byte Java BigInteger encoding in
authenticated transcripts. Public callers use `pairing`; `dptest` uses the
server-side emulator.
