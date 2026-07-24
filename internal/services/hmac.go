package services

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"strings"
)

type HMACValidator struct {
	secret    string
	algorithm string
}

func NewHMACValidator(secret, algorithm string) *HMACValidator {
	return &HMACValidator{
		secret:    secret,
		algorithm: strings.ToLower(algorithm),
	}
}

func (v *HMACValidator) getHash() func() hash.Hash {
	switch v.algorithm {
	case "sha1":
		return sha1.New
	case "sha512":
		return sha512.New
	default:
		return sha256.New
	}
}

func (v *HMACValidator) Validate(payload []byte, signature string) bool {
	if v.secret == "" {
		return true
	}

	sig := signature
	if idx := strings.Index(signature, "="); idx != -1 {
		sig = signature[idx+1:]
	}

	mac := hmac.New(v.getHash(), []byte(v.secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(sig), []byte(expected))
}

func GetSignatureHeader(headers map[string][]string) string {
	if sig := headers["X-Hub-Signature-256"]; len(sig) > 0 {
		return sig[0]
	}
	if sig := headers["X-Gitlab-Token"]; len(sig) > 0 {
		return sig[0]
	}
	if sig := headers["X-Signature"]; len(sig) > 0 {
		return sig[0]
	}
	return ""
}
