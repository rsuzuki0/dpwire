// Package aeskw implements AES Key Wrap as specified by RFC 3394.
package aeskw

import (
	"crypto/aes"
	"crypto/subtle"
	"encoding/binary"
	"errors"
)

var (
	// ErrInvalidLength reports a KEK or payload incompatible with RFC 3394.
	ErrInvalidLength = errors.New("aeskw: invalid length")
	// ErrIntegrity reports an unwrap integrity-check failure.
	ErrIntegrity = errors.New("aeskw: integrity check failed")
)

var defaultIV = [8]byte{0xa6, 0xa6, 0xa6, 0xa6, 0xa6, 0xa6, 0xa6, 0xa6}

// Wrap wraps plaintext key data with kek.
func Wrap(kek, plaintext []byte) ([]byte, error) {
	if len(plaintext) < 16 || len(plaintext)%8 != 0 {
		return nil, ErrInvalidLength
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, ErrInvalidLength
	}
	n := len(plaintext) / 8
	a := defaultIV
	r := append([]byte(nil), plaintext...)
	b := make([]byte, 16)
	for j := 0; j <= 5; j++ {
		for i := 1; i <= n; i++ {
			copy(b[:8], a[:])
			copy(b[8:], r[(i-1)*8:i*8])
			block.Encrypt(b, b)
			t := uint64(n*j + i)
			binary.BigEndian.PutUint64(a[:], binary.BigEndian.Uint64(b[:8])^t)
			copy(r[(i-1)*8:i*8], b[8:])
		}
	}
	return append(a[:], r...), nil
}

// Unwrap unwraps ciphertext and verifies its RFC 3394 integrity value.
func Unwrap(kek, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < 24 || len(ciphertext)%8 != 0 {
		return nil, ErrInvalidLength
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, ErrInvalidLength
	}
	n := len(ciphertext)/8 - 1
	var a [8]byte
	copy(a[:], ciphertext[:8])
	r := append([]byte(nil), ciphertext[8:]...)
	b := make([]byte, 16)
	for j := 5; j >= 0; j-- {
		for i := n; i >= 1; i-- {
			t := uint64(n*j + i)
			binary.BigEndian.PutUint64(b[:8], binary.BigEndian.Uint64(a[:])^t)
			copy(b[8:], r[(i-1)*8:i*8])
			block.Decrypt(b, b)
			copy(a[:], b[:8])
			copy(r[(i-1)*8:i*8], b[8:])
		}
	}
	if subtle.ConstantTimeCompare(a[:], defaultIV[:]) != 1 {
		return nil, ErrIntegrity
	}
	return r, nil
}
