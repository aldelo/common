package waf2

import (
	"context"
	"errors"
	"fmt"
	"net"
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
	wafLockMaxRetry         = 3
	wafLockRetryBackoff     = 150 * time.Millisecond
	wafRetryableMaxAttempts = 3
	wafRetryableBackoff     = 200 * time.Millisecond
	awsCallTimeout          = 10 * time.Second
)

// helper to validate IP/CIDR before hitting AWS
func validateIPOrCIDR(addr string) (string, string, error) {
	ip, ipNet, err := net.ParseCIDR(addr)
	if err != nil {
		return "", "", fmt.Errorf("address '%s' must be CIDR (e.g., 1.2.3.4/32 or 2001:db8::/128): %w", addr, err)
	}

	isIPv4 := ip.To4() != nil
	ones, _ := ipNet.Mask.Size()

	if isIPv4 {
		// AWS WAFv2 IPv4 CIDR bounds: /8 to /32
		if ones < 8 || ones > 32 {
			return "", "", fmt.Errorf("address '%s' IPv4 CIDR /%d is out of AWS WAFv2 allowed range /8-/32", addr, ones)
		}
	} else {
		// AWS WAFv2 IPv6 CIDR bounds: /24 to /128
		if ones < 24 || ones > 128 {
			return "", "", fmt.Errorf("address '%s' IPv6 CIDR /%d is out of AWS WAFv2 allowed range /24-/128", addr, ones)
		}
	}

	// Canonicalize to avoid duplicates from formatting differences
	family := "IPV6"
	if isIPv4 {
		family = "IPV4"
	}
	normalized := ipNet.String()

	return family, normalized, nil
}

