package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func TestGroupLockExclusiveAndReleased(t *testing.T) {
	path := filepath.Join(t.TempDir(), "g.lock")
	f, err := acquireGroupLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if !groupLockHeld(path) {
		t.Fatal("held lock not reported as held")
	}
	if _, err := acquireGroupLock(path); err != errGroupInUse {
		t.Fatalf("second acquire: %v", err)
	}
	releaseGroupLock(f, path)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lock file not removed: %v", err)
	}
	if groupLockHeld(path) {
		t.Fatal("released lock still reported as held")
	}
}

// A lock file left behind (e.g. delete failed at exit) must not lock the group
// when nobody holds the OS lock, even if the recorded PID is alive.
func TestLeftoverLockFileIsFree(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "g.lock")
	// New-format file whose owner is "alive" (this test process) but holds no OS lock.
	os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"+groupLockMarker+"\n"), 0644)
	if groupLockHeld(path) {
		t.Fatal("new-format leftover treated as held")
	}
	// Legacy file pointing at an exited process.
	cmd := exec.Command("cmd", "/c", "exit")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte(strconv.Itoa(cmd.Process.Pid)), 0644)
	if groupLockHeld(path) {
		t.Fatal("legacy leftover of exited process treated as held")
	}
	// Legacy file pointing at a live process that is not ChannelNotes (PID reuse).
	sleeper := exec.Command("cmd", "/c", "ping -n 5 127.0.0.1 >nul")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	defer sleeper.Process.Kill()
	os.WriteFile(path, []byte(strconv.Itoa(sleeper.Process.Pid)), 0644)
	if groupLockHeld(path) {
		t.Fatal("legacy leftover with recycled PID treated as held")
	}
}

// Even while another process keeps the lock file open (antivirus, indexer),
// the group becomes free once the owner releases it.
func TestReleaseWithForeignOpenHandle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "g.lock")
	f, err := acquireGroupLock(path)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path) // Go opens without FILE_SHARE_DELETE
	if err != nil {
		t.Fatal(err)
	}
	releaseGroupLock(f, path)
	if groupLockHeld(path) {
		t.Fatal("group still held after release")
	}
	reader.Close()
}

func TestLockFreedWhenOwnerProcessKilled(t *testing.T) {
	if p := os.Getenv("CN_LOCK_HOLDER"); p != "" {
		f, err := acquireGroupLock(p)
		if err != nil {
			os.Exit(2)
		}
		os.Stdout.WriteString("held\n")
		time.Sleep(time.Minute)
		runtime.KeepAlive(f)
	}
	path := filepath.Join(t.TempDir(), "g.lock")
	cmd := exec.Command(os.Args[0], "-test.run=TestLockFreedWhenOwnerProcessKilled")
	cmd.Env = append(os.Environ(), "CN_LOCK_HOLDER="+path)
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 5)
	if _, err := out.Read(buf); err != nil || string(buf[:4]) != "held" {
		t.Fatalf("holder did not start: %q %v", buf, err)
	}
	if !groupLockHeld(path) {
		t.Fatal("lock of live process not reported")
	}
	cmd.Process.Kill()
	cmd.Wait()
	if _, err := os.Stat(path); err != nil {
		t.Fatal("expected leftover file after kill")
	}
	if groupLockHeld(path) {
		t.Fatal("lock still held after owner was killed")
	}
}
