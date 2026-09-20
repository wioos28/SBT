//go:build linux

// Package sysnet contains the small amount of raw netlink code SBT needs to
// raise the loopback interface inside a private network namespace. SBT does not
// shell out to `ip` so the sandbox never depends on host tooling for its own
// network policy.
package sysnet

import (
	"encoding/binary"
	"fmt"
	"syscall"
)

const (
	rtmNewLink = 16
	iflaIfname = 3
	iffUp      = 0x1
)

// BringUpLoopback enables the loopback interface in the current network
// namespace. It requires CAP_NET_ADMIN in that namespace, which the sandbox
// helper holds until capabilities are dropped.
func BringUpLoopback() error {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW, syscall.NETLINK_ROUTE)
	if err != nil {
		return fmt.Errorf("netlink socket: %w", err)
	}
	defer syscall.Close(fd)

	if err := syscall.Bind(fd, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}); err != nil {
		return fmt.Errorf("netlink bind: %w", err)
	}

	const seq = 1
	msg := buildNewLinkUp("lo", seq)
	if err := syscall.Sendto(fd, msg, 0, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}); err != nil {
		return fmt.Errorf("netlink send RTM_NEWLINK: %w", err)
	}
	return waitAck(fd)
}

func buildNewLinkUp(name string, seq uint32) []byte {
	nameBytes := append([]byte(name), 0)

	// struct ifinfomsg
	info := make([]byte, 16)
	info[0] = syscall.AF_UNSPEC
	binary.LittleEndian.PutUint16(info[2:], 0) // type
	binary.LittleEndian.PutUint32(info[4:], 0) // index (resolved by name)
	binary.LittleEndian.PutUint32(info[8:], iffUp)
	binary.LittleEndian.PutUint32(info[12:], iffUp)

	// struct rtattr { len, type, data }
	attrLen := 4 + len(nameBytes)
	padded := (attrLen + 3) &^ 3
	attr := make([]byte, padded)
	binary.LittleEndian.PutUint16(attr[0:], uint16(attrLen))
	binary.LittleEndian.PutUint16(attr[2:], iflaIfname)
	copy(attr[4:], nameBytes)

	body := append(info, attr...)
	total := 16 + len(body)
	head := make([]byte, 16)
	binary.LittleEndian.PutUint32(head[0:], uint32(total))
	binary.LittleEndian.PutUint16(head[4:], rtmNewLink)
	binary.LittleEndian.PutUint16(head[6:], syscall.NLM_F_REQUEST|syscall.NLM_F_ACK)
	binary.LittleEndian.PutUint32(head[8:], seq)
	return append(head, body...)
}

func waitAck(fd int) error {
	buf := make([]byte, 4096)
	n, _, err := syscall.Recvfrom(fd, buf, 0)
	if err != nil {
		return fmt.Errorf("netlink recv ack: %w", err)
	}
	if n < 20 {
		return nil
	}
	if typ := binary.LittleEndian.Uint16(buf[4:]); typ == syscall.NLMSG_ERROR {
		code := int32(binary.LittleEndian.Uint32(buf[16:]))
		if code != 0 {
			return syscall.Errno(-code)
		}
	}
	return nil
}
