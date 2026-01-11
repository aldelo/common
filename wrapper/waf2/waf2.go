package waf2

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	util "github.com/aldelo/common"
	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/wafv2"
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

// bounded retries for optimistic locking
const (
	wafLockMaxRetry     = 3
	wafLockRetryBackoff = 150 * time.Millisecond
)

// helper to detect optimistic lock errors
func isOptimisticLock(err error) bool {
	var e awserr.Error
	if errors.As(err, &e) && e.Code() == wafv2.ErrCodeWAFOptimisticLockException {
		return true
	}
	return false
}

// Connect will establish a connection to the WAF2 service
func (w *WAF2) Connect() error {
	// clean up prior session reference
	w.sess = nil
	w.waf2Obj = nil

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
	sess, err := session.NewSession(
		&aws.Config{
			Region:     aws.String(w.AwsRegion.Key()),
			HTTPClient: httpCli,
		})
	if err != nil {
		// aws session error
		return fmt.Errorf("Connect To WAF2 Failed: (AWS Session Error) " + err.Error())
	}

	// aws session obtained
	w.sess = sess
	w.waf2Obj = wafv2.New(sess)

	if w.waf2Obj == nil {
		return fmt.Errorf("Connect To WAF2 Object Failed: (New WAF2 Connection) " + "Connection Object Nil")
	}

	// session stored to struct
	return nil
}

// UpdateIPSet will update an existing IPSet with new addresses specified
// ipsetName = exact name from WAF2 IP Set already created
// ipsetId = exact id from WAF2 IP Set already created
// scope = 'REGIONAL' or other scope per aws WAF2 doc (defaults to REGIONAL if blank)
// newAddr = addresses to add to ip set
//
// note: aws limit is 10000 ip per ip set
func (w *WAF2) UpdateIPSet(ipsetName string, ipsetId string, scope string, newAddr []string) error {
	if util.LenTrim(ipsetName) == 0 {
		return fmt.Errorf("UpdateIPSet Failed: ipsetName is Required")
	}

	if util.LenTrim(ipsetId) == 0 {
		return fmt.Errorf("UpdateIPSet Failed: ipsetId is Required")
	}

	if util.LenTrim(scope) == 0 {
		scope = "REGIONAL"
	} else {
		// normalize scope
		scope = strings.ToUpper(strings.TrimSpace(scope))
	}

	// validate scope against allowed values to fail fast
	if scope != "REGIONAL" && scope != "CLOUDFRONT" {
		return fmt.Errorf("UpdateIPSet Failed: scope must be REGIONAL or CLOUDFRONT; scope value '%s' is Invalid", scope)
	}

	// trim & drop empty/whitespace inputs before proceeding
	trimmed := make([]string, 0, len(newAddr))
	for _, a := range newAddr {
		if t := strings.TrimSpace(a); t != "" {
			trimmed = append(trimmed, t)
		}
	}
	if len(trimmed) == 0 {
		return fmt.Errorf("UpdateIPSet Failed: New Address to Add is Required")
	}

	// guard against nil client (call Connect first)
	if w.waf2Obj == nil {
		return fmt.Errorf("UpdateIPSet Failed: WAF2 Client Not Connected - Call Connect() First")
	}

	var lastErr error // track last optimistic-lock error
	for attempt := 1; attempt <= wafLockMaxRetry; attempt++ {
		getOutput, err := w.waf2Obj.GetIPSet(&wafv2.GetIPSetInput{
			Name:  aws.String(ipsetName),
			Id:    aws.String(ipsetId),
			Scope: aws.String(scope),
		})
		if err != nil {
			return fmt.Errorf("Get IP Set Failed: %s", err.Error())
		}

		if getOutput == nil || getOutput.IPSet == nil {
			return fmt.Errorf("Get IP Set Failed: Empty IPSet payload returned")
		}
		if getOutput.LockToken == nil {
			return fmt.Errorf("Get IP Set Failed: LockToken is nil")
		}

		lockToken := getOutput.LockToken
		addrList := aws.StringValueSlice(getOutput.IPSet.Addresses)

		existing := make(map[string]struct{}, len(addrList))
		for _, a := range addrList {
			existing[a] = struct{}{}
		}

		newAddedCount := 0 // track whether anything changed
		for _, a := range trimmed {
			if _, ok := existing[a]; !ok {
				addrList = append(addrList, a)
				existing[a] = struct{}{}
				newAddedCount++
			}
		}

		// short-circuit when there is nothing new to add
		if newAddedCount == 0 {
			return nil
		}

		// fail fast instead of silently truncating and losing data
		if len(addrList) > 10000 {
			return fmt.Errorf("UpdateIPSet Failed: Resulting address count %d exceeds AWS WAF2 IP Set limit of 10000 addresses", len(addrList))
		}

		_, err = w.waf2Obj.UpdateIPSet(&wafv2.UpdateIPSetInput{
			Name:      aws.String(ipsetName),
			Id:        aws.String(ipsetId),
			Scope:     aws.String(scope),
			Addresses: aws.StringSlice(addrList),
			LockToken: lockToken,
		})

		if err == nil {
			// error encountered
			return nil
		}

		if isOptimisticLock(err) && attempt < wafLockMaxRetry {
			lastErr = err
			time.Sleep(wafLockRetryBackoff * time.Duration(attempt))
			continue
		}

		return fmt.Errorf("Update IP Set Failed: %s", err.Error())
	}

	// provide clear retry-exhausted error
	return fmt.Errorf("Update IP Set Failed after %d optimistic-lock retries: %v", wafLockMaxRetry, lastErr)
}

