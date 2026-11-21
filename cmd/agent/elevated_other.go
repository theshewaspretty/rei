//go:build !windows

package main

import "os"

// isElevated 는 root 로 실행 중인지 확인한다.
func isElevated() bool {
	return os.Geteuid() == 0
}
