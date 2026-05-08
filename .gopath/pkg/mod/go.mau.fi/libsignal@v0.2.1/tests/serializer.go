package tests

import (
	"go.mau.fi/libsignal/serialize"
)

// newSerializer will return a JSON serializer for testing.
func newSerializer() *serialize.Serializer {
	serializer := serialize.NewProtoBufSerializer()

	return serializer
}
