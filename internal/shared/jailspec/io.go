package jailspec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Load reads a spec from disk.
func Load(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read jail spec: %w", err)
	}
	var s Spec
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("invalid jail spec: %w", err)
	}
	return &s, nil
}

// Save writes the spec atomically so a partially written file is never used.
func Save(path string, s *Spec) error {
	data, err := json.MarshalIndent(s, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ResultJSON encodes a result for the control pipe.
func ResultJSON(r Result) []byte {
	data, _ := json.Marshal(r)
	return append(data, '\n')
}
