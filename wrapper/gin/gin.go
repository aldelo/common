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
	util "github.com/aldelo/common"
	"github.com/aldelo/common/wrapper/gin/ginbindtype"
	"github.com/aldelo/common/wrapper/gin/ginhttpmethod"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"log"
)

// Gin struct provides a wrapper for gin-gonic based web server operations
type Gin struct {
	// web server descriptive name (used for display and logging only)
	Name string

	// web server port to run gin web server
	Port uint

	// web server routes to handle
	Routes []*Route

	// web server instance
	_ginEngine *gin.Engine
}

// Route struct defines each http route handler endpoint
//
// RelativePath = (required) route path such as /HelloWorld, this is the path to trigger the route handler
// Method = (required) GET, POST, PUT, DELETE
// Binding = (optional) various input data binding to target type option
// Handler = (required) function handler to be executed per method defined (actual logic goes inside handler)
type Route struct {
	// relative path to the endpoint for the route to handle
	RelativePath string

	// http method such as GET, POST, PUT, DELETE
	Method ginhttpmethod.GinHttpMethod

	// input value binding to be performed
	Binding ginbindtype.GinBindType

	// actual handler method to be triggered,
	// bindingInput if any, is the binding resolved object passed into the handler method
	Handler func(c *gin.Context, bindingInput interface{})
}

// NewServer returns a gin-gongic web server wrapper ready for setup
func NewServer() *Gin {
	gin.SetMode(gin.ReleaseMode)

	g := gin.Default()
	g.GET("/health", func(c *gin.Context) {
		c.String(200, "OK")
	})

	return &Gin{
		_ginEngine: g,
	}
}

// RunServer starts gin-gonic web server,
// method will run in go routine so it does not block
// if run server failed, an error is returned
func (g *Gin) RunServer() error {
	if g._ginEngine == nil {
		return fmt.Errorf("Run Web Server Failed: %s", "Server Engine Not Defined")
	}

	if util.LenTrim(g.Name) == 0 {
		return fmt.Errorf("Run Web Server Failed: %s", "Web Server Name Not Defined")
	}

	if g.Port > 65535 {
		return fmt.Errorf("Run Web Server Failed: %s", "Port Number Cannot Exceed 65535")
	}

	// setup routes
	if g.setupRoutes() <= 0 {
		return fmt.Errorf("Run Web Server Failed: %s", "Http Routes Not Defined")
	}

	log.Println("Starting Web Server '" + g.Name + "'...")

	errInfo := make(chan string)

	go func() {
		if err := g._ginEngine.Run(); err != nil {
			errInfo <- fmt.Sprintf("Run Web Server Failed: %s", err.Error())
		}
	}()

	select {
	case s := <-errInfo:
		log.Println("!!! " + s + " !!!")
		return fmt.Errorf(s)
	default:
		log.Println("... Web Server Started")
		return nil
	}
}

// bindInput will attempt to bind input data to target binding output, for example json string to struct mapped to json elements
func (g *Gin) bindInput(c *gin.Context, bindType ginbindtype.GinBindType, bindObj *struct{}) (err error) {
	if c == nil {
		return fmt.Errorf("Binding Context is Nil")
	}

	if !bindType.Valid() || bindType == ginbindtype.UNKNOWN {
		return fmt.Errorf("Binding Type Not Valid")
	}

	if bindObj == nil {
		return fmt.Errorf("Binding Target Object Not Defined")
	}

	switch bindType {
	case ginbindtype.BindHeader:
		err = c.ShouldBindHeader(bindObj)
	case ginbindtype.BindJson:
		err = c.ShouldBindJSON(bindObj)
	case ginbindtype.BindProtoBuf:
		err = c.ShouldBindWith(bindObj, binding.ProtoBuf)
	case ginbindtype.BindQuery:
		err = c.ShouldBindQuery(bindObj)
	case ginbindtype.BindUri:
		err = c.ShouldBindUri(bindObj)
	case ginbindtype.BindXml:
		err = c.ShouldBindXML(bindObj)
	case ginbindtype.BindYaml:
		err = c.ShouldBindYAML(bindObj)
	}

	if err != nil {
		return err
	}

	return nil
}

