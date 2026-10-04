package filesystem

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// SHA-256 is used by SBT for INTEGRITY ONLY. It is not encryption and it is not
// a confidentiality mechanism: it answers "is this the same content as before",
// never "is this secret". Do not describe it as encryption in output.

// HashBytes returns the hex SHA-256 of data.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// HashFile returns the hex SHA-256 of a file's contents.
func HashFile(path string) (string, error) {
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

// IntegrityResult is the comparison of an expected and a current hash.
type IntegrityResult struct {
	Expected string
	Current  string
	Match    bool
}

// String renders the integrity verdict with the correct wording.
func (r IntegrityResult) String() string {
	status := "INTEGRITY OK"
	if !r.Match {
		status = "INTEGRITY FAILED"
	}
	return fmt.Sprintf("Expected SHA-256: %s\nCurrent  SHA-256: %s\n%s", r.Expected, r.Current, status)
}

// VerifyFile compares a file's current hash with an expected value.
func VerifyFile(path, expected string) (IntegrityResult, error) {
	cur, err := HashFile(path)
	if err != nil {
		return IntegrityResult{}, err
	}
	return IntegrityResult{Expected: expected, Current: cur, Match: expected != "" && cur == expected}, nil
}
