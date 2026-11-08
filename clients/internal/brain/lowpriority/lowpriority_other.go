//go:build !linux && !windows

package lowpriority

import "os/exec"

func prepare(*exec.Cmd) {}

func lower(int) {}
