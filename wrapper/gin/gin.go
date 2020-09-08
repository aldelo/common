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
	"github.com/aldelo/common/wrapper/gin/gingzipcompression"
	"github.com/aldelo/common/wrapper/gin/ginhttpmethod"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-contrib/sessions/redis"
	"github.com/utrack/gin-csrf"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/patrickmn/go-cache"
	"golang.org/x/time/rate"
	"log"
	"time"
)

// Gin struct provides a wrapper for gin-gonic based web server operations
//
// Name = (required) web server descriptive display name
// Port = (required) tcp port that this web server will run on
// TlsCertPemFile / TlsCertKeyFile = (optional) when both are set, web server runs secured mode using tls cert; path to pem and key file
// Routes = (required) map of http route handlers to be registered, middleware to be configured,
//					   for gin engine or route groups,
//					   key = * indicates base gin engine routes, otherwise key refers to routeGroup to be created
// SessionMiddleware = (optional) defines the cookie or redis session middleware to setup for the gin engine
// CsrfMiddleware = (optional) defines the csrf middleware to setup for the gin engine (requires SessionMiddleware setup)
type Gin struct {
	// web server descriptive name (used for display and logging only)
	Name string

	// web server port to run gin web server
	Port uint

	// web server tls certificate pem and key file path
	TlsCertPemFile string
	TlsCertKeyFile string

	// web server routes to handle
	// string = routeGroup path if defined, otherwise, if * refers to base
	Routes map[string][]*RouteDefinition

	// define the session middleware for the gin engine
	SessionMiddleware *SessionConfig

	// define the csrf middleware for the gin engine
	CsrfMiddleware *CsrfConfig

	// define html template renderer
	HtmlTemplateRenderer *GinTemplate

	// define http status error handler
	HttpStatusErrorHandler func(status int, trace string, c *gin.Context)

	// web server instance
	_ginEngine *gin.Engine
	_ginJwtAuth *GinJwt
	_limiterCache *cache.Cache
}

// RouteDefinition struct contains per route group or gin engine's handlers and middleware
//
// Note:
//		1) route definition's map key = * means gin engine; key named refers to Route Group
//
// Routes = (required) one or more route handlers defined for current route group or base engine
// CorsMiddleware = (optional) current cors middleware to use if setup for current route group or base engine
// MaxLimitMiddleware = (optional) current max rate limit middleware, controls how many concurrent handlers can process actions for the current route group or base engine
// PerClientQpsMiddleware = (optional) to enable per client Ip QPS limiter middleware, all 3 options must be set
// UseAuthMiddleware = (optional) to indicate if this route group uses auth middleware
// CustomMiddleware = (optional) slice of additional custom HandleFunc middleware
type RouteDefinition struct {
	Routes []*Route

	CorsMiddleware *cors.Config
	MaxLimitMiddleware *int
	PerClientQpsMiddleware *PerClientQps
	GZipMiddleware *GZipConfig
	UseAuthMiddleware bool
	CustomMiddleware []gin.HandlerFunc
}

// Route struct defines each http route handler endpoint
//
// RelativePath = (required) route path such as /HelloWorld, this is the path to trigger the route handler
// Method = (required) GET, POST, PUT, DELETE
// Binding = (optional) various input data binding to target type option
// BindingInputPtr = (conditional) binding input object pointer, required if binding type is set
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

	// binding input object pointer
	BindingInputPtr interface{}

	// actual handler method to be triggered,
	// bindingInputPtr if any, is the binding resolved object passed into the handler method
	Handler func(c *gin.Context, bindingInputPtr interface{})
}

// PerClientQps defines options for the PerClientQps based rate limit middleware
type PerClientQps struct {
	Qps int
	Burst int
	TTL time.Duration
}

// GZipConfig defines options for the GZip middleware
type GZipConfig struct {
	Compression gingzipcompression.GinGZipCompression
	ExcludedExtensions []string
	ExcludedPaths []string
	ExcludedPathsRegex []string
}

// GetGZipCompression returns gzip compression value
func (gz *GZipConfig) GetGZipCompression() int {
	switch gz.Compression {
	case gingzipcompression.Default:
		return gzip.DefaultCompression
	case gingzipcompression.BestCompression:
		return gzip.BestCompression
	case gingzipcompression.BestSpeed:
		return gzip.BestSpeed
	default:
		return gzip.NoCompression
	}
}

// GetGZipExcludedExtensions return gzip option for excluded extensions if any
func (gz *GZipConfig) GetGZipExcludedExtensions() gzip.Option {
	if len(gz.ExcludedExtensions) > 0 {
		return gzip.WithExcludedExtensions(gz.ExcludedExtensions)
	} else {
		return nil
	}
}

