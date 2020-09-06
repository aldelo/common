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
	"github.com/aldelo/common/wrapper/gin/ginjwtsignalgorithm"
	"net/http"
	"time"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
)

// NewGinJwtMiddleware helper method returns a GinJwt struct object ready for setup and use
//
// Realm = (required) Realm name to display to the user
// IdentityKey = (required) IdentityKey defines the key used for storing identity info in jwt claim
// SigningSecretKey = (required) Secret key used for signing
// AuthenticateBindingType = (required) AuthenticateBindingType defines the binding type to use for login form field data
func NewGinJwtMiddleware(realm string, identityKey string, signingSecretKey string, authenticateBindingType ginbindtype.GinBindType) *GinJwt {
	return &GinJwt{
		Realm: realm,
		IdentityKey: identityKey,
		SigningSecretKey: signingSecretKey,
		SigningAlgorithm: ginjwtsignalgorithm.HS256,
		AuthenticateBindingType: authenticateBindingType,
	}
}

// GinJwt struct encapsulates gin authentication and authorization related services, along with jwt handling
//
// *** Basic Setup ***
// Realm = (required) Realm name to display to the user
// IdentityKey = (required) IdentityKey defines the key used for storing identity info in jwt claim
// SigningSecretKey = (required) Secret key used for signing
// SigningAlgorithm = (required) HS256, HS384, HS512, RS256, RS384 or RS512 Optional, default is HS256
// PrivateKeyFile = (optional) Private key file for asymmetric algorithms
// PublicKeyFile = (optional) Public key file for asymmetric algorithms
// TokenValidDuration = (required) Duration that a jwt token is valid. Optional, defaults to one hour. (aka Timeout)
// TokenMaxRefreshDuration = (required) This field allows clients to refresh their token until MaxRefresh has passed,
//										Note that clients can refresh their token in the last moment of MaxRefresh,
//										This means that the maximum validity timespan for a token is TokenTime + MaxRefresh,
//										Optional, defaults to 0 meaning not refreshable
// SendAuthorization = (optional) SendAuthorization allow return authorization header for every request, default = false
// DisableAbort = (optional) Disable abort() of context, default = false
// TokenLookup = (optional) TokenLookup is a string in the form of "<source>:<name>" that is used to extract token from the request,
//							Optional. Default value = "header: Authorization",
//							Possible values:
// 								- "header:<name>"
//								- "query:<name>"
//								- "cookie:<name>"
//								- "param:<name>"
//							Examples:
//								TokenLookup: "header: Authorization, query: token, cookie: jwt",
//								TokenLookup: "query:token",
//								TokenLookup: "cookie:token",
// TokenHeadName = (optional) TokenHeadName is a string in the header. Default value = "Bearer"
//
// *** Cookie Setup ***
// SendCookie = (optional) Optionally return the token as a cookie, default = false
// CookieHttpOnly = (optional) Allow cookies to be accessed client side for development, default = true
// SecureCookie = (optional) Allow insecure cookies for development over http, default = true
// CookieMaxAge = (optional) Duration that a cookie is valid. Optional, by default = Timeout value
// CookieDomain = (optional) Allow cookie domain change for development, default = ""
// CookieName = (optional) CookieName allow cookie name change for development, default = ""
// CookieSameSite = (optional) CookieSameSite allow use http.SameSite cookie param, default = SameSiteDefaultMode
//							   values = default, lax, strict, none
//
// *** Authentication Setup ***
// AuthenticateBindingType = (required) AuthenticateBindingType defines the binding type to use for login form field data
// LoginRoutePath = (required) LoginRoutePath defines the relative path to the gin jwt middleware's built-in LoginHandler action, sets up as POST
// LogoutRoutePath = (optional) LogoutRoutePath defines the relative path to the gin jwt middleware's built-in LogoutHandler action, sets up as POST
// RefreshTokenRoutePath = (optional) RefreshTokenRoutePath defines the relative path to the middleware's built-in RefreshHandler action, sets up as GET
// AuthenticateHandler = (required) AuthenticateHandler func is called by Authenticator,
//							 		receives loginFields for authentication use,
//							 		if authentication succeeds, returns the loggedInCredential object
//							 		(which typically is a user object containing user information logged in)
// AddClaimsHandler = (optional) LoggedInMapClaimsHandler func is called during Authenticator action upon success,
//								 so that this handler when coded can insert jwt additional payload data,
//								    - loggedInCredential = the returning loggedInCredential from LoginHandler,
//								    - identityKeyValue = string value for the named identityKey defined within the struct
// GetIdentityHandler = (optional) GetIdentityHandler func is called when IdentityHandler is triggered,
//								   field values from claims will be parsed and returned via object by the implementation code
// LoginResponseHandler = (optional) Callback function to handle custom login response
// LogoutResponseHandler = (optional) Callback function to handle custom logout response
// RefreshTokenResponseHandler = (optional) Callback function to handle custom token refresh response
//
// *** Authorization Setup ***
// AuthorizerHandler = (optional) AuthorizerHandler func is called during authorization after authentication,
//								  to validate if the current credential has access rights to certain parts of the target site,
//	 							  the loggedInCredential is the object that LoginHandler returns upon successful authentication,
//								     - return value of true indicates authorization success,
//								     - return value of false indicates authorization failure
// UnauthorizedHandler = (optional) UnauthorizedHandler func is called when the authorization is not authorized,
//									this handler will return the unauthorized message content to caller,
//									such as via c.JSON, c.HTML, etc as dictated by the handler implementation process,
//									   - c *gin.Context = context used to return the unauthorized access content
//									   - code / message = unauthorized code and message as given by the web server to respond back to the caller
//
// *** Other Handlers ***
// TimeHandler = (optional) TimeHandler provides the current time,
//							override it to use another time value,
//							useful for testing or if server uses a different time zone than the tokens,
//							default = time.Now()
// NoRouteHandler = (optional) Defines the route handler to execute when no route is encountered
// MiddlewareErrorEvaluator = (optional) HTTP Status messages for when something in the JWT middleware fails,
//										 Check error (e) to determine the appropriate error message
type GinJwt struct {
	// -----------------------------------------------------------------------------------------------------------------
	// gin jwt setup fields
	// -----------------------------------------------------------------------------------------------------------------

	// Realm name to display to the user. Required.
	Realm string

	// IdentityKey defines the key used for storing identity info in jwt claim
	IdentityKey string

	// Secret key used for signing. Required.
	SigningSecretKey string

	// HS256, HS384, HS512, RS256, RS384 or RS512 Optional, default is HS256.
	SigningAlgorithm ginjwtsignalgorithm.GinJwtSignAlgorithm

	// Private key file for asymmetric algorithms
	PrivateKeyFile string

	// Public key file for asymmetric algorithms
	PublicKeyFile string

	// Duration that a jwt token is valid. Optional, defaults to one hour. (aka Timeout)
	TokenValidDuration time.Duration

	// This field allows clients to refresh their token until MaxRefresh has passed.
	// Note that clients can refresh their token in the last moment of MaxRefresh.
	// This means that the maximum validity timespan for a token is TokenTime + MaxRefresh.
	// Optional, defaults to 0 meaning not refreshable.
	TokenMaxRefreshDuration time.Duration

	// SendAuthorization allow return authorization header for every request
	SendAuthorization bool

	// Disable abort() of context.
	DisableAbort bool

	// TokenLookup is a string in the form of "<source>:<name>" that is used to extract token from the request.
	// Optional. Default value "header:Authorization".
	// Possible values:
	// - "header:<name>"
	// - "query:<name>"
	// - "cookie:<name>"
	// - "param:<name>"
	// TokenLookup: "header: Authorization, query: token, cookie: jwt",
	// TokenLookup: "query:token",
	// TokenLookup: "cookie:token",
	TokenLookup string

	// TokenHeadName is a string in the header. Default value is "Bearer"
	// "Bearer"
	TokenHeadName string

	// -----------------------------------------------------------------------------------------------------------------
	// cookie setup
	// -----------------------------------------------------------------------------------------------------------------
	SendCookie bool
	CookieMaxAge time.Duration
	SecureCookie *bool
	CookieHTTPOnly *bool
	CookieDomain string
	CookieName string
	CookieSameSite *http.SameSite

	// -----------------------------------------------------------------------------------------------------------------
	// authentication setup
	// -----------------------------------------------------------------------------------------------------------------

	// AuthenticateBindingType defines the binding type to use for login form field data
	AuthenticateBindingType ginbindtype.GinBindType

	// LoginRoutePath defines the relative path to the middleware's built-in LoginHandler,
	// this route path is setup as POST with the gin engine
	LoginRoutePath string

	// LogoutRoutePath defines the relative path to the middleware's built-in LogoutHandler,
	// this route path is setup as POST with the gin engine
	LogoutRoutePath string

	// RefreshTokenRoutePath defines the relative path to the middleware's built-in RefreshHandler,
	// this route path is setup as GET with the gin engine
	RefreshTokenRoutePath string

	// AuthenticateHandler func is called by Authenticator,
	// receives loginFields for authentication use,
	// if authentication succeeds, returns the loggedInCredential object
	// (which typically is a user object containing user information logged in)
	AuthenticateHandler func(loginFields interface{}) (loggedInCredential interface{})

	// AddClaimsHandler func is called during Authenticator action upon success,
	// so that this handler when coded can insert jwt additional payload data
	//
	// loggedInCredential = the returning loggedInCredential from LoginHandler
	// identityKeyValue = string value for the named identityKey defined within the struct
	AddClaimsHandler func(loggedInCredential interface{}) (identityKeyValue string, claims map[string]interface{})

	// GetIdentityHandler func is called when IdentityHandler is triggered,
	// field values from claims will be parsed and returned via object by the implementation code
	GetIdentityHandler func(claims jwt.MapClaims) interface{}

	// Callback function to handle custom login response
	LoginResponseHandler func(c *gin.Context, statusCode int, token string, expires time.Time)

	// Callback function to handle custom logout response
	LogoutResponseHandler func(c *gin.Context, statusCode int)

	// Callback function to handle custom token refresh response
	RefreshTokenResponseHandler func(c *gin.Context, statusCode int, token string, expires time.Time)

	// -----------------------------------------------------------------------------------------------------------------
	// authorization setup
	// -----------------------------------------------------------------------------------------------------------------

	// AuthorizerHandler func is called during authorization after authentication,
	// to validate if the current credential has access rights to certain parts of the target site,
	// the loggedInCredential is the object that LoginHandler returns upon successful authentication,
	// return value of true indicates authorization success,
	// return value of false indicates authorization failure
	AuthorizerHandler func(loggedInCredential interface{}, c *gin.Context) bool

	// UnauthorizedHandler func is called when the authorization is not authorized,
	// this handler will return the unauthorized message content to caller,
	// such as via c.JSON, c.HTML, etc as dictated by the handler implementation process
	//
	// c *gin.Context = context used to return the unauthorized access content
	// code / message = unauthorized code and message as given by the web server to respond back to the caller
	UnauthorizedHandler func(c *gin.Context, code int, message string)

	// -----------------------------------------------------------------------------------------------------------------
	// other handlers
	// -----------------------------------------------------------------------------------------------------------------

	// TimeFunc provides the current time.
	// override it to use another time value.
	// useful for testing or if server uses a different time zone than the tokens.
	TimeHandler func() time.Time

	// NoRouteHandler is called when no route situation is encountered
	NoRouteHandler func(claims jwt.MapClaims, c *gin.Context)

	// HTTP Status messages for when something in the JWT middleware fails.
	// Check error (e) to determine the appropriate error message.
	MiddlewareErrorEvaluator func(e error, c *gin.Context) string

	// -----------------------------------------------------------------------------------------------------------------
	// local var
	// -----------------------------------------------------------------------------------------------------------------
	_ginJwtMiddleware *jwt.GinJWTMiddleware
}

