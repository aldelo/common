package route53

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
	"errors"
	"net/http"
	"sync"

	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/wrapper/xray"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	awsxray "github.com/aws/aws-xray-sdk-go/xray"
)

// ================================================================================================================
// STRUCTS
// ================================================================================================================

// Route53 struct encapsulates the AWS Route53 partial functionality
type Route53 struct {
	// custom http2 client options
	HttpOptions *awshttp2.HttpClientSettings

	// store aws session object
	sess *session.Session

	// store route 53 object
	r53Client      *route53.Route53
	r53ClientMutex sync.RWMutex

	_parentSegment      *xray.XRayParentSegment
	_parentSegmentMutex sync.RWMutex
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the Route53 service
func (r *Route53) Connect(parentSegment ...*xray.XRayParentSegment) (err error) {
	if r == nil {
		return errors.New("Route53 Connect Failed: (Struct Pointer Nil) " + "Route53 Struct Pointer is Nil")
	}

	if xray.XRayServiceOn() {
		r._parentSegmentMutex.Lock()

		if len(parentSegment) > 0 {
			r._parentSegment = parentSegment[0]
		}

		seg := xray.NewSegment("Route53-Connect", r._parentSegment)

		r._parentSegmentMutex.Unlock()

		defer seg.Close()
		defer func() {
			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = r.connectInternal()

		if err == nil {
			r.r53ClientMutex.RLock()
			awsxray.AWS(r.r53Client.Client)
			r.r53ClientMutex.RUnlock()
		}

		return err
	} else {
		return r.connectInternal()
	}
}

// Connect will establish a connection to the Route53 service
func (r *Route53) connectInternal() error {
	if r == nil {
		return errors.New("Route53 connectInternal Failed: (Struct Pointer Nil) " + "Route53 Struct Pointer is Nil")
	}

	r.r53ClientMutex.Lock()
	defer r.r53ClientMutex.Unlock()

	// clean up prior session reference
	r.sess = nil

	// create custom http2 client if needed
	var httpCli *http.Client
	var httpErr error

	if r.HttpOptions == nil {
		r.HttpOptions = new(awshttp2.HttpClientSettings)
	}

	// use custom http2 client
	h2 := &awshttp2.AwsHttp2Client{
		Options: r.HttpOptions,
	}

	if httpCli, httpErr = h2.NewHttp2Client(); httpErr != nil {
		return errors.New("Connect to Route53 Failed: (AWS Session Error) " + "Create Custom Http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection and keep session object in struct
	if sess, err := session.NewSession(
		&aws.Config{
			HTTPClient: httpCli,
		}); err != nil {
		// aws session error
		return errors.New("Connect To Route53 Failed: (AWS Session Error) " + err.Error())
	} else {
		// aws session obtained
		r.sess = sess

		// create cached objects for shared use
		r.r53Client = route53.New(r.sess)

		if r.r53Client == nil {
			return errors.New("Connect To Route53 Client Failed: (New Route53 Client Connection) " + "Connection Object Nil")
		}

		// session stored to struct
		return nil
	}
}

// Disconnect will disjoin from aws session by clearing it
func (r *Route53) Disconnect() {
	if r == nil {
		return
	}

	r.r53ClientMutex.Lock()
	defer r.r53ClientMutex.Unlock()

	r.r53Client = nil
	r.sess = nil
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (r *Route53) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	if r == nil {
		return
	}

	r._parentSegmentMutex.Lock()
	defer r._parentSegmentMutex.Unlock()

	r._parentSegment = parentSegment
}

// ----------------------------------------------------------------------------------------------------------------
// basic resource record set functions
// ----------------------------------------------------------------------------------------------------------------

// CreateUpdateResourceRecordset will create or update a dns recordset to route53
//
// hostedZoneID = root domain hosted zone id (from aws route 53)
// url = fully qualified domain name url, such as abc.example.com
// ip = recordset IPv4 address
// ttl = 15 - 300 (defaults to 60 if ttl is 0)
// recordType = A (currently only A is supported via this function)
func (r *Route53) CreateUpdateResourceRecordset(hostedZoneID string, url string, ip string, ttl uint, recordType string) (err error) {
	if r == nil {
		return errors.New("Route53 CreateUpdateResourceRecordset Failed: (Struct Pointer Nil) " + "Route53 Struct Pointer is Nil")
	}

	if recordType != "A" {
		return errors.New("Route53 CreateUpdateResourceRecordset Failed: " + "Only 'A' Record Type is Supported")
	}

	var segCtx context.Context
	segCtx = nil

	r._parentSegmentMutex.RLock()
	seg := xray.NewSegmentNullable("Route53-CreateUpdateResourceRecordset", r._parentSegment)
	r._parentSegmentMutex.RUnlock()

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Route53-HostedZoneID", hostedZoneID)
			_ = seg.Seg.AddMetadata("Route53-URL", url)
			_ = seg.Seg.AddMetadata("Route53-IP", ip)
			_ = seg.Seg.AddMetadata("Route53-TTL", ttl)
			_ = seg.Seg.AddMetadata("Route53-RecordType", recordType)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	r.r53ClientMutex.RLock()
	r53Client := r.r53Client
	r.r53ClientMutex.RUnlock()

	// validate
	if r53Client == nil {
		err = errors.New("CreateUpdateResourceRecordset Failed: " + "Route53 Client is Required")
		return err
	}

	if len(hostedZoneID) <= 0 {
		err = errors.New("CreateUpdateResourceRecordset Failed: " + "Hosted Zone ID is Required")
		return err
	}

	if len(url) <= 0 {
		err = errors.New("CreateUpdateResourceRecordset Failed: " + "URL is Required")
		return err
	}

	if len(ip) <= 0 {
		err = errors.New("CreateUpdateResourceRecordset Failed: " + "IP is Required")
		return err
	}

	if ttl == 0 {
		ttl = 60
	} else if ttl < 15 {
		ttl = 15
	} else if ttl > 300 {
		ttl = 300
	}

	if recordType != "A" {
		recordType = "A"
	}

	// create
	input := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String("UPSERT"),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: aws.String(url),
						ResourceRecords: []*route53.ResourceRecord{
							{
								Value: aws.String(ip),
							},
						},
						TTL:  aws.Int64(int64(ttl)),
						Type: aws.String(recordType),
					},
				},
			},
		},
		HostedZoneId: aws.String(hostedZoneID),
	}

	if segCtx == nil {
		_, err = r53Client.ChangeResourceRecordSets(input)
	} else {
		_, err = r53Client.ChangeResourceRecordSetsWithContext(segCtx, input)
	}

	if err != nil {
		err = errors.New("CreateUpdateResourceRecordset Failed: " + err.Error())
		return err
	} else {
		return nil
	}
}

