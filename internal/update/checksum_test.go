package update

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAndVerifySHA256SUMS(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("hello update")
	name := "gotify-desktop-linux-amd64"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	hexSum := hex.EncodeToString(sum[:])

	sumsText := hexSum + "  " + name + "\n"
	sums, err := ParseSHA256SUMS(strings.NewReader(sumsText))
	if err != nil {
		t.Fatal(err)
	}
	want, err := ExpectedHash(sums, name)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyFile(path, want); err != nil {
		t.Fatal(err)
	}
	if err := VerifyFile(path, strings.Repeat("0", 64)); err == nil {
		t.Fatal("expected mismatch")
	}
	if _, err := ExpectedHash(sums, "missing-file"); err == nil {
		t.Fatal("expected missing checksum")
	}
}

func TestParseSHA256SUMSBinaryMode(t *testing.T) {
	line := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789 *file.bin\n"
	sums, err := ParseSHA256SUMS(strings.NewReader(line))
	if err != nil {
		t.Fatal(err)
	}
	if sums["file.bin"] != "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789" {
		t.Fatalf("%v", sums)
	}
}