// BuildGinJwtMiddleware sets up auth jwt middleware for gin web server,
// including adding login, logout, refreshtoken, and other routes where applicable
func (j *GinJwt) BuildGinJwtMiddleware(g *Gin) error {
	j._ginJwtMiddleware = nil

	if g == nil {
		return fmt.Errorf("Gin Wrapper Object is Required")
	}

	if g._ginEngine == nil {
		return fmt.Errorf("Gin Engine is Required")
	}

	if !j.AuthenticateBindingType.Valid() || j.AuthenticateBindingType == ginbindtype.UNKNOWN {
		return fmt.Errorf("Authenticate Binding Type is Required")
	}

	if j.AuthenticateHandler == nil {
		return fmt.Errorf("Authenticate Handler is Required")
	}

	// the jwt middleware
	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{
		Realm: j.Realm,
		IdentityKey: j.IdentityKey,
		Key: []byte(j.SigningSecretKey),
		SigningAlgorithm: j.SigningAlgorithm.Key(),
		PrivKeyFile: j.PrivateKeyFile,
		PubKeyFile: j.PublicKeyFile,
		Timeout: j.TokenValidDuration,
		MaxRefresh: j.TokenMaxRefreshDuration,
		SendAuthorization: j.SendAuthorization,
		DisabledAbort: j.DisableAbort,
		TokenLookup: j.TokenLookup,
		TokenHeadName: j.TokenHeadName,

		// TimeFunc provides the current time.
		// override it to use another time value.
		// useful for testing or if server uses a different time zone than the tokens.
		TimeFunc: time.Now,

		// Callback function that should perform the authentication of the user based on login info.
		// Must return user data as user identifier,
		// it will be stored in Claim Array. Required.
		// Check error (e) to determine the appropriate error message.
		Authenticator: func(c *gin.Context) (interface{}, error) {
			if j.AuthenticateHandler == nil {
				return nil, jwt.ErrMissingAuthenticatorFunc
			}

			// loginFields struct represents the login form fields serialized from context input
			var loginFields struct{}

			if err := g.bindInput(c, j.AuthenticateBindingType, &loginFields); err != nil {
				return nil, jwt.ErrMissingLoginValues
			}

			if loggedInCredential := j.AuthenticateHandler(loginFields); loggedInCredential != nil {
				return loggedInCredential, nil
			} else {
				return nil, jwt.ErrFailedAuthentication
			}
		},

		// Callback function that should perform the authorization of the authenticated user.
		// Called only after an authentication success.
		// Must return true on success, false on failure.
		// Optional, default to success.
		Authorizator: func(data interface{}, c *gin.Context) bool {
			if j.AuthorizerHandler != nil {
				return j.AuthorizerHandler(data, c)
			} else {
				return true
			}
		},

		// UnauthorizedHandler func is called when the authorization is not authorized,
		// this handler will return the unauthorized message content to caller,
		// such as via c.JSON, c.HTML, etc as dictated by the handler implementation process
		//
		// c *gin.Context = context used to return the unauthorized access content
		// code / message = unauthorized code and message as given by the web server to respond back to the caller
		Unauthorized: func(c *gin.Context, code int, message string) {
			if j.UnauthorizedHandler != nil {
				j.UnauthorizedHandler(c, code, message)
			} else {
				c.JSON(code, gin.H{
					"code": code,
					"message": message,
				})
			}
		},
	})

	if err != nil {
		return fmt.Errorf("Setup Gin Jwt Middleware Failed: %s", err.Error())
	}

	if j.AddClaimsHandler != nil {
		// Callback function that will be called during login.
		// Using this function it is possible to add additional payload data to the web token.
		// The data is then made available during requests via c.Get("JWT_PAYLOAD").
		// Note that the payload is not encrypted.
		// The attributes mentioned on jwt.io can't be used as keys for the map.
		// Optional, by default no additional data will be set
		//
		// reserved claims: do not use
		//		iss = issuer of the jwt
		//		sub = subject of the jwt (the user)
		//		aud = audience / recipient for which the jwt is intended
		//		exp = expiration time after which the jwt expires
		//		nbf = not before time which the jwt must not be accepted for processing
		//		iat = issued at time which the jwt was issued, can be used to determine the age of the jwt
		//		jti = jwt id, the unique identifier, used to prevent jwt from being replayed (allows a token to be used only once)
		//		more jwt reserved tokens, see = https://www.iana.org/assignments/jwt/jwt.xhtml#claims
		//
		// notes:
		//		1) data interface{} = this object represents the Authenticator return object (loggedInCredential interface{})
		//		2) internal code can assert the loggedInCredential to the actual struct to retrieve its field values
		//				such as: v, ok := data.(*User)
		authMiddleware.PayloadFunc = func(data interface{}) jwt.MapClaims {
			if j.AddClaimsHandler != nil && data != nil {
				if identVal, customMap := j.AddClaimsHandler(data); util.LenTrim(identVal) > 0 || customMap != nil {
					if customMap == nil {
						return jwt.MapClaims{
							j.IdentityKey: identVal,
						}
					} else {
						if util.LenTrim(identVal) > 0 {
							customMap[j.IdentityKey] = identVal
						}

						return customMap
					}
				} else {
					return nil
				}
			} else {
				return nil
			}
		}
	}

	// Callback function to retrieve the identity info via gin context's jwt claims by identityKey
	if j.GetIdentityHandler != nil {
		authMiddleware.IdentityHandler = func(context *gin.Context) interface{} {
			if j.GetIdentityHandler == nil {
				return nil
			}

			claims := jwt.ExtractClaims(context)

			if claims != nil {
				return j.GetIdentityHandler(claims)
			} else {
				return nil
			}
		}
	}

	// Callback function to handle custom login response,
	// i = status code
	// s = token
	// t = expires
	if j.LoginResponseHandler != nil {
		authMiddleware.LoginResponse = func(context *gin.Context, i int, s string, t time.Time) {
			j.LoginResponseHandler(context, i, s, t)
		}
	}

	// Callback function to handle custom logout response,
	// i = status code
	if j.LogoutResponseHandler != nil {
		authMiddleware.LogoutResponse = func(context *gin.Context, i int) {
			j.LogoutResponseHandler(context, i)
		}
	}

	// Callback function to handle custom token refresh response,
	// i = status code,
	// s = token
	// t = expires
	if j.RefreshTokenResponseHandler != nil {
		authMiddleware.RefreshResponse = func(context *gin.Context, i int, s string, t time.Time) {
			j.RefreshTokenResponseHandler(context, i, s, t)
		}
	}

	// TimeFunc provides the current time.
	// override it to use another time value.
	// useful for testing or if server uses a different time zone than the tokens.
	if j.TimeHandler != nil {
		authMiddleware.TimeFunc = func() time.Time {
			return j.TimeHandler()
		}
	}

	// HTTP Status messages for when something in the JWT middleware fails,
	// Check error (e) to determine the appropriate error message
	if j.MiddlewareErrorEvaluator != nil {
		authMiddleware.HTTPStatusMessageFunc = func(e error, c *gin.Context) string {
			return j.MiddlewareErrorEvaluator(e, c)
		}
	}

	// setup cookie options if applicable
	authMiddleware.SendCookie = j.SendCookie
	authMiddleware.CookieMaxAge = j.TokenValidDuration

	if j.SecureCookie == nil {
		authMiddleware.SecureCookie = true
	} else {
		authMiddleware.SecureCookie = *j.SecureCookie
	}

	if j.CookieHTTPOnly == nil {
		authMiddleware.CookieHTTPOnly = true
	} else {
		authMiddleware.CookieHTTPOnly = *j.CookieHTTPOnly
	}

	authMiddleware.CookieDomain = j.CookieDomain
	authMiddleware.CookieName = j.CookieName

	if j.CookieSameSite == nil {
		authMiddleware.CookieSameSite = http.SameSiteDefaultMode
	} else {
		authMiddleware.CookieSameSite = *j.CookieSameSite
	}

	// now init middleware to set default values if required var not set during setup
	if errInit := authMiddleware.MiddlewareInit(); errInit != nil {
		return fmt.Errorf("Init Gin Jwt Middleware Failed: %s", errInit.Error())
	}

	// setup login route for LoginHandler
	if util.LenTrim(j.LoginRoutePath) > 0 {
		g._ginEngine.POST(j.LoginRoutePath, authMiddleware.LoginHandler)
	}

	// setup logout route for LogoutHandler
	if util.LenTrim(j.LogoutRoutePath) > 0 {
		g._ginEngine.POST(j.LogoutRoutePath, authMiddleware.LogoutHandler)
	}

	// setup refresh token route for RefreshHandler
	if util.LenTrim(j.RefreshTokenRoutePath) > 0 {
		g._ginEngine.GET(j.RefreshTokenRoutePath, authMiddleware.RefreshHandler)
	}

	// setup no route handler
	if j.NoRouteHandler != nil {
		g._ginEngine.NoRoute(authMiddleware.MiddlewareFunc(), func(context *gin.Context) {
			claims := jwt.ExtractClaims(context)
			j.NoRouteHandler(claims, context)
		})
	}

	// setup middleware successful
	j._ginJwtMiddleware = authMiddleware
	return nil
}

