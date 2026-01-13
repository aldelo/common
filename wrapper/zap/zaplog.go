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

	// clean up
	if z.sugarLogger != nil {
		z.sugarLogger = nil
	}

	if z.zapLogger != nil {
		z.zapLogger = nil
	}

	// init zap logger
	var err error

	if !z.OutputToConsole {
		// log to file
		prod := zap.NewProductionConfig()

		prod.Development = true
		prod.DisableCaller = true

		prod.Encoding = "json"

		prod.OutputPaths = []string{z.AppName + ".log"}
		prod.ErrorOutputPaths = []string{z.AppName + "-internal-err.log"}

		z.zapLogger, err = prod.Build()
	} else {
		// log to console
		z.zapLogger, err = zap.NewProduction()
	}

	if err != nil {
		return errors.New("Init Logger Failed: " + err.Error())
	}

	// init zap sugared logger
	z.sugarLogger = z.zapLogger.Sugar()

	// init success
	return nil
}

// Sync will flush log buffer to disk
func (z *ZapLog) Sync() {
	if z.zapLogger != nil { // allow sync even when DisableLogger is true
		_ = z.zapLogger.Sync()
	}
}

// Printf is alias method to Infof
func (z *ZapLog) Printf(format string, items ...interface{}) {
	z.Infof(format, items...)
}

// Infof is a Sugared Logging, allows template variable such as %s
func (z *ZapLog) Infof(logTemplateData string, args ...interface{}) {
	if z.sugarLogger != nil && !z.DisableLogger {
		z.sugarLogger.Infof(logTemplateData, args...)
	}
}

// Infow is a Sugared Logging, allows key value pairs variadic
func (z *ZapLog) Infow(logMessageData string, keyValuePairs ...interface{}) {
	if z.sugarLogger != nil && !z.DisableLogger {
		logMessageData = strings.ReplaceAll(logMessageData, "\n", "")
		logMessageData = strings.ReplaceAll(logMessageData, "\r", "")
		z.sugarLogger.Infow(logMessageData, keyValuePairs...)
	}
}

// Info is faster Logging, but requires import of zap package, uses zap.String(), zap.Int(), etc in fields parameters
func (z *ZapLog) Info(logMessageData string, fields ...zap.Field) {
	if z.zapLogger != nil && !z.DisableLogger {
		logMessageData = strings.ReplaceAll(logMessageData, "\n", "")
		logMessageData = strings.ReplaceAll(logMessageData, "\r", "")
		z.zapLogger.Info(logMessageData, fields...)
	}
}

// Debugf is a Sugared Logging, allows template variable such as %s
func (z *ZapLog) Debugf(logTemplateData string, args ...interface{}) {
	if z.sugarLogger != nil && !z.DisableLogger {
		z.sugarLogger.Debugf(logTemplateData, args...)
	}
}

// Debugw is a Sugared Logging, allows key value pairs variadic
func (z *ZapLog) Debugw(logMessageData string, keyValuePairs ...interface{}) {
	if z.sugarLogger != nil && !z.DisableLogger {
		logMessageData = strings.ReplaceAll(logMessageData, "\n", "")
		logMessageData = strings.ReplaceAll(logMessageData, "\r", "")
		z.sugarLogger.Debugw(logMessageData, keyValuePairs...)
	}
}

// Debug is faster logging, but requires import of zap package, uses zap.String(), zap.Int(), etc in fields parameters
func (z *ZapLog) Debug(logMessageData string, fields ...zap.Field) {
	if z.zapLogger != nil && !z.DisableLogger {
		logMessageData = strings.ReplaceAll(logMessageData, "\n", "")
		logMessageData = strings.ReplaceAll(logMessageData, "\r", "")
		z.zapLogger.Debug(logMessageData, fields...)
	}
}

// Warnf is a Sugared Logging, allows template variable such as %s
func (z *ZapLog) Warnf(logTemplateData string, args ...interface{}) {
	if z.sugarLogger != nil && !z.DisableLogger {
		z.sugarLogger.Warnf(logTemplateData, args...)
	}
}

// Warnw is a Sugared Logging, allows key value pairs variadic
func (z *ZapLog) Warnw(logMessageData string, keyValuePairs ...interface{}) {
	if z.sugarLogger != nil && !z.DisableLogger {
		logMessageData = strings.ReplaceAll(logMessageData, "\n", "")
		logMessageData = strings.ReplaceAll(logMessageData, "\r", "")
		z.sugarLogger.Warnw(logMessageData, keyValuePairs...)
	}
}

