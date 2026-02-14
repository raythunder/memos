package v1

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"strings"

	"github.com/pkg/errors"
)

const sensitiveValueCipherPrefix = "enc:v1:"

func encryptSensitiveValue(secret string, plainText string) (string, error) {
	if plainText == "" {
		return "", nil
	}

	aead, err := newSensitiveValueCipher(secret)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", errors.Wrap(err, "failed to generate nonce")
	}

	cipherText := aead.Seal(nil, nonce, []byte(plainText), nil)
	payload := append(nonce, cipherText...)
	return sensitiveValueCipherPrefix + base64.StdEncoding.EncodeToString(payload), nil
}

func decryptSensitiveValue(secret string, encryptedText string) (string, error) {
	if encryptedText == "" {
		return "", nil
	}

	// Backward compatibility: values without prefix are treated as legacy plaintext.
	if !strings.HasPrefix(encryptedText, sensitiveValueCipherPrefix) {
		return encryptedText, nil
	}

	encodedPayload := strings.TrimPrefix(encryptedText, sensitiveValueCipherPrefix)
	payload, err := base64.StdEncoding.DecodeString(encodedPayload)
	if err != nil {
		return "", errors.Wrap(err, "failed to decode encrypted payload")
	}

	aead, err := newSensitiveValueCipher(secret)
	if err != nil {
		return "", err
	}
	nonceSize := aead.NonceSize()
	if len(payload) <= nonceSize {
		return "", errors.New("encrypted payload is too short")
	}

	plainText, err := aead.Open(nil, payload[:nonceSize], payload[nonceSize:], nil)
	if err != nil {
		return "", errors.Wrap(err, "failed to decrypt payload")
	}
	return string(plainText), nil
}

func newSensitiveValueCipher(secret string) (cipher.AEAD, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, errors.New("secret is required")
	}

	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cipher")
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create gcm")
	}
	return aead, nil
}
