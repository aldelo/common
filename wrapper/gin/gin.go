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
	"github.com/gin-contrib/cors"
	"github.com/patrickmn/go-cache"
	"golang.org/x/time/rate"
	"log"
	"time"
)

// Gin struct provides a wrapper for gin-gonic based web server operations
//
// Name = (required) web server descriptive display name
// Port = (required) tcp port that this web server will run on
// Routes = (required) map of http route handlers to be registered, middleware to be configured,
//					   for gin engine or route groups,
//					   key = * indicates base gin engine routes, otherwise key refers to routeGroup to be created
type Gin struct {
	// web server descriptive name (used for display and logging only)
	Name string

	// web server port to run gin web server
	Port uint

	// web server routes to handle
	// string = routeGroup path if defined, otherwise, if * refers to base
	Routes map[string][]*RouteDefinition

	// web server instance
	_ginEngine *gin.Engine
	_limiterCache *cache.Cache
}

// Route struct defines each http route handler endpoint
//
// RelativePath = (required) route path such as /HelloWorld, this is the path to trigger the route handler
// Method = (required) GET, POST, PUT, DELETE
// Binding = (optional) various input data binding to target type option
// Handler = (required) function handler to be executed per method defined (actual logic goes inside handler)
//		1) gin.Context Value Return Helpers:
//		      a) c.String(), c.HTML(), c.JSON(), c.JSONP(), c.PureJSON(), c.AsciiJSON(), c.SecureJSON(), c.XML(), c.YAML(),
//		         c.ProtoBuf(), c.Redirect(), c.Data(), c.DataFromReader(), c.Render()
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

// RouteDefinition struct contains per route group or gin engine's handlers and middleware
//
// Routes = (required) one or more route handlers defined for current route group or base engine
// CorsConfig = (optional) current cors middleware to use if setup for current route group or base engine
// MaxLimitConfig = (optional) current max rate limit middleware, controls how many concurrent handlers can process actions for the current route group or base engine
// PerClientIpQpsConfig / BurstConfig / LimiterTTLConfig = (optional) to enable per client Ip QPS limiter middleware, all 3 options must be set
type RouteDefinition struct {
	Routes []*Route
	CorsConfig *cors.Config
	MaxLimitConfig *int
	PerClientIpQpsConfig *int
	PerClientIpBurstConfig *int
	PerClientIpLimiterTTLConfig *time.Duration
	CustomMiddlewareHandlers []gin.HandlerFunc
}

// NewServer returns a gin-gongic web server wrapper ready for setup
func NewServer(name string, port uint, releaseMode bool) *Gin {
	mode := gin.ReleaseMode

	if !releaseMode {
		mode = gin.DebugMode
	}

	gin.SetMode(mode)
	g := gin.Default()

	return &Gin{
		Name: name,
		Port: port,
		_ginEngine: g,
		_limiterCache: cache.New(5*time.Minute, 10*time.Minute),
	}
}

