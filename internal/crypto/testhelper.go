package crypto

// ResetForTest re-reads the secret key from environment, intended for tests
// that need to switch the active key via t.Setenv across packages.
// Production code MUST NOT call this.
func ResetForTest() {
	initKey()
}
