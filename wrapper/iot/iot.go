package iot

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
	"sync"

	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aldelo/common/wrapper/xray"
	"github.com/aws/aws-sdk-go-v2/config"
	awsiot "github.com/aws/aws-sdk-go-v2/service/iot"
	"github.com/aws/aws-sdk-go/aws"
)

// ================================================================================================================
// STRUCTS
// ================================================================================================================

// IoT struct encapsulates the AWS IoT access functionality
type IoT struct {
	// define the AWS region that IoT is located at
	AwsRegion awsregion.AWSRegion

	// custom http2 client options
	HttpOptions *awshttp2.HttpClientSettings

	// store IoT client object
	iotClient *awsiot.Client

	_parentSegment *xray.XRayParentSegment

	mu sync.RWMutex
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

func (s *IoT) getClient() *awsiot.Client {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.iotClient
}

func (s *IoT) getParentSegment() *xray.XRayParentSegment {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s._parentSegment
}

// ----------------------------------------------------------------------------------------------------------------
// connection functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the IoT service
func (s *IoT) Connect(parentSegment ...*xray.XRayParentSegment) (err error) {
	if s == nil {
		return errors.New("IoT Connect Failed: " + "IoT receiver is nil")
	}

	s.mu.RLock()
	region := s.AwsRegion
	s.mu.RUnlock()

	if xray.XRayServiceOn() {
		if len(parentSegment) > 0 {
			s.mu.Lock()
			s._parentSegment = parentSegment[0]
			s.mu.Unlock()
		}

		seg := xray.NewSegment("IoT-Connect", s.getParentSegment())
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("IoT-AWS-Region", region)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = s.connectInternal(seg.Ctx)

		return err
	} else {
		return s.connectInternal(context.Background())
	}
}

// Connect will establish a connection to the IoT service
func (s *IoT) connectInternal(ctx context.Context) error {
	if s == nil {
		return errors.New("IoT connectInternal Failed: " + "IoT receiver is nil")
	}

	// clean up prior sqs client reference
	s.mu.Lock()
	s.iotClient = nil
	region := s.AwsRegion
	httpOpts := s.HttpOptions
	if httpOpts == nil {
		httpOpts = new(awshttp2.HttpClientSettings)
		s.HttpOptions = httpOpts
	}
	s.mu.Unlock()

	if !region.Valid() || region == awsregion.UNKNOWN {
		return errors.New("Connect to IoT Failed: (AWS Session Error) " + "Region is Required")
	}

	h2 := &awshttp2.AwsHttp2Client{
		Options: httpOpts,
	}

	httpCli, httpErr := h2.NewHttp2Client()
	if httpErr != nil {
		return errors.New("Connect to IoT Failed: (AWS Session Error) " + "Create Custom http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection
	cfg, err := config.LoadDefaultConfig(ctx, config.WithHTTPClient(httpCli), config.WithRegion(region.String()))
	if err != nil {
		// aws session error
		return errors.New("Connect to IoT Failed: (AWS Session Error) " + err.Error())
	}

	// create cached objects for shared use
	client := awsiot.NewFromConfig(cfg)
	if client == nil {
		return errors.New("Connect to IoT Client Failed: (New IoT Client Connection) " + "Connection Object Nil")
	}

	s.mu.Lock()
	s.iotClient = client
	s.mu.Unlock()

	// connect successful
	return nil
}

// Disconnect will clear iot client
func (s *IoT) Disconnect() {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.iotClient = nil
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (s *IoT) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s._parentSegment = parentSegment
}

// ----------------------------------------------------------------------------------------------------------------
// Policy functions
// ----------------------------------------------------------------------------------------------------------------

func (s *IoT) AttachPolicy(policyName, target string) (err error) {
	if s == nil {
		return errors.New("IoT AttachPolicy Failed: " + "IoT receiver is nil")
	}

	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("IoT-AttachPolicy", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("IoT-AttachPolicy-PolicyName", policyName)
			_ = seg.Seg.AddMetadata("IoT-AttachPolicy-Target", target)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()

	if client == nil {
		err = errors.New("AttachPolicy Failed: " + "IoT Client is Required")
		return err
	}

	if len(policyName) <= 0 {
		err = errors.New("AttachPolicy Failed: " + "Policy Name is Required")
		return err
	}

	if len(target) <= 0 {
		err = errors.New("AttachPolicy Failed: " + "Target is Required")
		return err
	}

	// create input object
	input := &awsiot.AttachPolicyInput{
		PolicyName: aws.String(policyName),
		Target:     aws.String(target),
	}

	if segCtxSet {
		_, err = client.AttachPolicy(segCtx, input)
	} else {
		_, err = client.AttachPolicy(context.Background(), input)
	}

	// evaluate result
	if err != nil {
		return err
	}

	return nil
}
