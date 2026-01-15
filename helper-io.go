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
	"os"
	"path/filepath"
)

// FileRead will read all file content of given file in path,
// return as string if successful,
// if failed, error will contain the error reason
func FileRead(path string) (string, error) {
	data, err := os.ReadFile(path)

	if err != nil {
		return "", err
	}

	return string(data), nil
}

// FileReadBytes will read all file content and return slice of byte
func FileReadBytes(path string) ([]byte, error) {
	data, err := os.ReadFile(path)

	if err != nil {
		return []byte{}, err
	}

	return data, nil
}

// FileWrite will write data into file at the given path,
// if successful, no error is returned (nil)
func FileWrite(path string, data string) error {
	err := os.WriteFile(path, []byte(data), 0644)

	if err != nil {
		return err
	}

	return nil
}

// FileWriteBytes will write byte data into file at the given path,
// if successful, no error is returned (nil)
func FileWriteBytes(path string, data []byte) error {
	err := os.WriteFile(path, data, 0644)

	if err != nil {
		return err
	}

	return nil
}

// FileExists checks if input file in path exists
func FileExists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	} else {
		return !errors.Is(err, os.ErrNotExist) // distinguish not-exist
	}
}

// CopyFile - File copies a single file from src to dst
func CopyFile(src string, dst string) error {
	var err error
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

	if srcfd, err = os.Open(src); err != nil {
		return err
	}
	defer srcfd.Close()

	if dstfd, err = os.Create(dst); err != nil {
		return err
	}
	defer dstfd.Close()

	if _, err = io.Copy(dstfd, srcfd); err != nil {
		return err
	}

	// preserve source mode after copy
	return os.Chmod(dst, srcinfo.Mode())
}

// CopyDir - Dir copies a whole directory recursively
func CopyDir(src string, dst string) error {
	var err error
	var srcinfo os.FileInfo

	if srcinfo, err = os.Stat(src); err != nil {
		return err
	}

	if err = os.MkdirAll(dst, srcinfo.Mode()); err != nil {
		return err
	}

	// use os.ReadDir and propagate errors
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcfp := filepath.Join(src, entry.Name()) // filepath.Join
		dstfp := filepath.Join(dst, entry.Name()) // filepath.Join

		if entry.IsDir() {
			if err = CopyDir(srcfp, dstfp); err != nil { // propagate error
				return err
			}
		} else {
			if err = CopyFile(srcfp, dstfp); err != nil { // propagate error
				return err
			}
		}
	}
	return nil
}
