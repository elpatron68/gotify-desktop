package update

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// ParseSHA256SUMS parses GNU sha256sum output ("<hash>  <filename>" or "<hash> *<filename>").
func ParseSHA256SUMS(r io.Reader) (map[string]string, error) {
	out := make(map[string]string)
	sc := bufio.NewScanner(r)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		hash, name, ok := splitChecksumLine(line)
		if !ok {
			return nil, fmt.Errorf("SHA256SUMS line %d: invalid format", lineNo)
		}
		if len(hash) != 64 {
			return nil, fmt.Errorf("SHA256SUMS line %d: invalid hash length", lineNo)
		}
		if _, err := hex.DecodeString(hash); err != nil {
			return nil, fmt.Errorf("SHA256SUMS line %d: invalid hash", lineNo)
		}
		out[name] = strings.ToLower(hash)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func splitChecksumLine(line string) (hash, name string, ok bool) {
	// GNU text mode: "hash  name" (two spaces). Binary mode: "hash *name".
	if len(line) < 66 {
		return "", "", false
	}
	hash = line[:64]
	rest := line[64:]
	switch {
	case strings.HasPrefix(rest, "  "):
		name = strings.TrimSpace(rest[2:])
	case strings.HasPrefix(rest, " *"):
		name = strings.TrimSpace(rest[2:])
	default:
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return "", "", false
		}
		hash = fields[0]
		name = fields[1]
		name = strings.TrimPrefix(name, "*")
	}
	if hash == "" || name == "" {
		return "", "", false
	}
	return hash, name, true
}

// ExpectedHash looks up filename in a parsed SHA256SUMS map.
func ExpectedHash(sums map[string]string, filename string) (string, error) {
	h, ok := sums[filename]
	if !ok {
		// Also try basename if a path was stored.
		base := filename
		if i := strings.LastIndexAny(filename, "/\\"); i >= 0 {
			base = filename[i+1:]
		}
		h, ok = sums[base]
	}
	if !ok {
		return "", fmt.Errorf("no checksum for %q", filename)
	}
	return h, nil
}

// FileSHA256 returns the lowercase hex SHA-256 of a file.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyFile checks that path matches expected lowercase hex SHA-256.
func VerifyFile(path, expectedHex string) error {
	got, err := FileSHA256(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, expectedHex) {
		return fmt.Errorf("checksum mismatch for %s: got %s want %s", path, got, expectedHex)
	}
	return nil
}
