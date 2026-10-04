//go:build linux

package security

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type mount struct {
	target   string
	writable bool
}

func readStatus(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		i := strings.IndexByte(line, ':')
		if i < 0 {
			continue
		}
		out[strings.TrimSpace(line[:i])] = strings.TrimSpace(line[i+1:])
	}
	return out
}

func readMounts(path string) []mount {
	var out []mount
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 6 {
			continue
		}
		ro := false
		for _, o := range strings.Split(fields[5], ",") {
			if o == "ro" {
				ro = true
			}
		}
		out = append(out, mount{target: fields[4], writable: !ro})
	}
	return out
}

func hostSocketExposed(mounts []mount) (string, bool) {
	for _, m := range mounts {
		for _, marker := range []string{"docker.sock", "containerd.sock", "podman.sock", "crio.sock"} {
			if strings.Contains(m.target, marker) {
				return m.target, true
			}
		}
	}
	return "", false
}

// isHostTree reports whether a path is a top-level host tree that should not be
// writable inside a sandbox.
func isHostTree(p string) bool {
	for _, tree := range []string{"/usr", "/etc", "/bin", "/sbin", "/lib", "/lib64", "/var", "/boot", "/root", "/home", "/opt", "/srv"} {
		if p == tree || strings.HasPrefix(p, tree+"/") {
			return true
		}
	}
	return false
}

func within(path, base string) bool {
	if base == "" {
		return false
	}
	return path == base || strings.HasPrefix(path, strings.TrimRight(base, "/")+"/")
}

func nonLoopbackInterfaces(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		i := strings.IndexByte(line, ':')
		if i <= 0 {
			continue
		}
		name := strings.TrimSpace(line[:i])
		if name == "lo" || name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

// readLimits returns a map of limit name to "is unlimited".
func readLimits(path string) (map[string]bool, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	out := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		i := strings.IndexByte(line, '(')
		if i < 0 {
			continue
		}
		out[strings.TrimSpace(line[:i])] = strings.Contains(line, "unlimited")
	}
	return out, len(out) > 0
}

func countProc(procRoot string) (int, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err == nil {
			n++
		}
	}
	return n, nil
}

// unsafeEnv returns environment variable names that look like credentials.
func unsafeEnv(env []string) []string {
	var out []string
	for _, kv := range env {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		name := kv[:i]
		lower := strings.ToLower(name)
		for _, marker := range []string{
			"ssh_auth_sock", "aws_", "azure_", "gcp_", "google_", "token",
			"secret", "password", "credential", "api_key", "private_key",
			"kubeconfig", "docker_host",
		} {
			if strings.Contains(lower, marker) {
				out = append(out, name)
				break
			}
		}
	}
	return out
}
