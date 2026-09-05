package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// totpPeriod is the standard 30-second TOTP step.
const totpPeriod = 30

// GenerateTOTPSecret creates a new 160-bit base32 secret (RFC 4226).
func GenerateTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.EncodeToString(b), "="), nil
}

// ProvisioningURI returns an otpauth:// URI for authenticator apps. We do not
// render QR codes (avoids an image dependency); users may type the secret.
func ProvisioningURI(issuer, account, secret string) string {
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", "6")
	v.Set("period", fmt.Sprintf("%d", totpPeriod))
	label := url.PathEscape(issuer + ":" + account)
	return "otpauth://totp/" + label + "?" + v.Encode()
}

func hotp(secretB32 string, counter uint64) (string, error) {
	secret, err := decodeB32(secretB32)
	if err != nil {
		return "", err
	}
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[off]&0x7f) << 24) |
		(uint32(sum[off+1]) << 16) |
		(uint32(sum[off+2]) << 8) |
		uint32(sum[off+3])
	return fmt.Sprintf("%06d", value%1_000_000), nil
}

func decodeB32(s string) ([]byte, error) {
	if pad := len(s) % 8; pad != 0 {
		s += strings.Repeat("=", 8-pad)
	}
	return base32.StdEncoding.DecodeString(strings.ToUpper(s))
}

// ValidateTOTP checks a 6-digit code with ±1 step drift tolerance.
func ValidateTOTP(secret, code string, at time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	step := uint64(at.Unix() / totpPeriod)
	for drift := int64(-1); drift <= 1; drift++ {
		c, err := hotp(secret, uint64(int64(step)+drift))
		if err != nil {
			return false
		}
		if hmac.Equal([]byte(c), []byte(code)) {
			return true
		}
	}
	return false
}

// TOTPCodeAt exposes code generation for tests.
func TOTPCodeAt(secret string, at time.Time) (string, error) {
	return hotp(secret, uint64(at.Unix()/totpPeriod))
}
