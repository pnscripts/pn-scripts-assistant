//go:build !(linux && cgo) && !windows && !darwin

package vault

// platformKeyring: none this build can reach, so the key file.
func platformKeyring() (Keyring, bool) { return nil, false }