// RunServer starts gin-gonic web server,
// method will run in blocking mode, until gin server exits,
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

	log.Println("Web Server '" + g.Name + "' Started..." + util.GetLocalIP() + ":" + util.UintToStr(g.Port))

	if err := g._ginEngine.Run(fmt.Sprintf(":%d", g.Port)); err != nil {
		return fmt.Errorf("Web Server '" + g.Name + "' Failed To Start: " + err.Error())
	} else {
		log.Println("Web Server '" + g.Name + "' Stopped")
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

// setupRoutes prepares gin engine with route handlers, middleware and etc.
func (g *Gin) setupRoutes() int {
	if g.Routes == nil {
		return 0
	}

	count := 0

	g._ginEngine.GET("/health", func(c *gin.Context) {
		c.String(200, "OK")
	})

	for mk, mv := range g.Routes {
		var rg *gin.RouterGroup

		if mk != "*" {
			// route group
			rg = g._ginEngine.Group(mk)
		}

		routeFn := func() gin.IRoutes{
			if rg == nil {
				return g._ginEngine
			} else {
				return rg
			}
		}

		for _, v := range mv {
			if v == nil {
				continue
			}

			//
			// config any middleware first
			//
			if v.CorsConfig != nil {
				g.setupCorsMiddleware(routeFn(), v.CorsConfig)
			}

			if v.MaxLimitConfig != nil && *v.MaxLimitConfig > 0 {
				g.setupMaxLimitMiddleware(routeFn(), *v.MaxLimitConfig)
			}

			if v.PerClientIpQpsConfig != nil && v.PerClientIpBurstConfig != nil && v.PerClientIpLimiterTTLConfig != nil {
				g.setupPerClientIpQpsMiddleware(routeFn(), *v.PerClientIpQpsConfig, *v.PerClientIpBurstConfig, *v.PerClientIpLimiterTTLConfig)
			}

			if len(v.CustomMiddlewareHandlers) > 0 {
				routeFn().Use(v.CustomMiddlewareHandlers...)
			}

			//
			// setup route handlers
			//
			for _, h := range v.Routes{
				if !h.Method.Valid() || h.Method == ginhttpmethod.UNKNOWN {
					continue
				}

				if !h.Binding.Valid() {
					continue
				}

				if util.LenTrim(h.RelativePath) == 0 {
					continue
				}

				if h.Handler == nil {
					continue
				}

				// add route
				switch h.Method {
				case ginhttpmethod.GET:
					routeFn().GET(h.RelativePath, func(c *gin.Context) {
						// bind input
						if h.Binding != ginbindtype.UNKNOWN {
							// will perform binding
							var bindObj struct{}
							if err := g.bindInput(c, h.Binding, &bindObj); err != nil {
								// binding error
								_ = c.AbortWithError(500, fmt.Errorf("GET %s Failed on %s Binding: %s", h.RelativePath, h.Binding.Key(), err.Error()))
							} else {
								// continue processing
								h.Handler(c, bindObj)
							}
						} else {
							// no binding requested
							h.Handler(c, nil)
						}
					})
				case ginhttpmethod.POST:
					routeFn().POST(h.RelativePath, func(c *gin.Context) {
						// bind input
						if h.Binding != ginbindtype.UNKNOWN {
							// will perform binding
							var bindObj struct{}
							if err := g.bindInput(c, h.Binding, &bindObj); err != nil {
								// binding error
								_ = c.AbortWithError(500, fmt.Errorf("POST %s Failed on %s Binding: %s", h.RelativePath, h.Binding.Key(), err.Error()))
							} else {
								// continue processing
								h.Handler(c, bindObj)
							}
						} else {
							// no binding requested
							h.Handler(c, nil)
						}
					})
				case ginhttpmethod.PUT:
					routeFn().PUT(h.RelativePath, func(c *gin.Context) {
						// bind input
						if h.Binding != ginbindtype.UNKNOWN {
							// will perform binding
							var bindObj struct{}
							if err := g.bindInput(c, h.Binding, &bindObj); err != nil {
								// binding error
								_ = c.AbortWithError(500, fmt.Errorf("PUT %s Failed on %s Binding: %s", h.RelativePath, h.Binding.Key(), err.Error()))
							} else {
								// continue processing
								h.Handler(c, bindObj)
							}
						} else {
							// no binding requested
							h.Handler(c, nil)
						}
					})
				case ginhttpmethod.DELETE:
					routeFn().DELETE(h.RelativePath, func(c *gin.Context) {
						// bind input
						if h.Binding != ginbindtype.UNKNOWN {
							// will perform binding
							var bindObj struct{}
							if err := g.bindInput(c, h.Binding, &bindObj); err != nil {
								// binding error
								_ = c.AbortWithError(500, fmt.Errorf("DELETE %s Failed on %s Binding: %s", h.RelativePath, h.Binding.Key(), err.Error()))
							} else {
								// continue processing
								h.Handler(c, bindObj)
							}
						} else {
							// no binding requested
							h.Handler(c, nil)
						}
					})

				default:
					continue
				}

				// add handler counter
				count++
			}
		}
	}

	return count
}

// setupCorsMiddleware is a helper to setup gin middleware
func (g *Gin) setupCorsMiddleware(rg gin.IRoutes, corsConfig *cors.Config) {
	if rg != nil && corsConfig != nil {
		config := cors.DefaultConfig()

		if len(corsConfig.AllowOrigins) > 0 {
			config.AllowOrigins = corsConfig.AllowOrigins
		}

		if len(corsConfig.AllowMethods) > 0 {
			config.AllowMethods = corsConfig.AllowMethods
		}

		if len(corsConfig.AllowHeaders) > 0 {
			config.AllowHeaders = corsConfig.AllowHeaders
		}

		if len(corsConfig.ExposeHeaders) > 0 {
			config.ExposeHeaders = corsConfig.ExposeHeaders
		}

		if corsConfig.AllowOriginFunc != nil {
			config.AllowOriginFunc = corsConfig.AllowOriginFunc
		}

		if corsConfig.MaxAge > 0 {
			config.MaxAge = corsConfig.MaxAge
		}

		config.AllowAllOrigins = corsConfig.AllowAllOrigins
		config.AllowBrowserExtensions = corsConfig.AllowBrowserExtensions
		config.AllowCredentials = corsConfig.AllowCredentials
		config.AllowFiles = corsConfig.AllowFiles
		config.AllowWebSockets = corsConfig.AllowWebSockets
		config.AllowWildcard = corsConfig.AllowWildcard

		rg.Use(cors.New(config))
	}
}

// setupMaxLimitMiddleware sets up max concurrent handler execution rate limiter middleware
func (g *Gin) setupMaxLimitMiddleware(rg gin.IRoutes, maxLimit int) {
	if rg != nil && maxLimit > 0 {
		rg.Use(func() gin.HandlerFunc {
			s := make(chan struct{}, maxLimit)

			acquire := func() {
				s <- struct{}{}
			}

			release := func() {
				<-s
			}

			return func(c *gin.Context) {
				acquire()
				defer release()
				c.Next()
			}
		}())
	}
}

// setupPerClientIpQpsMiddleware sets up per client ip qps limiter middleware
func (g *Gin) setupPerClientIpQpsMiddleware(rg gin.IRoutes, qps int, burst int, ttl time.Duration) {
	if rg != nil && g._limiterCache != nil && qps > 0 && qps <= 1000000 && burst > 0 && ttl > 0 {
		fn := func(key func(c *gin.Context) string,
				   createLimiter func(c *gin.Context) (*rate.Limiter, time.Duration),
				   abort func(c *gin.Context)) gin.HandlerFunc{
			return func(cc *gin.Context) {
				k := key(cc)
				limiter, ok := g._limiterCache.Get(k)

				if !ok {
					var expire time.Duration
					limiter, expire = createLimiter(cc)
					g._limiterCache.Set(k, limiter, expire)
				}

				ok = limiter.(*rate.Limiter).Allow()

				if !ok {
					abort(cc)
					return
				}

				cc.Next()
			}
		}

		rg.Use(fn(func(c *gin.Context) string {
			return c.ClientIP()
		}, func(c *gin.Context) (*rate.Limiter, time.Duration) {
			n := 1000000 / qps
			return rate.NewLimiter(rate.Every(time.Duration(n)*time.Microsecond), burst), ttl
		}, func(c *gin.Context) {
			c.AbortWithStatus(429) // exceed rate limit request
		})) // code based on github.com/yangxikun/gin-limit-by-key
	}
}

/*

// method_descriptions_only lists most of gin context methods, its method signature, and method descriptions,
// for simpler reference in one place rather having to refer to documentation separately
func method_descriptions_only() {
	c := gin.Context{}

	// -----------------------------------------------------------------------------------------------------------------
	// http handler return value methods
	// -----------------------------------------------------------------------------------------------------------------

	// String writes the given string into the response body.
	c.String(code int, format string, values ...interface{})

	// HTML renders the HTTP template specified by its file name.
	// It also updates the HTTP code and sets the Content-Type as "text/html".
	// See http://golang.org/doc/articles/wiki/
	c.HTML(code int, name string, obj interface{})

	// JSON serializes the given struct as JSON into the response body.
	// It also sets the Content-Type as "application/json".
	c.JSON(code int, obj interface{})

	// JSONP serializes the given struct as JSON into the response body.
	// It add padding to response body to request data from a server residing in a different domain than the client.
	// It also sets the Content-Type as "application/javascript".
	c.JSONP(code int, obj interface{})

	// PureJSON serializes the given struct as JSON into the response body.
	// PureJSON, unlike JSON, does not replace special html characters with their unicode entities.
	c.PureJSON(code int, obj interface{})

	// AsciiJSON serializes the given struct as JSON into the response body with unicode to ASCII string.
	// It also sets the Content-Type as "application/json".
	c.AsciiJSON(code int, obj interface{})

	// SecureJSON serializes the given struct as Secure JSON into the response body.
	// Default prepends "while(1)," to response body if the given struct is array values.
	// It also sets the Content-Type as "application/json".
	c.SecureJSON(code int, obj interface{})

	// XML serializes the given struct as XML into the response body.
	// It also sets the Content-Type as "application/xml".
	c.XML(code int, obj interface{})

	// YAML serializes the given struct as YAML into the response body.
	c.YAML(code int, obj interface{})

	// ProtoBuf serializes the given struct as ProtoBuf into the response body.
	c.ProtoBuf(code int, obj interface{})

	// Redirect returns a HTTP redirect to the specific location.
	c.Redirect(code int, location string)

	// Data writes some data into the body stream and updates the HTTP code.
	c.Data(code int, contentType string, data []byte)

	// DataFromReader writes the specified reader into the body stream and updates the HTTP code.
	c.DataFromReader(code int, contentLength int64, contentType string, reader io.Reader, extraHeaders map[string]string)

	// Render writes the response headers and calls render.Render to render data.
	c.Render(code int, r render.Render)

	// -----------------------------------------------------------------------------------------------------------------
	// request's query-parameter key-value methods
	// -----------------------------------------------------------------------------------------------------------------

	// DefaultQuery returns the keyed url query value if it exists,
	// otherwise it returns the specified defaultValue string.
	// See: Query() and GetQuery() for further information.
	//    GET /?name=Manu&lastname=
	//    c.DefaultQuery("name", "unknown") == "Manu"
	//    c.DefaultQuery("id", "none") == "none"
	//    c.DefaultQuery("lastname", "none") == ""
	c.DefaultQuery(key string, defaultValue string) string

	// Query returns the keyed url query value if it exists,
	// otherwise it returns an empty string `("")`.
	// It is shortcut for `c.Request.URL.Query().Get(key)`
	//    GET /path?id=1234&name=Manu&value=
	//    c.Query("id") == "1234"
	//    c.Query("name") == "Manu"
	//    c.Query("value") == ""
	//    c.Query("wtf") == ""
	c.Query(key string) string

	// QueryArray returns a slice of strings for a given query key.
	// The length of the slice depends on the number of params with the given key.
	c.QueryArray(key string) []string

	// QueryMap returns a map for a given query key.
	c.QueryMap(key string) map[string]string

	// GetQuery is like Query(),
	// it returns the keyed url query value if it exists `(value, true)` (even when the value is an empty string),
	// otherwise it returns `("", false)`. It is shortcut for `c.Request.URL.Query().Get(key)`
	//    GET /?name=Manu&lastname=
	//    ("Manu", true) == c.GetQuery("name")
	//    ("", false) == c.GetQuery("id")
	//    ("", true) == c.GetQuery("lastname")
	c.GetQuery(key string) (string, bool)

	// GetQueryArray returns a slice of strings for a given query key,
	// plus a boolean value whether at least one value exists for the given key.
	c.GetQueryArray(key string) ([]string, bool)

	// GetQueryMap returns a map for a given query key,
	// plus a boolean value whether at least one value exists for the given key.
	c.GetQueryMap(key string) (map[string]string, bool)

	// -----------------------------------------------------------------------------------------------------------------
	// request's form-post key-value methods
	// -----------------------------------------------------------------------------------------------------------------

	// DefaultPostForm returns the specified key from a POST urlencoded form or multipart form when it exists,
	// otherwise it returns the specified defaultValue string.
	// See: PostForm() and GetPostForm() for further information.
	c.DefaultPostForm(key string, defaultValue string) string

	// PostForm returns the specified key from a POST urlencoded form or multipart form when it exists,
	// otherwise it returns an empty string `("")`.
	c.PostForm(key string) string

	// PostFormArray returns a slice of strings for a given form key.
	// The length of the slice depends on the number of params with the given key.
	c.PostFormArray(key string) []string

	// PostFormMap returns a map for a given form key.
	c.PostFormMap(key string) map[string]string

	// GetPostForm is like PostForm(key).
	// It returns the specified key from a POST urlencoded form or multipart form when it exists `(value, true)` (even when the value is an empty string),
	// otherwise it returns ("", false). For example, during a PATCH request to update the user's email:
	//    email=mail@example.com  -->  ("mail@example.com", true) := GetPostForm("email") 	// set email to "mail@example.com"
	//    email=                  -->  ("", true) := GetPostForm("email") 					// set email to ""
	//                            -->  ("", false) := GetPostForm("email") 					// do nothing with email
	c.GetPostForm(key string) (string, bool)

	// GetPostFormArray returns a slice of strings for a given form key,
	// plus a boolean value whether at least one value exists for the given key.
	c.GetPostFormArray(key string) ([]string, bool)

	// GetPostFormMap returns a map for a given form key,
	// plus a boolean value whether at least one value exists for the given key.
	c.GetPostFormMap(key string) (map[string]string, bool)

	// MultipartForm is the parsed multipart form, including file uploads.
	c.MultipartForm() (*multipart.Form, error)

	// FormFile returns the first file for the provided form key.
	c.FormFile(name string) (*multipart.FileHeader, error)

	// -----------------------------------------------------------------------------------------------------------------
	// request abort methods
	// -----------------------------------------------------------------------------------------------------------------

	// Abort prevents pending handlers from being called.
	// Note that this will not stop the current handler.
	// Let's say you have an authorization middleware that validates that the current request is authorized.
	// If the authorization fails (ex: the password does not match),
	// call Abort to ensure the remaining handlers for this request are not called.
	c.Abort()

	// AbortWithError calls `AbortWithStatus()` and `Error()` internally.
	// This method stops the chain, writes the status code and pushes the specified error to `c.Errors`.
	// See Context.Error() for more details.
	c.AbortWithError(code int, err error) *Error

	// AbortWithStatus calls `Abort()` and writes the headers with the specified status code.
	// For example, a failed attempt to authenticate a request could use: context.AbortWithStatus(401).
	c.AbortWithStatus(code int)

	// AbortWithStatusJSON calls `Abort()` and then `JSON` internally.
	// This method stops the chain, writes the status code and return a JSON body.
	// It also sets the Content-Type as "application/json".
	c.AbortWithStatusJSON(code int, jsonObj interface{})

	// -----------------------------------------------------------------------------------------------------------------
	// request and response related objects
	// -----------------------------------------------------------------------------------------------------------------

	// access to the underlying http request object
	c.Request *http.Request

	// access to the underlying http response object
	c.Writer ResponseWriter

	// access to the context keys object
	c.Keys map[string]interface{}

	// access to the context params object
	c.Params Params

	// -----------------------------------------------------------------------------------------------------------------
	// request and response helper methods
	// -----------------------------------------------------------------------------------------------------------------

	// ClientIP implements a best effort algorithm to return the real client IP,
	// it parses X-Real-IP and X-Forwarded-For in order to work properly with reverse-proxies such us: nginx or haproxy.
	// Use X-Forwarded-For before X-Real-Ip as nginx uses X-Real-Ip with the proxy's IP.
	c.ClientIP() string

	// ContentType returns the Content-Type header of the request.
	c.ContentType() string

	// Header is a intelligent shortcut for c.Writer.Header().Set(key, value).
	// It writes a header in the response.
	// If value == "", this method removes the header `c.Writer.Header().Del(key)`
	c.Header(key string, value string)

	// Cookie returns the named cookie provided in the request or ErrNoCookie if not found.
	// And return the named cookie is unescaped.
	// If multiple cookies match the given name, only one cookie will be returned.
	c.Cookie(name string) (string, error)

	// FullPath returns a matched route full path. For not found routes returns an empty string.
	//    router.GET("/user/:id", func(c *gin.Context) {
	//        c.FullPath() == "/user/:id" // true
	//    })
	c.FullPath() string

	// SaveUploadedFile uploads the form file to specific dst.
	c.SaveUploadedFile(file *multipart.FileHeader, dst string) error

	// Status sets the HTTP response code.
	c.Status(code int)

	// Value returns the value associated with this context for key,
	// or nil if no value is associated with key.
	// Successive calls to Value with the same key returns the same result.
	c.Value(key interface{}) interface{}

	// Param returns the value of the URL param.
	// It is a shortcut for c.Params.ByName(key)
	//    router.GET("/user/:id", func(c *gin.Context) {
	//        // a GET request to /user/john
	//        id := c.Param("id") // id == "john"
	//    })
	c.Param(key string) string

	// File writes the specified file into the body stream in a efficient way.
	c.File(filepath string)

	// FileAttachment writes the specified file into the body stream in an efficient way On the client side,
	// the file will typically be downloaded with the given filename
	c.FileAttachment(filepath string, filename string)

	// FileFromFS writes the specified file from http.FileSytem into the body stream in an efficient way.
	c.FileFromFS(filepath string, fs http.FileSystem)

	// Error attaches an error to the current context.
	// The error is pushed to a list of errors.
	// It's a good idea to call Error for each error that occurred during the resolution of a request.
	//
	// A middleware can be used to collect all the errors and push them to a database together,
	// print a log, or append it in the HTTP response.
	// Error will panic if err is nil.
	c.Error(err error) *Error

	// -----------------------------------------------------------------------------------------------------------------
	// set helpers
	// -----------------------------------------------------------------------------------------------------------------

	// Set is used to store a new key/value pair exclusively for this context.
	// It also lazy initializes c.Keys if it was not used previously.
	// c.Set(key string, value interface{})

	// SetAccepted sets Accept header data.
	c.SetAccepted(formats ...string)

	// SetCookie adds a Set-Cookie header to the ResponseWriter's headers.
	// The provided cookie must have a valid Name. Invalid cookies may be silently dropped.
	c.SetCookie(name string, value string, maxAge int, path string, domain string, secure bool, httpOnly bool)

	// SetSameSite with cookie
	c.SetSameSite(samesite http.SameSite)

	// -----------------------------------------------------------------------------------------------------------------
	// get helpers
	// -----------------------------------------------------------------------------------------------------------------

	// Get returns the value for the given key, ie: (value, true).
	// If the value does not exists it returns (nil, false)
	c.Get(key string) (value interface{}, exists bool)

	// GetHeader returns value from request headers.
	c.GetHeader(key string) string

	// GetInt returns the value associated with the key as an integer.
	c.GetInt(key string) (i int)

	// GetInt64 returns the value associated with the key as an integer.
	c.GetInt64(key string) (i64 int64)

	// GetFloat64 returns the value associated with the key as a float64
	c.GetFloat64(key string) (f64 float64)

	// GetTime returns the value associated with the key as time.
	c.GetTime(key string) (t time.Time)

	// GetDuration returns the value associated with the key as a duration.
	c.GetDuration(key string) (d time.Duration)

	// GetBool returns the value associated with the key as a boolean.
	c.GetBool(key string) (b bool)

	// GetString returns the value associated with the key as a string.
	c.GetString(key string) (s string)

	// GetStringMap returns the value associated with the key as a map of interfaces.
	c.GetStringMap(key string) (sm map[string]interface{})

	// GetStringMapString returns the value associated with the key as a map of strings.
	c.GetStringMapString(key string) (sms map[string]string)

	// GetStringMapStringSlice returns the value associated with the key as a map to a slice of strings.
	c.GetStringMapStringSlice(key string) (smss map[string][]string)

	// -----------------------------------------------------------------------------------------------------------------
	// stream helpers
	// -----------------------------------------------------------------------------------------------------------------

	// Stream sends a streaming response and returns a boolean indicates "Is client disconnected in middle of stream"
	c.Stream(step func(w io.Writer) bool) bool

	// GetRawData return stream data.
	c.GetRawData() ([]byte, error)

	// SSEvent writes a Server-Sent Event into the body stream.
	c.SSEvent(name string, message interface{})

	// -----------------------------------------------------------------------------------------------------------------
	// other helpers
	// -----------------------------------------------------------------------------------------------------------------

	// Copy returns a copy of the current context that can be safely used outside the request's scope.
	// This has to be used when the context has to be passed to a goroutine.
	c.Copy() *Context

	// IsAborted returns true if the current context was aborted.
	c.IsAborted() bool

	// IsWebsocket returns true if the request headers
	// indicate that a websocket handshake is being initiated by the client.
	c.IsWebsocket() bool

	// Negotiate calls different Render according acceptable Accept format.
	c.Negotiate(code int, config Negotiate)

	// NegotiateFormat returns an acceptable Accept format.
	c.NegotiateFormat(offered ...string) string
}

*/

/*
	Gin Middleware to Review for Inclusion:
	1) Prometheus Export:
			https://github.com/chenjiandongx/ginprom
			https://github.com/zsais/go-gin-prometheus
	2) Request Response Interceptor:
			https://github.com/averageflow/goscope
			https://github.com/tpkeeper/gin-dump
	3) Session:
			https://github.com/go-session/gin-session
	4) Jwt:
			https://github.com/appleboy/gin-jwt
			https://github.com/ScottHuangZL/gin-jwt-session
	5) OAuth2:
			https://github.com/zalando/gin-oauth2
	6) Templating:
			https://github.com/michelloworld/ez-gin-template
	7) Static Bin
			https://github.com/olebedev/staticbin
	8) Recovery Override:
			https://github.com/ekyoung/gin-nice-recovery
	9) Csrf:
			https://github.com/utrack/gin-csrf

*/


