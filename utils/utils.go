package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

var (
	MSGENOUGH = errors.New("The number of consensus messages at the current stage has reached the requirements")
)

func Hash(content []byte) string {
	h := sha256.New()
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}
