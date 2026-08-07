// Package registration implements the unauthenticated Digital Paper client
// registration wire protocol. It is kept internal; public callers use the
// pairing package.
package registration

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"math/big"
)

const group14PrimeHex = "FFFFFFFFFFFFFFFFC90FDAA22168C234C4C6628B80DC1CD129024E088A67CC74020BBEA63B139B22514A08798E3404DDEF9519B3CD3A431B302B0A6DF25F14374FE1356D6D51C245E485B576625E7EC6F44C42E9A637ED6B0BFF5CB6F406B7EDEE386BFB5A899FA5AE9F24117C4B1FE649286651ECE45B3DC2007CB8A163BF0598DA48361C55D39A69163FA8FD24CF5F83655D23DCA3AD961C62F356208552BB9ED529077096966D670C354E4ABC9804F1746C08CA18217C32905E462E36CE3BE39E772C180E86039B2783A2EC07A28FB5C55DF06F4C52C9DE2BCBF6955817183995497CEA956AE515D2261898FA051015728E5A8AACAA68FFFFFFFFFFFFFFFF"

var group14Prime, _ = new(big.Int).SetString(group14PrimeHex, 16)

type dhExchange struct{ private *big.Int }

func newDH(random io.Reader) (*dhExchange, error) {
	if random == nil {
		return nil, errors.New("registration: nil randomness source")
	}
	for attempts := 0; attempts < 8; attempts++ {
		value := make([]byte, 32)
		if _, err := io.ReadFull(random, value); err != nil {
			return nil, err
		}
		private := new(big.Int).SetBytes(value)
		if private.Sign() != 0 {
			return &dhExchange{private: private}, nil
		}
	}
	return nil, errors.New("registration: randomness produced a zero DH key")
}

// publicBytes reproduces the 257-byte positive Java BigInteger representation
// used by known DPT-RP1 registration implementations.
func (d *dhExchange) publicBytes() []byte {
	public := new(big.Int).Exp(big.NewInt(2), d.private, group14Prime)
	result := make([]byte, 257)
	public.FillBytes(result[1:])
	return result
}

func (d *dhExchange) sharedBytes(otherRaw []byte) ([]byte, error) {
	if len(otherRaw) != 256 && !(len(otherRaw) == 257 && otherRaw[0] == 0) {
		return nil, errors.New("registration: invalid device DH contribution length")
	}
	other := new(big.Int).SetBytes(otherRaw)
	upper := new(big.Int).Sub(group14Prime, big.NewInt(2))
	if other.Cmp(big.NewInt(2)) < 0 || other.Cmp(upper) > 0 {
		return nil, errors.New("registration: invalid device DH public key")
	}
	q := new(big.Int).Rsh(new(big.Int).Sub(group14Prime, big.NewInt(1)), 1)
	if new(big.Int).Exp(other, q, group14Prime).Cmp(big.NewInt(1)) != 0 {
		return nil, errors.New("registration: device DH public key is outside the expected subgroup")
	}
	shared := new(big.Int).Exp(other, d.private, group14Prime)
	result := make([]byte, 256)
	shared.FillBytes(result)
	return result, nil
}

func deriveKeys(shared, salt []byte) (authKey, wrapKey []byte) {
	derived := pbkdf2SHA256(shared, salt, 10_000, 48)
	return derived[:32], derived[32:]
}

func pbkdf2SHA256(password, salt []byte, iterations, length int) []byte {
	result := make([]byte, 0, length)
	for block := uint32(1); len(result) < length; block++ {
		counter := []byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)}
		mac := hmac.New(sha256.New, password)
		_, _ = mac.Write(salt)
		_, _ = mac.Write(counter)
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for index := 1; index < iterations; index++ {
			mac = hmac.New(sha256.New, password)
			_, _ = mac.Write(u)
			u = mac.Sum(nil)
			for offset := range t {
				t[offset] ^= u[offset]
			}
		}
		result = append(result, t...)
	}
	return result[:length]
}

func hashMAC(key []byte, values ...[]byte) []byte {
	mac := hmac.New(sha256.New, key)
	for _, value := range values {
		_, _ = mac.Write(value)
	}
	return mac.Sum(nil)
}

func wrap(data, authKey, wrapKey []byte, random io.Reader) ([]byte, error) {
	block, err := aes.NewCipher(wrapKey)
	if err != nil {
		return nil, fmt.Errorf("registration: create wrapping cipher: %w", err)
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(random, iv); err != nil {
		return nil, err
	}
	kwa := hashMAC(authKey, data)[:8]
	plain := pkcs7Pad(append(append([]byte(nil), data...), kwa...), aes.BlockSize)
	ciphertext := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, plain)
	return append(ciphertext, iv...), nil
}

func unwrap(data, authKey, wrapKey []byte) ([]byte, error) {
	if len(data) < 2*aes.BlockSize || len(data)%aes.BlockSize != 0 {
		return nil, errors.New("registration: invalid wrapped value length")
	}
	block, err := aes.NewCipher(wrapKey)
	if err != nil {
		return nil, fmt.Errorf("registration: create unwrapping cipher: %w", err)
	}
	iv, ciphertext := data[len(data)-aes.BlockSize:], data[:len(data)-aes.BlockSize]
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ciphertext)
	plain, err = pkcs7Unpad(plain, aes.BlockSize)
	if err != nil || len(plain) < 8 {
		return nil, errors.New("registration: invalid wrapped value padding")
	}
	value, kwa := plain[:len(plain)-8], plain[len(plain)-8:]
	want := hashMAC(authKey, value)[:8]
	if subtle.ConstantTimeCompare(kwa, want) != 1 {
		return nil, errors.New("registration: wrapped value integrity check failed")
	}
	return value, nil
}

func pkcs7Pad(value []byte, size int) []byte {
	padding := size - len(value)%size
	result := make([]byte, len(value)+padding)
	copy(result, value)
	for index := len(value); index < len(result); index++ {
		result[index] = byte(padding)
	}
	return result
}

func pkcs7Unpad(value []byte, size int) ([]byte, error) {
	if len(value) == 0 || len(value)%size != 0 {
		return nil, errors.New("invalid padding")
	}
	padding := int(value[len(value)-1])
	if padding == 0 || padding > size || padding > len(value) {
		return nil, errors.New("invalid padding")
	}
	valid := 1
	for _, item := range value[len(value)-padding:] {
		valid &= subtle.ConstantTimeByteEq(item, byte(padding))
	}
	if valid != 1 {
		return nil, errors.New("invalid padding")
	}
	return value[:len(value)-padding], nil
}
