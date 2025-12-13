package bedrockruntime

import (
	"context"
	"errors"
	"sync"

	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aldelo/common/wrapper/xray"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go/aws"
)

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

// ================================================================================================================
// STRUCTS
// ================================================================================================================

// BedrockRuntime struct encapsulates the AWS BedrockRuntime access functionality
type BedrockRuntime struct {
	// define the AWS region that BedrockRuntime is located at
	AwsRegion awsregion.AWSRegion

	// custom http2 client options
	HttpOptions *awshttp2.HttpClientSettings

	// store BedrockRuntime client object
	bedrockruntimeClient *bedrockruntime.Client

	_parentSegment *xray.XRayParentSegment
	mu             sync.RWMutex
}

// internal helpers
func (s *BedrockRuntime) getClient() *bedrockruntime.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bedrockruntimeClient
}

func (s *BedrockRuntime) getParentSegment() *xray.XRayParentSegment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s._parentSegment
}

func (s *BedrockRuntime) getOptions() (*awshttp2.HttpClientSettings, awsregion.AWSRegion) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.HttpOptions, s.AwsRegion
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the BedrockRuntime service
func (s *BedrockRuntime) Connect(parentSegment ...*xray.XRayParentSegment) (err error) {
	if xray.XRayServiceOn() {
		if len(parentSegment) > 0 {
			s.mu.Lock()
			s._parentSegment = parentSegment[0]
			s.mu.Unlock()
		}

		seg := xray.NewSegment("BedrockRuntime-Connect", s.getParentSegment())
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("BedrockRuntime-AWS-Region", s.AwsRegion)

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

// Connect will establish a connection to the BedrockRuntime service
func (s *BedrockRuntime) connectInternal(ctx context.Context) error {
	// snapshot options/region under lock
	httpOpts, region := s.getOptions()

	// clean up prior client reference
	s.mu.Lock()
	s.bedrockruntimeClient = nil
	s.mu.Unlock()

	if !region.Valid() || region == awsregion.UNKNOWN {
		return errors.New("Connect to BedrockRuntime Failed: (AWS Session Error) " + "Region is Required")
	}

	// create custom http2 client if needed
	if httpOpts == nil {
		httpOpts = new(awshttp2.HttpClientSettings)
	}

	h2 := &awshttp2.AwsHttp2Client{
		Options: httpOpts,
	}

	httpCli, httpErr := h2.NewHttp2Client()
	if httpErr != nil {
		return errors.New("Connect to BedrockRuntime Failed: (AWS Session Error) " + "Create Custom http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection with custom http client and region
	cfg, err := config.LoadDefaultConfig(ctx, config.WithHTTPClient(httpCli), config.WithRegion(region.Key()))
	if err != nil {
		// aws session error
		return errors.New("Connect to BedrockRuntime Failed: (AWS Session Error) " + err.Error())
	}

	client := bedrockruntime.NewFromConfig(cfg)
	if client == nil {
		return errors.New("Connect to BedrockRuntime Client Failed: (New BedrockRuntime Client Connection) " + "Connection Object Nil")
	}

	s.mu.Lock()
	s.HttpOptions = httpOpts
	s.bedrockruntimeClient = client
	s.mu.Unlock()

	return nil
}

// Disconnect will clear bedrockruntime client
func (s *BedrockRuntime) Disconnect() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bedrockruntimeClient = nil
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (s *BedrockRuntime) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s._parentSegment = parentSegment
}

// ----------------------------------------------------------------------------------------------------------------
// Invoke functions
// ----------------------------------------------------------------------------------------------------------------

// Invokes the specified Amazon Bedrock model to run inference using the prompt
// and inference parameters provided in the request body. You use model inference
// to generate text, images, and embeddings. For example code, see Invoke model
// code examples in the Amazon Bedrock User Guide. This operation requires
// permission for the bedrock:InvokeModel action.
func (s *BedrockRuntime) InvokeModel(modelId string, requestBody []byte) (responseBody []byte, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("BedrockRuntime-InvokeModel", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("BedrockRuntime-InvokeModel-ResponseBody", responseBody)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("InvokeModel Failed: " + "BedrockRuntime Client is Required")
		return nil, err
	}

	if len(modelId) <= 0 {
		err = errors.New("InvokeModel Failed: " + "Model ID is Required")
		return nil, err
	}

	if len(requestBody) <= 0 {
		err = errors.New("InvokeModel Failed: " + "Request Body is Required")
		return nil, err
	}

	// create input object
	input := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(modelId),
		Body:        requestBody,
		Accept:      aws.String("application/json"),
		ContentType: aws.String("application/json"),
	}

	// perform action
	var output *bedrockruntime.InvokeModelOutput

	if segCtxSet {
		output, err = client.InvokeModel(segCtx, input)
	} else {
		output, err = client.InvokeModel(context.Background(), input)
	}

	// evaluate result
	if err != nil {
		return nil, err
	}
	if len(output.Body) == 0 {
		return nil, errors.New("InvokeModel Failed: " + "No Response Body Returned")
	}

	return output.Body, nil
}
