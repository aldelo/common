package textract

/*
 * Copyright 2020-2024 Aldelo, LP
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
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the Textract service
func (s *Textract) Connect(parentSegment ...*xray.XRayParentSegment) (err error) {
	if xray.XRayServiceOn() {
		if len(parentSegment) > 0 {
			s._parentSegment = parentSegment[0]
		}

		seg := xray.NewSegment("Textract-Connect", s._parentSegment)
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Textract-AWS-Region", s.AwsRegion)

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

// Connect will establish a connection to the Textract service
func (s *Textract) connectInternal(ctx context.Context) error {
	// clean up prior textract client reference
	s.textractClient = nil

	if !s.AwsRegion.Valid() || s.AwsRegion == awsregion.UNKNOWN {
		return errors.New("Connect to Textract Failed: (AWS Session Error) " + "Region is Required")
	}

	// create custom http2 client if needed
	var httpCli *http.Client
	var httpErr error

	if s.HttpOptions == nil {
		s.HttpOptions = new(awshttp2.HttpClientSettings)
	}

	// use custom http2 client
	h2 := &awshttp2.AwsHttp2Client{
		Options: s.HttpOptions,
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
		s.textractClient = textract.NewFromConfig(cfg)

		if s.textractClient == nil {
			return errors.New("Connect to Textract Client Failed: (New Textract Client Connection) " + "Connection Object Nil")
		}

		// connect successful
		return nil
	}
}

// Disconnect will clear textract client
func (s *Textract) Disconnect() {
	s.textractClient = nil
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (s *Textract) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	s._parentSegment = parentSegment
}

// ----------------------------------------------------------------------------------------------------------------
// Analysis functions
// ----------------------------------------------------------------------------------------------------------------

// Analyzes identity documents for relevant information. This information is
// extracted and returned as IdentityDocumentFields , which records both the
// normalized field and value of the extracted text. Unlike other Amazon Textract
// operations, AnalyzeID doesn't return any Geometry data.
func (s *Textract) AnalyzeID(data []byte) (fields map[string]string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Textract-AnalyzeID", s._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Textract-AnalyzeID-IdentityFields", fields)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	if s.textractClient == nil {
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
		output, err = s.textractClient.AnalyzeID(segCtx, input)
	} else {
		output, err = s.textractClient.AnalyzeID(context.Background(), input)
	}

	// evaluate result
	if err != nil {
		return nil, err
	}
	if len(output.IdentityDocuments) == 0 {
		return nil, errors.New("AnalyzeID Failed: " + "No Identity Documents Found")
	}

	ids := map[string]string{}
	for _, val := range output.IdentityDocuments[0].IdentityDocumentFields {
		ids[*val.Type.Text] = *val.ValueDetection.Text
	}
	return ids, nil
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

	seg := xray.NewSegmentNullable("Textract-DetectDocumentText", s._parentSegment)

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
	if s.textractClient == nil {
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
		output, err = s.textractClient.DetectDocumentText(segCtx, input)
	} else {
		output, err = s.textractClient.DetectDocumentText(context.Background(), input)
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
