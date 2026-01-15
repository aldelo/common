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
			prefixed = false // on panic, allow caller to add prefix instead of skipping it
		}
	}()

	type singleUnwrapper interface{ Unwrap() error }
	type multiUnwrapper interface{ Unwrap() []error }

	const maxWalk = 256 // cap traversal to prevent cycles from hanging

	seenComparable := make(map[error]struct{})
	seenPtr := make(map[uintptr]struct{}) // track non-comparable errors by pointer

	// avoid reflect.Type.Comparable (Go 1.20+) and safely detect comparables via map insert with recover
	addComparable := func(e error) (added bool, dup bool) {
		defer func() {
			if r := recover(); r != nil { // non-comparable dynamic type will panic when used as a map key
				added, dup = false, false
			}
		}()
		_, exists := seenComparable[e]
		seenComparable[e] = struct{}{}
		return true, exists
	}

	stack := []error{err}
	steps := 0

	for len(stack) > 0 {
		if steps++; steps > maxWalk { // fail-safe against pathological cycles
			return false // treat exhaustion as not-prefixed so caller still annotates once
		}

		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if e == nil {
			continue
		}

		if added, dup := addComparable(e); added {
			if dup {
				continue
			}
		} else { // dedupe non-comparable errors when pointer identity is available
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

		msg := e.Error()
		// detect prefix even if the leading newline was stripped (e.g., by formatting)
		if strings.Contains(msg, "\nLogE:") || strings.Contains(msg, "LogE:") {
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
