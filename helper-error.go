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
	if err == nil { // guard nil to avoid panic on err.Error()
		return nil
	}
	annotated := addLineTimeFileInfo(err.Error()) // keep annotated message
	return fmt.Errorf("%s: %w", annotated, err)   // wrap to preserve original cause
}

func ErrNewAddLineTimeFileInfo(msg string) error {
	return errors.New(addLineTimeFileInfo(msg))
}

func addLineTimeFileInfo(msg string) string {
	if strings.HasPrefix(msg, "\nLogE:") { // safe prefix check, no slicing panic
		return msg
	}

	_, file, line, ok := runtime.Caller(2)
	if !ok { // fallback when caller info is unavailable
		file = "unknown"
		line = 0
	}

	file = filepath.ToSlash(file) // normalize Windows-style paths
	base := filepath.Base(file)   // get file name
	dir := filepath.Base(filepath.Dir(file))
	shortFile := base
	if dir != "." && dir != "/" && dir != "" { // CHANGED: include parent dir when present
		shortFile = dir + "/" + base
	}

	logmessage := fmt.Sprintf("\nLogE: %v %v:%v:%v ",
		time.Now().UTC().Format("2006-01-02 15:04:05.000"),
		shortFile,
		line,
		msg)

	return logmessage
}
