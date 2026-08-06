package aeskw

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"
)

func decodeHex(t testing.TB, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestRFC3394Vectors(t *testing.T) {
	tests := []struct {
		name, kek, key, wrapped string
	}{
		{"128-bit KEK", "000102030405060708090A0B0C0D0E0F", "00112233445566778899AABBCCDDEEFF", "1FA68B0A8112B447AEF34BD8FB5A7B829D3E862371D2CFE5"},
		{"192-bit KEK", "000102030405060708090A0B0C0D0E0F1011121314151617", "00112233445566778899AABBCCDDEEFF", "96778B25AE6CA435F92B5B97C050AED2468AB8A17AD84E5D"},
		{"256-bit KEK", "000102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F", "00112233445566778899AABBCCDDEEFF", "64E8C3F9CE0F5BA263E9777905818A2A93C8191E7D6E8AE7"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kek, key, want := decodeHex(t, test.kek), decodeHex(t, test.key), decodeHex(t, test.wrapped)
			got, err := Wrap(kek, key)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("Wrap() = %X, want %X", got, want)
			}
			plain, err := Unwrap(kek, got)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(plain, key) {
				t.Fatalf("Unwrap() = %X, want %X", plain, key)
			}
		})
	}
}

func TestIntegrityFailure(t *testing.T) {
	kek := decodeHex(t, "000102030405060708090A0B0C0D0E0F")
	wrapped := decodeHex(t, "1FA68B0A8112B447AEF34BD8FB5A7B829D3E862371D2CFE5")
	wrapped[len(wrapped)-1] ^= 1
	if _, err := Unwrap(kek, wrapped); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("Unwrap() error = %v, want ErrIntegrity", err)
	}
}

func TestInvalidLengths(t *testing.T) {
	if _, err := Wrap([]byte("short"), []byte("0123456789abcdef")); !errors.Is(err, ErrInvalidLength) {
		t.Fatalf("Wrap() error = %v, want ErrInvalidLength", err)
	}
	if _, err := Wrap(make([]byte, 16), []byte("short")); !errors.Is(err, ErrInvalidLength) {
		t.Fatalf("Wrap() error = %v, want ErrInvalidLength", err)
	}
	if _, err := Unwrap(make([]byte, 16), []byte("short")); !errors.Is(err, ErrInvalidLength) {
		t.Fatalf("Unwrap() error = %v, want ErrInvalidLength", err)
	}
}

func FuzzWrapRoundTrip(f *testing.F) {
	f.Add([]byte("0123456789abcdef"), []byte("0123456789abcdef"))
	f.Fuzz(func(t *testing.T, kek, key []byte) {
		if len(kek) != 16 && len(kek) != 24 && len(kek) != 32 {
			t.Skip()
		}
		if len(key) < 16 || len(key)%8 != 0 || len(key) > 4096 {
			t.Skip()
		}
		wrapped, err := Wrap(kek, key)
		if err != nil {
			t.Fatal(err)
		}
		plain, err := Unwrap(kek, wrapped)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(plain, key) {
			t.Fatal("round trip changed key")
		}
	})
}
