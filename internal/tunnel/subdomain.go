package tunnel

import (
	"crypto/rand"
	"math/big"
)

const (
	subdomainLen     = 8
	subdomainCharset = "abcdefghijklmnopqrstuvwxyz0123456789"
)

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