// GetGZipExcludedPaths return gzip option for excluded paths if any
func (gz *GZipConfig) GetGZipExcludedPaths() gzip.Option {
	if len(gz.ExcludedPaths) > 0 {
		return gzip.WithExcludedPaths(gz.ExcludedPaths)
	} else {
		return nil
	}
}

// GetGZipExcludedPathsRegex return gzip option for excluded paths refex if any
func (gz *GZipConfig) GetGZipExcludedPathsRegex() gzip.Option {
	if len(gz.ExcludedPathsRegex) > 0 {
		return gzip.WithExcludedPathsRegexs(gz.ExcludedPathsRegex)
	} else {
		return nil
	}
}

// SessionConfig defines redis or cookie session configuration options
//
// SecretKey = (required) for redis or cookie session, the secret key to use
// SessionNames = (required) for redis or cookie session, defined session names to use by middleware
// RedisMaxIdleConnections = (optional) for redis session, maximum number of idle connections to keep for redis client
// RedisHostAndPort = (optional) the redis endpoint host name and port, in host:port format (if not set, then cookie session is assumed)
//
// To Use Sessions in Handler:
//		[Single Session]
//			session := sessions.Default(c)
//			v := session.Get("xyz")
//			session.Set("xyz", xyz)
//			session.Save()
//		[Multiple Sessions]
// 			sessionA := sessions.DefaultMany(c, "a")
// 			sessionB := sessions.DefaultMany(c, "b")
// 			sessionA.Get("xyz")
//			sessionA.Set("xyz", xyz)
//			sessionA.Save()
type SessionConfig struct {
	SecretKey string
	SessionNames []string
	RedisMaxIdleConnections int
	RedisHostAndPort string
}

// CsrfConfig defines csrf protection middleware options
//
// Secret = (required) csrf secret key used for csrf protection
// ErrorFunc = (required) csrf invalid token error handler
// TokenGetter = (optional) csrf get token action override from default (in case implementation not using default keys)
//						    default gets token from:
//							   - FormValue("_csrf")
//							   - Url.Query().Get("_csrf")
//							   - Header.Get("X-CSRF-TOKEN")
//							   - Header.Get("X-XSRF-TOKEN")
type CsrfConfig struct {
	Secret string
	ErrorFunc func(c *gin.Context)
	TokenGetter func(c *gin.Context) string
}

// NewServer returns a gin-gongic web server wrapper ready for setup
//
// customRecovery = indicates if the gin engine default recovery will be replaced, with one that has more custom render
// customHttpErrorHandler = func to custom handle http error
//
// if gin default logger is to be replaced, it must be replaced via zaplogger parameter,
// zaplogger must be fully setup and passed into NewServer in order for zaplogger replacement to be effective,
// zaplogger will not be setup after gin object is created
func NewServer(name string, port uint, releaseMode bool, customRecovery bool, customHttpErrorHandler func(status int, trace string, c *gin.Context), zaplogger ...*GinZap) *Gin {
	var z *GinZap

	if len(zaplogger) > 0 {
		z = zaplogger[0]
	}

	mode := gin.ReleaseMode

	if !releaseMode {
		mode = gin.DebugMode
	}

	gin.SetMode(mode)

	gw := &Gin{
		Name: name,
		Port: port,
		HttpStatusErrorHandler: customHttpErrorHandler,
		_ginEngine: nil,
		_ginJwtAuth: nil,
		_limiterCache: cache.New(5*time.Minute, 10*time.Minute),
	}

	if z != nil && util.LenTrim(z.LogName) > 0 {
		if err := z.Init(); err == nil {
			gw._ginEngine = gin.New()
			gw._ginEngine.Use(z.NormalLogger())
			gw._ginEngine.Use(z.PanicLogger())
			log.Println("Using Zap Logger...")
		} else {
			if customRecovery {
				gw._ginEngine = gin.New()
				gw._ginEngine.Use(gin.Logger())
				gw._ginEngine.Use(NiceRecovery(func(c *gin.Context, err interface{}) {
					if gw.HttpStatusErrorHandler == nil {
						c.String(500, err.(error).Error())
					} else {
						gw.HttpStatusErrorHandler(500, err.(error).Error(), c)
					}
				}))

				log.Println("Using Custom Recovery...")
			} else {
				gw._ginEngine = gin.Default()
				log.Println("Using Default Recovery, Logger...")
			}
		}
	} else {
		if customRecovery {
			gw._ginEngine = gin.New()
			gw._ginEngine.Use(gin.Logger())
			gw._ginEngine.Use(NiceRecovery(func(c *gin.Context, err interface{}) {
				if gw.HttpStatusErrorHandler == nil {
					c.String(500, err.(error).Error())
				} else {
					gw.HttpStatusErrorHandler(500, err.(error).Error(), c)
				}
			}))
			log.Println("Using Custom Recovery...")
		} else {
			gw._ginEngine = gin.Default()
			log.Println("Using Default Recovery, Logger...")
		}
	}

	return gw
}

