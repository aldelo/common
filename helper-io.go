package helper

/*
 * Copyright 2020-2026 Aldelo, LP
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// FileRead will read all file content of given file in path,
// return as string if successful,
// if failed, error will contain the error reason
func FileRead(path string) (string, error) {
	if strings.TrimSpace(path) == "" { // guard empty/whitespace paths
		return "", fmt.Errorf("path is empty")
	}
	if err := ensureNoSymlinkDirs(filepath.Dir(path)); err != nil { // block parent-dir symlink traversal
		return "", err
	}

	f, _, err := openRegularRead(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// FileReadBytes will read all file content and return slice of byte
func FileReadBytes(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("path is empty")
	}
	if err := ensureNoSymlinkDirs(filepath.Dir(path)); err != nil {
		return nil, err
	}

	f, _, err := openRegularRead(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// FileWrite will write data into file at the given path,
// if successful, no error is returned (nil)
func FileWrite(path string, data string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path is empty")
	}
	dir := filepath.Dir(path)
	if err := ensureNoSymlinkDirs(dir); err != nil {
		return err
	}

	// default to preserving existing file mode when present; otherwise keep temp's umask-respecting default
	var mode os.FileMode
	var haveMode bool

	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path is a symlink: %s", path)
		}
		if info.IsDir() {
			return fmt.Errorf("path is a directory: %s", path)
		}
		if !info.Mode().IsRegular() { // forbid writing over special files
			return fmt.Errorf("path is not a regular file: %s", path)
		}
		mode, haveMode = info.Mode(), true
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := ensureNoSymlinkDirs(dir); err != nil { // re-validate parent to shrink TOCTTOU window
		return err
	}

	// use unique temp file in target directory to avoid races/symlink attacks
	base := filepath.Base(path)
	tmpFile, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmp := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()

	if _, err := io.WriteString(tmpFile, data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil { // ensure final mode
		return err
	}

	if err := ensureNoSymlinkDirs(dir); err != nil { // re-validate just before rename
		return err
	}

	if err := os.Rename(tmp, path); err != nil {
		// surface failure to remove old destination before retry
		if errors.Is(err, fs.ErrExist) || runtime.GOOS == "windows" {
			// revalidate destination before removing to avoid deleting dirs/symlinks
			if info, statErr := os.Lstat(path); statErr == nil {
				if info.Mode()&os.ModeSymlink != 0 {
					return fmt.Errorf("destination is a symlink: %s", path)
				}
				if info.IsDir() {
					return fmt.Errorf("destination is a directory: %s", path)
				}
				if !info.Mode().IsRegular() {
					return fmt.Errorf("destination is not a regular file: %s", path)
				}
			} else if !os.IsNotExist(statErr) {
				return statErr
			}

			_ = os.Chmod(path, 0o666) // best-effort; ignore error
			if remErr := os.Remove(path); remErr != nil && !os.IsNotExist(remErr) {
				return fmt.Errorf("rename failed: %v; cleanup failed: %v", err, remErr)
			}
			if err2 := os.Rename(tmp, path); err2 != nil {
				return err2
			}
		} else {
			return err
		}
	}

	// fsync parent directory to make rename durable
	// skip directory fsync on Windows (not supported)
	if runtime.GOOS != "windows" {
		if dir, err := os.Open(filepath.Dir(path)); err == nil {
			if syncErr := dir.Sync(); syncErr != nil {
				_ = dir.Close()
				return syncErr
			}
			_ = dir.Close()
		} else {
			return err
		}
	}

	cleanup = false
	return nil
}

// FileWriteBytes will write byte data into file at the given path,
// if successful, no error is returned (nil)
func FileWriteBytes(path string, data []byte) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path is empty")
	}
	dir := filepath.Dir(path)
	if err := ensureNoSymlinkDirs(dir); err != nil {
		return err
	}

	// default to preserving existing file mode when present; otherwise keep temp's umask-respecting default
	var mode os.FileMode
	var haveMode bool

	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path is a symlink: %s", path)
		}
		if info.IsDir() {
			return fmt.Errorf("path is a directory: %s", path)
		}
		if !info.Mode().IsRegular() { // forbid writing over special files
			return fmt.Errorf("path is not a regular file: %s", path)
		}
		mode, haveMode = info.Mode(), true
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := ensureNoSymlinkDirs(dir); err != nil { // re-validate parent to shrink TOCTTOU window
		return err
	}

	// use unique temp file in target directory to avoid races/symlink attacks
	base := filepath.Base(path)
	tmpFile, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmp := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil { // ensure final mode
		return err
	}

	if err := ensureNoSymlinkDirs(dir); err != nil { // re-validate just before rename
		return err
	}

	if err := os.Rename(tmp, path); err != nil { // handle overwrite on Windows
		// surface failure to remove old destination before retry
		if errors.Is(err, fs.ErrExist) || runtime.GOOS == "windows" {
			// revalidate destination before removing to avoid deleting dirs/symlinks
			if info, statErr := os.Lstat(path); statErr == nil {
				if info.Mode()&os.ModeSymlink != 0 {
					return fmt.Errorf("destination is a symlink: %s", path)
				}
				if info.IsDir() {
					return fmt.Errorf("destination is a directory: %s", path)
				}
				if !info.Mode().IsRegular() {
					return fmt.Errorf("destination is not a regular file: %s", path)
				}
			} else if !os.IsNotExist(statErr) {
				return statErr
			}

			_ = os.Chmod(path, 0o666) // best-effort; ignore error
			if remErr := os.Remove(path); remErr != nil && !os.IsNotExist(remErr) {
				return fmt.Errorf("rename failed: %v; cleanup failed: %v", err, remErr)
			}
			if err2 := os.Rename(tmp, path); err2 != nil {
				return err2
			}
		} else {
			return err
		}
	}

	// fsync parent directory to make rename durable
	// skip directory fsync on Windows (not supported)
	if runtime.GOOS != "windows" {
		if dirFd, err := os.Open(dir); err == nil {
			if syncErr := dirFd.Sync(); syncErr != nil {
				_ = dirFd.Close()
				return syncErr
			}
			_ = dirFd.Close()
		} else {
			return err
		}
	}

	cleanup = false
	return nil
}

// FileExists checks if input file in path exists
func FileExists(path string) bool {
	if strings.TrimSpace(path) == "" { // guard empty/whitespace paths
		return false
	}

	info, err := os.Lstat(path)
	if err != nil {
		return !os.IsNotExist(err)
	}

	if info.Mode()&os.ModeSymlink != 0 { // do not treat symlinks as existing files
		return false
	}

	return info.Mode().IsRegular()
}

// CopyFile - File copies a single file from src to dst
func CopyFile(src string, dst string) (err error) { // named return for close error propagation
	if strings.TrimSpace(src) == "" || strings.TrimSpace(dst) == "" {
		return fmt.Errorf("source or destination path is empty")
	}

	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return err
	}

	// ensure no symlink in ancestor directories for either side
	if err := ensureNoSymlinkDirs(filepath.Dir(srcAbs)); err != nil {
		return err
	}
	if err := ensureNoSymlinkDirs(filepath.Dir(dstAbs)); err != nil {
		return err
	}

	// resolve real paths only for guard checks, not for copying behavior
	srcReal := srcAbs
	if real, err := filepath.EvalSymlinks(srcAbs); err == nil {
		srcReal = real
	}
	dstReal := dstAbs
	if real, err := filepath.EvalSymlinks(dstAbs); err == nil {
		dstReal = real
	}

	// guard using both raw and resolved paths to avoid copying into self via symlinks
	if pathsEqual(srcAbs, dstAbs) || pathWithin(dstAbs, srcAbs) || pathWithin(dstReal, srcReal) {
		return fmt.Errorf("destination is the same as or within the source: %s -> %s", src, dst)
	}

	// Use Lstat on the original path so top-level symlinks are preserved
	srcinfo, err := os.Lstat(srcAbs)
	if err != nil {
		return err
	}

	// preserve top-level symlink instead of dereferencing it
	if srcinfo.Mode()&os.ModeSymlink != 0 {
		if err := os.MkdirAll(filepath.Dir(dstAbs), 0o755); err != nil {
			return err
		}
		target, err := os.Readlink(srcAbs)
		if err != nil {
			return err
		}
		if err := os.RemoveAll(dstAbs); err != nil && !os.IsNotExist(err) {
			return err
		}
		return os.Symlink(target, dstAbs)
	}

	// enforce file-copy semantics (not directory)
	if srcinfo.IsDir() {
		return fmt.Errorf("source is a directory: %s", src)
	}
	if !srcinfo.Mode().IsRegular() {
		return fmt.Errorf("unsupported file type: %s", src)
	}

	// prevent writing into a directory path or overwriting a symlink
	if dstInfo, err := os.Lstat(dstAbs); err == nil {
		if dstInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("destination is a symlink: %s", dst)
		}
		if dstInfo.IsDir() {
			return fmt.Errorf("destination is a directory: %s", dst)
		}
		if !dstInfo.Mode().IsRegular() { // CHANGED: block special files (sockets, devices, FIFOs)
			return fmt.Errorf("destination is not a regular file: %s", dst)
		} // CHANGED
	}

	if err := os.MkdirAll(filepath.Dir(dstAbs), 0o755); err != nil {
		return err
	}

	// copy via temp file in destination dir for atomic replace
	dir := filepath.Dir(dstAbs)
	base := filepath.Base(dstAbs)
	tmpFile, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmp := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()

	// open source after temp exists, then revalidate to avoid TOCTTOU
	srcf, err := os.Open(srcAbs)
	if err != nil {
		_ = tmpFile.Close()
		return err
	}
	finfo, err := srcf.Stat()
	if err != nil {
		_ = srcf.Close()
		_ = tmpFile.Close()
		return err
	}
	linfo2, err := os.Lstat(srcAbs)
	if err != nil {
		_ = srcf.Close()
		_ = tmpFile.Close()
		return err
	}
	if linfo2.Mode()&os.ModeSymlink != 0 || !os.SameFile(finfo, linfo2) {
		_ = srcf.Close()
		_ = tmpFile.Close()
		return fmt.Errorf("source changed or became symlink: %s", src)
	}
	if !finfo.Mode().IsRegular() {
		_ = srcf.Close()
		_ = tmpFile.Close()
		return fmt.Errorf("unsupported file type: %s", src)
	}

	_, err = io.Copy(tmpFile, srcf) // actual file copy
	_ = srcf.Close()
	if err != nil {
		_ = tmpFile.Close()
		return err
	}

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, srcinfo.Mode()); err != nil { // match source mode
		return err
	}

	if err := ensureNoSymlinkDirs(dir); err != nil { // re-validate just before rename
		return err
	}

	// atomic replace with Windows overwrite handling
	if err := os.Rename(tmp, dstAbs); err != nil {
		if errors.Is(err, fs.ErrExist) || runtime.GOOS == "windows" {
			// revalidate destination before removing to avoid deleting dirs/symlinks
			if info, statErr := os.Lstat(dstAbs); statErr == nil {
				if info.Mode()&os.ModeSymlink != 0 {
					return fmt.Errorf("destination is a symlink: %s", dst)
				}
				if info.IsDir() {
					return fmt.Errorf("destination is a directory: %s", dst)
				}
				if !info.Mode().IsRegular() {
					return fmt.Errorf("destination is not a regular file: %s", dst)
				}
			} else if !os.IsNotExist(statErr) {
				return statErr
			}

			_ = os.Chmod(dstAbs, 0o666) // best-effort; ignore error
			if remErr := os.Remove(dstAbs); remErr != nil && !os.IsNotExist(remErr) {
				return fmt.Errorf("rename failed: %v; cleanup failed: %v", err, remErr)
			}
			if err2 := os.Rename(tmp, dstAbs); err2 != nil {
				return err2
			}
		} else {
			return err
		}
	}

	// fsync parent directory to make rename durable (POSIX only)
	if runtime.GOOS != "windows" {
		if dirFd, err := os.Open(dir); err == nil {
			if syncErr := dirFd.Sync(); syncErr != nil {
				_ = dirFd.Close()
				return syncErr
			}
			_ = dirFd.Close()
		} else {
			return err
		}
	}

	cleanup = false
	return nil
}

// CopyDir - Dir copies a whole directory recursively
func CopyDir(src string, dst string) error {
	if strings.TrimSpace(src) == "" || strings.TrimSpace(dst) == "" {
		return fmt.Errorf("source or destination path is empty")
	}

	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}

	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return err
	}

	// ensure no symlink in ancestor directories
	if err := ensureNoSymlinkDirs(filepath.Dir(srcAbs)); err != nil {
		return err
	}
	if err := ensureNoSymlinkDirs(filepath.Dir(dstAbs)); err != nil {
		return err
	}

	// normalize canonical (symlink-resolved) paths without discarding originals
	srcCanon := srcAbs
	if real, err := filepath.EvalSymlinks(srcAbs); err == nil { // normalize source
		srcCanon = real
	}
	dstCanon := dstAbs
	if real, err := filepath.EvalSymlinks(dstAbs); err == nil { // normalize destination if it exists
		dstCanon = real
	}

	// canonical guard to prevent copy-into-self via resolved symlinks
	if pathsEqual(srcCanon, dstCanon) || pathWithin(dstCanon, srcCanon) {
		return fmt.Errorf("destination is the same as or within the source: %s -> %s", src, dst)
	}

	// re-validate canonical parents to catch swapped-in symlink ancestors
	if err := ensureNoSymlinkDirs(filepath.Dir(srcCanon)); err != nil {
		return err
	}
	if err := ensureNoSymlinkDirs(filepath.Dir(dstCanon)); err != nil {
		return err
	}

	// case-aware, symlink-aware self/into-self guard
	if pathsEqual(srcAbs, dstAbs) || pathWithin(dstAbs, srcAbs) {
		return fmt.Errorf("destination is the same as or within the source: %s -> %s", src, dst)
	}

	// reject copying into an existing symlink destination
	if dstInfo, err := os.Lstat(dst); err == nil {
		if dstInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("destination is a symlink: %s", dst)
		}
		if !dstInfo.IsDir() { // explicit guard for non-directory destinations
			return fmt.Errorf("destination is not a directory: %s", dst)
		}
	}

	srcinfo, err := os.Lstat(src)
	if err != nil {
		return err
	}

	// if src is a symlink, recreate the symlink instead of copying target
	if srcinfo.Mode()&os.ModeSymlink != 0 {
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil { // ensure parent exists
			return err
		}
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		// remove any existing path (file/dir/symlink) before recreating the symlink
		if err := os.RemoveAll(dst); err != nil && !os.IsNotExist(err) {
			return err
		}
		return os.Symlink(target, dst)
	}

	if !srcinfo.IsDir() {
		return fmt.Errorf("source is not a directory: %s", src)
	}

	dirHandle, err := os.Open(src) // hold directory handle to prevent TOCTTOU
	if err != nil {
		return err
	}
	defer dirHandle.Close()

	dirStat, err := dirHandle.Stat() // revalidate using opened handle
	if err != nil {
		return err
	}
	linfo2, err := os.Lstat(src) // double-check path has not become a symlink or different inode
	if err != nil {
		return err
	}
	if linfo2.Mode()&os.ModeSymlink != 0 || !os.SameFile(dirStat, linfo2) {
		return fmt.Errorf("source changed or became symlink: %s", src)
	}
	if !dirStat.IsDir() { // ensure still a directory
		return fmt.Errorf("source is not a directory: %s", src)
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return err
	}
	if err = os.Chmod(dst, srcinfo.Mode()); err != nil { // ensure mode matches source
		return err
	}

	entries, err := dirHandle.Readdir(-1) // enumerate via the validated handle
	if err != nil {
		return err
	}

	for _, entryInfo := range entries { // loop over FileInfo from Readdir
		srcfp := filepath.Join(src, entryInfo.Name())
		dstfp := filepath.Join(dst, entryInfo.Name())

		mode := entryInfo.Mode()

		// handle symlinks explicitly
		if mode&os.ModeSymlink != 0 {
			if err := os.MkdirAll(filepath.Dir(dstfp), 0o755); err != nil { // ensure parent exists
				return err
			}
			target, err := os.Readlink(srcfp)
			if err != nil {
				return err
			}
			// remove any existing path (file/dir/symlink) before recreating the symlink
			if err := os.RemoveAll(dstfp); err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := os.Symlink(target, dstfp); err != nil {
				return err
			}
			continue
		}

		if mode.IsDir() {
			if err = CopyDir(srcfp, dstfp); err != nil {
				return err
			}
		} else if mode.IsRegular() { // guard non-regular files
			if err = CopyFile(srcfp, dstfp); err != nil {
				return err
			}
		} else { // explicit unsupported type handling
			return fmt.Errorf("unsupported file type at %s", srcfp)
		}
	}
	return nil
}

// helper functions for safe, case-aware path comparison
func normPath(p string) string {
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" { // treat macOS default case-insensitive FS conservatively
		p = strings.ToLower(p)
	}
	return p
}

func pathsEqual(a, b string) bool {
	return normPath(a) == normPath(b)
}

func pathWithin(path, parent string) bool {
	// use filepath.Rel to robustly detect descendant paths, including root and drive roots
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	rel = normPath(rel)
	// inside if rel is "." or does not start with ".."
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "..")
}

// helper to open a regular file safely without following swapped-in symlinks.
func openRegularRead(path string) (*os.File, os.FileInfo, error) {
	linfo, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if linfo.Mode()&os.ModeSymlink != 0 {
		return nil, nil, fmt.Errorf("path is a symlink: %s", path)
	}
	if linfo.IsDir() {
		return nil, nil, fmt.Errorf("path is a directory: %s", path)
	}
	if !linfo.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("path is not a regular file: %s", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}

	finfo, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}

	linfo2, err := os.Lstat(path)
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	if linfo2.Mode()&os.ModeSymlink != 0 || !os.SameFile(finfo, linfo2) {
		_ = f.Close()
		return nil, nil, fmt.Errorf("path changed during open: %s", path)
	}
	if !finfo.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, fmt.Errorf("path is not a regular file: %s", path)
	}

	return f, finfo, nil
}

// ensureNoSymlinkDirs walks ancestor directories (existing ones) to reject symlinked parents.
// This prevents path redirection via swapped-in parent symlinks.
func ensureNoSymlinkDirs(dir string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}

	visited := map[string]struct{}{} // loop guard to prevent cycles

	for {
		if _, seen := visited[absDir]; seen {
			return fmt.Errorf("symlink loop detected: %s", absDir)
		}
		visited[absDir] = struct{}{} // CHANGED

		info, err := os.Lstat(absDir)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			// tolerate stable macOS system symlinks (/var, /tmp, /etc) by following once
			if runtime.GOOS == "darwin" && isDarwinSystemSymlink(absDir) {
				target, err := filepath.EvalSymlinks(absDir)
				if err != nil {
					return err
				}
				absDir = target
				continue
			}
			return fmt.Errorf("ancestor directory is a symlink: %s", absDir)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}

		parent := filepath.Dir(absDir)
		if parent == absDir {
			return nil
		}
		absDir = parent
	}
}

// CHANGED: allowlist macOS system symlinks so default /var, /tmp, /etc paths work
func isDarwinSystemSymlink(path string) bool {
	switch filepath.Clean(path) {
	case "/var", "/tmp", "/etc", "/private/var", "/private/tmp", "/private/etc":
		return true
	default:
		return false
	}
}
