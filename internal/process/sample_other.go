//go:build !linux

package process

func alive(pid int) bool { return pid > 0 }

func rss(pid int) int64 { return 0 }
