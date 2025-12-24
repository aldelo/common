package textract

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
	"net/http"
	"sync"

	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aldelo/common/wrapper/xray"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/textract"
	"github.com/aws/aws-sdk-go-v2/service/textract/types"
)

// ================================================================================================================
// STRUCTS
// ================================================================================================================

// Textract struct encapsulates the AWS Textract access functionality
type Textract struct {
	// define the AWS region that Textract is located at
	AwsRegion awsregion.AWSRegion

	// custom http2 client options
	HttpOptions *awshttp2.HttpClientSettings

	// store Textract client object
	textractClient *textract.Client

	_parentSegment *xray.XRayParentSegment

	mu sync.RWMutex
}

func (s *Textract) getAwsRegion() awsregion.AWSRegion {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.AwsRegion
}

func (s *Textract) setAwsRegion(r awsregion.AWSRegion) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AwsRegion = r
}

func (s *Textract) getHttpOptions() *awshttp2.HttpClientSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.HttpOptions
}

func (s *Textract) setHttpOptions(o *awshttp2.HttpClientSettings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.HttpOptions = o
}

func (s *Textract) setClient(cli *textract.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.textractClient = cli
}

func (s *Textract) getClient() *textract.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.textractClient
}

func (s *Textract) setParentSegment(p *xray.XRayParentSegment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s._parentSegment = p
}

func (s *Textract) getParentSegment() *xray.XRayParentSegment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s._parentSegment
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the Textract service
func (s *Textract) Connect(parentSegment ...*xray.XRayParentSegment) (err error) {
	if len(parentSegment) > 0 {
		s.setParentSegment(parentSegment[0])
	}

	if xray.XRayServiceOn() {
		seg := xray.NewSegment("Textract-Connect", s.getParentSegment())
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Textract-AWS-Region", s.getAwsRegion())

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = s.connectInternal(seg.Ctx)
		return err
	}

	return s.connectInternal(context.Background())
}

// Connect will establish a connection to the Textract service
func (s *Textract) connectInternal(ctx context.Context) error {
	// clean up prior textract client reference
	s.setClient(nil)

	region := s.getAwsRegion()
	if !region.Valid() || region == awsregion.UNKNOWN {
		return errors.New("Connect to Textract Failed: (AWS Session Error) Region is Required")
	}

	// create custom http2 client if needed
	var httpCli *http.Client
	var httpErr error

	httpOpts := s.getHttpOptions()
	if httpOpts == nil {
		s.setHttpOptions(new(awshttp2.HttpClientSettings))
	}

	// use custom http2 client
	h2 := &awshttp2.AwsHttp2Client{
		Options: s.getHttpOptions(),
	}

	if httpCli, httpErr = h2.NewHttp2Client(); httpErr != nil {
		return errors.New("Connect to Textract Failed: (AWS Session Error) " + "Create Custom http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection
	if cfg, err := config.LoadDefaultConfig(ctx, config.WithHTTPClient(httpCli)); err != nil {
		// aws session error
		return errors.New("Connect to Textract Failed: (AWS Session Error) " + err.Error())
	} else {
		// create cached objects for shared use
		cli := textract.NewFromConfig(cfg)
		if cli == nil {
			return errors.New("Connect to Textract Client Failed: (New Textract Client Connection) " + "Connection Object Nil")
		}
		s.setClient(cli)

		// connect successful
		return nil
	}
}

// Disconnect will clear textract client
func (s *Textract) Disconnect() {
	s.setClient(nil)
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (s *Textract) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	s.setParentSegment(parentSegment)
}

// ----------------------------------------------------------------------------------------------------------------
// Analysis functions
// ----------------------------------------------------------------------------------------------------------------

// Analyzes identity documents for relevant information. This information is
// extracted and returned as IdentityDocumentFields , which records both the
// normalized field and value of the extracted text. Unlike other Amazon Textract
// operations, AnalyzeID doesn't return any Geometry data.
func (s *Textract) AnalyzeID(data []byte) (doc *types.IdentityDocument, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Textract-AnalyzeID", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Textract-AnalyzeID-IdentityFields", doc)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	cli := s.getClient()
	if cli == nil {
		err = errors.New("AnalyzeID Failed: " + "Textract Client is Required")
		return nil, err
	}

	if len(data) <= 0 {
		err = errors.New("AnalyzeID Failed: " + "Document is Required")
		return nil, err
	}

	// create input object
	input := &textract.AnalyzeIDInput{
		DocumentPages: []types.Document{
			{
				Bytes: data,
			},
		},
	}

	// perform action
	var output *textract.AnalyzeIDOutput

	if segCtxSet {
		output, err = cli.AnalyzeID(segCtx, input)
	} else {
		output, err = cli.AnalyzeID(context.Background(), input)
	}

	// evaluate result
	if err != nil {
		return nil, err
	}
	if len(output.IdentityDocuments) == 0 {
		return nil, errors.New("AnalyzeID Failed: " + "No Identity Documents Found")
	}

	return &output.IdentityDocuments[0], nil
}

// Detects text in the input document. Amazon Textract can detect lines of text
// and the words that make up a line of text. The input document must be in one of
// the following image formats: JPEG, PNG, PDF, or TIFF. DetectDocumentText
// returns the detected text in an array of Block objects. Each document page has
// as an associated Block of type PAGE. Each PAGE Block object is the parent of
// LINE Block objects that represent the lines of detected text on a page. A LINE
// Block object is a parent for each word that makes up the line. Words are
// represented by Block objects of type WORD. DetectDocumentText is a synchronous
// operation. To analyze documents asynchronously, use StartDocumentTextDetection .
// For more information, see Document Text Detection (https://docs.aws.amazon.com/textract/latest/dg/how-it-works-detecting.html)
// .
func (s *Textract) DetectDocumentText(data []byte) (blocks []types.Block, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Textract-DetectDocumentText", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Textract-DetectDocumentText-DetectedBlocks", blocks)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	cli := s.getClient()
	if cli == nil {
		err = errors.New("DetectDocumentText Failed: " + "Textract Client is Required")
		return nil, err
	}

	if len(data) <= 0 {
		err = errors.New("DetectDocumentText Failed: " + "Document is Required")
		return nil, err
	}

	// create input object
	input := &textract.DetectDocumentTextInput{
		Document: &types.Document{
			Bytes: data,
		},
	}

	// perform action
	var output *textract.DetectDocumentTextOutput

	if segCtxSet {
		output, err = cli.DetectDocumentText(segCtx, input)
	} else {
		output, err = cli.DetectDocumentText(context.Background(), input)
	}

	// evaluate result
	if err != nil {
		return nil, err
	}
	if len(output.Blocks) == 0 {
		return nil, errors.New("DetectDocumentText Failed: " + "No Blocks Detected")
	}

	return output.Blocks, nil
}
