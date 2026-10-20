package system

import (
	"errors"
	"os"
	"syscall"
)

func signals() []os.Signal                               { return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP} }
func set_sys_proc_attr_for_daemon() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }
func terminate_process(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
func is_process_running(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
