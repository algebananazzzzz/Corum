//go:build unix

package lockfile

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestTryAcquireRejectsCrossProcessContention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "course.lock")
	command := exec.Command(os.Args[0], "-test.run=^TestLockHelperProcess$")
	command.Env = append(os.Environ(), "CORUM_LOCK_HELPER="+path)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "locked" {
		t.Fatalf("helper did not acquire lock: %q, %v", scanner.Text(), scanner.Err())
	}
	if lock, err := TryAcquire(path); !errors.Is(err, ErrLocked) {
		if lock != nil {
			_ = lock.Close()
		}
		t.Fatalf("TryAcquire() error = %v, want ErrLocked", err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	lock, err := TryAcquire(path)
	if err != nil {
		t.Fatalf("TryAcquire() after process exit: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLockHelperProcess(t *testing.T) {
	path := os.Getenv("CORUM_LOCK_HELPER")
	if path == "" {
		return
	}
	lock, err := Acquire(path)
	if err != nil {
		os.Exit(91)
	}
	defer lock.Close()
	_, _ = os.Stdout.WriteString("locked\n")
	_, _ = bufio.NewReader(os.Stdin).ReadByte()
}
