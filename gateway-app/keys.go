package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/charmbracelet/keygen"

	sshlib "charm.land/ssh"
)

// loadHostSigners loads the gateway's host identity from dir. It expects
// ssh_host_ed25519_key (generating one there only if missing, so the
// server's identity survives pod restarts) and, if present,
// ssh_host_rsa_key -- the same two keys the old shell-based gateway used,
// so clients that already trust them don't see a host-key warning.
func loadHostSigners(dir string) ([]sshlib.Signer, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil { // no-op when the directory is a mounted volume
		return nil, fmt.Errorf("creating host key directory %s: %w", dir, err)
	}

	var signers []sshlib.Signer

	ed25519Path := filepath.Join(dir, "ssh_host_ed25519_key")
	kp, err := keygen.New(ed25519Path, keygen.WithKeyType(keygen.Ed25519))
	if err != nil {
		return nil, fmt.Errorf("loading/generating ed25519 host key %s: %w", ed25519Path, err)
	}
	// keygen.WithWrite chmods the parent directory to 0700, which a non-root
	// process can't do on a mounted volume, so write a new key pair ourselves.
	if _, err := os.Stat(ed25519Path); errors.Is(err, fs.ErrNotExist) {
		if err := os.WriteFile(ed25519Path, kp.RawPrivateKey(), 0o600); err != nil {
			return nil, fmt.Errorf("writing ed25519 host key %s: %w", ed25519Path, err)
		}
		if err := os.WriteFile(ed25519Path+".pub", kp.RawAuthorizedKey(), 0o644); err != nil {
			return nil, fmt.Errorf("writing ed25519 host public key %s.pub: %w", ed25519Path, err)
		}
	}
	signers = append(signers, kp.Signer())

	rsaPath := filepath.Join(dir, "ssh_host_rsa_key")
	if _, err := os.Stat(rsaPath); err == nil {
		rkp, err := keygen.New(rsaPath, keygen.WithKeyType(keygen.RSA))
		if err != nil {
			return nil, fmt.Errorf("loading rsa host key %s: %w", rsaPath, err)
		}
		signers = append(signers, rkp.Signer())
	}

	return signers, nil
}

// authorizedKeysChecker re-reads the authorized_keys file on every call, so
// updates to the backing ConfigMap take effect without restarting the
// gateway.
type authorizedKeysChecker struct {
	path string
}

func newAuthorizedKeysChecker(path string) *authorizedKeysChecker {
	return &authorizedKeysChecker{path: path}
}

// Allowed reports whether key matches any key in the authorized_keys file.
func (a *authorizedKeysChecker) Allowed(key sshlib.PublicKey) bool {
	data, err := os.ReadFile(a.path)
	if err != nil {
		return false
	}
	for len(data) > 0 {
		parsed, _, _, rest, err := sshlib.ParseAuthorizedKey(data)
		if err != nil {
			// Skip a malformed/blank line and keep scanning the rest.
			if i := indexByte(data, '\n'); i >= 0 {
				data = data[i+1:]
				continue
			}
			break
		}
		if sshlib.KeysEqual(parsed, key) {
			return true
		}
		data = rest
	}
	return false
}

func indexByte(b []byte, c byte) int {
	for i, bb := range b {
		if bb == c {
			return i
		}
	}
	return -1
}
