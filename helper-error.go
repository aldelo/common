package helper

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"
)

func ErrAddLineTimeFileInfo(err error) error {
	if err == nil {
		return nil
	}
	if alreadyLogPrefixed(err) { // detect existing prefix anywhere in unwrap chain
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

// idempotent check that walks the unwrap chain for existing LogE prefix
func alreadyLogPrefixed(err error) (prefixed bool) {
	if err == nil {
		return false
	}

	// prevent panics from Error/Unwrap implementations from crashing the caller
	defer func() {
		if r := recover(); r != nil {
			prefixed = true // assume prefixed on panic to avoid double-prefixing
		}
	}()

	type singleUnwrapper interface{ Unwrap() error }
	type multiUnwrapper interface{ Unwrap() []error }

	const maxWalk = 256 // CHANGED: cap traversal to prevent cycles from hanging

	seenComparable := make(map[error]struct{})
	seenPtr := make(map[uintptr]struct{}) // CHANGED: track non-comparable errors by pointer

	stack := []error{err}
	steps := 0

	for len(stack) > 0 {
		if steps++; steps > maxWalk { // CHANGED: fail-safe against pathological cycles
			return true
		}

		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if e == nil {
			continue
		}

		if t := reflect.TypeOf(e); t != nil && t.Comparable() {
			if _, ok := seenComparable[e]; ok {
				continue
			}
			seenComparable[e] = struct{}{}
		} else { // CHANGED: dedupe non-comparable errors when pointer identity is available
			v := reflect.ValueOf(e)
			if v.Kind() == reflect.Pointer || v.Kind() == reflect.UnsafePointer {
				if ptr := v.Pointer(); ptr != 0 {
					if _, ok := seenPtr[ptr]; ok {
						continue
					}
					seenPtr[ptr] = struct{}{}
				}
			}
		}

		if strings.HasPrefix(e.Error(), "\nLogE:") {
			return true
		}

		if mw, ok := e.(multiUnwrapper); ok {
			stack = append(stack, mw.Unwrap()...)
			continue
		}
		if sw, ok := e.(singleUnwrapper); ok {
			stack = append(stack, sw.Unwrap())
		}
	}
	return false
}

// logPrefix builds the LogE prefix with caller/time info.
func logPrefix(skip int) string { // new helper for shared caller/time logic
	_, file, line, ok := runtime.Caller(skip + 3) // adjusted skip to account for new helper
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