// setupRoutes will create route handlers from Routes slice,
// and set into web server
func (g *Gin) setupRoutes() int {
	if g.Routes == nil {
		return 0
	}

	count := 0

	for _, v := range g.Routes {
		if v == nil {
			continue
		}

		if !v.Method.Valid() || v.Method == ginhttpmethod.UNKNOWN {
			continue
		}

		if !v.Binding.Valid() {
			continue
		}

		if util.LenTrim(v.RelativePath) == 0 {
			continue
		}

		if v.Handler == nil {
			continue
		}

		// add route
		switch v.Method {
		case ginhttpmethod.GET:
			g._ginEngine.GET(v.RelativePath, func(c *gin.Context){
				// bind input
				if v.Binding != ginbindtype.UNKNOWN {
					// will perform binding
					var bindObj struct{}
					if err := g.bindInput(c, v.Binding, &bindObj); err != nil {
						// binding error
						_ = c.AbortWithError(500, fmt.Errorf("GET %s Failed on %s Binding: %s", v.RelativePath, v.Binding.Key(), err.Error()))
					} else {
						// continue processing
						v.Handler(c, bindObj)
					}
				} else {
					// no binding requested
					v.Handler(c, nil)
				}
			})
		case ginhttpmethod.POST:
			g._ginEngine.POST(v.RelativePath, func(c *gin.Context){
				// bind input
				if v.Binding != ginbindtype.UNKNOWN {
					// will perform binding
					var bindObj struct{}
					if err := g.bindInput(c, v.Binding, &bindObj); err != nil {
						// binding error
						_ = c.AbortWithError(500, fmt.Errorf("POST %s Failed on %s Binding: %s", v.RelativePath, v.Binding.Key(), err.Error()))
					} else {
						// continue processing
						v.Handler(c, bindObj)
					}
				} else {
					// no binding requested
					v.Handler(c, nil)
				}
			})
		case ginhttpmethod.PUT:
			g._ginEngine.PUT(v.RelativePath, func(c *gin.Context){
				// bind input
				if v.Binding != ginbindtype.UNKNOWN {
					// will perform binding
					var bindObj struct{}
					if err := g.bindInput(c, v.Binding, &bindObj); err != nil {
						// binding error
						_ = c.AbortWithError(500, fmt.Errorf("PUT %s Failed on %s Binding: %s", v.RelativePath, v.Binding.Key(), err.Error()))
					} else {
						// continue processing
						v.Handler(c, bindObj)
					}
				} else {
					// no binding requested
					v.Handler(c, nil)
				}
			})
		case ginhttpmethod.DELETE:
			g._ginEngine.DELETE(v.RelativePath, func(c *gin.Context){
				// bind input
				if v.Binding != ginbindtype.UNKNOWN {
					// will perform binding
					var bindObj struct{}
					if err := g.bindInput(c, v.Binding, &bindObj); err != nil {
						// binding error
						_ = c.AbortWithError(500, fmt.Errorf("DELETE %s Failed on %s Binding: %s", v.RelativePath, v.Binding.Key(), err.Error()))
					} else {
						// continue processing
						v.Handler(c, bindObj)
					}
				} else {
					// no binding requested
					v.Handler(c, nil)
				}
			})

		default:
			continue
		}

		// add handler counter
		count++
	}

	return count
}

/*
	List of gin methods:

	c := &gin.Context{}

	c.DefaultQuery()
	c.Query()
	c.QueryArray()
	c.QueryMap()
	c.GetQuery()
	c.GetQueryArray()
	c.GetQueryMap()

	c.DefaultPostForm()
	c.PostForm()
	c.PostFormArray()
	c.PostFormMap()
	c.GetPostForm()
	c.GetPostFormArray()
	c.GetPostFormMap()

	c.MultipartForm()
	c.FormFile()


	c.String()
	c.HTML()
	c.JSON()
	c.JSONP()
	c.PureJSON()
	c.AsciiJSON()
	c.SecureJSON()
	c.XML()
	c.YAML()
	c.ProtoBuf()
	c.Redirect()

	c.Request
	c.ClientIP()
	c.ContentType()
	c.Cookie()
	c.Keys
	c.Params
	c.Param()
	c.Header()
	c.Status()
	c.Writer
	c.Error()
	c.Abort()
	c.AbortWithError()
	c.AbortWithStatus()
	c.AbortWithStatusJSON()

	c.Set()
	c.SetAccepted()
	c.SetCookie()
	c.SetSameSite()

	c.Get()
	c.GetHeader()
	c.GetInt()
	c.GetInt64()
	c.GetFloat64()
	c.GetTime()
	c.GetDuration()
	c.GetBool()
	c.GetString()
	c.GetStringMap()
	c.GetStringMapString()
	c.GetStringMapStringSlice()

	c.Stream()
	c.GetRawData()
	c.SSEvent()


	c.Value()
	c.Copy()
	c.Data()
	c.DataFromReader()
	c.File()
	c.FileAttachment()
	c.FileFromFS()
	c.FullPath()
	c.IsAborted()
	c.IsWebsocket()
	c.Negotiate()
	c.NegotiateFormat()
	c.Render()
	c.SaveUploadedFile()
*/


