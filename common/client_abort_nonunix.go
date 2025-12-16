//go:build !unix

package common

func isUnixClientGoneSyscallError(err error) bool {
	return false
}
