//go:build linux

package platform

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/wioos28/sbt/internal/platform/linux/ns"
	"github.com/wioos28/sbt/internal/shared/helpermode"
)

// runHelperProbe executes one of SBT's hidden helper modes inside a fresh user
// namespace and returns its stdout. The user namespace is created here (in the
// parent) because the kernel requires the uid/gid mapping to be written by a
// process that is still in the original user namespace.
func runHelperProbe(self, mode string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ProbeTimeout)
	defer cancel()

	// The probe builds throw-away mount points under this directory. The parent
	// owns it so that cleanup never runs inside the mount namespace (which would
	// recurse into the mounted procfs/tmpfs instead of the directory).
	probeRoot, perr := os.MkdirTemp("", "sbt-probe-")
	if perr != nil {
		return nil, fmt.Errorf("cannot create probe directory: %w", perr)
	}
	defer os.RemoveAll(probeRoot)

	cmd := exec.CommandContext(ctx, self)
	cmd.Env = append(os.Environ(), helpermode.ModeEnv+"="+mode, "SBT_PROBE_ROOT="+probeRoot)
	cmd.Stderr = os.Stderr
	var out bytes.Buffer
	cmd.Stdout = &out

	// The probe helper is started the same way as a sandbox helper: a user and pid
	// namespace created by this (trusted) parent process.
	cmd.SysProcAttr = ns.SysProcAttr()
	err := cmd.Run()
	if ctx.Err() != nil {
		return out.Bytes(), fmt.Errorf("probe timed out after %s", ProbeTimeout)
	}
	return out.Bytes(), err
}

func readSysctl(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}
