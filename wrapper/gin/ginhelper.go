package gin

import (
	"bytes"
	"fmt"
	util "github.com/aldelo/common"
	"github.com/aldelo/common/rest"
	"github.com/gin-gonic/gin"
)

// response body writer interceptor
type ResponseBodyWriterInterceptor struct {
	gin.ResponseWriter
	RespBody *bytes.Buffer
}

func (r ResponseBodyWriterInterceptor) Write(b []byte) (int, error) {
	r.RespBody.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r ResponseBodyWriterInterceptor) WriteString(s string) (int, error) {
	r.RespBody.WriteString(s)
	return r.ResponseWriter.WriteString(s)
}

// ReCAPTCHAResponseIFace interface
type ReCAPTCHAResponseIFace interface {
	GetReCAPTCHAResponse() string
}

// BindPostDataFailed will return a 412 response to caller via gin context
func BindPostDataFailed(c *gin.Context) {
	if c != nil {
		c.String(412, func() string{
			rawBody, _ := c.GetRawData()
			return fmt.Sprintf(`{"errortype":"bind-post-data","error":"%s"}`, "Bind Post Data to Struct Failed: " + string(rawBody))
		}())
	}
}

// VerifyGoogleReCAPTCHAv2Failed will return a 412 response to caller via gin context
func VerifyGoogleReCAPTCHAv2Failed(c *gin.Context, errInfo string) {
	if c != nil {
		c.String(412, func() string{
			return fmt.Sprintf(`{"errortype":"verify-google-recaptcha-v2","error":"%s"}`, errInfo)
		}())
	}
}

// MarshalQueryParametersFailed will return a 412 response to caller via gin context
func MarshalQueryParametersFailed(c *gin.Context, errInfo string) {
	if c != nil {
		c.String(412, func() string{
			return fmt.Sprintf(`{"errortype":"marshal-query-parameters","error":"%s"}`, errInfo)
		}())
	}
}

// ActionServerFailed will return a 500 response to caller via gin context
func ActionServerFailed(c *gin.Context, errInfo string) {
	if c != nil {
		c.String(500, func() string{
			return fmt.Sprintf(`{"errortype":"action-server-failed","error":"%s"}`, errInfo)
		}())
	}
}

// ActionStatusNotOK will return a 404 response to caller via gin context
func ActionStatusNotOK(c *gin.Context, errInfo string) {
	if c != nil {
		c.String(404, func() string{
			return fmt.Sprintf(`{"errortype":"action-status-not-ok","error":"%s"}`, errInfo)
		}())
	}
}

// VerifyGoogleReCAPTCHAv2 is a helper for use gin web server,
// it will verify the given recaptcha response with the recaptcha secret pass in from gin context,
// and if verify sucessful, nil is returned
func VerifyGoogleReCAPTCHAv2(c *gin.Context, recaptchaResponse string, recaptchaRequired bool) (err error) {
	if c == nil {
		return fmt.Errorf("Verify Google ReCAPTCHA v2 Requires GIN Context")
	}

	if !recaptchaRequired {
		if util.LenTrim(recaptchaResponse) == 0 {
			return nil
		}
	} else {
		if util.LenTrim(recaptchaResponse) == 0 {
			return fmt.Errorf("Google ReCAPTCHA v2 Verification is Required")
		}
	}

	if key, ok := c.Get("google_recaptcha_secret"); ok {
		if success, _, _, e := util.VerifyGoogleReCAPTCHAv2(recaptchaResponse, key.(string)); e != nil {
			return e
		} else {
			if success {
				return nil
			} else {
				return fmt.Errorf("Verify Google ReCAPTCHA v2 Result = Not Successful")
			}
		}
	} else {
		return nil
	}
}

// HandleReCAPTCHAv2 is a helper to simplify handler prep code to bind and perform recaptcha service,
// if false is returned, response is given to caller as failure, further processing stops
// if true is returned, continue with handler code
func HandleReCAPTCHAv2(c *gin.Context, bindingInputPtr interface{}) bool {
	if c == nil {
		return false
	}

	if bindingInputPtr == nil {
		return false
	}

	if r, ok := bindingInputPtr.(ReCAPTCHAResponseIFace); !ok {
		BindPostDataFailed(c)
		return false
	} else {
		if e := VerifyGoogleReCAPTCHAv2(c, r.GetReCAPTCHAResponse(), true); e != nil {
			VerifyGoogleReCAPTCHAv2Failed(c, e.Error())
			return false
		} else {
			// recaptcha verify success
			return true
		}
	}
}

// PostDataToHost will post data from struct pointer object via gin context to target host,
// the tagName and excludeTagName is used for query parameters marshaling
func PostDataToHost(c *gin.Context, structPtr interface{}, tagName string, excludeTagName string, postUrl string) {
	if c != nil {
		if structPtr == nil {
			MarshalQueryParametersFailed(c, "Post Data Struct is Nil")
		} else if util.LenTrim(postUrl) == 0 {
			MarshalQueryParametersFailed(c, "Post Target URL is Required")
		} else {
			if util.LenTrim(tagName) == 0 {
				tagName = "json"
			}

			if qp, e := util.MarshalStructToQueryParams(structPtr, tagName, excludeTagName); e != nil {
				MarshalQueryParametersFailed(c, e.Error())
			} else {
				if status, resp, e := rest.POST(postUrl, nil, qp); e != nil {
					ActionServerFailed(c, e.Error())
				} else if status != 200 {
					ActionStatusNotOK(c, resp)
				} else {
					// success
					c.String(200, resp)
				}
			}
		}
	}
}
