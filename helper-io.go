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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// FileRead will read all file content of given file in path,
// return as string if successful,
// if failed, error will contain the error reason
func FileRead(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory: %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// FileReadBytes will read all file content and return slice of byte
func FileReadBytes(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory: %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// FileWrite will write data into file at the given path,
// if successful, no error is returned (nil)
func FileWrite(path string, data string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tmp := path + ".tmp" // write atomically to temp file
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(f, data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	if err := os.Rename(tmp, path); err != nil {
		// surface failure to remove old destination before retry
		if remErr := os.Remove(path); remErr != nil && !os.IsNotExist(remErr) {
			_ = os.Remove(tmp)
			return fmt.Errorf("rename failed: %v; cleanup failed: %v", err, remErr)
		}
		if err2 := os.Rename(tmp, path); err2 != nil {
			_ = os.Remove(tmp)
			return err2
		}
	}

	// fsync parent directory to make rename durable
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		if syncErr := dir.Sync(); syncErr != nil {
			_ = dir.Close()
			return syncErr
		}
		_ = dir.Close()
	} else {
		return err
	}

	return nil
}

// FileWriteBytes will write byte data into file at the given path,
// if successful, no error is returned (nil)
func FileWriteBytes(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tmp := path + ".tmp" // write atomically to temp file
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	if err := os.Rename(tmp, path); err != nil { // handle overwrite on Windows
		// surface failure to remove old destination before retry
		if remErr := os.Remove(path); remErr != nil && !os.IsNotExist(remErr) {
			_ = os.Remove(tmp)
			return fmt.Errorf("rename failed: %v; cleanup failed: %v", err, remErr)
		}
		if err2 := os.Rename(tmp, path); err2 != nil {
			_ = os.Remove(tmp)
			return err2
		}
	}

	// fsync parent directory to make rename durable
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		if syncErr := dir.Sync(); syncErr != nil {
			_ = dir.Close()
			return syncErr
		}
		_ = dir.Close()
	} else {
		return err
	}

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
	var srcfd *os.File
	var dstfd *os.File
	var srcinfo os.FileInfo

	// validate source is a regular file before copying
	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}
	if !srcinfo.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file: %s", src)
	}

	// prevent copying onto the same file (path or hardlink), which would truncate the source
	if dstinfo, statErr := os.Stat(dst); statErr == nil {
		if os.SameFile(srcinfo, dstinfo) {
			return fmt.Errorf("source and destination are the same file: %s", src)
		}
	} else if !os.IsNotExist(statErr) { // propagate unexpected stat errors
		return statErr
	}

	// ensure destination directory exists
	if err = os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	tmp := dst + ".tmp" // copy to temp file first
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmp)
		}
	}()

	if srcfd, err = os.Open(src); err != nil {
		return err
	}
	defer func() { // propagate src close error too
		if cerr := srcfd.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	if dstfd, err = os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcinfo.Mode()); err != nil {
		return err
	}
	defer func() {        // propagate close error
		if dstfd != nil { // avoid double-close after explicit close
			if cerr := dstfd.Close(); err == nil && cerr != nil {
				err = cerr
			}
		}
	}()

	if _, err = io.Copy(dstfd, srcfd); err != nil {
		return err
	}
	if err = dstfd.Sync(); err != nil { // ensure data on disk
		return err
	}

	// close before rename to avoid Windows issues
	if cerr := dstfd.Close(); err == nil && cerr != nil { // explicit close before rename
		err = cerr
		return err
	}
	dstfd = nil // prevent deferred double-close

	if err = os.Chmod(tmp, srcinfo.Mode()); err != nil { // preserve source mode after copy (in case umask altered it)
		return err
	}

	if err = os.Rename(tmp, dst); err != nil { // atomic replace
		if remErr := os.Remove(dst); remErr != nil && !os.IsNotExist(remErr) {
			return fmt.Errorf("failed to replace destination %s: %v; cleanup error: %v", dst, err, remErr)
		}
		if err = os.Rename(tmp, dst); err != nil {
			return err
		}
	}

	cleanupTmp = false

	// fsync parent directory to make rename durable
	if dir, dirErr := os.Open(filepath.Dir(dst)); dirErr == nil {
		if syncErr := dir.Sync(); syncErr != nil {
			_ = dir.Close()
			return syncErr
		}
		_ = dir.Close()
	} else {
		return dirErr
	}

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
