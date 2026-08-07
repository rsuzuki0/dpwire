package registration

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

func TestPBKDF2SHA256Vector(t *testing.T) {
	want, _ := hex.DecodeString("120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b")
	if got := pbkdf2SHA256([]byte("password"), []byte("salt"), 1, 32); !bytes.Equal(got, want) {
		t.Fatalf("PBKDF2 = %x, want %x", got, want)
	}
}

func TestRegistrationWrapRoundTrip(t *testing.T) {
	authKey := bytes.Repeat([]byte{0x11}, 32)
	wrapKey := bytes.Repeat([]byte{0x22}, 16)
	plain := []byte("registration payload")
	wrapped, err := wrap(plain, authKey, wrapKey, bytes.NewReader(bytes.Repeat([]byte{0x33}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := unwrap(wrapped, authKey, wrapKey)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("unwrap = %x, %v", got, err)
	}
	wrapperCorrupt := append([]byte(nil), wrapped...)
	wrapperCorrupt[0] ^= 1
	if _, err := unwrap(wrapperCorrupt, authKey, wrapKey); err == nil {
		t.Fatal("corrupt wrapped value succeeded")
	}
}

func TestDHJavaEncodingAndSharedKey(t *testing.T) {
	left, err := newDH(bytes.NewReader(bytes.Repeat([]byte{0x11}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	right, err := newDH(bytes.NewReader(bytes.Repeat([]byte{0x22}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	leftPublic, rightPublic := left.publicBytes(), right.publicBytes()
	if len(leftPublic) != 257 || leftPublic[0] != 0 || len(rightPublic) != 257 || rightPublic[0] != 0 {
		t.Fatal("public keys do not use the 257-byte Java-compatible encoding")
	}
	leftShared, err := left.sharedBytes(rightPublic)
	if err != nil {
		t.Fatal(err)
	}
	rightShared, err := right.sharedBytes(leftPublic)
	if err != nil || !bytes.Equal(leftShared, rightShared) || len(leftShared) != 256 {
		t.Fatalf("shared keys differ or have wrong size: %v", err)
	}
	if _, err := left.sharedBytes([]byte{1}); err == nil {
		t.Fatal("invalid public key accepted")
	}
}

func TestDeviceRawBigIntegerBytesAffectTranscript(t *testing.T) {
	authKey := bytes.Repeat([]byte{0x44}, 32)
	n1, mac, n2, ya := []byte("n1"), []byte("mac"), []byte("n2"), []byte("ya")
	yb256 := bytes.Repeat([]byte{0x80}, 256)
	yb257 := append([]byte{0}, yb256...)
	first := hashMAC(authKey, n1, mac, yb256, n1, n2, mac, ya)
	second := hashMAC(authKey, n1, mac, yb257, n1, n2, mac, ya)
	if bytes.Equal(first, second) {
		t.Fatal("raw 257-byte BigInteger representation was normalized in transcript")
	}
}

func TestCryptoRejectsInvalidInputs(t *testing.T) {
	if _, err := newDH(nil); err == nil {
		t.Fatal("nil randomness accepted")
	}
	if _, err := unwrap([]byte("short"), make([]byte, 32), make([]byte, 16)); err == nil {
		t.Fatal("short wrapped value accepted")
	}
	if _, err := pkcs7Unpad([]byte(strings.Repeat("x", 15)+"\x00"), 16); err == nil {
		t.Fatal("bad padding accepted")
	}
	if _, err := wrap([]byte("x"), make([]byte, 32), []byte("short"), bytes.NewReader(make([]byte, 16))); err == nil || !strings.Contains(err.Error(), "cipher") {
		t.Fatalf("bad wrapping key error = %v", err)
	}
	if _, err := unwrap(make([]byte, 32), make([]byte, 32), []byte("short")); err == nil || !strings.Contains(err.Error(), "cipher") {
		t.Fatalf("bad unwrapping key error = %v", err)
	}
}
