package main

// Group lock files.
//
// The previous implementation decided whether a group was in use by reading
// the PID stored in locks/<gid>.lock and calling OpenProcess on it. That is
// unreliable:
//   - OpenProcess also succeeds for a process that has already exited while
//     any other process (WebView2 child processes, antivirus, shell, ...)
//     still holds a handle to it, so a dead owner looked alive.
//   - PIDs are recycled quickly, so an unrelated process could inherit the
//     PID written in a stale lock file.
//   - os.Remove at exit can fail with a sharing violation while another
//     process (antivirus scanning the just-written file, another instance
//     calling LockedGroups) has the file open, leaving the stale file behind.
//
// The lock is now an OS byte-range lock (LockFileEx) held on an open handle
// for as long as the group is in use. Windows releases it automatically when
// the handle is closed or the process dies for any reason, so a leftover file
// no longer means "locked". The PID is still written for diagnostics and for
// compatibility with older builds, which only understand the PID.

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	win "golang.org/x/sys/windows"
)

var errGroupInUse = errors.New("다른 창에서 사용 중인 그룹입니다")

// The locked byte lies far beyond the PID text so that other processes can
// still read the file contents while the lock is held.
const (
	groupLockOffsetHigh = 1
	groupLockMarker     = "channelnotes-oslock"
)

func openGroupLockHandle(path string) (*os.File, error) {
	p, err := win.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for i := 0; i < 10; i++ {
		h, err := win.CreateFile(p, win.GENERIC_READ|win.GENERIC_WRITE,
			win.FILE_SHARE_READ|win.FILE_SHARE_WRITE|win.FILE_SHARE_DELETE,
			nil, win.OPEN_ALWAYS, win.FILE_ATTRIBUTE_NORMAL, 0)
		if err == nil {
			return os.NewFile(uintptr(h), path), nil
		}
		lastErr = err
		// ERROR_ACCESS_DENIED is returned while the file is pending deletion
		// (an owner is releasing it right now); sharing violations come from
		// short-lived scanners. Both clear up quickly.
		if !errors.Is(err, win.ERROR_ACCESS_DENIED) && !errors.Is(err, win.ERROR_SHARING_VIOLATION) {
			return nil, err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, lastErr
}

func tryOSLock(f *os.File) (bool, error) {
	ol := new(win.Overlapped)
	ol.OffsetHigh = groupLockOffsetHigh
	err := win.LockFileEx(win.Handle(f.Fd()), win.LOCKFILE_EXCLUSIVE_LOCK|win.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, win.ERROR_LOCK_VIOLATION) || errors.Is(err, win.ERROR_IO_PENDING) {
		return false, nil
	}
	return false, err
}

// readLockOwner returns the PID written in the lock file and whether the file
// was written by a build that uses OS locks.
func readLockOwner(f *os.File) (pid int, osLocked bool) {
	buf := make([]byte, 256)
	n, _ := f.ReadAt(buf, 0)
	fields := strings.Fields(string(buf[:n]))
	if len(fields) == 0 {
		return 0, false
	}
	pid, _ = strconv.Atoi(fields[0])
	for _, s := range fields[1:] {
		if s == groupLockMarker {
			osLocked = true
		}
	}
	return pid, osLocked
}

// legacyOwnerAlive is only consulted for lock files written by older builds
// (PID only, no OS lock). It requires the process to still be running and to
// be a ChannelNotes executable, which rules out exited-but-referenced
// processes and recycled PIDs.
func legacyOwnerAlive(pid int) bool {
	if pid <= 0 || pid == os.Getpid() {
		return false
	}
	h, err := win.OpenProcess(win.SYNCHRONIZE|win.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer win.CloseHandle(h)
	if ev, err := win.WaitForSingleObject(h, 0); err != nil || ev != uint32(win.WAIT_TIMEOUT) {
		return false // signaled = exited
	}
	buf := make([]uint16, win.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := win.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return true // running but unreadable; be conservative
	}
	other := strings.ToLower(filepath.Base(win.UTF16ToString(buf[:size])))
	self, err := os.Executable()
	if err != nil {
		return true
	}
	return other == strings.ToLower(filepath.Base(self))
}

// acquireGroupLock takes the lock for path. The returned file must stay open
// for as long as the group is in use and be released with releaseGroupLock.
func acquireGroupLock(path string) (*os.File, error) {
	for attempt := 0; attempt < 5; attempt++ {
		f, err := openGroupLockHandle(path)
		if err != nil {
			if errors.Is(err, win.ERROR_ACCESS_DENIED) || errors.Is(err, win.ERROR_SHARING_VIOLATION) {
				return nil, errGroupInUse
			}
			return nil, err
		}
		ok, err := tryOSLock(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		if !ok {
			f.Close()
			return nil, errGroupInUse
		}
		// The owner may have deleted the path between our open and our lock;
		// in that case we locked an orphan and must retry on the new file.
		fi, err1 := f.Stat()
		pi, err2 := os.Stat(path)
		if err1 != nil || err2 != nil || !os.SameFile(fi, pi) {
			f.Close()
			continue
		}
		if pid, osLocked := readLockOwner(f); !osLocked && legacyOwnerAlive(pid) {
			f.Close()
			return nil, errGroupInUse
		}
		content := strconv.Itoa(os.Getpid()) + "\n" + groupLockMarker + "\n"
		if err := f.Truncate(0); err == nil {
			_, err = f.WriteAt([]byte(content), 0)
		}
		if err != nil {
			f.Close()
			return nil, err
		}
		_ = f.Sync()
		return f, nil
	}
	return nil, errGroupInUse
}

// releaseGroupLock deletes the file while the lock is still held, then closes
// the handle (which releases the OS lock). A failed delete is harmless: an
// unlocked file is treated as free.
func releaseGroupLock(f *os.File, path string) {
	if f == nil {
		return
	}
	for i := 0; i < 3; i++ {
		if err := os.Remove(path); err == nil || errors.Is(err, os.ErrNotExist) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	f.Close()
}

// groupLockHeld reports whether another process (or another handle in this
// process) currently holds the lock for path. Stale files are removed.
func groupLockHeld(path string) bool {
	f, err := acquireGroupLock(path)
	if err != nil {
		return errors.Is(err, errGroupInUse)
	}
	releaseGroupLock(f, path)
	return false
}
