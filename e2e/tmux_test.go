//go:build e2e

// Package e2e drives the sidebar and the tab bar through a real tmux client: a throwaway tmux server on its own
// socket and TMPDIR, and a pty-attached client that receives raw SGR mouse events.
//
//	MUXRAIL_BIN      the muxrail binary the scripts and the tab bar run (default: muxrail on PATH)
//	MUXRAIL_SCRIPTS  directory holding sidebar.sh                       (default: ../scripts)
//	MUXRAIL_TAB_CMD  command printing a window's tab icon for a window id and an optional `plain`
//	                 (default: "$MUXRAIL_BIN tab", or "muxrail tab")
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// server is a private tmux server. Every command goes through tm, which always passes -L, so the author's
// default server is never addressed.
type server struct {
	t      *testing.T
	tmux   string
	socket string
	dir    string
	env    []string
}

func newServer(t *testing.T, name string) *server {
	t.Helper()
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		tmux = "/opt/homebrew/bin/tmux"
		if _, statErr := os.Stat(tmux); statErr != nil {
			t.Fatalf("tmux not found on PATH or at %s", tmux)
		}
	}
	dir, err := os.MkdirTemp("", "mxe")
	if err != nil {
		t.Fatal(err)
	}
	s := &server{t: t, tmux: tmux, socket: fmt.Sprintf("muxrail-e2e-%s-%d", name, os.Getpid()), dir: dir, env: privateEnv(dir)}
	t.Cleanup(func() {
		s.tm("kill-server")
		_ = os.RemoveAll(dir)
	})
	return s
}

// privateEnv keeps an enclosing tmux session from leaking in: TMUX would make the client nest or pick another server.
func privateEnv(dir string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "TMUX=") && !strings.HasPrefix(kv, "TMUX_PANE=") {
			env = append(env, kv)
		}
	}
	return append(env, "TMPDIR="+dir+"/", "XDG_STATE_HOME="+dir+"/state")
}

func (s *server) command(args ...string) *exec.Cmd {
	cmd := exec.Command(s.tmux, append([]string{"-L", s.socket}, args...)...)
	cmd.Dir, cmd.Env = s.dir, s.env
	return cmd
}

// tm runs one tmux command and returns its trimmed stdout; a failing command yields whatever it printed.
func (s *server) tm(args ...string) string {
	out, _ := s.command(args...).Output()
	return strings.TrimSpace(string(out))
}

// hook runs `muxrail hook args...` the way an agent's hook would: inside the given pane of this server, without a terminal.
func (s *server) hook(pane string, args ...string) {
	s.t.Helper()
	cmd := exec.Command(os.Getenv("MUXRAIL_BIN"), append([]string{"hook"}, args...)...)
	tmuxEnv := s.tm("display-message", "-p", "#{socket_path},#{pid},0")
	cmd.Dir, cmd.Env = s.dir, append(s.env, "TMUX="+tmuxEnv, "TMUX_PANE="+pane)
	if out, err := cmd.CombinedOutput(); err != nil || len(out) != 0 {
		s.t.Fatalf("muxrail hook %v: err=%v output=%q, want silent success", args, err, out)
	}
}

// start creates the detached session "t" with its first window, reading conf as the server's config file.
func (s *server) start(conf string, cols, rows int, firstWindow string) {
	s.t.Helper()
	path := s.dir + "/t.conf"
	if err := os.WriteFile(path, []byte(conf), 0o600); err != nil {
		s.t.Fatal(err)
	}
	args := []string{"-f", path, "new-session", "-d", "-s", "t", "-x", fmt.Sprint(cols), "-y", fmt.Sprint(rows), "-n", firstWindow}
	if out, err := s.command(args...).CombinedOutput(); err != nil {
		s.t.Fatalf("new-session: %v: %s", err, out)
	}
}

// client is a tmux client attached through a pty; a goroutine keeps reading so tmux never blocks on a full pty.
type client struct {
	master *os.File
	cmd    *exec.Cmd
	mu     sync.Mutex
	buf    []byte
}

func (s *server) attach(rows, cols uint16) *client {
	s.t.Helper()
	master, slaveName, err := openPTY()
	if err != nil {
		s.t.Fatalf("open pty: %v", err)
	}
	slave, err := os.OpenFile(slaveName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		s.t.Fatalf("open pty slave: %v", err)
	}
	defer slave.Close()
	err = control(slave, func(fd int) error {
		return unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, &unix.Winsize{Row: rows, Col: cols})
	})
	if err != nil {
		s.t.Fatalf("set window size: %v", err)
	}

	cmd := s.command("attach", "-t", "t")
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		s.t.Fatalf("attach: %v", err)
	}
	c := &client{master: master, cmd: cmd}
	go c.drain()
	s.t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = master.Close()
	})
	return c
}

func (c *client) drain() {
	chunk := make([]byte, 65536)
	for {
		n, err := c.master.Read(chunk)
		c.mu.Lock()
		c.buf = append(c.buf, chunk[:n]...)
		c.mu.Unlock()
		if err != nil {
			return
		}
	}
}

// output is everything the client has drawn so far, escape sequences included.
func (c *client) output() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.ToValidUTF8(string(c.buf), "")
}

// send types raw bytes into the client and gives tmux and the sidebar a moment to react.
func (c *client) send(t *testing.T, data string, wait time.Duration) {
	t.Helper()
	if _, err := c.master.WriteString(data); err != nil {
		t.Fatalf("write to client: %v", err)
	}
	time.Sleep(wait)
}

func check(t *testing.T, name string, ok bool, detail string) {
	t.Helper()
	if ok {
		t.Logf("PASS  %s", name)
		return
	}
	t.Errorf("FAIL  %s   %s", name, detail)
}

// expect polls until get() equals want: the sidebar redraws a moment after the input, and the first start is slow.
func expect[T any](t *testing.T, name string, get func() T, want T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	got := get()
	for !reflect.DeepEqual(got, want) && time.Now().Before(deadline) {
		time.Sleep(150 * time.Millisecond)
		got = get()
	}
	check(t, name, reflect.DeepEqual(got, want), fmt.Sprintf("want %#v, got %#v", want, got))
}

// control runs fn with the raw descriptor without switching the file to blocking mode.
func control(f *os.File, fn func(fd int) error) error {
	rc, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var inner error
	if err := rc.Control(func(fd uintptr) { inner = fn(int(fd)) }); err != nil {
		return err
	}
	return inner
}
