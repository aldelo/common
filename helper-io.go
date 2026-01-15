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
	// default to 0644 but preserve existing file mode when present
	mode := os.FileMode(0o644)

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
		mode = info.Mode()
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	// use unique temp file in target directory to avoid races/symlink attacks
	dir := filepath.Dir(path)
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

	if err := os.Rename(tmp, path); err != nil {
		// surface failure to remove old destination before retry
		if errors.Is(err, fs.ErrExist) || runtime.GOOS == "windows" {
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
	// default to 0644 but preserve existing file mode when present
	mode := os.FileMode(0o644)

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
		mode = info.Mode()
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	// use unique temp file in target directory to avoid races/symlink attacks
	dir := filepath.Dir(path)
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

	if err := os.Rename(tmp, path); err != nil { // handle overwrite on Windows
		// surface failure to remove old destination before retry
		if errors.Is(err, fs.ErrExist) || runtime.GOOS == "windows" {
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
	// use Lstat so broken symlinks are reported as existing paths
	if _, err := os.Lstat(path); err != nil {
		return !os.IsNotExist(err)
	}
	return true
}

// CopyFile - File copies a single file from src to dst
func CopyFile(src string, dst string) (err error) { // named return for close error propagation
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}

	dstAbs, err := filepath.Abs(dst)
	if err != nil {
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

	// prevent writing into a directory path
	if dstInfo, err := os.Lstat(dstAbs); err == nil && dstInfo.IsDir() {
		return fmt.Errorf("destination is a directory: %s", dst)
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

	srcf, err := os.Open(srcAbs)
	if err != nil {
		_ = tmpFile.Close()
		return err
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

	// atomic replace with Windows overwrite handling
	if err := os.Rename(tmp, dstAbs); err != nil {
		if errors.Is(err, fs.ErrExist) || runtime.GOOS == "windows" {
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
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(srcAbs); err == nil { // normalize source
		srcAbs = real
	}

	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(dstAbs); err == nil { // normalize destination if it exists
		dstAbs = real
	}

	// case-aware, symlink-aware self/into-self guard
	if pathsEqual(srcAbs, dstAbs) || pathWithin(dstAbs, srcAbs) {
		return fmt.Errorf("destination is the same as or within the source: %s -> %s", src, dst)
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

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return err
	}
	if err = os.Chmod(dst, srcinfo.Mode()); err != nil { // ensure mode matches source
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcfp := filepath.Join(src, entry.Name())
		dstfp := filepath.Join(dst, entry.Name())

		// use Lstat so symlink type is detected without following it
		info, err := os.Lstat(srcfp)
		if err != nil {
			return err
		}

		// handle symlinks explicitly
		if info.Mode()&os.ModeSymlink != 0 {
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

		if info.IsDir() {
			if err = CopyDir(srcfp, dstfp); err != nil {
				return err
			}
		} else if info.Mode().IsRegular() { // guard non-regular files
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
	if runtime.GOOS == "windows" {
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
