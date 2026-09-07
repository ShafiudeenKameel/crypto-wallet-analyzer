package ethaddr

import "testing"

// Vectors from EIP-55's own spec (github.com/ethereum/ercs, ERCS/erc-55.md).
func TestValidate_SpecVectors(t *testing.T) {
	mixedCase := []string{
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		"0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359",
		"0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB",
		"0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb",
	}
	for _, addr := range mixedCase {
		got, err := Validate(addr)
		if err != nil {
			t.Errorf("Validate(%q) failed: %v", addr, err)
			continue
		}
		if got != addr {
			t.Errorf("Validate(%q) = %q, want unchanged (already correctly checksummed)", addr, got)
		}
	}

	allLower := []string{
		"0xde709f2102306220921060314715629080e2fb77",
		"0x27b1fdb04752bbc536007a920d24acb045561c26",
	}
	for _, addr := range allLower {
		if _, err := Validate(addr); err != nil {
			t.Errorf("Validate(%q) failed: %v, want accepted (no checksum info to fail)", addr, err)
		}
	}
}

func TestValidate_RejectsBadChecksum(t *testing.T) {
	valid := "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed"
	// Flip one letter's case - a single mistyped character is exactly what
	// EIP-55 exists to catch.
	tampered := "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAeD"

	if _, err := Validate(tampered); err == nil {
		t.Errorf("Validate(%q) = nil error, want a checksum-mismatch error", tampered)
	}
	if _, err := Validate(valid); err != nil {
		t.Errorf("sanity check: Validate(%q) failed: %v", valid, err)
	}
}

func TestValidate_RejectsMalformedInput(t *testing.T) {
	cases := []string{
		"",
		"not an address",
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeA",     // too short
		"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAedFF", // too long
		"5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",     // missing 0x
		"0xZZZeb6053F3E94C9b9A09f33669435E7Ef1BeAed",   // non-hex characters
	}
	for _, addr := range cases {
		if _, err := Validate(addr); err == nil {
			t.Errorf("Validate(%q) = nil error, want a format error", addr)
		}
	}
}
