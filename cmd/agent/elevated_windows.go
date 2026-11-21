package main

import "golang.org/x/sys/windows"

// isElevated 는 관리자 권한으로 실행 중인지 확인한다.
func isElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
