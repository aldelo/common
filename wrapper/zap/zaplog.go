package data

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
	"os"
	"strings"
	"sync"

	util "github.com/aldelo/common"
	"go.uber.org/zap"
)

// ZapLog is a wrapper for Zap logger package
//
// DisableLogger = disables the logger from operations, this allows code to be left in place while not performing logging action
// OutputToConsole = redirects log output to console instead of file
// AppName = required, this app's name
type ZapLog struct {
	// operating var
	DisableLogger bool

	OutputToConsole bool
	AppName         string

	// store zap client object
	zapLogger   *zap.Logger
	sugarLogger *zap.SugaredLogger

	mu sync.RWMutex
}

// helper to normalize log messages
func sanitizeLogMessage(msg string) string { // centralized newline stripping
	return strings.ReplaceAll(strings.ReplaceAll(msg, "\n", ""), "\r", "")
}

// Init will initialize and prepare the zap log wrapper for use,
//
// ...-internal-err.log = internal zap errors logged, this file may be created but may contain no data as there may not be any internal zap errors
// log output to file is 'appname.log'
func (z *ZapLog) Init() error {
	// validate
	if util.LenTrim(z.AppName) <= 0 {
		return errors.New("Init Logger Failed: " + "App Name is Required")
	}

	z.mu.Lock()
	defer z.mu.Unlock()

	// gracefully close previous logger to avoid FD leaks on re-init
	if z.zapLogger != nil {
		_ = z.zapLogger.Sync()
		z.zapLogger = nil
		z.sugarLogger = nil
	}

	var (
		logger *zap.Logger
		err    error
	)

	if !z.OutputToConsole {
		// log to file
		prod := zap.NewProductionConfig()

		prod.Development = true
		prod.DisableCaller = true

		prod.Encoding = "json"

		prod.OutputPaths = []string{z.AppName + ".log"}
		prod.ErrorOutputPaths = []string{z.AppName + "-internal-err.log"}

		logger, err = prod.Build()
	} else {
		// log to console
		logger, err = zap.NewProduction()
	}

	if err != nil {
		return errors.New("Init Logger Failed: " + err.Error())
	}

	// init zap sugared logger
	z.zapLogger = logger
	z.sugarLogger = logger.Sugar()

	// init success
	return nil
}

// Sync will flush log buffer to disk
func (z *ZapLog) Sync() error {
	z.mu.RLock()
	defer z.mu.RUnlock()

	if z.zapLogger != nil { // allow sync even when DisableLogger is true
		if err := z.zapLogger.Sync(); err != nil { // surface sync errors
			return err
		}
	}
	return nil
}

// Printf is alias method to Infof
func (z *ZapLog) Printf(format string, items ...interface{}) {
	z.Infof(format, items...)
}

// Infof is a Sugared Logging, allows template variable such as %s
func (z *ZapLog) Infof(logTemplateData string, args ...interface{}) {
	z.mu.RLock()
	logger := z.sugarLogger
	disabled := z.DisableLogger
	z.mu.RUnlock()

	if logger != nil && !disabled {
		logTemplateData = sanitizeLogMessage(logTemplateData) // normalize to prevent newline/tab injection
		logger.Infof(logTemplateData, args...)
	}
}

// Infow is a Sugared Logging, allows key value pairs variadic
func (z *ZapLog) Infow(logMessageData string, keyValuePairs ...interface{}) {
	z.mu.RLock() // guard concurrent access
	logger := z.sugarLogger
	disabled := z.DisableLogger
	z.mu.RUnlock()

	if logger != nil && !disabled {
		logMessageData = sanitizeLogMessage(logMessageData)
		logger.Infow(logMessageData, keyValuePairs...)
	}
}

// Info is faster Logging, but requires import of zap package, uses zap.String(), zap.Int(), etc in fields parameters
func (z *ZapLog) Info(logMessageData string, fields ...zap.Field) {
	z.mu.RLock() // guard concurrent access
	logger := z.zapLogger
	disabled := z.DisableLogger
	z.mu.RUnlock()

	if logger != nil && !disabled {
		logMessageData = sanitizeLogMessage(logMessageData)
		logger.Info(logMessageData, fields...)
	}
}

// Debugf is a Sugared Logging, allows template variable such as %s
func (z *ZapLog) Debugf(logTemplateData string, args ...interface{}) {
	z.mu.RLock()
	logger := z.sugarLogger
	disabled := z.DisableLogger
	z.mu.RUnlock()

	if logger != nil && !disabled {
		logTemplateData = sanitizeLogMessage(logTemplateData)
		logger.Debugf(logTemplateData, args...)
	}
}

// Debugw is a Sugared Logging, allows key value pairs variadic
func (z *ZapLog) Debugw(logMessageData string, keyValuePairs ...interface{}) {
	z.mu.RLock()
	logger := z.sugarLogger
	disabled := z.DisableLogger
	z.mu.RUnlock()

	if logger != nil && !disabled {
		logMessageData = sanitizeLogMessage(logMessageData)
		logger.Debugw(logMessageData, keyValuePairs...)
	}
}

// Debug is faster logging, but requires import of zap package, uses zap.String(), zap.Int(), etc in fields parameters
func (z *ZapLog) Debug(logMessageData string, fields ...zap.Field) {
	z.mu.RLock()
	logger := z.zapLogger
	disabled := z.DisableLogger
	z.mu.RUnlock()

	if logger != nil && !disabled {
		logMessageData = sanitizeLogMessage(logMessageData)
		logger.Debug(logMessageData, fields...)
	}
}

