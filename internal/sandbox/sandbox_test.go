package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegisterLoadList(t *testing.T) {
	t.Setenv("SBT_HOME", t.TempDir())
	if _, err := Register("ai-coder"); err != nil {
		t.Fatalf("register: %v", err)
	}
	info, err := Load("ai-coder")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if info.Profile.Language != "en-US" || info.Profile.AIModel != "qwen3-0.6b" {
		t.Fatalf("bad default profile: %+v", info.Profile)
	}
	list, err := List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", list, err)
	}
}

func TestInvalidSandboxNamesRejected(t *testing.T) {
	t.Setenv("SBT_HOME", t.TempDir())
	for _, bad := range []string{"", ".", "..", "../evil", "a/b", "semi;colon", "has space"} {
		if _, err := Register(bad); err == nil {
			t.Fatalf("Register(%q) should fail", bad)
		}
	}
}

func TestDestroyStaysInsideManagedState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SBT_HOME", home)

	// A sentinel file OUTSIDE the managed tree must survive destruction.
	sentinel := filepath.Join(home, "precious.txt")
	if err := os.WriteFile(sentinel, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A fake home layout that must never be touched.
	fakeHome := filepath.Join(home, "fakehome", ".ssh")
	if err := os.MkdirAll(fakeHome, 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := Register("doomed"); err != nil {
		t.Fatal(err)
	}
	dir, _ := SandboxDir("doomed")
	if err := os.WriteFile(filepath.Join(dir, "state.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	rep, err := Destroy("doomed", true)
	if err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if !rep.Steps.Runtime || !rep.Steps.HostKept {
		t.Fatalf("bad report: %+v", rep.Steps)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("managed sandbox dir should be gone")
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatal("file outside managed state was touched")
	}
	if _, err := os.Stat(fakeHome); err != nil {
		t.Fatal("fake home was touched")
	}
}

func TestFreezeAndStop(t *testing.T) {
	t.Setenv("SBT_HOME", t.TempDir())
	if _, err := Register("frosty"); err != nil {
		t.Fatal(err)
	}
	if err := Freeze("frosty"); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	info, _ := Load("frosty")
	if info.Status != StatusFrozen {
		t.Fatalf("status = %s", info.Status)
	}
	if err := Stop("frosty"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	info, _ = Load("frosty")
	if info.Status != StatusStopped {
		t.Fatalf("status = %s", info.Status)
	}
}
