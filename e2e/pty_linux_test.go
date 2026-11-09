//go:build e2e && linux

package e2e

import (
	"os"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// openPTY returns the master side and the path of the slave device.
func openPTY() (*os.File, string, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, "", err
	}
	var name string
	err = control(master, func(fd int) error {
		if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
			return err
		}
		n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
		name = "/dev/pts/" + strconv.Itoa(n)
		return err
	})
	if err != nil {
		_ = master.Close()
		return nil, "", err
	}
	return master, name, nil
}
