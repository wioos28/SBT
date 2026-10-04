package process

import (
	"os"
	"testing"
)

func TestRegisterListStop(t *testing.T) {
	Unregister(os.Getpid())
	Unregister(424242)
	Register(Entry{PID: os.Getpid(), Name: "self", Kind: KindSandbox, Sandbox: "x"})
	list := List()
	found := false
	for _, i := range list {
		if i.PID == os.Getpid() {
			found = true
		}
	}
	if !found {
		t.Fatal("registered process missing from list")
	}
	Unregister(os.Getpid())
}

func TestStopRefusesUnknownPID(t *testing.T) {
	if err := Stop(424242); err == nil {
		t.Fatal("stopping an untracked pid must be refused")
	}
}
