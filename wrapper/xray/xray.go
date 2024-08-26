package xray

/*
 * Copyright 2020-2023 Aldelo, LP
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

// =================================================================================================================
// AWS CREDENTIAL:
//		use $> aws configure (to set aws access key and secret to target machine)
//		Store AWS Access ID and Secret Key into Default Profile Using '$ aws configure' cli
//
// To Install & Setup AWS CLI on Host:
//		1) https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2-linux.html
//				On Ubuntu, if host does not have zip and unzip:
//					$> sudo apt install zip
//					$> sudo apt install unzip
//				On Ubuntu, to install AWS CLI v2:
//					$> curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
//					$> unzip awscliv2.zip
//					$> sudo ./aws/install
//		2) $> aws configure set region awsRegionName --profile default
// 		3) $> aws configure
//				follow prompts to enter Access ID and Secret Key
//
// AWS Region Name Reference:
//		us-west-2, us-east-1, ap-northeast-1, etc
//		See: https://docs.aws.amazon.com/general/latest/gr/rande.html
// =================================================================================================================

import (
	"context"
	"fmt"
	util "github.com/aldelo/common"
	"github.com/aws/aws-xray-sdk-go/awsplugins/ecs"
	"github.com/aws/aws-xray-sdk-go/header"
	"github.com/aws/aws-xray-sdk-go/xray"
	"net/http"
	"os"
	"sync"
)

//
// aws xray helpers provides some wrapper functions for aws xray service access functionality
//
// aws xray deploy to EC2 (Linux) requires xray daemon, see deploy info below:
// 		https://docs.aws.amazon.com/xray/latest/devguide/xray-daemon.html
//
// aws ec2 / ecs xray daemon install:
//		EC2 (Linux)
//			#!/bin/bash
//			curl https://s3.dualstack.us-east-1.amazonaws.com/aws-xray-assets.us-east-1/xray-daemon/aws-xray-daemon-3.x.deb -o /home/ubuntu/xray.deb
//			sudo apt install /home/ubuntu/xray.deb
//		ECS
//			Create a folder and download the daemon
//				~$ mkdir xray-daemon && cd xray-daemon
//				~/xray-daemon$ curl https://s3.dualstack.us-east-1.amazonaws.com/aws-xray-assets.us-east-1/xray-daemon/aws-xray-daemon-linux-3.x.zip -o ./aws-xray-daemon-linux-3.x.zip
//				~/xray-daemon$ unzip -o aws-xray-daemon-linux-3.x.zip -d .
//			Create a Dockerfile with the following content
//				*~/xray-daemon/Dockerfile*
//				FROM ubuntu:12.04
//				COPY xray /usr/bin/xray-daemon
//				CMD xray-daemon -f /var/log/xray-daemon.log &
//			Build the image
//				~/xray-daemon$ docker build -t xray .
//
// aws xray daemon service launch
//		On ubuntu, after apt install, daemon service starts automatically
//
// aws xray Security Setup
//		IAM-Role-EC2 (The Role Starting EC2)
//			Requires “xray:PutTraceSegments, etc.” Security
//
// aws xray concepts documentation:
//		https://docs.aws.amazon.com/xray/latest/devguide/xray-concepts.html

type XRayParentSegment struct {
	SegmentID string
	TraceID   string
}

// ================================================================================================================
// aws xray helper functions
// ================================================================================================================

// indicates if xray service tracing is on or off
var _xrayServiceOn bool
var _mu sync.RWMutex

// Init will configure xray daemon address and service version
func Init(daemonAddr string, serviceVersion string) error {

	// conditionally load plugin
	if os.Getenv("ENVIRONMENT") == "ECS" {
		ecs.Init()
	}

	if util.LenTrim(daemonAddr) == 0 {
		// if daemon address is not set,
		// use default value
		daemonAddr = "127.0.0.1:2000"
	}

	if util.LenTrim(serviceVersion) == 0 {
		// if service version is not set,
		// use default value
		serviceVersion = "1.2.0"
	}

	return xray.Configure(xray.Config{
		DaemonAddr:     daemonAddr,
		ServiceVersion: serviceVersion,
	})
}

// SetXRayServiceOn turns on xray service for new objects,
// so that wrappers and code supporting xray will start using xray for tracing,
// the service is set to on during its init, open, or connect etc actions,
// existing objects are not affected by this function action
func SetXRayServiceOn() {
	_mu.Lock()
	defer _mu.Unlock()
	_xrayServiceOn = true
}

// SetXRayServiceOff turns off xray service for new objects,
// so that wrappers and code supporting xray will not start using xray for tracing when it is init, connect or open,
// existing objects are not affected by this function action
func SetXRayServiceOff() {
	_mu.Lock()
	defer _mu.Unlock()
	_xrayServiceOn = false
}

// XRayServiceOn returns whether xray tracing service is on or off
func XRayServiceOn() bool {
	_mu.RLock()
	defer _mu.RUnlock()
	return _xrayServiceOn
}

// DisableTracing disables xray tracing
func DisableTracing() {
	_ = os.Setenv("AWS_XRAY_SDK_DISABLED", "TRUE")
}

// EnableTracing re-enables xray tracing
func EnableTracing() {
	_ = os.Setenv("AWS_XRAY_SDK_DISABLED", "FALSE")
}

// GetXRayHeader gets header from http.request,
// if headerName is not specified, defaults to x-amzn-trace-id
func GetXRayHeader(req *http.Request, headerName ...string) *header.Header {
	if req == nil {
		return nil
	} else if req.Header == nil {
		return nil
	}

	headerKey := "X-Amzn-Trace-Id"

	if len(headerName) > 0 && util.LenTrim(headerName[0]) > 0 {
		headerKey = headerName[0]
	}

	return header.FromString(req.Header.Get(headerKey))
}

// GetXRayHttpRequestName returns the segment name based on http request method and path,
// or if segmentNamer is defined, gets the name from http request host
func GetXRayHttpRequestName(req *http.Request, segNamer ...xray.SegmentNamer) string {
	if req == nil {
		return ""
	}

	if len(segNamer) > 0 && segNamer[0] != nil {
		return segNamer[0].Name(req.Host)
	}

	if req.URL != nil {
		return fmt.Sprintf("%s %s%s", req.Method, req.Host, req.URL.Path)
	} else {
		return fmt.Sprintf("%s %s", req.Method, req.Host)
	}
}

// ================================================================================================================
// aws xray helper struct
// ================================================================================================================

// XSegment struct provides wrapper function for xray segment, subsegment, context, and related actions
//
// always use NewSegment() to create XSegment object ptr
type XSegment struct {
	Ctx context.Context
	Seg *xray.Segment

	// indicates if this segment is ready for use
	_segReady bool
}

// XTraceData contains maps of data to add during trace activity
type XTraceData struct {
	Meta        map[string]interface{}
	Annotations map[string]interface{}
	Errors      map[string]error
}

// AddMeta adds xray metadata to trace,
// metadata = key value pair with value of any type, including objects or lists, not indexed,
//
//	used to record data that need to be stored in the trace, but don't need to be indexed for searching
func (t *XTraceData) AddMeta(key string, data interface{}) {
	if util.LenTrim(key) == 0 {
		return
	}

	if data == nil {
		return
	}

	if t.Meta == nil {
		t.Meta = make(map[string]interface{})
	}

	t.Meta[key] = data
}

// AddAnnotation adds xray annotation to trace,
// each trace limits to 50 annotations,
// annotation = key value pair indexed for use with filter expression
func (t *XTraceData) AddAnnotation(key string, data interface{}) {
	if util.LenTrim(key) == 0 {
		return
	}

	if data == nil {
		return
	}

	if t.Annotations == nil {
		t.Annotations = make(map[string]interface{})
	}

	t.Annotations[key] = data
}

// AddError adds xray error to trace,
// Error = client errors (4xx other than 429)
// Fault = server faults (5xx)
// Throttle = throttle errors (429 too many requests)
func (t *XTraceData) AddError(key string, err error) {
	if util.LenTrim(key) == 0 {
		return
	}

	if err == nil {
		return
	}

	if t.Errors == nil {
		t.Errors = make(map[string]error)
	}

	t.Errors[key] = err
}

// NewSubSegmentFromContext begins a new subsegment under the parent segment context,
// context can not be empty, and must contains parent segment info
func NewSubSegmentFromContext(ctx context.Context, serviceNameOrUrl string) *XSegment {
	if util.LenTrim(serviceNameOrUrl) == 0 {
		serviceNameOrUrl = "no.service.name.defined"
	}
	subCtx, seg := xray.BeginSubsegment(ctx, serviceNameOrUrl)

	return &XSegment{
		Ctx:       subCtx,
		Seg:       seg,
		_segReady: true,
	}
}

// NewSegment begins a new segment for a named service or url,
// the context.Background() is used as the base context when creating a new segment
//
// NOTE = ALWAYS CALL CLOSE() to End Segment After Tracing of Segment is Complete
func NewSegment(serviceNameOrUrl string, parentSegment ...*XRayParentSegment) *XSegment {
	if util.LenTrim(serviceNameOrUrl) == 0 {
		serviceNameOrUrl = "no.service.name.defined"
	}

	ctx, seg := xray.BeginSegment(context.Background(), serviceNameOrUrl)

	if seg != nil && len(parentSegment) > 0 {
		if parentSegment[0] != nil {
			seg.ParentID = parentSegment[0].SegmentID
			seg.TraceID = parentSegment[0].TraceID
		}
	}

	return &XSegment{
		Ctx:       ctx,
		Seg:       seg,
		_segReady: true,
	}
}

// NewSegmentNullable returns a new segment for the named service or url, if _xrayServiceOn = true,
// otherwise, nil is returned for *XSegment.
func NewSegmentNullable(serviceNameOrUrl string, parentSegment ...*XRayParentSegment) *XSegment {
	_mu.RLock()
	defer _mu.RUnlock()
	if _xrayServiceOn {
		return NewSegment(serviceNameOrUrl, parentSegment...)
	} else {
		return nil
	}
}

// NewSegmentFromHeader begins a new segment for a named service or url based on http request,
// the http.Request Context is used as the base context when creating a new segment
//
// NOTE = ALWAYS CALL CLOSE() to End Segment After Tracing of Segment is Complete
func NewSegmentFromHeader(req *http.Request, traceHeaderName ...string) *XSegment {
	if req == nil {
		return &XSegment{
			Ctx:       context.Background(),
			Seg:       nil,
			_segReady: false,
		}
	}

	name := GetXRayHttpRequestName(req)

	if util.LenTrim(name) == 0 {
		return &XSegment{
			Ctx:       context.Background(),
			Seg:       nil,
			_segReady: false,
		}
	}

	hdr := GetXRayHeader(req, traceHeaderName...)

	if hdr == nil {
		return &XSegment{
			Ctx:       context.Background(),
			Seg:       nil,
			_segReady: false,
		}
	}

	ctx, seg := xray.NewSegmentFromHeader(req.Context(), name, req, hdr)

	return &XSegment{
		Ctx:       ctx,
		Seg:       seg,
		_segReady: true,
	}
}

// SetParentSegment sets segment's ParentID and TraceID as indicated by input parameters
func (x *XSegment) SetParentSegment(parentID string, traceID string) {
	if x._segReady && x.Seg != nil && x.Ctx != nil {
		x.Seg.ParentID = parentID
		x.Seg.TraceID = traceID
	}
}

// NewSubSegment begins a new subsegment under the parent segment context,
// the subSegmentName defines a descriptive name of this sub segment for tracing,
// subsegment.Close(nil) should be called before its parent segment Close is called
//
// NOTE = ALWAYS CALL CLOSE() to End Segment After Tracing of Segment is Complete
func (x *XSegment) NewSubSegment(subSegmentName string) *XSegment {
	if !x._segReady {
		return &XSegment{
			Ctx:       context.Background(),
			Seg:       nil,
			_segReady: false,
		}
	}

	if util.LenTrim(subSegmentName) == 0 {
		subSegmentName = "no.subsegment.name.defined"
	}

	ctx, seg := xray.BeginSubsegment(x.Ctx, subSegmentName)

	return &XSegment{
		Ctx:       ctx,
		Seg:       seg,
		_segReady: true,
	}
}

// Ready checks if segment is ready for operations
func (x *XSegment) Ready() bool {
	if x.Ctx == nil || x.Seg == nil || !x._segReady {
		return false
	} else {
		return true
	}
}

// Close will close a segment (or subsegment),
// always close subsegments first before closing its parent segment
func (x *XSegment) Close() {
	if x._segReady && x.Seg != nil {
		x.Seg.Close(nil)
	}
}

// Capture wraps xray.Capture, by beginning and closing a subsegment with traceName,
// and synchronously executes the provided executeFunc which contains source application's logic
//
// traceName = descriptive name for the tracing session being tracked
// executeFunc = custom logic to execute within capture tracing context (context is segment context)
// traceData = optional additional data to add to the trace (meta, annotation, error)
func (x *XSegment) Capture(traceName string, executeFunc func() error, traceData ...*XTraceData) {
	if !x._segReady || x.Ctx == nil || x.Seg == nil {
		return
	}

	if executeFunc == nil {
		return
	}

	if util.LenTrim(traceName) == 0 {
		traceName = "no.synchronous.trace.name.defined"
	}

	_ = xray.Capture(x.Ctx, traceName, func(ctx context.Context) error {
		// execute logic
		err := executeFunc()

		// add additional trace data if any to xray
		if len(traceData) > 0 {
			if m := traceData[0]; m != nil {
				if m.Meta != nil && len(m.Meta) > 0 {
					for k, v := range m.Meta {
						_ = xray.AddMetadata(ctx, k, v)
					}
				}

				if m.Annotations != nil && len(m.Annotations) > 0 {
					for k, v := range m.Annotations {
						_ = xray.AddAnnotation(ctx, k, v)
					}
				}

				if m.Errors != nil && len(m.Errors) > 0 {
					for _, v := range m.Errors {
						_ = xray.AddError(ctx, v)
					}
				}
			}
		}

		if s := xray.GetSegment(ctx); s != nil {
			if err != nil {
				s.Error = true
				_ = xray.AddError(ctx, err)
			}
		}

		// always return nil
		return nil
	})
}

// CaptureAsync wraps xray.CaptureAsync, by beginning and closing a subsegment with traceName,
// and Asynchronously executes the provided executeFunc which contains source application's logic
// note = use CaptureAsync when tracing goroutine (rather than Capture)
// note = do not manually call Capture within goroutine to ensure segment is flushed properly
//
// traceName = descriptive name for the tracing session being tracked
// executeFunc = custom logic to execute within capture tracing context (context is segment context)
// traceData = optional additional data to add to the trace (meta, annotation, error)
func (x *XSegment) CaptureAsync(traceName string, executeFunc func() error, traceData ...*XTraceData) {
	if !x._segReady || x.Ctx == nil || x.Seg == nil {
		return
	}

	if executeFunc == nil {
		return
	}

	if util.LenTrim(traceName) == 0 {
		traceName = "no.asynchronous.trace.name.defined"
	}

	xray.CaptureAsync(x.Ctx, traceName, func(ctx context.Context) error {
		// execute logic
		err := executeFunc()

		// add additional trace data if any to xray
		if len(traceData) > 0 {
			if m := traceData[0]; m != nil {
				if m.Meta != nil && len(m.Meta) > 0 {
					for k, v := range m.Meta {
						_ = xray.AddMetadata(ctx, k, v)
					}
				}

				if m.Annotations != nil && len(m.Annotations) > 0 {
					for k, v := range m.Annotations {
						_ = xray.AddAnnotation(ctx, k, v)
					}
				}

				if m.Errors != nil && len(m.Errors) > 0 {
					for _, v := range m.Errors {
						_ = xray.AddError(ctx, v)
					}
				}
			}
		}

		if s := xray.GetSegment(ctx); s != nil && err != nil {
			s.Error = true
			_ = xray.AddError(ctx, err)
		}

		// always return nil
		return nil
	})
}
