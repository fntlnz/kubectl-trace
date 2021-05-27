package pty

import (
	"os"
	"os/exec"
)

// Note that because this is not defined in the upstream package,
// we must stub it here so that builds won't fail for the trace-runner on windows
// this is fine, because the trace-runner cannot possibly work on windows anyways.
func Start(c *exec.Cmd) (*os.File, error) {
	return nil, nil
}
