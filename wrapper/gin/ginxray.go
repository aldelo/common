package gin

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
	"bytes"
	"log"
	"net/http"
	"strings"

	util "github.com/aldelo/common"
	"github.com/aldelo/common/wrapper/xray"
	awsxray "github.com/aws/aws-xray-sdk-go/xray"
	"github.com/gin-gonic/gin"
)

const X_AMZN_TRACE_ID string = "X-Amzn-Trace-Id"
const X_AMZN_SEG_ID string = "X-Amzn-Seg-Id"
const X_AMZN_TR_ID string = "X-Amzn-Tr-Id"

// XRayMiddleware to trace gin actions with aws xray
//
// if the method call is related to a prior xray segment,
// use Headers "X-Amzn-Seg-Id" and "X-Amzn-Tr-Id" to deliver the parent SegmentID and TraceID to this call stack
func XRayMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c == nil {
			log.Println("!!! XRay Middleware Failed: Gin Context Nil !!!")
			return
		}

		if strings.ToLower(util.Right(c.Request.URL.Path, 7)) != "/health" {
			if seg := xray.NewSegmentFromHeader(c.Request); seg != nil && seg.Ready() {
				// if there were parent segment ID and trace ID, relate it to newly created segment here
				parentSegID := c.GetHeader(X_AMZN_SEG_ID)
				traceID := c.GetHeader(X_AMZN_TR_ID)

				if util.LenTrim(parentSegID) > 0 && util.LenTrim(traceID) > 0 {
					seg.SetParentSegment(parentSegID, traceID)
				}

				// close segment
				defer seg.Close()

				c.Request = c.Request.WithContext(seg.Ctx)

				w := &ResponseBodyWriterInterceptor{
					ResponseWriter: c.Writer,
					RespBody:       &bytes.Buffer{},
				}
				c.Writer = w

				traceRequestData(c, seg.Seg)
				c.Next()
				traceResponseData(c, seg.Seg, w.RespBody)
			} else {
				c.Next()
			}
		} else {
			c.Next()
		}
	}
}

func traceRequestData(c *gin.Context, seg *awsxray.Segment) {
	if c == nil {
		log.Println("!!! XRay Middleware Request Trace Failed: Gin Context Nil !!!")
		return
	}

	if c.Request == nil {
		log.Println("!!! XRay Middleware Request Trace Failed: Gin Context Http Request Nil !!!")
		return
	}

	if c.Request.Header == nil {
		log.Println("!!! XRay Middleware Request Trace Failed: Gin Context Http Request Header Nil !!!")
		return
	}

	if c.Writer == nil {
		log.Println("!!! XRay Middleware Request Trace Failed: Gin Context Http Response Writer Nil !!!")
		return
	}

	if seg == nil {
		log.Println("!!! XRay Middleware Request Trace Failed: XRay Segment Nil !!!")
		return
	}

	req := c.Request

	seg.Lock()

	if segReq := getSegmentRequest(seg); segReq != nil {
		segReq.Method = req.Method

		if req.URL != nil {
			segReq.URL = req.URL.String()
		}

		if xForwardedFor := req.Header.Get("X-Forwarded-For"); util.LenTrim(xForwardedFor) > 0 {
			segReq.XForwardedFor = true
			segReq.ClientIP = util.Trim(strings.Split(xForwardedFor, ",")[0])
		} else {
			segReq.XForwardedFor = false
			segReq.ClientIP = req.RemoteAddr
		}

		segReq.UserAgent = req.UserAgent()

		c.Writer.Header().Set(X_AMZN_TRACE_ID, getAmznTraceHeader(req, seg))

		seg.Unlock()

		reqHdr, _ := util.ReadHttpRequestHeaders(req)
		if reqHdr != nil {
			_ = seg.AddMetadata("Request_Headers", reqHdr)
		} else {
			_ = seg.AddMetadata("Request_Headers", "")
		}

		reqBdy, _ := util.ReadHttpRequestBody(req)
		_ = seg.AddMetadata("Request_Body", string(reqBdy))
	} else {
		seg.Unlock()
	}
}