// Warnf is a Sugared Logging, allows template variable such as %s
func (z *ZapLog) Warnf(logTemplateData string, args ...interface{}) {
	z.mu.RLock()
	logger := z.sugarLogger
	disabled := z.DisableLogger
	z.mu.RUnlock()

	if logger != nil && !disabled {
		logTemplateData = sanitizeLogMessage(logTemplateData)
		logger.Warnf(logTemplateData, args...)
	}
}

// Warnw is a Sugared Logging, allows key value pairs variadic
func (z *ZapLog) Warnw(logMessageData string, keyValuePairs ...interface{}) {
	z.mu.RLock()
	logger := z.sugarLogger
	disabled := z.DisableLogger
	z.mu.RUnlock()

	if logger != nil && !disabled {
		logMessageData = sanitizeLogMessage(logMessageData)
		logger.Warnw(logMessageData, keyValuePairs...)
	}
}

// Warn is faster logging, but requires import of zap package, uses zap.String(), zap.Int(), etc in fields parameters
func (z *ZapLog) Warn(logMessageData string, fields ...zap.Field) {
	z.mu.RLock()
	logger := z.zapLogger
	disabled := z.DisableLogger
	z.mu.RUnlock()

	if logger != nil && !disabled {
		logMessageData = sanitizeLogMessage(logMessageData)
		logger.Warn(logMessageData, fields...)
	}
}

// Errorf is a Sugared Logging, allows template variable such as %s
func (z *ZapLog) Errorf(logTemplateData string, args ...interface{}) {
	z.mu.RLock()
	logger := z.sugarLogger
	disabled := z.DisableLogger
	z.mu.RUnlock()

	if logger != nil && !disabled {
		logTemplateData = sanitizeLogMessage(logTemplateData)
		logger.Errorf(logTemplateData, args...)
	}
}

// Errorw is a Sugared Logging, allows key value pairs variadic
func (z *ZapLog) Errorw(logMessageData string, keyValuePairs ...interface{}) {
	z.mu.RLock()
	logger := z.sugarLogger
	disabled := z.DisableLogger
	z.mu.RUnlock()

	if logger != nil && !disabled {
		logMessageData = sanitizeLogMessage(logMessageData)
		logger.Errorw(logMessageData, keyValuePairs...)
	}
}

// Error is faster logging, but requires import of zap package, uses zap.String(), zap.Int(), etc in fields parameters
func (z *ZapLog) Error(logMessageData string, fields ...zap.Field) {
	z.mu.RLock()
	logger := z.zapLogger
	disabled := z.DisableLogger
	z.mu.RUnlock()

	if logger != nil && !disabled {
		logMessageData = sanitizeLogMessage(logMessageData)
		logger.Error(logMessageData, fields...)
	}
}

// Panicf is a Sugared Logging, allows template variable such as %s
func (z *ZapLog) Panicf(logTemplateData string, args ...interface{}) {
	logTemplateData = sanitizeLogMessage(logTemplateData) // normalize
	z.mu.RLock()
	logger := z.sugarLogger
	z.mu.RUnlock()

	if logger != nil {
		logger.Panicf(logTemplateData, args...)
		return
	}
	panic(fmt.Sprintf(logTemplateData, args...))
}

// Panicw is a Sugared Logging, allows key value pairs variadic
func (z *ZapLog) Panicw(logMessageData string, keyValuePairs ...interface{}) {
	logMessageData = sanitizeLogMessage(logMessageData)
	z.mu.RLock()
	logger := z.sugarLogger
	z.mu.RUnlock()

	if logger != nil {
		logger.Panicw(logMessageData, keyValuePairs...)
		return
	}
	panic(logMessageData)
}

// Panic is faster logging, but requires import of zap package, uses zap.String(), zap.Int(), etc in fields parameters
func (z *ZapLog) Panic(logMessageData string, fields ...zap.Field) {
	logMessageData = sanitizeLogMessage(logMessageData)
	z.mu.RLock()
	logger := z.zapLogger
	z.mu.RUnlock()

	if logger != nil {
		logger.Panic(logMessageData, fields...)
		return
	}
	panic(logMessageData)
}

// Fatalf is a Sugared Logging, allows template variable such as %s
func (z *ZapLog) Fatalf(logTemplateData string, args ...interface{}) {
	logTemplateData = sanitizeLogMessage(logTemplateData) // normalize
	z.mu.RLock()
	logger := z.sugarLogger
	z.mu.RUnlock()

	if logger != nil {
		logger.Fatalf(logTemplateData, args...)
		return
	}
	fmt.Printf(logTemplateData, args...)
	os.Exit(1)
}

// Fatalw is a Sugared Logging, allows key value pairs variadic
func (z *ZapLog) Fatalw(logMessageData string, keyValuePairs ...interface{}) {
	logMessageData = sanitizeLogMessage(logMessageData)
	z.mu.RLock()
	logger := z.sugarLogger
	z.mu.RUnlock()

	if logger != nil {
		logger.Fatalw(logMessageData, keyValuePairs...)
		return
	}
	fmt.Println(logMessageData)
	os.Exit(1)
}

// Fatal is faster logging, but requires import of zap package, uses zap.String(), zap.Int(), etc in fields parameters
func (z *ZapLog) Fatal(logMessageData string, fields ...zap.Field) {
	logMessageData = sanitizeLogMessage(logMessageData)
	z.mu.RLock()
	logger := z.zapLogger
	z.mu.RUnlock()

	if logger != nil {
		logger.Fatal(logMessageData, fields...)
		return
	}
	fmt.Println(logMessageData)
	os.Exit(1)
}
