package helper

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"
)

func ErrAddLineTimeFileInfo(err error) error {

	return errors.New(addLineTimeFileInfo(err.Error()))
}

func ErrNewAddLineTimeFileInfo(msg string) error {
	return errors.New(addLineTimeFileInfo(msg))
}

func addLineTimeFileInfo(msg string) string {

	if strings.Contains(msg[:4], "LogE") {
		return msg
	}

	_, file, line, _ := runtime.Caller(1)
	indexFunc := func(file string) string {
		backup := "/" + file
		lastSlashIndex := strings.LastIndex(backup, "/")
		if lastSlashIndex < 0 {
			return backup
		}
		secondLastSlashIndex := strings.LastIndex(backup[:lastSlashIndex], "/")
		if secondLastSlashIndex < 0 {
			return backup[lastSlashIndex+1:]
		}
		return backup[secondLastSlashIndex+1:]
	}

	logmessage := fmt.Sprintf("\nLogE: %v %v:%v:%v ", time.Now().UTC().Format("2006-01-02 15:04:05.000"), indexFunc(file), line, msg)

	return logmessage
}
