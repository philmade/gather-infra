// Package hashcash implements proof-of-work challenge-response for spam prevention.
//
// The server issues a random challenge string. The client must find a nonce such that
// SHA-256(challenge + ":" + nonce) has at least N leading zero bits. Finding the nonce
// requires brute-force work; verifying takes a single hash.
//
// Difficulty 20 ≈ 1 second, 22 ≈ 3-5 seconds, 24 ≈ 10-20 seconds on a modern CPU.
package hashcash

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// NewChallenge generates a random 16-byte hex challenge string.
func NewChallenge() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate challenge: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Verify checks that SHA-256(challenge + ":" + nonce) has at least `difficulty` leading zero bits.
func Verify(challenge, nonce string, difficulty int) bool {
	if challenge == "" || nonce == "" || difficulty < 1 || difficulty > 32 {
		return false
	}
	hash := sha256.Sum256([]byte(challenge + ":" + nonce))
	return hasLeadingZeroBits(hash[:], difficulty)
}

// hasLeadingZeroBits checks whether the first `n` bits of data are zero.
func hasLeadingZeroBits(data []byte, n int) bool {
	fullBytes := n / 8
	remainBits := n % 8

	if len(data) < fullBytes+1 && remainBits > 0 {
		return false
	}

	for i := 0; i < fullBytes; i++ {
		if data[i] != 0 {
			return false
		}
	}

	if remainBits > 0 {
		// Check that the top `remainBits` of the next byte are zero.
		// e.g. remainBits=3 → mask=0b11100000=0xE0
		mask := byte(0xFF << (8 - remainBits))
		if data[fullBytes]&mask != 0 {
			return false
		}
	}

	return true
}