// AuthMiddleware returns the GinJwt Middleware HandlerFunc,
// so that it can perform jwt related auth services,
// for all route path defined within the same router group
//
// For example:
//		ginJwt := <Gin Jwt Struct Object Obtained>
//		authGroup := g.Group("/auth")
//		authGroup.Use(ginJwt.AuthMiddleware())
//		authGroup.GET("/hello", ...) // route path within this auth group now secured by AuthMiddleware
func (j *GinJwt) AuthMiddleware() gin.HandlerFunc {
	if j._ginJwtMiddleware != nil {
		return j._ginJwtMiddleware.MiddlewareFunc()
	} else {
		return nil
	}
}

// =====================================================================================================================
// Identity Helper Structs
// =====================================================================================================================

//
// UserLogin is a helper struct for use in authentication,
// this struct represents a common use case for user based login request data,
// however, any other custom struct can be used instead as desired
//
type UserLogin struct {
	Username string		`form:"username" json:"username" binding:"required"`
	Password string		`form:"password" json:"password" binding:"required"`
}

//
// SystemLogin is a helper struct for use in authentication,
// this struct represents a common use case for system based login request data,
// however, any other custom struct can be used instead as desired
//
type SystemLogin struct {
	AccessID string		`form:"accessid" json:"accessid" binding:"required"`
	SecretKey string	`form:"secretkey" json:"secretkey" binding:"required"`
}

//
// UserInfo is a helper struct for use in authentication, authorization and identity,
// this struct represents a common use case for user based identity info,
// however, any other custom struct can be used instead as desired
//
type UserInfo struct {
	UserName string
	FirstName string
	LastName string
	Scopes []string
}

//
// SystemInfo is a helper struct for use in authentication, authorization, and identity,
// this struct represents a common use case for system based identity info,
// however, any other custom struct can be used instead as desired
//
type SystemInfo struct {
	SystemName string
	Scopes []string
}

/*
	Example:

	func helloHandler(c *gin.Context) {
		claims := jwt.ExtractClaims(c)
		user, _ := c.Get(identityKey)

		c.JSON(200, gin.H{
			"userID": claims[identityKey],
			"userName": user.(*User).UserName,
			"text": "Hello World",
		})
	}
*/