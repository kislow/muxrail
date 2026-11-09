//go:build e2e && darwin

package e2e

import (
	"os"
	"syscall"
	"unsafe"

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
		if err := unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0); err != nil {
			return err
		}
		if err := unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0); err != nil {
			return err
		}
		var buf [128]byte
		if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.TIOCPTYGNAME, uintptr(unsafe.Pointer(&buf[0]))); errno != 0 {
			return errno
		}
		name = unix.ByteSliceToString(buf[:])
		return nil
	})
	if err != nil {
		_ = master.Close()
		return nil, "", err
	}
	return master, name, nil
}
