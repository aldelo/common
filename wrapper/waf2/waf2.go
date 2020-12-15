package waf2

import (
	"fmt"
	util "github.com/aldelo/common"
	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/wafv2"
	"net/http"
)

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

type WAF2 struct {
	// define the AWS region that s3 is located at
	AwsRegion awsregion.AWSRegion

	// custom http2 client options
	HttpOptions *awshttp2.HttpClientSettings

	// store aws session object
	sess *session.Session

	// store waf2 object
	waf2Obj *wafv2.WAFV2
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the WAF2 service
func (w *WAF2) Connect() error {
	// clean up prior session reference
	w.sess = nil

	if !w.AwsRegion.Valid() || w.AwsRegion == awsregion.UNKNOWN {
		return fmt.Errorf("Connect To WAF2 Failed: (AWS Session Error) " + "Region is Required")
	}

	// create custom http2 client if needed
	var httpCli *http.Client
	var httpErr error

	if w.HttpOptions == nil {
		w.HttpOptions = new(awshttp2.HttpClientSettings)
	}

	// use custom http2 client
	h2 := &awshttp2.AwsHttp2Client{
		Options: w.HttpOptions,
	}

	if httpCli, httpErr = h2.NewHttp2Client(); httpErr != nil {
		return fmt.Errorf("Connect to WAF2 Failed: (AWS Session Error) " + "Create Custom Http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection and keep session object in struct
	if sess, err := session.NewSession(
		&aws.Config{
			Region:      aws.String(w.AwsRegion.Key()),
			HTTPClient:  httpCli,
		}); err != nil {
		// aws session error
		return fmt.Errorf("Connect To WAF2 Failed: (AWS Session Error) " + err.Error())
	} else {
		// aws session obtained
		w.waf2Obj = wafv2.New(sess)

		if w.waf2Obj == nil {
			return fmt.Errorf("Connect To WAF2 Object Failed: (New WAF2 Connection) " + "Connection Object Nil")
		}

		// session stored to struct
		return nil
	}
}

// UpdateIPSet will update an existing IPSet with new addresses specified
// ipsetName = exact name from WAF2 IP Set already created
// ipsetId = exact id from WAF2 IP Set already created
// scope = 'REGIONAL' or other scope per aws WAF2 doc (defaults to REGIONAL if blank)
// newAddr = addresses to add to ip set
func (w *WAF2) UpdateIPSet(ipsetName string, ipsetId string, scope string, newAddr []string) error {
	if util.LenTrim(ipsetName) == 0 {
		return fmt.Errorf("UpdateIPSet Failed: ipsetName is Required")
	}

	if util.LenTrim(ipsetId) == 0 {
		return fmt.Errorf("UpdateIPSet Failed: ipsetId is Required")
	}

	if util.LenTrim(scope) == 0 {
		scope = "REGIONAL"
	}

	if len(newAddr) == 0 {
		return fmt.Errorf("UpdateIPSet Failed: New Address to Add is Required")
	}

	var lockToken *string
	var addrList []string

	if getOutput, err := w.waf2Obj.GetIPSet(&wafv2.GetIPSetInput{
		Name: aws.String(ipsetName),
		Id: aws.String(ipsetId),
		Scope: aws.String(scope),
	}); err != nil {
		// error
		return fmt.Errorf("Get IP Set Failed: %s", err.Error())
	} else {
		lockToken = getOutput.LockToken
		addrList = aws.StringValueSlice(getOutput.IPSet.Addresses)
		addrList = append(addrList, newAddr...)
	}

	if _, err := w.waf2Obj.UpdateIPSet(&wafv2.UpdateIPSetInput{
		Name: aws.String(ipsetName),
		Id: aws.String(ipsetId),
		Scope: aws.String(scope),
		Addresses: aws.StringSlice(addrList),
		LockToken: lockToken,
	}); err != nil {
		// error encountered
		return fmt.Errorf("Update IP Set Failed: %s", err.Error())
	} else {
		// update completed
		return nil
	}
}

