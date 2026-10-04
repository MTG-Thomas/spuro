//go:build windows

package process

import "os/exec"

func isolate(cmd *exec.Cmd) {}