func getSegmentRequest(seg *awsxray.Segment) *awsxray.RequestData {
	if seg != nil {
		if h := seg.GetHTTP(); h != nil {
			if r := h.GetRequest(); r != nil {
				return r
			}
		}
	}

	return nil
}

func getSegmentResponse(seg *awsxray.Segment) *awsxray.ResponseData {
	if seg != nil {
		if h := seg.GetHTTP(); h != nil {
			if r := h.GetResponse(); r != nil {
				return r
			}
		}
	}

	return nil
}

func getAmznTraceHeader(req *http.Request, seg *awsxray.Segment) string {
	if req == nil {
		log.Println("!!! XRay Middleware Request Trace Failed: (getAmznTraceHeader) Gin Context Http Request Nil !!!")
		return ""
	}

	if req.Header == nil {
		log.Println("!!! XRay Middleware Request Trace Failed: (getAmznTraceHeader) Gin Context Http Request Header Nil !!!")
		return ""
	}

	if seg == nil {
		log.Println("!!! XRay Middleware Request Trace Failed: (getAmznTraceHeader) XRay Segment Nil !!!")
		return ""
	}

	// parse x-amzn-trace-id header parts to trace map
	trace := make(map[string]string)
	hdr := req.Header.Get(X_AMZN_TRACE_ID)

	for _, p := range strings.Split(hdr, ";") {
		kv := strings.SplitN(p, "=", 2)
		k := util.Trim(kv[0])
		v := ""
		if len(kv) > 1 {
			v = util.Trim(kv[1])
		}
		trace[k] = v
	}

	if util.LenTrim(trace["Root"]) > 0 {
		seg.TraceID = trace["Root"]
		seg.RequestWasTraced = true
	}

	if util.LenTrim(trace["Parent"]) > 0 {
		seg.ParentID = trace["Parent"]
	}

	// build outbound header with Root, Parent, Sampled
	parentID := seg.ID
	sampled := "0"
	if seg.Sampled {
		sampled = "1"
	}

	buf := bytes.Buffer{}
	buf.WriteString("Root=")
	buf.WriteString(seg.TraceID)
	if util.LenTrim(parentID) > 0 {
		buf.WriteString(";Parent=")
		buf.WriteString(parentID)
	}
	buf.WriteString(";Sampled=")
	buf.WriteString(sampled)

	return buf.String()
}

func traceResponseData(c *gin.Context, seg *awsxray.Segment, respBody *bytes.Buffer) {
	if c == nil {
		log.Println("!!! XRay Middleware Response Trace Failed: Gin Context Nil !!!")
		return
	}

	if c.Writer == nil {
		log.Println("!!! XRay Middleware Response Trace Failed: Gin Context Http Response Writer Nil !!!")
		return
	}

	if seg == nil {
		log.Println("!!! XRay Middleware Request Trace Failed: XRay Segment Nil !!!")
		return
	}

	status := c.Writer.Status()

	seg.Lock()

	if segResp := getSegmentResponse(seg); segResp != nil {
		segResp.Status = status
		segResp.ContentLength = c.Writer.Size()

		if status >= 400 && status < 500 {
			seg.Error = true
		}

		if status == 429 {
			seg.Throttle = true
		}

		if status >= 500 && status < 600 {
			seg.Fault = true
		}

		seg.Unlock()

		respHdr, _ := util.ParseHttpHeader(c.Writer.Header())
		if respHdr != nil {
			_ = seg.AddMetadata("Response_Headers", respHdr)
		} else {
			_ = seg.AddMetadata("Response_Headers", "")
		}

		if respBody != nil {
			_ = seg.AddMetadata("Response_Body", string(respBody.Bytes()))
		} else {
			_ = seg.AddMetadata("Response_Body", "")
		}
	} else {
		seg.Unlock()
	}
}
