package service

import (
	"crypto/rand"
	"fmt"
)

// RealCodeGenerator implements the CodeGenerator interface using
// crypto/rand to produce 6-digit verification codes.
type RealCodeGenerator struct{}

// Generate produces a random 6-digit code (100000–999999).
func (RealCodeGenerator) Generate() string {
	b := make([]byte, 3) // 3 bytes → 24 bits → 0–16777215
	if _, err := rand.Read(b); err != nil {
		// Fallback to a non-zero code on RNG failure (extremely rare).
		return "123456"
	}
	// Mask to 0–899999 and shift to 100000–999999.
	n := int(b[0])<<16 | int(b[1])<<8 | int(b[2])
	n = n%900000 + 100000
	return fmt.Sprintf("%06d", n)
}
