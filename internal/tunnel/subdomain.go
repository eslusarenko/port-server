package tunnel

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

const (
	subdomainLen     = 8
	subdomainCharset = "abcdefghijklmnopqrstuvwxyz0123456789"
	subdomainMaxLen  = 32
)

var validSubdomainRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)

// ValidateSubdomain checks that s is a legal tunnel subdomain.
func ValidateSubdomain(s string) error {
	if len(s) > subdomainMaxLen {
		return fmt.Errorf("subdomain %q too long (max %d characters)", s, subdomainMaxLen)
	}
	if !validSubdomainRe.MatchString(s) {
		return fmt.Errorf("subdomain %q is invalid: use lowercase letters, digits, and hyphens only", s)
	}
	return nil
}

// GenerateSubdomain returns a random 8-character lowercase alphanumeric string
// suitable for use as a tunnel subdomain.
func GenerateSubdomain() (string, error) {
	b := make([]byte, subdomainLen)
	max := big.NewInt(int64(len(subdomainCharset)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = subdomainCharset[n.Int64()]
	}
	return string(b), nil
}

// IsReserved reports whether subdomain (case-insensitive) is in the reserved list.
// The list itself is purely operator-supplied via PORT_RESERVED_SUBDOMAINS / config.
func IsReserved(subdomain string, reserved []string) bool {
	lower := strings.ToLower(subdomain)
	for _, r := range reserved {
		if lower == strings.ToLower(r) {
			return true
		}
	}
	return false
}
