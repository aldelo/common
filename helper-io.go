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

	// use consistent octal literal and bubble error directly
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
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

	// use consistent octal literal and bubble error directly
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}

	return nil
}

// FileExists checks if input file in path exists
func FileExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
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

	// ensure destination directory exists
	if err = os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	if srcfd, err = os.Open(src); err != nil {
		return err
	}
	defer srcfd.Close()

	if dstfd, err = os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcinfo.Mode()); err != nil {
		return err
	}
	defer func() { // propagate close error
		if cerr := dstfd.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(dstfd, srcfd); err != nil {
		return err
	}

	// preserve source mode after copy (in case umask altered it)
	return os.Chmod(dst, srcinfo.Mode())
}

// CopyDir - Dir copies a whole directory recursively
func CopyDir(src string, dst string) error {
	srcinfo, err := os.Lstat(src)
	if err != nil {
		return err
	}

	// NEW: if src is a symlink, recreate the symlink instead of copying target
	if srcinfo.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		_ = os.Remove(dst)
		return os.Symlink(target, dst)
	}

	if !srcinfo.IsDir() { // NEW
		return fmt.Errorf("source is not a directory: %s", src)
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcfp := filepath.Join(src, entry.Name())
		dstfp := filepath.Join(dst, entry.Name())

		// CHANGED: use Lstat so symlink type is detected without following it
		info, err := os.Lstat(srcfp)
		if err != nil {
			return err
		}

		// handle symlinks explicitly
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(srcfp)
			if err != nil {
				return err
			}
			_ = os.Remove(dstfp) // remove existing file/symlink if any
			if err := os.Symlink(target, dstfp); err != nil {
				return err
			}
			continue
		}

		if info.IsDir() {
			if err = CopyDir(srcfp, dstfp); err != nil {
				return err
			}
		} else if info.Mode().IsRegular() { // NEW: guard non-regular files
			if err = CopyFile(srcfp, dstfp); err != nil {
				return err
			}
		} else { // NEW: explicit unsupported type handling
			return fmt.Errorf("unsupported file type at %s", srcfp)
		}
	}
	return nil
}
