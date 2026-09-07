// Package ethaddr validates and canonicalizes EVM addresses (EIP-55
// checksum). This is where wallet address input is sanitized before it
// reaches any downstream provider call - a security requirement, not a
// nice-to-have, since a malformed or mistyped address should never
// silently pass through to Etherscan/CoinGecko.
package ethaddr

import (
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/crypto/sha3"
)

var hexAddress = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

// Validate checks addr is a well-formed EVM address and, if it carries
// EIP-55 mixed-case checksum information, that the checksum is correct -
// this is what catches a single mistyped character in a pasted address.
// It returns the canonical checksummed form.
func Validate(addr string) (string, error) {
	if !hexAddress.MatchString(addr) {
		return "", fmt.Errorf("ethaddr: %q is not a well-formed address", addr)
	}

	body := addr[2:]
	checksummed := checksum(body)

	// All-lowercase or all-uppercase input carries no checksum information
	// (EIP-55 predates it being universal) - accept it as-is.
	if body == strings.ToLower(body) || body == strings.ToUpper(body) {
		return checksummed, nil
	}

	if body != checksummed[2:] {
		return "", fmt.Errorf("ethaddr: %q fails EIP-55 checksum - likely a typo", addr)
	}
	return checksummed, nil
}

// checksum applies EIP-55: uppercase a hex letter where the corresponding
// nibble of Keccak256(lowercase address) is >= 8.
func checksum(body string) string {
	lower := strings.ToLower(body)

	hash := sha3.NewLegacyKeccak256()
	hash.Write([]byte(lower))
	hashHex := hex.EncodeToString(hash.Sum(nil))

	var out strings.Builder
	out.WriteString("0x")
	for i, c := range lower {
		if c >= 'a' && c <= 'f' && hashHex[i] >= '8' {
			out.WriteRune(c - 'a' + 'A')
			continue
		}
		out.WriteRune(c)
	}
	return out.String()
}
