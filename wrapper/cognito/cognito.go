package cognito

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

	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aldelo/common/wrapper/xray"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	awscognito "github.com/aws/aws-sdk-go/service/cognitoidentity"
	awsxray "github.com/aws/aws-xray-sdk-go/xray"
)

// ================================================================================================================
// STRUCTS
// ================================================================================================================

// Cognito struct encapsulates the AWS Cognito access functionality
type Cognito struct {
	// define the AWS region that Cognito is located at
	AwsRegion awsregion.AWSRegion

	// custom http2 client options
	HttpOptions *awshttp2.HttpClientSettings

	// store Cognito client object
	cognitoClient *awscognito.CognitoIdentity

	_parentSegment *xray.XRayParentSegment
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// connection functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the Cognito service
func (s *Cognito) Connect(parentSegment ...*xray.XRayParentSegment) (err error) {
	if xray.XRayServiceOn() {
		if len(parentSegment) > 0 {
			s._parentSegment = parentSegment[0]
		}

		seg := xray.NewSegment("Cognito-Connect", s._parentSegment)
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cognito-AWS-Region", s.AwsRegion)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = s.connectInternal()

		if err == nil {
			awsxray.AWS(s.cognitoClient.Client)
		}

		return err
	} else {
		return s.connectInternal()
	}
}

// Connect will establish a connection to the Cognito service
func (s *Cognito) connectInternal() error {
	// clean up prior sqs client reference
	s.cognitoClient = nil

	if !s.AwsRegion.Valid() || s.AwsRegion == awsregion.UNKNOWN {
		return errors.New("Connect to Cognito Failed: (AWS Session Error) " + "Region is Required")
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
		return errors.New("Connect to Cognito Failed: (AWS Session Error) " + "Create Custom http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection
	if sess, err := session.NewSession(
		&aws.Config{
			Region:     aws.String(s.AwsRegion.Key()),
			HTTPClient: httpCli,
		}); err != nil {
		// aws session error
		return errors.New("Connect to Cognito Failed: (AWS Session Error) " + err.Error())
	} else {
		// create cached objects for shared use
		s.cognitoClient = awscognito.New(sess)

		if s.cognitoClient == nil {
			return errors.New("Connect to Cognito Client Failed: (New Cognito Client Connection) " + "Connection Object Nil")
		}

		// connect successful
		return nil
	}
}

// Disconnect will clear sqs client
func (s *Cognito) Disconnect() {
	s.cognitoClient = nil
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (s *Cognito) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	s._parentSegment = parentSegment
}

// ----------------------------------------------------------------------------------------------------------------
// OpenId Token functions
// ----------------------------------------------------------------------------------------------------------------

func (s *Cognito) GetOpenIdTokenForDeveloperIdentity(identityPoolId, developerProviderID, developerProviderName string) (identityId, token string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("Cognito-GetOpenIdTokenForDeveloperIdentity", s._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("Cognito-GetOpenIdTokenForDeveloperIdentity-IdentityId", identityId)
			_ = seg.Seg.AddMetadata("Cognito-GetOpenIdTokenForDeveloperIdentity-Token", token)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	if s.cognitoClient == nil {
		err = errors.New("GetOpenIdTokenForDeveloperIdentity Failed: " + "Cognito Client is Required")
		return "", "", err
	}

	if len(identityPoolId) <= 0 {
		err = errors.New("GetOpenIdTokenForDeveloperIdentity Failed: " + "Identity Pool ID is Required")
		return "", "", err
	}

	if len(developerProviderID) <= 0 {
		err = errors.New("GetOpenIdTokenForDeveloperIdentity Failed: " + "Developer Provider ID is Required")
		return "", "", err
	}

	if len(developerProviderName) <= 0 {
		err = errors.New("GetOpenIdTokenForDeveloperIdentity Failed: " + "Developer Provider Name is Required")
		return "", "", err
	}

	// create input object
	input := &awscognito.GetOpenIdTokenForDeveloperIdentityInput{
		IdentityPoolId: aws.String(identityPoolId),
		Logins: map[string]*string{
			developerProviderName: aws.String(developerProviderID),
		},
		TokenDuration: aws.Int64(86400), // Token duration in seconds (1 day)
	}

	// perform action
	var output *awscognito.GetOpenIdTokenForDeveloperIdentityOutput

	if segCtxSet {
		output, err = s.cognitoClient.GetOpenIdTokenForDeveloperIdentityWithContext(segCtx, input)
	} else {
		output, err = s.cognitoClient.GetOpenIdTokenForDeveloperIdentity(input)
	}

	// evaluate result
	if err != nil {
		return "", "", err
	}
	if len(*output.IdentityId) == 0 {
		return "", "", errors.New("GetOpenIdTokenForDeveloperIdentity Failed: " + "No Identity ID Found")
	}
	if len(*output.Token) == 0 {
		return "", "", errors.New("GetOpenIdTokenForDeveloperIdentity Failed: " + "No OpenID Token Found")
	}

	return *output.IdentityId, *output.Token, nil
}
