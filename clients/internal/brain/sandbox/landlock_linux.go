//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

/*
 * Landlock: the kernel's own answer to "this program may write here and
 * nowhere else".
 *
 * Its own file because it is the one part of confining a program that only
 * Linux has. Everything else in this package — a clean environment, a process
 * group of its own, no shell — works the same anywhere, and used to be
 * unbuildable on macOS and Windows because it shared a file with these
 * syscalls. The program could not be compiled for either, and the build
 * script offered both.
 */

// ABI is the Landlock version this kernel offers.
func ABI() (int, error) {
	abi, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if errno != 0 {
		return 0, fmt.Errorf("this kernel has no Landlock: %w", errno)
	}

	return int(abi), nil
}

// writeRights is every kind of change a Landlock ABI can refuse. Reading and
// running are left alone: confinement here is about what is changed.
func writeRights(abi int) uint64 {
	rights := uint64(unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
		unix.LANDLOCK_ACCESS_FS_REMOVE_FILE | unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
		unix.LANDLOCK_ACCESS_FS_MAKE_DIR | unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_SOCK | unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
		unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK | unix.LANDLOCK_ACCESS_FS_MAKE_SYM)

	if abi >= 2 {
		rights |= unix.LANDLOCK_ACCESS_FS_REFER
	}

	if abi >= 3 {
		rights |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}

	if abi >= 5 {
		rights |= unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
	}

	return rights
}

// Restrict confines this process, and whatever it becomes, to writing only
// beneath dirs.
func Restrict(dirs []string) error {
	abi, err := ABI()
	if err != nil {
		return err
	}

	rights := writeRights(abi)

	attr := unix.LandlockRulesetAttr{Access_fs: rights}

	fd, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET,
		uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if errno != 0 {
		return fmt.Errorf("could not make a Landlock ruleset: %w", errno)
	}

	defer unix.Close(int(fd))

	for _, dir := range dirs {
		f, err := unix.Open(dir, unix.O_PATH|unix.O_CLOEXEC, 0)
		if err != nil {
			continue
		}

		allowed := rights

		var st unix.Stat_t
		if unix.Fstat(f, &st) == nil && st.Mode&unix.S_IFMT != unix.S_IFDIR {
			// A file can only be written, never made things in.
			allowed = rights & (unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_TRUNCATE |
				unix.LANDLOCK_ACCESS_FS_IOCTL_DEV)
		}

		rule := unix.LandlockPathBeneathAttr{Allowed_access: allowed, Parent_fd: int32(f)}

		_, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, fd, unix.LANDLOCK_RULE_PATH_BENEATH,
			uintptr(unsafe.Pointer(&rule)), 0, 0, 0)

		unix.Close(f)

		if errno != 0 {
			return fmt.Errorf("could not allow %s: %w", dir, errno)
		}
	}

	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("could not drop new privileges: %w", err)
	}

	if _, _, errno := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, fd, 0, 0); errno != 0 {
		return fmt.Errorf("could not confine: %w", errno)
	}

	return nil
}

/*
 * Main is the confining copy of this program, when that is what it was
 * started as. Called first thing in main: it returns false for every other
 * start, and on its own start it never returns.
 */
func Main() bool {
	if len(os.Args) < 3 || os.Args[1] != helperArg {
		return false
	}

	n, err := strconv.Atoi(os.Args[2])
	if err != nil || len(os.Args) < 3+n+2 || os.Args[3+n] != "--" {
		fmt.Fprintln(os.Stderr, "sandbox: malformed confinement")
		os.Exit(126)
	}

	dirs := os.Args[3 : 3+n]
	argv := os.Args[3+n+1:]

	program, err := exec.LookPath(argv[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
		os.Exit(127)
	}

	if err := Restrict(dirs); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
		os.Exit(126)
	}

	err = syscall.Exec(program, argv, os.Environ())
	fmt.Fprintf(os.Stderr, "sandbox: could not start %s: %v\n", argv[0], err)
	os.Exit(126)

	return true
}
