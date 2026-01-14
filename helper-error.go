package helper

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func ErrAddLineTimeFileInfo(err error) error {
	if err == nil {
		return nil
	}
	if strings.HasPrefix(err.Error(), "\nLogE:") { // CHANGED: avoid double annotation/wrapping
		return err
	}
	return fmt.Errorf("%s%w", logPrefix(0), err) // CHANGED: prefix once, preserve cause
}

func ErrNewAddLineTimeFileInfo(msg string) error {
	return errors.New(logPrefix(0) + msg)
}

func addLineTimeFileInfo(msg string) string {
	return logPrefix(0) + msg
}

// logPrefix builds the LogE prefix with caller/time info.
func logPrefix(skip int) string { // new helper for shared caller/time logic
	_, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		file = "unknown"
		line = 0
	}

	file = filepath.ToSlash(file)
	base := filepath.Base(file)
	dir := filepath.Base(filepath.Dir(file))
	shortFile := base
	if dir != "." && dir != "/" && dir != "" {
		shortFile = dir + "/" + base
	}

	return fmt.Sprintf("\nLogE: %v %v:%v: ",
		time.Now().UTC().Format("2006-01-02 15:04:05.000"),
		shortFile,
		line)
}
