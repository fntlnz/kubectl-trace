// +build !windows

package pty

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

func Start(c *exec.Cmd) (*os.File, error) {
	return pty.StartWithAttrs(c, nil, &syscall.SysProcAttr{})
}
