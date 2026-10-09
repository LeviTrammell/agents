package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The host key dir is a mounted volume the process can't chmod, so loading
// must never touch the directory's mode, and must reuse a key it generated.
func TestLoadHostSignersGeneratesOnceWithoutChmod(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o775); err != nil {
		t.Fatal(err)
	}
	first, err := loadHostSigners(dir)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o775 {
		t.Errorf("directory mode changed to %o", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(dir, "ssh_host_ed25519_key.pub")); err != nil {
		t.Errorf("public key not written: %v", err)
	}
	second, err := loadHostSigners(dir)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if string(first[0].PublicKey().Marshal()) != string(second[0].PublicKey().Marshal()) {
		t.Error("host key changed between loads")
	}
}