// helper to detect retryable throttling/5xx
func isRetryableWAF(err error) bool {
	if err == nil {
		return false
	}

	var e awserr.RequestFailure
	if errors.As(err, &e) {
		if e.StatusCode() == http.StatusTooManyRequests || e.StatusCode() >= 500 {
			return true
		}
	}

	var ae awserr.Error
	if errors.As(err, &ae) {
		code := strings.ToLower(ae.Code())
		if code == "throttling" ||
			code == "throttlingexception" ||
			code == "requesttimeout" ||
			code == "requesttimeoutexception" {
			return true
		}
	}

	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}

	return false
}

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
	// require cdir, enforce aws mask bounds, and ensure a single ip family
	trimmed := make([]string, 0, len(newAddr))
	seenInput := make(map[string]struct{}, len(newAddr))
	var inputIPFamily string
	for _, a := range newAddr {
		if t := strings.TrimSpace(a); t != "" {
			fam, norm, err := validateIPOrCIDR(t) // returns "IPV4"/"IPV6" + canonical CIDR
			if err != nil {
				return fmt.Errorf("UpdateIPSet Failed: %w", err)
			}
			if inputIPFamily == "" {
				inputIPFamily = fam
			} else if inputIPFamily != fam {
				return fmt.Errorf("UpdateIPSet Failed: mixed IPv4/IPv6 CIDRs are not allowed in a single WAFv2 IP set")
			}
			if _, exists := seenInput[norm]; exists {
				continue // drop duplicates in input
			}
			seenInput[norm] = struct{}{}
			trimmed = append(trimmed, norm)
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
		ctx, cancel := context.WithTimeout(context.Background(), awsCallTimeout)
		getOutput, err := w.waf2Obj.GetIPSetWithContext(ctx, &wafv2.GetIPSetInput{
			Name:  aws.String(ipsetName),
			Id:    aws.String(ipsetId),
			Scope: aws.String(scope),
		})
		cancel()

		if err != nil {
			if isRetryableWAF(err) && attempt < wafRetryableMaxAttempts {
				lastErr = err
				time.Sleep(wafRetryableBackoff * time.Duration(attempt))
				continue
			}
			return fmt.Errorf("Get IP Set Failed: %s", err.Error())
		}

		if getOutput == nil || getOutput.IPSet == nil {
			return fmt.Errorf("Get IP Set Failed: Empty IPSet payload returned")
		}
		if getOutput.LockToken == nil {
			return fmt.Errorf("Get IP Set Failed: LockToken is nil")
		}

		// ensure caller input family matches the IP set's configured family
		if getOutput.IPSet.IPAddressVersion != nil && inputIPFamily != "" {
			if !strings.EqualFold(*getOutput.IPSet.IPAddressVersion, inputIPFamily) {
				return fmt.Errorf("UpdateIPSet Failed: IP family mismatch - IPSet expects %s but input addresses are %s",
					*getOutput.IPSet.IPAddressVersion, inputIPFamily)
			}
		}

		lockToken := getOutput.LockToken
		addrList := aws.StringValueSlice(getOutput.IPSet.Addresses)
		if addrList == nil {
			addrList = make([]string, 0)
		}

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

		ctx, cancel = context.WithTimeout(context.Background(), awsCallTimeout)
		_, err = w.waf2Obj.UpdateIPSetWithContext(ctx, &wafv2.UpdateIPSetInput{
			Name:      aws.String(ipsetName),
			Id:        aws.String(ipsetId),
			Scope:     aws.String(scope),
			Addresses: aws.StringSlice(addrList),
			LockToken: lockToken,
		})
		cancel()

		if err == nil {
			return nil // explicit success comment
		}

		if isOptimisticLock(err) {
			lastErr = err
			if attempt == wafLockMaxRetry {
				break // exhaust retries and report below
			}
			time.Sleep(wafLockRetryBackoff * time.Duration(attempt))
			continue
		}

		// retry throttling/5xx with bounded backoff
		if isRetryableWAF(err) && attempt < wafRetryableMaxAttempts {
			lastErr = err
			time.Sleep(wafRetryableBackoff * time.Duration(attempt))
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
// this method will take the newest regex pattern to replace the older patterns
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

	// normalize & de-dup new patterns before hitting AWS
	uniqueNew := make([]string, 0, len(newRegexPatterns))
	seen := make(map[string]struct{}, len(newRegexPatterns))
	for _, v := range newRegexPatterns {
		if t := strings.TrimSpace(v); t != "" {
			if _, ok := seen[t]; ok {
				continue
			}
			seen[t] = struct{}{}
			uniqueNew = append(uniqueNew, t)
		}
	}
	if len(uniqueNew) == 0 {
		return fmt.Errorf("UpdateRegexPatternSet Failed: New Regex Pattern to Add is Required")
	}
	if len(uniqueNew) > 10 {
		return fmt.Errorf("UpdateRegexPatternSet Failed: Resulting regex pattern count %d exceeds AWS WAF2 Regex Pattern Set limit of 10 patterns", len(uniqueNew))
	}

	// guard against nil client (call Connect first)
	if w.waf2Obj == nil {
		return fmt.Errorf("UpdateRegexPatternSet Failed: WAF2 Client Not Connected - Call Connect() First")
	}

	var lastErr error // track last optimistic-lock error
	for attempt := 1; attempt <= wafLockMaxRetry; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), awsCallTimeout)
		getOutput, err := w.waf2Obj.GetRegexPatternSetWithContext(ctx, &wafv2.GetRegexPatternSetInput{
			Name:  aws.String(regexPatternSetName),
			Id:    aws.String(regexPatternSetId),
			Scope: aws.String(scope),
		})
		cancel()

		if err != nil {
			if isRetryableWAF(err) && attempt < wafRetryableMaxAttempts {
				lastErr = err
				time.Sleep(wafRetryableBackoff * time.Duration(attempt))
				continue
			}
			return fmt.Errorf("Get Regex Pattern Set Failed: %s", err.Error())
		}

		if getOutput == nil || getOutput.RegexPatternSet == nil {
			return fmt.Errorf("Get Regex Pattern Set Failed: Empty RegexPatternSet payload returned")
		}
		if getOutput.LockToken == nil {
			return fmt.Errorf("Get Regex Pattern Set Failed: LockToken is nil")
		}

		lockToken := getOutput.LockToken

		// Replace existing patterns with the caller-supplied set (per docstring)
		newList := make([]*wafv2.Regex, 0, len(uniqueNew))
		for _, v := range uniqueNew {
			newList = append(newList, &wafv2.Regex{RegexString: aws.String(v)})
		}

		ctx, cancel = context.WithTimeout(context.Background(), awsCallTimeout)
		_, err = w.waf2Obj.UpdateRegexPatternSetWithContext(ctx, &wafv2.UpdateRegexPatternSetInput{
			Name:                  aws.String(regexPatternSetName),
			Id:                    aws.String(regexPatternSetId),
			Scope:                 aws.String(scope),
			RegularExpressionList: newList,
			LockToken:             lockToken,
		})
		cancel()

		if err == nil {
			return nil
		}

		if isOptimisticLock(err) {
			lastErr = err
			if attempt == wafLockMaxRetry {
				break // exhaust retries and report below
			}
			time.Sleep(wafLockRetryBackoff * time.Duration(attempt))
			continue
		}

		// retry throttling/5xx with bounded backoff
		if isRetryableWAF(err) && attempt < wafRetryableMaxAttempts {
			lastErr = err
			time.Sleep(wafRetryableBackoff * time.Duration(attempt))
			continue
		}

		return fmt.Errorf("Update Regex Patterns Set Failed: %s", err.Error())
	}

	// clearer exhausted-retry error
	return fmt.Errorf("Update Regex Patterns Set Failed after %d optimistic-lock retries: %v", wafLockMaxRetry, lastErr)
}
