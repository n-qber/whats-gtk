package tests

import (
	"testing"

	"go.mau.fi/libsignal/logger"
	"go.mau.fi/libsignal/serialize"
	"go.mau.fi/libsignal/util/keyhelper"
)

// TestPreKeys checks generating prekeys.
func TestPreKeys(t *testing.T) {

	// Create a serializer object that will be used to encode/decode data.
	serializer := serialize.NewSerializer()
	serializer.SignalMessage = &serialize.ProtoBufSignalMessageSerializer{}
	serializer.PreKeySignalMessage = &serialize.ProtoBufPreKeySignalMessageSerializer{}
	serializer.SignedPreKeyRecord = &serialize.JSONSignedPreKeyRecordSerializer{}

	logger.Info("Testing prekey generation...")
	identityKeyPair, err := keyhelper.GenerateIdentityKeyPair()
	if err != nil {
		t.Error(err)
	}

	logger.Info("Generating prekeys")
	preKeys, _ := keyhelper.GeneratePreKeys(1, 100, serializer.PreKeyRecord)
	logger.Info("Generated PreKeys: ", preKeys)

	logger.Info("Generating Signed PreKey")
	signedPreKey, _ := keyhelper.GenerateSignedPreKey(identityKeyPair, 1, serializer.SignedPreKeyRecord)
	logger.Info("Signed PreKey: ", signedPreKey)
}
