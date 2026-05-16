package chatops

import (
	"crypto/rand"
	"fmt"
)

// Character set excluding ambiguous characters: 0, O, 1, l, I
// Total: 31 characters (2^5 per char, 8 chars = 40 bits entropy ≈ 10^12 combinations)
const bindcodeCharset = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// GenerateBindCode generates an 8-character unambiguous binding code using CSPRNG.
// Uses charset without 0/O/1/l/I to reduce user error in manual entry.
// Returns error only on crypto/rand failure (should be extremely rare).
func GenerateBindCode() (string, error) {
	const codeLength = 8
	const charsetLen = len(bindcodeCharset)

	// We need rejection sampling to avoid modulo bias.
	// Maximum valid byte value when distributed uniformly: 31*8 = 248
	// Bytes in range [0, 247] map evenly to [0, 30]; reject 248-255.
	const maxValidByte = byte(charsetLen * 8)

	result := make([]byte, codeLength)
	randomBytes := make([]byte, codeLength+codeLength) // Extra buffer for rejection retries

	for attempts := 0; attempts < 1000; attempts++ {
		if _, err := rand.Read(randomBytes); err != nil {
			return "", fmt.Errorf("failed to read random bytes: %w", err)
		}

		validIdx := 0
		for _, b := range randomBytes {
			if validIdx >= codeLength {
				break
			}
			// Rejection sampling: only accept bytes that distribute evenly
			if b < maxValidByte {
				result[validIdx] = bindcodeCharset[b%byte(charsetLen)]
				validIdx++
			}
		}

		if validIdx == codeLength {
			return string(result), nil
		}
	}

	return "", fmt.Errorf("failed to generate bindcode after 1000 attempts (crypto/rand exhaustion unlikely)")
}