// DeleteResourceRecordset will delete a dns recordset from route53
//
// hostedZoneID = root domain hosted zone id (from aws route 53)
// url = fully qualified domain name url, such as abc.example.com
// ip = recordset IPv4 address
// ttl = 15 - 300 (defaults to 60 if ttl is 0) => must match original ttl used when creating recordset
// recordType = A (currently only A is supported via this function)
func (r *Route53) DeleteResourceRecordset(hostedZoneID string, url string, ip string, ttl uint, recordType string) (err error) {
	if r == nil {
		return errors.New("Route53 DeleteResourceRecordset Failed: (Struct Pointer Nil) " + "Route53 Struct Pointer is Nil")
	}

	if recordType != "A" {
		return errors.New("Route53 DeleteResourceRecordset Failed: " + "Only 'A' Record Type is Supported")
	}

	var segCtx context.Context
	segCtx = nil

	r._parentSegmentMutex.RLock()
	seg := xray.NewSegmentNullable("Route53-DeleteResourceRecordset", r._parentSegment)
	r._parentSegmentMutex.RUnlock()

	if seg != nil {
		segCtx = seg.Ctx

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Route53-HostedZoneID", hostedZoneID)
			_ = seg.Seg.AddMetadata("Route53-URL", url)
			_ = seg.Seg.AddMetadata("Route53-IP", ip)
			_ = seg.Seg.AddMetadata("Route53-TTL", ttl)
			_ = seg.Seg.AddMetadata("Route53-RecordType", recordType)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	r.r53ClientMutex.RLock()
	r53Client := r.r53Client
	r.r53ClientMutex.RUnlock()

	// validate
	if r53Client == nil {
		err = errors.New("DeleteResourceRecordset Failed: " + "Route53 Client is Required")
		return err
	}

	if len(hostedZoneID) <= 0 {
		err = errors.New("DeleteResourceRecordset Failed: " + "Hosted Zone ID is Required")
		return err
	}

	if len(url) <= 0 {
		err = errors.New("DeleteResourceRecordset Failed: " + "URL is Required")
		return err
	}

	if len(ip) <= 0 {
		err = errors.New("DeleteResourceRecordset Failed: " + "IP is Required")
		return err
	}

	if ttl == 0 {
		ttl = 60
	} else if ttl < 15 {
		ttl = 15
	} else if ttl > 300 {
		ttl = 300
	}

	if recordType != "A" {
		recordType = "A"
	}

	// create
	input := &route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String("DELETE"),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: aws.String(url),
						ResourceRecords: []*route53.ResourceRecord{
							{
								Value: aws.String(ip),
							},
						},
						TTL:  aws.Int64(int64(ttl)),
						Type: aws.String(recordType),
					},
				},
			},
		},
		HostedZoneId: aws.String(hostedZoneID),
	}

	if segCtx == nil {
		_, err = r53Client.ChangeResourceRecordSets(input)
	} else {
		_, err = r53Client.ChangeResourceRecordSetsWithContext(segCtx, input)
	}

	if err != nil {
		err = errors.New("DeleteResourceRecordset Failed: " + err.Error())
		return err
	} else {
		return nil
	}
}
