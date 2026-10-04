package shell

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/wioos28/sbt/internal/platform/linux/ns"
	"github.com/wioos28/sbt/internal/shared/helpermode"
	"github.com/wioos28/sbt/internal/shared/jailspec"
)

// startHelper launches the Linux jail helper in fresh user and pid namespaces,
// waits for its startup report, and returns the command once the jail is up.
//
// It returns an error in every case where the jail was not established: a
// helper that could not be spawned, one that never reported, and one that
// reported a failed setup all mean the same thing to the caller - there is no
// sandbox, so nothing may run.
func startHelper(spec *jailspec.Spec, specPath string, in, out *os.File) (*exec.Cmd, error) {
	controlR, controlW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("cannot create the sandbox control pipe: %w", err)
	}
	started, err := ns.SpawnMapped(ns.Options{
		Mode:     helpermode.ModeJail,
		SpecPath: specPath,
		Control:  controlW,
		Stdin:    in,
		Stdout:   out,
		Stderr:   os.Stderr,
	})
	_ = controlW.Close()
	if err != nil {
		_ = controlR.Close()
		return nil, fmt.Errorf("cannot start the sandbox helper: %w", err)
	}
	result, rerr := readControlResult(controlR)
	_ = controlR.Close()
	if rerr != nil {
		_ = started.Cmd.Process.Kill()
		_, _ = started.Cmd.Process.Wait()
		return nil, fmt.Errorf("sandbox did not report startup: %w", rerr)
	}
	if !result.OK {
		_, _ = started.Cmd.Process.Wait()
		reason := result.Reason
		if reason == "" {
			reason = "the sandbox refused to start"
		}
		return nil, fmt.Errorf("%s", reason)
	}
	return started.Cmd, nil
}

// readControlResult reads one newline terminated JSON report from the control
// pipe, bounded by controlReadTimeout so a helper that dies silently cannot
// hang the session.
func readControlResult(r *os.File) (jailspec.Result, error) {
	var res jailspec.Result
	_ = r.SetReadDeadline(time.Now().Add(controlReadTimeout))
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil {
		return res, err
	}
	if err := json.Unmarshal([]byte(line), &res); err != nil {
		return res, fmt.Errorf("invalid sandbox report: %w", err)
	}
	return res, nil
}
