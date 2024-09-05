package util

import (
	"os/exec"
	"syscall"
)

func ShellOpen(path string) error {
	return exec.Command("cmd", "/C", "start", "explorer.exe", path).Start()
}

func ShellRun(name string, arg ...string) (*exec.Cmd, error) {
	cmd := exec.Command(name, arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} // Hide the window
	cmd.Stdout = GetLogger().GetWriter()
	cmd.Stderr = GetLogger().GetWriter()
	cmdErr := cmd.Start()
	if cmdErr != nil {
		return nil, cmdErr
	}

	return cmd, nil
}

func ShellRunOutput(name string, arg ...string) ([]byte, error) {
	cmd := exec.Command(name, arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} // Hide the window
	return cmd.Output()
}

func ShellOpenFileInFolder(path string) error {
	return exec.Command("cmd", "/C", "start", "explorer.exe", path).Start()
}
