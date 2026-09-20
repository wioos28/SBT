//go:build linux

package internalhelper

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/wioos28/sbt/internal/platform/linux/ns"
)

// TestSpawnMappedProbe starts the nsprobe helper the way the CLI does (mapping
// written by the parent after a readiness handshake) and checks that the probe
// report comes back and the helper exits.
func TestSpawnMappedProbe(t *testing.T) {
	dir, err := os.MkdirTemp("", "sbt-probe-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	started, err := ns.SpawnMapped(ns.Options{
		Mode:     ModeProbe,
		ExtraEnv: []string{"SBT_PROBE_ROOT=" + dir},
		Stdout:   stdoutW,
	})
	if err != nil {
		t.Fatalf("SpawnMapped: %v", err)
	}
	// The parent end of the report pipe must be closed so the read below sees
	// EOF once the helper exits.
	_ = stdoutW.Close()

	out := make(chan []byte, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(stdoutR)
		out <- buf.Bytes()
	}()

	done := make(chan error, 1)
	go func() { done <- started.Cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("helper exit: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("SpawnMapped helper timed out")
	}

	select {
	case report := <-out:
		if !bytes.Contains(report, []byte("linux-namespaces")) {
			t.Fatalf("unexpected probe report: %s", report)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("probe report never reached EOF")
	}
}