// NewAuthMiddleware will create a new jwt auth middleware with basic info provided,
// then this new middleware is set into Gin wrapper internal var,
// this middleware's additional fields and handlers must then be defined by accessing the AuthMiddleware func,
// once middleware's completely prepared, then call the RunServer which automatically builds the auth middleware for use
func (g *Gin) NewAuthMiddleware(realm string, identityKey string, signingSecretKey string, authenticateBinding ginbindtype.GinBindType, setup ...func(j *GinJwt)) bool {
	g._ginJwtAuth = nil

	if g._ginEngine == nil {
		return false
	}

	g._ginJwtAuth = NewGinJwtMiddleware(realm, identityKey, signingSecretKey, authenticateBinding)

	if len(setup) > 0 {
		setup[0](g._ginJwtAuth)
	}

	return true
}

// AuthMiddleware returns the GinJwt struct object built by NewAuthMiddleware,
// prepare the necessary field values and handlers via this method's return object access
func (g *Gin) AuthMiddleware() *GinJwt {
	return g._ginJwtAuth
}

// ExtractJwtClaims will extra jwt claims from context and return via map
func (g *Gin) ExtractJwtClaims(c *gin.Context) map[string]interface{} {
	if g._ginJwtAuth != nil {
		return g._ginJwtAuth.ExtractClaims(c)
	} else {
		return nil
	}
}

