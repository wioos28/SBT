package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Import registers a local GGUF file into the store without executing it.
func Import(path string) (Info, error) {
	src, err := ParseSource(path)
	if err != nil {
		return Info{}, err
	}
	if src.Kind != KindLocal {
		return Info{}, fmt.Errorf("import expects a local .gguf path; use install for %s", src.Kind)
	}
	f, err := os.Open(src.Raw)
	if err != nil {
		return Info{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return Info{}, err
	}
	name := strings.TrimSuffix(filepath.Base(src.Raw), filepath.Ext(src.Raw))
	reg := loadRegistry()
	info := Info{
		Name: name, Source: "local:" + src.Raw, Path: src.Raw,
		SHA256: hex.EncodeToString(h.Sum(nil)), Size: n,
	}
	reg[name] = info
	if err := saveRegistry(reg); err != nil {
		return Info{}, err
	}
	return info, nil
}

// Remove unregisters a model. It only ever deletes files inside the store that
// SBT placed there, never the original user file and never a host path.
func Remove(name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid model name %q", name)
	}
	reg := loadRegistry()
	info, ok := reg[name]
	if !ok {
		return fmt.Errorf("model %q is not registered", name)
	}
	if info.Path != "" && strings.HasPrefix(filepath.Clean(info.Path), filepath.Clean(Root())+string(filepath.Separator)) {
		_ = os.Remove(info.Path)
	}
	delete(reg, name)
	return saveRegistry(reg)
}

// Use marks a registered model as active, verifying its hash first.
func Use(name string) (Info, error) {
	reg := loadRegistry()
	info, ok := reg[name]
	if !ok {
		return Info{}, fmt.Errorf("model %q is not registered", name)
	}
	if info.Path != "" {
		if err := verify(info); err != nil {
			return Info{}, err
		}
		info.Verified = "hash matches"
	}
	for k, m := range reg {
		m.Active = k == name
		reg[k] = m
	}
	info.Active = true
	reg[name] = info
	if err := saveRegistry(reg); err != nil {
		return Info{}, err
	}
	return info, nil
}

func verify(info Info) error {
	if info.Path == "" || info.SHA256 == "" {
		return nil
	}
	f, err := os.Open(info.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != info.SHA256 {
		return fmt.Errorf("model integrity check failed: expected SHA-256 %s, got %s", info.SHA256, got)
	}
	return nil
}
