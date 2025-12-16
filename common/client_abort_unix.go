//go:build unix

package common

import (
	"errors"
	"syscall"
)

func isUnixClientGoneSyscallError(err error) bool {
	// These unwrap through *net.OpError / *os.SyscallError in most cases.
	return errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET)
}
