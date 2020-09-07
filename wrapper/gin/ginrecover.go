package gin

/*
 * Copyright 2020 Aldelo, LP
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
	"github.com/gin-gonic/gin"
	"log"
	"net/http/httputil"
	"github.com/go-errors/errors"
)

// NiceRecovery replaces default recovery, with custom content,
// to be called via gin.New() right after object init
//
// Credit: code based and revised from github.com/ekyoung/gin-nice-recovery
func NiceRecovery(f func(c *gin.Context, err interface{})) gin.HandlerFunc {
	return NiceRecoveryWithWriter(f, gin.DefaultErrorWriter)
}

// NiceRecoveryWithWriter replaces default recovery, with custom content,
// to be called via gin.New() right after object init
//
// Credit: code based and revised from github.com/ekyoung/gin-nice-recovery
func NiceRecoveryWithWriter(f func(c *gin.Context, err interface{}), out io.Writer) gin.HandlerFunc {
	var logger *log.Logger

	if out != nil {
		logger = log.New(out, "\n\n\x1b[31m", log.LstdFlags)
	}

	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				if logger != nil {
					httpRequest, _ := httputil.DumpRequest(c.Request, false)
					goErr := errors.Wrap(err, 3)
					reset := string([]byte{27, 91, 48, 109})
					logger.Printf(fmt.Sprintf("[Recovery] Panic Recovered:\n\n%s %s\n\n%s%s", httpRequest, goErr.Error(), goErr.Stack(), reset))
				}

				f(c, fmt.Errorf("Internal Server Error"))
			}
		}()

		c.Next() // execute all the handlers
	}
}
