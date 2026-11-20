//go:build !unix

package steamcmd

import (
	"os/exec"
	"time"
)

func configureProcessGroup(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.WaitDelay = 2 * time.Second
}
