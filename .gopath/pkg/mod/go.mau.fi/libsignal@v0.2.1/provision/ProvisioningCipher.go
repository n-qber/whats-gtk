package provision

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"go.mau.fi/libsignal/cipher"
	"go.mau.fi/libsignal/ecc"

	"go.mau.fi/libsignal/kdf"
	"go.mau.fi/libsignal/keys/root"
	"go.mau.fi/libsignal/util/bytehelper"
)

type ProvisionMessage struct {
	IdentityKeyPublic  []byte `json:"identity_key_public"`
	IdentityKeyPrivate []byte `json:"identity_key_private"`
	UserId             string `json:"user_id"`
	ProvisioningCode   string `json:"provisioning_code"`
	ProfileKey         []byte `json:"profile_key"`
}

type ProvisionEnvelope struct {
	PublicKey []byte `json:"public_key"`
	Body      []byte `json:"body"`
}

func verifyMAC(key, input, mac []byte) bool {
	m := hmac.New(sha256.New, key)
	m.Write(input)
	return hmac.Equal(m.Sum(nil), mac)
}

var (
	ErrBadVersionNumber = errors.New("bad version number in provisioning message")
	ErrVerifyMACFailed  = errors.New("failed to verify MAC in provisioning message")
)

func Decrypt(privateKey string, content string) (string, error) {
	ourPrivateKey, err := base64.StdEncoding.DecodeString(privateKey)
	if err != nil {
		return "", err
	}
	envelopeDecode, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		return "", err
	}

	var envelope ProvisionEnvelope
	if err := json.Unmarshal(envelopeDecode, &envelope); err != nil {
		return "", err
	}

	publicKeyable, _ := ecc.DecodePoint(envelope.PublicKey, 0)
	masterEphemeral := publicKeyable.PublicKey()
	message := envelope.Body
	if message[0] != 1 {
		return "", ErrBadVersionNumber
	}

	iv := message[1 : 16+1]
	mac := message[len(message)-32:]
	ivAndCiphertext := message[0 : len(message)-32]
	cipherText := message[16+1 : len(message)-32]

	sharedSecret := kdf.CalculateSharedSecret(masterEphemeral, bytehelper.SliceToArray(ourPrivateKey))
	derivedSecretBytes, err := kdf.DeriveSecrets(sharedSecret[:], nil, []byte("Mixin Provisioning Message"), root.DerivedSecretsSize)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	aesKey := derivedSecretBytes[:32]
	macKey := derivedSecretBytes[32:]

	if !verifyMAC(macKey, ivAndCiphertext, mac) {
		return "", ErrVerifyMACFailed
	}
	plaintext, err := cipher.DecryptCbc(iv, aesKey, cipherText)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
