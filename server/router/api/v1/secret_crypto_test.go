package v1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncryptDecryptSensitiveValue(t *testing.T) {
	secret := "test-secret"
	plainText := "sk-test-123"

	encryptedText, err := encryptSensitiveValue(secret, plainText)
	require.NoError(t, err)
	require.NotEqual(t, plainText, encryptedText)
	require.Contains(t, encryptedText, sensitiveValueCipherPrefix)

	decryptedText, err := decryptSensitiveValue(secret, encryptedText)
	require.NoError(t, err)
	require.Equal(t, plainText, decryptedText)
}

func TestDecryptSensitiveValue_LegacyPlaintext(t *testing.T) {
	secret := "test-secret"
	plainText := "legacy-plain-value"

	decryptedText, err := decryptSensitiveValue(secret, plainText)
	require.NoError(t, err)
	require.Equal(t, plainText, decryptedText)
}

func TestDecryptSensitiveValue_WrongSecret(t *testing.T) {
	encryptedText, err := encryptSensitiveValue("secret-1", "sk-test-123")
	require.NoError(t, err)

	_, err = decryptSensitiveValue("secret-2", encryptedText)
	require.Error(t, err)
}