// Warn is faster logging, but requires import of zap package, uses zap.String(), zap.Int(), etc in fields parameters
func (z *ZapLog) Warn(logMessageData string, fields ...zap.Field) {
	if z.zapLogger != nil && !z.DisableLogger {
		logMessageData = strings.ReplaceAll(logMessageData, "\n", "")
		logMessageData = strings.ReplaceAll(logMessageData, "\r", "")
		z.zapLogger.Warn(logMessageData, fields...)
	}
}

// Errorf is a Sugared Logging, allows template variable such as %s
func (z *ZapLog) Errorf(logTemplateData string, args ...interface{}) {
	if z.sugarLogger != nil && !z.DisableLogger {
		z.sugarLogger.Errorf(logTemplateData, args...)
	}
}

// Errorw is a Sugared Logging, allows key value pairs variadic
func (z *ZapLog) Errorw(logMessageData string, keyValuePairs ...interface{}) {
	if z.sugarLogger != nil && !z.DisableLogger {
		logMessageData = strings.ReplaceAll(logMessageData, "\n", "")
		logMessageData = strings.ReplaceAll(logMessageData, "\r", "")
		z.sugarLogger.Errorw(logMessageData, keyValuePairs...)
	}
}

// Error is faster logging, but requires import of zap package, uses zap.String(), zap.Int(), etc in fields parameters
func (z *ZapLog) Error(logMessageData string, fields ...zap.Field) {
	if z.zapLogger != nil && !z.DisableLogger {
		logMessageData = strings.ReplaceAll(logMessageData, "\n", "")
		logMessageData = strings.ReplaceAll(logMessageData, "\r", "")
		z.zapLogger.Error(logMessageData, fields...)
	}
}

// Panicf is a Sugared Logging, allows template variable such as %s
func (z *ZapLog) Panicf(logTemplateData string, args ...interface{}) {
	if z.sugarLogger != nil {
		z.sugarLogger.Panicf(logTemplateData, args...)
		return
	}
	// preserve panic behavior even when logger is disabled or uninitialized
	panic(fmt.Sprintf(logTemplateData, args...))
}

// Panicw is a Sugared Logging, allows key value pairs variadic
func (z *ZapLog) Panicw(logMessageData string, keyValuePairs ...interface{}) {
	logMessageData = sanitizeLogMessage(logMessageData) // use helper
	if z.sugarLogger != nil {
		z.sugarLogger.Panicw(logMessageData, keyValuePairs...)
		return
	}
	// preserve panic behavior
	panic(logMessageData)
}

// Panic is faster logging, but requires import of zap package, uses zap.String(), zap.Int(), etc in fields parameters
func (z *ZapLog) Panic(logMessageData string, fields ...zap.Field) {
	logMessageData = sanitizeLogMessage(logMessageData) // use helper
	if z.zapLogger != nil {
		z.zapLogger.Panic(logMessageData, fields...)
		return
	}
	// preserve panic behavior
	panic(logMessageData)
}

// Fatalf is a Sugared Logging, allows template variable such as %s
func (z *ZapLog) Fatalf(logTemplateData string, args ...interface{}) {
	if z.sugarLogger != nil {
		z.sugarLogger.Fatalf(logTemplateData, args...)
		return
	}
	// preserve fatal behavior even when logger is disabled or uninitialized
	fmt.Printf(logTemplateData, args...)
	os.Exit(1)
}

// Fatalw is a Sugared Logging, allows key value pairs variadic
func (z *ZapLog) Fatalw(logMessageData string, keyValuePairs ...interface{}) {
	logMessageData = sanitizeLogMessage(logMessageData) // use helper
	if z.sugarLogger != nil {
		z.sugarLogger.Fatalw(logMessageData, keyValuePairs...)
		return
	}
	// preserve fatal behavior
	fmt.Println(logMessageData)
	os.Exit(1)
}

// Fatal is faster logging, but requires import of zap package, uses zap.String(), zap.Int(), etc in fields parameters
func (z *ZapLog) Fatal(logMessageData string, fields ...zap.Field) {
	logMessageData = sanitizeLogMessage(logMessageData) // use helper
	if z.zapLogger != nil {
		z.zapLogger.Fatal(logMessageData, fields...)
		return
	}
	// preserve fatal behavior
	fmt.Println(logMessageData)
	os.Exit(1)
}
