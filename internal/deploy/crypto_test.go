package deploy

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testKey = []byte("0123456789abcdef0123456789abcdef") // 32 bytes

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	plaintext := "hello world, this is a secret message"

	ciphertext, err := Encrypt(plaintext, testKey)
	require.NoError(t, err)
	assert.NotEmpty(t, ciphertext)
	assert.NotEqual(t, plaintext, ciphertext)

	decrypted, err := Decrypt(ciphertext, testKey)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestEncrypt_EmptyKey(t *testing.T) {
	_, err := Encrypt("hello", []byte{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")
}

func TestEncrypt_WrongLengthKey(t *testing.T) {
	shortKey := []byte("short")
	_, err := Encrypt("hello", shortKey)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")

	longKey := []byte("this key is way too long for aes 256 gcm!!")
	_, err = Encrypt("hello", longKey)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")
}

func TestDecrypt_EmptyKey(t *testing.T) {
	_, err := Decrypt("somedata", []byte{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")
}

func TestDecrypt_WrongLengthKey(t *testing.T) {
	_, err := Decrypt("somedata", []byte("short"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	plaintext := "important data"

	ciphertext, err := Encrypt(plaintext, testKey)
	require.NoError(t, err)

	// Decode, flip a byte in the ciphertext body (after nonce), re-encode
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	require.NoError(t, err)
	data[len(data)-1] ^= 0xFF
	tampered := base64.StdEncoding.EncodeToString(data)

	_, err = Decrypt(tampered, testKey)
	assert.Error(t, err, "decryption of tampered ciphertext should fail")
}

func TestDecrypt_InvalidBase64(t *testing.T) {
	_, err := Decrypt("not-valid-base64!!!", testKey)
	assert.Error(t, err)
}

func TestEncrypt_EmptyPlaintext(t *testing.T) {
	ciphertext, err := Encrypt("", testKey)
	require.NoError(t, err)
	assert.NotEmpty(t, ciphertext)

	decrypted, err := Decrypt(ciphertext, testKey)
	require.NoError(t, err)
	assert.Equal(t, "", decrypted)
}
