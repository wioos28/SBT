//go:build linux

package platform

import (
	"os"
	"testing"
)

// TestDetectReportsVerifiedCapabilities is the first place SBT checks that its
// isolation claims match the host's real behaviour. It fails (rather than skips)
// when the probe claims something it cannot back up.
func TestDetectReportsVerifiedCapabilities(t *testing.T) {
	caps := Detect()
	for _, f := range caps.Features {
		t.Logf("feature %-20s %-12s %s", f.Key, f.Level.Label(), f.Reason)
	}
	t.Logf("backend=%q probe_error=%q elevated=%v container=%v", caps.Backend, caps.ProbeError, caps.Elevated, caps.InContainer)
	for _, w := range caps.Warnings {
		t.Logf("warning: %s", w)
	}

	userNS := caps.Feature("user_namespace")
	if caps.ProbeError != "" {
		t.Fatalf("platform probe failed: %s", caps.ProbeError)
	}
	if userNS.Level == Unknown {
		t.Fatalf("user namespace capability was not verified: %s", userNS.Reason)
	}
	if userNS.Level != Full {
		t.Skipf("unprivileged user namespaces unavailable on this host: %s", userNS.Reason)
	}
	// A container runtime can refuse mount/network namespaces from outside
	// (EPERM before the kernel even consults SBT). That is an environment
	// restriction, not a probe defect: skip instead of failing.
	if f := caps.Feature("mount_namespace"); f.Level == Unavailable {
		t.Skipf("mount namespaces refused by this environment (container runtime?): %s", f.Reason)
	}
	// With a working user namespace SBT must be able to verify these.
	for _, key := range []string{"mount_namespace", "filesystem_jail", "capability_drop"} {
		if f := caps.Feature(key); f.Level != Full {
			t.Errorf("%s: expected Full, got %s (%s)", key, f.Level.Label(), f.Reason)
		}
	}
	if caps.Backend == "" {
		t.Errorf("backend must be reported when user+mount namespaces are available")
	}
	// The mount namespace must never be claimed without evidence.
	if f := caps.Feature("mount_namespace"); f.Reason == "" {
		t.Errorf("mount namespace level has no evidence recorded")
	}
	_ = os.Getpid()
}