// UpdateRegexPatternSet will update an existing RegexPatternSet with new regex patterns specified
// regexPatternSetName = exact name from WAF2 Regex Pattern Set already created
// regexPatternSetId = exact id from WAF2 Regex Pattern Set already created
// scope = 'REGIONAL' or other scope per aws WAF2 doc (defaults to REGIONAL if blank)
// newRegexPatterns = regex patterns to add to regex pattern set
//
// NOTE = AWS limits to 10 regex expressions per regex set, and max of 10 regex sets
//
//	this method will take the newest regex pattern to replace the older patterns
func (w *WAF2) UpdateRegexPatternSet(regexPatternSetName string, regexPatternSetId string, scope string, newRegexPatterns []string) error {
	if util.LenTrim(regexPatternSetName) == 0 {
		return fmt.Errorf("UpdateRegexPatternSet Failed: regexPatternSetName is Required")
	}

	if util.LenTrim(regexPatternSetId) == 0 {
		return fmt.Errorf("UpdateRegexPatternSet Failed: regexPatternSetId is Required")
	}

	if util.LenTrim(scope) == 0 {
		scope = "REGIONAL"
	} else {
		// normalize scope
		scope = strings.ToUpper(strings.TrimSpace(scope))
	}

	// validate scope against allowed values to fail fast
	if scope != "REGIONAL" && scope != "CLOUDFRONT" {
		return fmt.Errorf("UpdateRegexPatternSet Failed: scope must be REGIONAL or CLOUDFRONT; scope value '%s' is Invalid", scope)
	}

	// trim & drop empty/whitespace regex entries before proceeding
	trimmed := make([]string, 0, len(newRegexPatterns))
	for _, v := range newRegexPatterns {
		if t := strings.TrimSpace(v); t != "" {
			trimmed = append(trimmed, t)
		}
	}
	if len(trimmed) == 0 {
		return fmt.Errorf("UpdateRegexPatternSet Failed: New Regex Pattern to Add is Required")
	}

	// guard against nil client (call Connect first)
	if w.waf2Obj == nil {
		return fmt.Errorf("UpdateRegexPatternSet Failed: WAF2 Client Not Connected - Call Connect() First")
	}

	var lastErr error // track last optimistic-lock error
	for attempt := 1; attempt <= wafLockMaxRetry; attempt++ {
		getOutput, err := w.waf2Obj.GetRegexPatternSet(&wafv2.GetRegexPatternSetInput{
			Name:  aws.String(regexPatternSetName),
			Id:    aws.String(regexPatternSetId),
			Scope: aws.String(scope),
		})
		if err != nil {
			return fmt.Errorf("Get Regex Pattern Set Failed: %s", err.Error())
		}

		if getOutput == nil || getOutput.RegexPatternSet == nil {
			return fmt.Errorf("Get Regex Pattern Set Failed: Empty RegexPatternSet payload returned")
		}
		if getOutput.LockToken == nil {
			return fmt.Errorf("Get Regex Pattern Set Failed: LockToken is nil")
		}

		lockToken := getOutput.LockToken
		patternsList := getOutput.RegexPatternSet.RegularExpressionList

		var oldList []string
		if len(patternsList) > 0 {
			for _, v := range patternsList {
				oldList = append(oldList, aws.StringValue(v.RegexString))
			}
		}

		existing := make(map[string]struct{}, len(oldList))
		for _, v := range oldList {
			existing[v] = struct{}{}
		}

		newAddedCount := 0
		for _, v := range trimmed {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			if _, ok := existing[v]; !ok {
				patternsList = append(patternsList, &wafv2.Regex{
					RegexString: aws.String(v),
				})
				oldList = append(oldList, v)
				existing[v] = struct{}{}
				newAddedCount++
			}
		}

		if newAddedCount == 0 {
			return nil
		}

		// fail fast instead of silently truncating and losing data
		if len(patternsList) > 10 {
			return fmt.Errorf("UpdateRegexPatternSet Failed: Resulting regex pattern count %d exceeds AWS WAF2 Regex Pattern Set limit of 10 patterns", len(patternsList))
		}

		_, err = w.waf2Obj.UpdateRegexPatternSet(&wafv2.UpdateRegexPatternSetInput{
			Name:                  aws.String(regexPatternSetName),
			Id:                    aws.String(regexPatternSetId),
			Scope:                 aws.String(scope),
			RegularExpressionList: patternsList,
			LockToken:             lockToken,
		})
		if err == nil {
			return nil
		}
		if isOptimisticLock(err) && attempt < wafLockMaxRetry {
			lastErr = err
			time.Sleep(wafLockRetryBackoff * time.Duration(attempt))
			continue
		}
		return fmt.Errorf("Update Regex Patterns Set Failed: %s", err.Error())
	}

	// clearer exhausted-retry error
	return fmt.Errorf("Update Regex Patterns Set Failed after %d optimistic-lock retries: %v", wafLockMaxRetry, lastErr)
}
