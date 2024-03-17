//go:build !windows

package nodejs

import "syscall"

func getSysProcAttr() *syscall.SysProcAttr {
	return nil
}
