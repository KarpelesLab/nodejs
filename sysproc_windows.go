//go:build windows

package nodejs

import "syscall"

const CREATE_NO_WINDOW = 0x08000000

func getSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: CREATE_NO_WINDOW,
	}
}
