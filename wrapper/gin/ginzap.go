package gin

/*
 * Copyright 2020-2021 Aldelo, LP
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
	util "github.com/aldelo/common"
	"go.uber.org/zap"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/aldelo/common/wrapper/zap"
	"github.com/gin-gonic/gin"
)

// NewGinZapMiddleware returns a newly created GinZap struct object
func NewGinZapMiddleware(logName string, outputToConsole bool) *GinZap {
	return &GinZap{
		LogName: logName,
		OutputToConsole: outputToConsole,
	}
}

// GinZap struct defines logger middleware for use with Gin, using Zap logging package,
// CREDIT：this code is based and modified from github.com/gin-contrib/zap
//
// LogName = (required) specifies the log name being written to
// OutputToConsole = (required) specifies if logger writes to console or disk
// TimeFormat = (optional) default = time.RFC3339
// TimeUtc = (optional) default = false
// PanicStack = (optional) when panic, log to include stack
type GinZap struct {
	LogName string
	OutputToConsole bool

	TimeFormat string	// default = time.RFC3339
	TimeUtc bool		// default = false
	PanicStack bool		// default = false

	_zapLog *data.ZapLog
}

// Init will initialize zap logger and prep ginzap struct object for middleware use
func (z *GinZap) Init() error {
	if util.LenTrim(z.LogName) == 0 {
		return fmt.Errorf("Log Name is Required")
	}

	z._zapLog = &data.ZapLog{
		DisableLogger: false,
		OutputToConsole: z.OutputToConsole,
		AppName: z.LogName,
	}

	if err := z._zapLog.Init(); err != nil {
		z._zapLog = nil
		return err
	} else {
		if util.LenTrim(z.TimeFormat) == 0 {
			z.TimeFormat = time.RFC3339
		}
		return nil
	}
}

// NormalLogger returns a gin.HandlerFunc (middleware) that logs requests using uber-go/zap.
//
// Requests with errors are logged using zap.Error().
// Requests without errors are logged using zap.Info().
func (z *GinZap) NormalLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		if z._zapLog == nil {
			c.Next()
			return
		}

		start := time.Now()

		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		c.Next()

		end := time.Now()
		latency := end.Sub(start)

		if z.TimeUtc {
			end = end.UTC()
		}

		hdr := ""

		if c.Request.Header != nil {
			for k, v := range c.Request.Header {
				if len(v) >= 1 {
					hdr += fmt.Sprintf("%s=%s, ", k, v[0])
				}
			}
		}

		if len(c.Errors) > 0 {
			for _, e := range c.Errors.Errors() {
				z._zapLog.Error(e,
					zap.String("method", c.Request.Method),
					zap.String("path", path),
					zap.String("header", hdr),
					zap.String("query", query),
					zap.String("ip", c.ClientIP()),
					zap.String("user-agent", c.Request.UserAgent()),
					zap.String("time", end.Format(z.TimeFormat)),
					zap.Duration("latency", latency))
			}
		} else {
			z._zapLog.Info(path,
				zap.Int("status", c.Writer.Status()),
				zap.String("method", c.Request.Method),
				zap.String("path", path),
				zap.String("header", hdr),
				zap.String("query", query),
				zap.String("ip", c.ClientIP()),
				zap.String("user-agent", c.Request.UserAgent()),
				zap.String("time", end.Format(z.TimeFormat)),
				zap.Duration("latency", latency))
		}
	}
}

// PanicLogger returns a gin.HandlerFunc (middleware)
//
// this logger recovers from any panics and logs requests using uber-go/zap
//
// All errors are logged using zap.Error()
func (z *GinZap) PanicLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if z._zapLog == nil {
				return
			}

			if err := recover(); err != nil {
				// Check for a broken connection, as it is not really a
				// condition that warrants a panic stack trace.
				var brokenPipe bool
				if ne, ok := err.(*net.OpError); ok {
					if se, ok := ne.Err.(*os.SyscallError); ok {
						if strings.Contains(strings.ToLower(se.Error()), "broken pipe") || strings.Contains(strings.ToLower(se.Error()), "connection reset by peer") {
							brokenPipe = true
						}
					}
				}

				httpRequest, _ := httputil.DumpRequest(c.Request, false)
				if brokenPipe {
					z._zapLog.Error(c.Request.URL.Path,
									zap.Any("error", err),
									zap.String("request", string(httpRequest)),
									)

					// If the connection is dead, we can't write a status to it.
					_ = c.Error(err.(error)) // nolint: errcheck
					c.Abort()
					return
				}

				t := time.Now()

				if z.TimeUtc {
					t = t.UTC()
				}

				if z.PanicStack {
					z._zapLog.Error("[Recovery From Panic]",
										zap.Time("time", t),
										zap.Any("error", err),
										zap.String("request", string(httpRequest)),
										zap.String("stack", string(debug.Stack())),
									)
				} else {
					z._zapLog.Error("[Recovery From Panic]",
										zap.Time("time", t),
										zap.Any("error", err),
										zap.String("request", string(httpRequest)),
									)
				}

				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()

		c.Next()
	}
}
