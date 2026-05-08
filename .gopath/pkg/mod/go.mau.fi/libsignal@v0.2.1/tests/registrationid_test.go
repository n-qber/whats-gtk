package tests

import (
	"fmt"
	"testing"

	"go.mau.fi/libsignal/util/keyhelper"
)

func TestRegistrationID(t *testing.T) {
	regID := keyhelper.GenerateRegistrationID()
	fmt.Println(regID)
}