// Engine represents the gin engine itself
func (g *Gin) Engine() *gin.Engine {
	if g._ginEngine == nil {
		return nil
	} else {
		return g._ginEngine
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

	// setup html template renderer
	if g.HtmlTemplateRenderer != nil {
		g.setupHtmlTemplateRenderer()
	}

	// setup auth middleware
	if g._ginJwtAuth != nil {
		if err := g._ginJwtAuth.BuildGinJwtMiddleware(g); err != nil {
			return fmt.Errorf("Run Web Server Failed: (%s) %s", "Build Auth Middleware Errored", err.Error())
		}
	}

	// setup routes
	if g.setupRoutes() <= 0 {
		return fmt.Errorf("Run Web Server Failed: %s", "Http Routes Not Defined")
	}

	log.Println("Web Server '" + g.Name + "' Started..." + util.GetLocalIP() + ":" + util.UintToStr(g.Port))

	var err error

	if util.LenTrim(g.TlsCertPemFile) > 0 && util.LenTrim(g.TlsCertKeyFile) > 0 {
		log.Println("Web Server Tls Mode")
		err = g._ginEngine.RunTLS(fmt.Sprintf(":%d", g.Port), g.TlsCertPemFile, g.TlsCertKeyFile)
	} else {
		log.Println("Web Server Non-Tls Mode")
		err = g._ginEngine.Run(fmt.Sprintf(":%d", g.Port))
	}

	if err != nil {
		return fmt.Errorf("Web Server '" + g.Name + "' Failed To Start: " + err.Error())
	} else {
		log.Println("Web Server '" + g.Name + "' Stopped")
		return nil
	}
}

// bindInput will attempt to bind input data to target binding output, for example json string to struct mapped to json elements
//
// bindObjPtr = pointer to the target object, cannot be nil
func (g *Gin) bindInput(c *gin.Context, bindType ginbindtype.GinBindType, bindObjPtr interface{}) (err error) {
	if c == nil {
		return fmt.Errorf("Binding Context is Nil")
	}

	if !bindType.Valid() {
		return fmt.Errorf("Binding Type Not Valid")
	}

	if bindObjPtr == nil {
		return fmt.Errorf("Binding Target Object Pointer Not Defined")
	}

	switch bindType {
	case ginbindtype.BindHeader:
		err = c.ShouldBindHeader(bindObjPtr)
	case ginbindtype.BindJson:
		err = c.ShouldBindJSON(bindObjPtr)
	case ginbindtype.BindProtoBuf:
		err = c.ShouldBindWith(bindObjPtr, binding.ProtoBuf)
	case ginbindtype.BindQuery:
		err = c.ShouldBindQuery(bindObjPtr)
	case ginbindtype.BindUri:
		err = c.ShouldBindUri(bindObjPtr)
	case ginbindtype.BindXml:
		err = c.ShouldBindXML(bindObjPtr)
	case ginbindtype.BindYaml:
		err = c.ShouldBindYAML(bindObjPtr)
	default:
		err = c.ShouldBind(bindObjPtr)
	}

	if err != nil {
		log.Println("Bind Error:", err.Error(), "; Bind Type:", bindType.Key())
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

	// setup health check route
	g._ginEngine.GET("/health", func(c *gin.Context) {
		c.String(200, "OK")
	})

	if g.HttpStatusErrorHandler != nil {
		g._ginEngine.NoRoute(func(context *gin.Context) {
			g.HttpStatusErrorHandler(404, "NoRoute", context)
		})

		g._ginEngine.NoMethod(func(context *gin.Context) {
			g.HttpStatusErrorHandler(404, "NoMethod", context)
		})
	}

	// setup session if configured
	if g.SessionMiddleware != nil {
		g.setupSessionMiddleware()
	}

	// setup csrf if configured
	if g.CsrfMiddleware != nil {
		g.setupCsrfMiddleware()
	}

	// setup routes for engine and route groups
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
			if v.CorsMiddleware != nil {
				g.setupCorsMiddleware(routeFn(), v.CorsMiddleware)
			}

			if v.MaxLimitMiddleware != nil && *v.MaxLimitMiddleware > 0 {
				g.setupMaxLimitMiddleware(routeFn(), *v.MaxLimitMiddleware)
			}

			if v.PerClientQpsMiddleware != nil {
				g.setupPerClientIpQpsMiddleware(routeFn(), v.PerClientQpsMiddleware.Qps, v.PerClientQpsMiddleware.Burst, v.PerClientQpsMiddleware.TTL)
			}

			if v.GZipMiddleware != nil {
				g.setupGZipMiddleware(routeFn(), v.GZipMiddleware)
			}

			if v.UseAuthMiddleware && g._ginJwtAuth != nil && g._ginJwtAuth._ginJwtMiddleware != nil {
				routeFn().Use(g._ginJwtAuth.AuthMiddleware())
				log.Println("Using Jwt Auth Middleware...")
			}

			if len(v.CustomMiddleware) > 0 {
				routeFn().Use(v.CustomMiddleware...)
				log.Println("Using Custom Middleware...")
			}

			//
			// setup route handlers
			//
			for _, h := range v.Routes{
				log.Println("Setting Up Route Handler: " + h.RelativePath)

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
					routeFn().GET(h.RelativePath, g.newRouteFunc(h.RelativePath, h.Method.Key(), h.Binding, h.BindingInputPtr, h.Handler))

				case ginhttpmethod.POST:
					routeFn().POST(h.RelativePath, g.newRouteFunc(h.RelativePath, h.Method.Key(), h.Binding, h.BindingInputPtr, h.Handler))

				case ginhttpmethod.PUT:
					routeFn().PUT(h.RelativePath, g.newRouteFunc(h.RelativePath, h.Method.Key(), h.Binding, h.BindingInputPtr, h.Handler))

				case ginhttpmethod.DELETE:
					routeFn().DELETE(h.RelativePath, g.newRouteFunc(h.RelativePath, h.Method.Key(), h.Binding, h.BindingInputPtr, h.Handler))

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

// newRouteFunc returns closure to route handler setup,
// if we define the route handler within the loop in Route Setup, the handler func were reused (not desired effect),
// however, using closure ensures each relative path uses its own route func
func (g *Gin) newRouteFunc(relativePath string, method string, bindingType ginbindtype.GinBindType, bindingInputPtr interface{},
						   handler func(c *gin.Context, bindingInputPtr interface{})) func(context *gin.Context) {
	return func(c *gin.Context) {
		if bindingInputPtr != nil {
			// will perform binding
			if err := g.bindInput(c, bindingType, bindingInputPtr); err != nil {
				// binding error
				_ = c.AbortWithError(500, fmt.Errorf("%s %s Failed on %s Binding: %s", method, relativePath, bindingType.Key(), err.Error()))
			} else {
				// continue processing
				handler(c, bindingInputPtr)
			}
		} else {
			// no binding requested
			handler(c, nil)
		}
	}
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

		log.Println("Using Cors Middleware...")
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

		log.Println("Using Max Limit Middleware...")
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

		log.Println("Using Per Client Ip Qps Middleware...")
	}
}

// setupGZipMiddleware sets up GZip middleware
func (g *Gin) setupGZipMiddleware(rg gin.IRoutes, gz *GZipConfig) {
	if gz != nil && gz.Compression.Valid() && gz.Compression != gingzipcompression.UNKNOWN {
		c := gz.GetGZipCompression()

		exts := gz.GetGZipExcludedExtensions()
		paths := gz.GetGZipExcludedPaths()
		pregex := gz.GetGZipExcludedPathsRegex()

		opts := []gzip.Option{}

		if exts != nil {
			opts = append(opts, exts)
		}

		if paths != nil {
			opts = append(opts, paths)
		}

		if pregex != nil {
			opts = append(opts, pregex)
		}

		if len(opts) > 0 {
			rg.Use(gzip.Gzip(c, opts...))
		} else {
			rg.Use(gzip.Gzip(c))
		}

		log.Println("Using GZip Middleware...")
	}
}

// setupSessionMiddleware sets up session middleware,
// session is setup on the gin engine level (rather than route groups)
func (g *Gin) setupSessionMiddleware() {
	if g._ginEngine != nil && g.SessionMiddleware != nil && util.LenTrim(g.SessionMiddleware.SecretKey) > 0 && len(g.SessionMiddleware.SessionNames) > 0 {
		var store sessions.Store

		if util.LenTrim(g.SessionMiddleware.RedisHostAndPort) == 0 {
			// cookie store
			store = cookie.NewStore([]byte(g.SessionMiddleware.SecretKey))
		} else {
			// redis store
			size := g.SessionMiddleware.RedisMaxIdleConnections

			if size <= 0 {
				size = 1
			}

			store, _ = redis.NewStore(size, "tcp", g.SessionMiddleware.RedisHostAndPort, "", []byte(g.SessionMiddleware.SecretKey))
		}

		if store != nil {
			if len(g.SessionMiddleware.SessionNames) == 1 {
				g._ginEngine.Use(sessions.Sessions(g.SessionMiddleware.SessionNames[0], store))
			} else {
				g._ginEngine.Use(sessions.SessionsMany(g.SessionMiddleware.SessionNames, store))
			}

			log.Println("Using Session Middleware...")
		}
	}
}

// setupCsrfMiddleware sets up csrf protection middleware,
// this middleware is setup on the gin engine level (rather than route groups),
// this middleware requires gin-contrib/sessions middleware setup and used before setting this up
func (g *Gin) setupCsrfMiddleware() {
	if g._ginEngine != nil && g.SessionMiddleware != nil && g.CsrfMiddleware != nil && util.LenTrim(g.SessionMiddleware.SecretKey) > 0 && len(g.SessionMiddleware.SessionNames) > 0 && util.LenTrim(g.CsrfMiddleware.Secret) > 0 {
		opt := csrf.Options{
			Secret: g.CsrfMiddleware.Secret,
		}

		if g.CsrfMiddleware.ErrorFunc != nil {
			opt.ErrorFunc = g.CsrfMiddleware.ErrorFunc
		} else {
			opt.ErrorFunc = func(c *gin.Context) {
				c.String(400, "CSRF Token Mismatch")
				c.Abort()
			}
		}

		if g.CsrfMiddleware.TokenGetter != nil {
			opt.TokenGetter = g.CsrfMiddleware.TokenGetter
		}

		g._ginEngine.Use(csrf.Middleware(opt))
		log.Println("Using Csrf Middleware...")
	}
}

// setupHtmlTemplateRenderer sets up html template renderer with gin engine
func (g *Gin) setupHtmlTemplateRenderer() {
	if g.HtmlTemplateRenderer != nil {
		if err := g.HtmlTemplateRenderer.LoadHtmlTemplates(); err != nil {
			log.Println("Load Html Template Renderer Failed: " + err.Error())
			return
		}

		if err := g.HtmlTemplateRenderer.SetHtmlRenderer(g); err != nil {
			log.Println("Set Html Template Renderer Failed: " + err.Error())
			return
		}

		log.Println("Html Template Renderer Set...")
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
	Additional Gin Middleware That Can Be Added via CustomMiddleware Slice:

	*) Request Response Interceptor / Tracer:
			https://github.com/averageflow/goscope
			https://github.com/tpkeeper/gin-dump
			https://github.com/gin-contrib/opengintracing
	*) Prometheus Export:
			https://github.com/zsais/go-gin-prometheus
			https://github.com/chenjiandongx/ginprom
	*) OAuth2:
			https://github.com/zalando/gin-oauth2
	*) Static Bin
			https://github.com/olebedev/staticbin
			https://github.com/gin-contrib/static
	*) Server Send Event (SSE):
			https://github.com/gin-contrib/sse
*/


