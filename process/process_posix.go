// +build linux freebsd openbsd darwin

package process

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/f00stx/gopsutil/internal/common"
	"golang.org/x/sys/unix"
)

// POSIX
func getTerminalMap() (map[uint64]string, error) {
	ret := make(map[uint64]string)
	var termfiles []string

	d, err := os.Open("/dev")
	if err != nil {
		return nil, err
	}
	defer d.Close()

	devnames, err := d.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	for _, devname := range devnames {
		if strings.HasPrefix(devname, "/dev/tty") {
			termfiles = append(termfiles, "/dev/tty/"+devname)
		}
	}

	var ptsnames []string
	ptsd, err := os.Open("/dev/pts")
	if err != nil {
		ptsnames, _ = filepath.Glob("/dev/ttyp*")
		if ptsnames == nil {
			return nil, err
		}
	}
	defer ptsd.Close()

	if ptsnames == nil {
		defer ptsd.Close()
		ptsnames, err = ptsd.Readdirnames(-1)
		if err != nil {
			return nil, err
		}
		for _, ptsname := range ptsnames {
			termfiles = append(termfiles, "/dev/pts/"+ptsname)
		}
	} else {
		termfiles = ptsnames
	}

	for _, name := range termfiles {
		stat := unix.Stat_t{}
		if err = unix.Stat(name, &stat); err != nil {
			return nil, err
		}
		rdev := uint64(stat.Rdev)
		ret[rdev] = strings.Replace(name, "/dev", "", -1)
	}
	return ret, nil
}

func PidExistsWithContext(ctx context.Context, pid int32) (bool, error) {
	if pid <= 0 {
		return false, fmt.Errorf("invalid pid %v", pid)
	}
	proc, err := os.FindProcess(int(pid))
	if err != nil {
		return false, err
	}

	if _, err := os.Stat(common.HostProc()); err == nil { //Means that proc filesystem exist
		// Checking PID existence based on existence of /<HOST_PROC>/proc/<PID> folder
		// This covers the case when running inside container with a different process namespace (by default)

		_, err := os.Stat(common.HostProc(strconv.Itoa(int(pid))))
		if os.IsNotExist(err) {
			return false, nil
		}
		return err == nil, err
	}

	//'/proc' filesystem is not exist, checking of PID existence is done via signalling the process
	//Make sense only if we run in the same process namespace
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true, nil
	}
	if err.Error() == "os: process already finished" {
		return false, nil
	}
	errno, ok := err.(syscall.Errno)
	if !ok {
		return false, err
	}
	switch errno {
	case syscall.ESRCH:
		return false, nil
	case syscall.EPERM:
		return true, nil
	}

	return false, err
}

func (p *Process) SendSignalWithContext(ctx context.Context, sig syscall.Signal) error {
	process, err := os.FindProcess(int(p.Pid))
	if err != nil {
		return err
	}

	err = process.Signal(sig)
	if err != nil {
		return err
	}

	return nil
}

func (p *Process) SuspendWithContext(ctx context.Context) error {
	return p.SendSignalWithContext(ctx, unix.SIGSTOP)
}

func (p *Process) ResumeWithContext(ctx context.Context) error {
	return p.SendSignalWithContext(ctx, unix.SIGCONT)
}

func (p *Process) TerminateWithContext(ctx context.Context) error {
	return p.SendSignalWithContext(ctx, unix.SIGTERM)
}

func (p *Process) KillWithContext(ctx context.Context) error {
	return p.SendSignalWithContext(ctx, unix.SIGKILL)
}

func (p *Process) UsernameWithContext(ctx context.Context) (string, error) {
	uids, err := p.UidsWithContext(ctx)
	if err != nil {
		return "", err
	}
	if len(uids) > 0 {
		u, err := user.LookupId(strconv.Itoa(int(uids[0])))
		if err != nil {
			return "", err
		}
		return u.Username, nil
	}
	return "", nil
}
