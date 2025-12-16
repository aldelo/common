package sns

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
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	util "github.com/aldelo/common"
	awshttp2 "github.com/aldelo/common/wrapper/aws"
	awsregion "github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aldelo/common/wrapper/sns/snsapplicationplatform"
	"github.com/aldelo/common/wrapper/sns/snscreatetopicattribute"
	"github.com/aldelo/common/wrapper/sns/snsendpointattribute"
	"github.com/aldelo/common/wrapper/sns/snsgetsubscriptionattribute"
	"github.com/aldelo/common/wrapper/sns/snsgettopicattribute"
	"github.com/aldelo/common/wrapper/sns/snsplatformapplicationattribute"
	"github.com/aldelo/common/wrapper/sns/snsprotocol"
	"github.com/aldelo/common/wrapper/sns/snssubscribeattribute"
	"github.com/aldelo/common/wrapper/xray"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
	awsxray "github.com/aws/aws-xray-sdk-go/xray"
)

// ================================================================================================================
// STRUCTS
// ================================================================================================================

// SNS struct encapsulates the AWS SNS access functionality
type SNS struct {
	// define the AWS region that SNS is located at
	AwsRegion awsregion.AWSRegion

	// custom http2 client options
	HttpOptions *awshttp2.HttpClientSettings

	// optional, sms sender name info
	SMSSenderName string

	// optional, indicates if sms message sent is transaction or promotional
	SMSTransactional bool

	// store sns client object
	snsClient *sns.SNS

	_parentSegment *xray.XRayParentSegment

	mu sync.RWMutex
}

// SubscribedTopic struct encapsulates the AWS SNS subscribed topic data
type SubscribedTopic struct {
	SubscriptionArn string
	TopicArn        string
	Protocol        snsprotocol.SNSProtocol
	Endpoint        string
	Owner           string
}

// --- internal helpers (thread-safe accessors) ---

func (s *SNS) getAwsRegion() awsregion.AWSRegion {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.AwsRegion
}

func (s *SNS) getSMSSenderName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SMSSenderName
}

func (s *SNS) getSMSTransactional() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SMSTransactional
}

func (s *SNS) getClient() *sns.SNS {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snsClient
}

func (s *SNS) setClient(cli *sns.SNS) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snsClient = cli
}

func (s *SNS) clearClient() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snsClient = nil
}

func (s *SNS) ensureHttpOptions() *awshttp2.HttpClientSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.HttpOptions == nil {
		s.HttpOptions = new(awshttp2.HttpClientSettings)
	}
	return s.HttpOptions
}

func (s *SNS) setParentSegment(seg *xray.XRayParentSegment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s._parentSegment = seg
}

func (s *SNS) getParentSegment() *xray.XRayParentSegment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s._parentSegment
}

var (
	e164Regexp      = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)
	senderIDRegexp  = regexp.MustCompile(`^[A-Za-z0-9]{3,11}$`)
	senderHasLetter = regexp.MustCompile(`[A-Za-z]`)
)

// validateE164Phone ensures the phone number is in E.164 format.
func validateE164Phone(phone string) error {
	if !e164Regexp.MatchString(phone) {
		return fmt.Errorf("phone number must be E.164 formatted (e.g. +12095551212)")
	}
	return nil
}

// validateSenderID enforces AWS rules: 3-11 alphanumeric chars, at least one letter.
func validateSenderID(id string) error {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	id = strings.TrimSpace(id)
	if !senderIDRegexp.MatchString(id) || !senderHasLetter.MatchString(id) {
		return fmt.Errorf("SMSSenderName must be 3-11 alphanumeric chars and contain at least one letter")
	}
	return nil
}

// smsLength returns the per-encoding limit and the used character count.
func smsLength(message string) (limit int, used int, encoding string) {
	// GSM 7-bit default and extended tables per 3GPP TS 23.038
	var (
		gsm7Default = map[rune]bool{
			'@': true, '£': true, '$': true, '¥': true, 'è': true, 'é': true, 'ù': true, 'ì': true, 'ò': true,
			'Ç': true, '\n': true, 'Ø': true, 'ø': true, '\r': true, 'Å': true, 'å': true, 'Δ': true, '_': true,
			'Φ': true, 'Γ': true, 'Λ': true, 'Ω': true, 'Π': true, 'Ψ': true, 'Σ': true, 'Θ': true, 'Ξ': true,
			'Æ': true, 'æ': true, 'ß': true, 'É': true, ' ': true, '!': true, '"': true, '#': true, '¤': true,
			'%': true, '&': true, '\'': true, '(': true, ')': true, '*': true, '+': true, ',': true, '-': true,
			'.': true, '/': true, '0': true, '1': true, '2': true, '3': true, '4': true, '5': true, '6': true,
			'7': true, '8': true, '9': true, ':': true, ';': true, '<': true, '=': true, '>': true, '?': true,
			'¡': true, 'A': true, 'B': true, 'C': true, 'D': true, 'E': true, 'F': true, 'G': true, 'H': true,
			'I': true, 'J': true, 'K': true, 'L': true, 'M': true, 'N': true, 'O': true, 'P': true, 'Q': true,
			'R': true, 'S': true, 'T': true, 'U': true, 'V': true, 'W': true, 'X': true, 'Y': true, 'Z': true,
			'Ä': true, 'Ö': true, 'Ñ': true, 'Ü': true, '§': true, '¿': true, 'a': true, 'b': true, 'c': true,
			'd': true, 'e': true, 'f': true, 'g': true, 'h': true, 'i': true, 'j': true, 'k': true, 'l': true,
			'm': true, 'n': true, 'o': true, 'p': true, 'q': true, 'r': true, 's': true, 't': true, 'u': true,
			'v': true, 'w': true, 'x': true, 'y': true, 'z': true, 'ä': true, 'ö': true, 'ñ': true, 'ü': true, 'à': true,
		}
		gsm7Extended = map[rune]bool{
			'^': true, '{': true, '}': true, '\\': true, '[': true, '~': true, ']': true, '|': true, '€': true,
		}
	)

	septets := 0
	for _, r := range message {
		switch {
		case gsm7Default[r]:
			septets += 1
		case gsm7Extended[r]:
			septets += 2 // escape + char
		default:
			// not representable in GSM-7 -> UCS-2
			used = utf8.RuneCountInString(message)
			encoding = "UCS-2"
			limit = 70
			// allow multipart UCS-2 (67 chars/segment after UDH)
			if used > limit {
				segments := (used + 66) / 67
				limit = segments * 67
			}
			return limit, used, encoding
		}
	}

	// GSM-7 path
	used = septets
	encoding = "GSM-7"
	limit = 160
	// allow multipart GSM-7 (153 septets/segment after UDH)
	if used > limit {
		segments := (used + 152) / 153
		limit = segments * 153
	}
	return limit, used, encoding
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the SNS service
func (s *SNS) Connect(parentSegment ...*xray.XRayParentSegment) (err error) {
	if len(parentSegment) > 0 {
		s.setParentSegment(parentSegment[0])
	}

	if xray.XRayServiceOn() {
		seg := xray.NewSegment("SNS-Connect", s.getParentSegment())
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-AWS-Region", s.getAwsRegion())
			_ = seg.Seg.AddMetadata("SNS-SMS-Sender-Name", s.getSMSSenderName())
			_ = seg.Seg.AddMetadata("SNS-SMS-Transactional", s.getSMSTransactional())

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = s.connectInternal()

		if err == nil {
			if cli := s.getClient(); cli != nil {
				awsxray.AWS(cli.Client)
			}
		}

		return err
	}

	return s.connectInternal()
}

// Connect will establish a connection to the SNS service
func (s *SNS) connectInternal() error {
	if !s.AwsRegion.Valid() || s.AwsRegion == awsregion.UNKNOWN {
		return errors.New("Connect To SNS Failed: (AWS Session Error) " + "Region is Required")
	}

	// create custom http2 client if needed
	httpOpts := s.ensureHttpOptions()
	h2 := &awshttp2.AwsHttp2Client{
		Options: httpOpts,
	}

	httpCli, httpErr := h2.NewHttp2Client()
	if httpErr != nil {
		return errors.New("Connect to SNS Failed: (AWS Session Error) " + "Create Custom Http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection and keep session object in struct
	sess, err := session.NewSession(
		&aws.Config{
			Region:     aws.String(s.AwsRegion.Key()),
			HTTPClient: httpCli,
		})
	if err != nil {
		// aws session error
		return errors.New("Connect To SNS Failed: (AWS Session Error) " + err.Error())
	}

	cli := sns.New(sess)
	if cli == nil {
		return errors.New("Connect To SNS Failed: (New SNS Client Connection) " + "Connection Object Nil")
	}

	s.setClient(cli)
	return nil
}

// Disconnect will disjoin from aws session by clearing it
func (s *SNS) Disconnect() {
	s.clearClient()
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (s *SNS) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	s.setParentSegment(parentSegment)
}

// ----------------------------------------------------------------------------------------------------------------
// internal helper methods
// ----------------------------------------------------------------------------------------------------------------

// toAwsCreateTopicAttributes will convert from strongly typed to aws accepted map
func (s *SNS) toAwsCreateTopicAttributes(attributes map[snscreatetopicattribute.SNSCreateTopicAttribute]string) (newMap map[string]*string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[string]*string)

	for k, v := range attributes {
		if k.Valid() && k != snscreatetopicattribute.UNKNOWN {
			newMap[k.Key()] = aws.String(v)
		}
	}

	return newMap
}

// fromAwsCreateTopicAttributes will convert from aws map to strongly typed map
func (s *SNS) fromAwsCreateTopicAttributes(attributes map[string]*string) (newMap map[snscreatetopicattribute.SNSCreateTopicAttribute]string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[snscreatetopicattribute.SNSCreateTopicAttribute]string)
	var conv snscreatetopicattribute.SNSCreateTopicAttribute

	for k, v := range attributes {
		if util.LenTrim(k) > 0 {
			v1 := aws.StringValue(v)

			if k1, err := conv.ParseByKey(k); err == nil {
				newMap[k1] = v1
			}
		}
	}

	return newMap
}

// toAwsGetTopicAttributes will convert from strongly typed to aws accepted map
func (s *SNS) toAwsGetTopicAttributes(attributes map[snsgettopicattribute.SNSGetTopicAttribute]string) (newMap map[string]*string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[string]*string)

	for k, v := range attributes {
		if k.Valid() && k != snsgettopicattribute.UNKNOWN {
			newMap[k.Key()] = aws.String(v)
		}
	}

	return newMap
}

// fromAwsGetTopicAttributes will convert from aws map to strongly typed map
func (s *SNS) fromAwsGetTopicAttributes(attributes map[string]*string) (newMap map[snsgettopicattribute.SNSGetTopicAttribute]string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[snsgettopicattribute.SNSGetTopicAttribute]string)
	var conv snsgettopicattribute.SNSGetTopicAttribute

	for k, v := range attributes {
		if util.LenTrim(k) > 0 {
			v1 := aws.StringValue(v)

			if k1, err := conv.ParseByKey(k); err == nil {
				newMap[k1] = v1
			}
		}
	}

	return newMap
}

// toAwsSubscribeAttributes will convert from strongly typed to aws accepted map
func (s *SNS) toAwsSubscribeAttributes(attributes map[snssubscribeattribute.SNSSubscribeAttribute]string) (newMap map[string]*string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[string]*string)

	for k, v := range attributes {
		if k.Valid() && k != snssubscribeattribute.UNKNOWN {
			newMap[k.Key()] = aws.String(v)
		}
	}

	return newMap
}

// fromAwsSubscribeAttributes will convert from aws map to strongly typed map
func (s *SNS) fromAwsSubscribeAttributes(attributes map[string]*string) (newMap map[snssubscribeattribute.SNSSubscribeAttribute]string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[snssubscribeattribute.SNSSubscribeAttribute]string)
	var conv snssubscribeattribute.SNSSubscribeAttribute

	for k, v := range attributes {
		if util.LenTrim(k) > 0 {
			v1 := aws.StringValue(v)

			if k1, err := conv.ParseByKey(k); err == nil {
				newMap[k1] = v1
			}
		}
	}

	return newMap
}

// toAwsGetSubscriptionAttributes will convert from strongly typed to aws accepted map
func (s *SNS) toAwsGetSubscriptionAttributes(attributes map[snsgetsubscriptionattribute.SNSGetSubscriptionAttribute]string) (newMap map[string]*string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[string]*string)

	for k, v := range attributes {
		if k.Valid() && k != snsgetsubscriptionattribute.UNKNOWN {
			newMap[k.Key()] = aws.String(v)
		}
	}

	return newMap
}

// fromAwsGetSubscriptionAttributes will convert from aws map to strongly typed map
func (s *SNS) fromAwsGetSubscriptionAttributes(attributes map[string]*string) (newMap map[snsgetsubscriptionattribute.SNSGetSubscriptionAttribute]string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[snsgetsubscriptionattribute.SNSGetSubscriptionAttribute]string)
	var conv snsgetsubscriptionattribute.SNSGetSubscriptionAttribute

	for k, v := range attributes {
		if util.LenTrim(k) > 0 {
			v1 := aws.StringValue(v)

			if k1, err := conv.ParseByKey(k); err == nil {
				newMap[k1] = v1
			}
		}
	}

	return newMap
}

// toAwsPlatformApplicationAttributes will convert from strongly typed to aws accepted map
func (s *SNS) toAwsPlatformApplicationAttributes(attributes map[snsplatformapplicationattribute.SNSPlatformApplicationAttribute]string) (newMap map[string]*string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[string]*string)

	for k, v := range attributes {
		if k.Valid() && k != snsplatformapplicationattribute.UNKNOWN {
			newMap[k.Key()] = aws.String(v)
		}
	}

	return newMap
}

// fromAwsPlatformApplicationAttributes will convert from aws map to strongly typed map
func (s *SNS) fromAwsPlatformApplicationAttributes(attributes map[string]*string) (newMap map[snsplatformapplicationattribute.SNSPlatformApplicationAttribute]string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[snsplatformapplicationattribute.SNSPlatformApplicationAttribute]string)
	var conv snsplatformapplicationattribute.SNSPlatformApplicationAttribute

	for k, v := range attributes {
		if util.LenTrim(k) > 0 {
			v1 := aws.StringValue(v)

			if k1, err := conv.ParseByKey(k); err == nil {
				newMap[k1] = v1
			}
		}
	}

	return newMap
}

// toAwsEndpointAttributes will convert from strongly typed to aws accepted map
func (s *SNS) toAwsEndpointAttributes(attributes map[snsendpointattribute.SNSEndpointAttribute]string) (newMap map[string]*string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[string]*string)

	for k, v := range attributes {
		if k.Valid() && k != snsendpointattribute.UNKNOWN {
			newMap[k.Key()] = aws.String(v)
		}
	}

	return newMap
}

// fromAwsEndpointAttributes will convert from aws map to strongly typed map
func (s *SNS) fromAwsEndpointAttributes(attributes map[string]*string) (newMap map[snsendpointattribute.SNSEndpointAttribute]string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[snsendpointattribute.SNSEndpointAttribute]string)
	var conv snsendpointattribute.SNSEndpointAttribute

	for k, v := range attributes {
		if util.LenTrim(k) > 0 {
			v1 := aws.StringValue(v)

			if k1, err := conv.ParseByKey(k); err == nil {
				newMap[k1] = v1
			}
		}
	}

	return newMap
}

// ----------------------------------------------------------------------------------------------------------------
// topic methods
// ----------------------------------------------------------------------------------------------------------------

// CreateTopic will create a new topic in SNS for clients to subscribe,
// once topic is created, the topicArn is returned for subsequent uses
//
// Parameters:
//  1. topicName = required, the name of the topic to create in SNS
//  2. attributes = optional, topic attributes to further customize the topic
//  3. timeOutDuration = optional, indicates timeout value for context
//
// Topic Attributes: (Key = Expected Value)
//  1. DeliveryPolicy = The JSON serialization of the topic's delivery policy
//  2. DisplayName = The human-readable name used in the From field for notifications to email and email-json endpoints
//  3. Policy = The JSON serialization of the topic's access control policy
//
// The following attribute applies only to server-side-encryption (https://docs.aws.amazon.com/sns/latest/dg/sns-server-side-encryption.html):
//
//	   	a) KmsMasterKeyId = The ID of an AWS-managed customer master key (CMK) for Amazon SNS or a custom CMK.
//							For more information, see Key Terms (https://docs.aws.amazon.com/sns/latest/dg/sns-server-side-encryption.html#sse-key-terms)
//	   						For more examples, see KeyId (https://docs.aws.amazon.com/kms/latest/APIReference/API_DescribeKey.html#API_DescribeKey_RequestParameters) in the AWS Key Management Service API Reference
func (s *SNS) CreateTopic(topicName string, attributes map[snscreatetopicattribute.SNSCreateTopicAttribute]string, timeOutDuration ...time.Duration) (topicArn string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-CreateTopic", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-CreateTopic-TopicName", topicName)
			_ = seg.Seg.AddMetadata("SNS-CreateTopic-Attributes", attributes)
			_ = seg.Seg.AddMetadata("SNS-CreateTopic-Result-TopicArn", topicArn)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("CreateTopic Failed: " + "SNS Client is Required")
		return "", err
	}

	if util.LenTrim(topicName) <= 0 {
		err = errors.New("CreateTopic Failed: " + "Topic Name is Required")
		return "", err
	}

	// create input object
	input := &sns.CreateTopicInput{
		Name: aws.String(topicName),
	}

	if attributes != nil {
		input.Attributes = s.toAwsCreateTopicAttributes(attributes)
	}

	// perform action
	var output *sns.CreateTopicOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.CreateTopicWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.CreateTopicWithContext(segCtx, input)
		} else {
			output, err = client.CreateTopic(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("CreateTopic Failed: (Create Action) " + err.Error())
		return "", err
	}

	topicArn = aws.StringValue(output.TopicArn)
	return topicArn, nil
}

// DeleteTopic will delete an existing SNS topic by topicArn,
// returns nil if successful
func (s *SNS) DeleteTopic(topicArn string, timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-DeleteTopic", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-DeleteTopic-TopicArn", topicArn)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("DeleteTopic Failed: " + "SNS Client is Required")
		return err
	}

	if util.LenTrim(topicArn) <= 0 {
		err = errors.New("DeleteTopic Failed: " + "Topic ARN is Required")
		return err
	}

	// create input object
	input := &sns.DeleteTopicInput{
		TopicArn: aws.String(topicArn),
	}

	// perform action
	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		_, err = client.DeleteTopicWithContext(ctx, input)
	} else {
		if segCtxSet {
			_, err = client.DeleteTopicWithContext(segCtx, input)
		} else {
			_, err = client.DeleteTopic(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("DeleteTopic Failed: (Delete Action) " + err.Error())
		return err
	}

	return nil
}

// ListTopics will list SNS topics, with optional nextToken for retrieving more list from a prior call
//
// Parameters:
//  1. nextToken = optional, if prior call returned more...token, pass in here to retrieve the related list
//  2. timeOutDuration = optional, indicates timeout value for context
//
// Return Values:
//  1. topicArnsList = string slice of topic ARNs, nil if not set
//  2. moreTopicArnsNextToken = if there are more topics, this token is filled, to query more, use the token as input parameter, blank if no more
//  3. err = error info if any
func (s *SNS) ListTopics(nextToken string, timeOutDuration ...time.Duration) (topicArnsList []string, moreTopicArnsNextToken string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-ListTopics", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-ListTopics-NextToken", nextToken)
			_ = seg.Seg.AddMetadata("SNS-ListTopics-Result-TopicArnsList", topicArnsList)
			_ = seg.Seg.AddMetadata("SNS-ListTopics-Result-NextToken", moreTopicArnsNextToken)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("ListTopics Failed: " + "SNS Client is Required")
		return nil, "", err
	}

	// create input object
	input := &sns.ListTopicsInput{}

	if util.LenTrim(nextToken) > 0 {
		input.NextToken = aws.String(nextToken)
	}

	// perform action
	var output *sns.ListTopicsOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.ListTopicsWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.ListTopicsWithContext(segCtx, input)
		} else {
			output, err = client.ListTopics(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("ListTopics Failed: (List Action) " + err.Error())
		return nil, "", err
	}

	moreTopicArnsNextToken = aws.StringValue(output.NextToken)

	for _, v := range output.Topics {
		buf := aws.StringValue(v.TopicArn)

		if util.LenTrim(buf) > 0 {
			topicArnsList = append(topicArnsList, buf)
		}
	}

	return topicArnsList, moreTopicArnsNextToken, nil
}

// GetTopicAttributes will retrieve a map of topic attributes based on topicArn
//
// Parameters:
//  1. topicArn = required, specify the topicArn to retrieve related topic attributes
//  2. timeOutDuration = optional, indicates timeout value for context
//
// Return Values:
//  1. attributes = map of sns get topic attributes key value pairs related to teh topic ARN being queried
//  2. err = error info if any
//
// Topic Attributes: (Key = Expected Value)
//  1. DeliveryPolicy = The JSON serialization of the topic's delivery policy (See Subscribe DeliveryPolicy Json Format)
//  2. DisplayName = The human-readable name used in the From field for notifications to email and email-json endpoints
//  3. Owner = The AWS account ID of the topic's owner
//  4. Policy = The JSON serialization of the topic's access control policy
//  5. SubscriptionsConfirmed = The number of confirmed subscriptions for the topic
//  6. SubscriptionsDeleted = The number of deleted subscriptions for the topic
//  7. SubscriptionsPending = The number of subscriptions pending confirmation for the topic
//  8. TopicArn = The topic's ARN
//  9. EffectiveDeliveryPolicy = Yhe JSON serialization of the effective delivery policy, taking system defaults into account
//
// The following attribute applies only to server-side-encryption (https://docs.aws.amazon.com/sns/latest/dg/sns-server-side-encryption.html):
//
//	   	a) KmsMasterKeyId = The ID of an AWS-managed customer master key (CMK) for Amazon SNS or a custom CMK.
//							For more information, see Key Terms (https://docs.aws.amazon.com/sns/latest/dg/sns-server-side-encryption.html#sse-key-terms)
//	   						For more examples, see KeyId (https://docs.aws.amazon.com/kms/latest/APIReference/API_DescribeKey.html#API_DescribeKey_RequestParameters) in the AWS Key Management Service API Reference
func (s *SNS) GetTopicAttributes(topicArn string, timeOutDuration ...time.Duration) (attributes map[snsgettopicattribute.SNSGetTopicAttribute]string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-GetTopicAttributes", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-GetTopicAttributes-TopicArn", topicArn)
			_ = seg.Seg.AddMetadata("SNS-GetTopicAttributes-Result-Attributes", attributes)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("GetTopicAttributes Failed: " + "SNS Client is Required")
		return nil, err
	}

	if util.LenTrim(topicArn) <= 0 {
		err = errors.New("GetTopicAttributes Failed: " + "Topic ARN is Required")
		return nil, err
	}

	// create input object
	input := &sns.GetTopicAttributesInput{
		TopicArn: aws.String(topicArn),
	}

	// perform action
	var output *sns.GetTopicAttributesOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.GetTopicAttributesWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.GetTopicAttributesWithContext(segCtx, input)
		} else {
			output, err = client.GetTopicAttributes(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("GetTopicAttributes Failed: (Get Action) " + err.Error())
		return nil, err
	}

	attributes = s.fromAwsGetTopicAttributes(output.Attributes)
	return attributes, nil
}

// SetTopicAttribute will set or update a topic attribute,
// For attribute value or Json format, see corresponding notes in CreateTopic where applicable
func (s *SNS) SetTopicAttribute(topicArn string,
	attributeName snscreatetopicattribute.SNSCreateTopicAttribute,
	attributeValue string,
	timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-SetTopicAttribute", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-SetTopicAttribute-TopicArn", topicArn)
			_ = seg.Seg.AddMetadata("SNS-SetTopicAttribute-AttributeName", attributeName)
			_ = seg.Seg.AddMetadata("SNS-SetTopicAttribute-AttributeValue", attributeValue)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("SetTopicAttribute Failed: " + "SNS Client is Required")
		return err
	}

	if util.LenTrim(topicArn) <= 0 {
		err = errors.New("SetTopicAttribute Failed: " + "Topic ARN is Required")
		return err
	}

	if !attributeName.Valid() || attributeName == snscreatetopicattribute.UNKNOWN {
		err = errors.New("SetTopicAttribute Failed: " + "Attribute Name is Required")
		return err
	}

	// create input object
	input := &sns.SetTopicAttributesInput{
		TopicArn:       aws.String(topicArn),
		AttributeName:  aws.String(attributeName.Key()),
		AttributeValue: aws.String(attributeValue),
	}

	// perform action
	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		_, err = client.SetTopicAttributesWithContext(ctx, input)
	} else {
		if segCtxSet {
			_, err = client.SetTopicAttributesWithContext(segCtx, input)
		} else {
			_, err = client.SetTopicAttributes(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("SetTopicAttribute Failed: (Set Action) " + err.Error())
		return err
	} else {
		return nil
	}
}

// ----------------------------------------------------------------------------------------------------------------
// subscriber methods
// ----------------------------------------------------------------------------------------------------------------

// Subscribe will allow client to subscribe to a SNS topic (previously created with CreateTopic method),
// the subscriptionArn is returned upon success,
//
//	if subscription needs client confirmation, then the string 'pending confirmation' is returned instead
//
// Parameters:
//  1. topicArn = required, subscribe to this topic ARN
//  2. protocol = required, SNS callback protocol, so that when publish to the topic occurs, this protocol is used as callback
//  3. endPoint = required, SNS callback endpoint, so that when publish to the topic occurs, this endpoint is triggered by the callback
//  4. attributes = optional, map of sns subscribe attribute key value pairs
//  5. timeOutDuration = optional, indicates timeout value for context
//
// Protocols: (Key = Expected Value)
//  1. http = delivery of JSON-encoded message via HTTP POST
//  2. https = delivery of JSON-encoded message via HTTPS POST
//  3. email = delivery of message via SMTP
//  4. email-json = delivery of JSON-encoded message via SMTP
//  5. sms = delivery of message via SMS
//  6. sqs = delivery of JSON-encoded message to an Amazon SQS queue
//  7. application = delivery of JSON-encoded message to an EndpointArn for a mobile app and device
//  8. lambda = delivery of JSON-encoded message to an Amazon Lambda function
//
// Endpoint To Receive Notifications: (Based on Protocol)
//  1. http protocol = the endpoint is an URL beginning with http://
//  2. https protocol = the endpoint is a URL beginning with https://
//  3. email protocol = the endpoint is an email address
//  4. email-json protocol = the endpoint is an email address
//  5. sms protocol = the endpoint is a phone number of an SMS-enabled device
//  6. sqs protocol = the endpoint is the ARN of an Amazon SQS queue
//  7. application protocol = the endpoint is the EndpointArn of a mobile app and device
//  8. lambda protocol = the endpoint is the ARN of an Amazon Lambda function
//
// Subscribe Attributes: (Key = Expected Value)
//  1. DeliveryPolicy = The policy that defines how Amazon SNS retries failed deliveries to HTTP/S endpoints
//     *) example to set delivery policy to 5 retries:
//     {
//     "healthyRetryPolicy": {
//     "minDelayTarget": <intValue>,
//     "maxDelayTarget": <intValue>,
//     "numRetries": <intValue>,
//     "numMaxDelayRetries": <intValue>,
//     "backoffFunction": "<linear|arithmetic|geometric|exponential>"
//     },
//     "throttlePolicy": {
//     "maxReceivesPerSecond": <intValue>
//     }
//     }
//     *) Not All Json Elements Need To Be Filled in Policy, Use What is Needed, such as:
//     { "healthyRetryPolicy": { "numRetries": 5 } }
//  2. FilterPolicy = The simple JSON object that lets your subscriber receive only a subset of messages,
//     rather than receiving every message published to the topic:
//     *) subscriber attribute controls filter allowance,
//     publish attribute indicates attributes contained in message
//     *) if any single attribute in this policy doesn't match an attribute assigned to message, this policy rejects the message:
//     {
//     "store": ["example_corp"],
//     "event": [{"anything-but": "order_cancelled"}],
//     "customer_interests": ["rugby", "football", "baseball"],
//     "price_usd": [{"numeric": [">=", 100]}]
//     }
//     *) "xyz": [{"anything-but": ...}] keyword indicates to match anything but the defined value ... Json element (... may be string or numeric)
//     *) "xyz": [{"prefix": ...}] keyword indicates to match value prefixed from the defined value ... in Json element
//     *) "xyz": [{"numeric": ["=", ...]}] keyword indicates numeric equal matching as indicated by numeric and =
//     *) "xyz": [{"numeric": [">", ...]}] keyword indicates numeric compare matching as indicated by numeric and >, <, >=, <=
//     *) "xyz": [{"numeric": [">", 0, "<", 100]}] keyword indicates numeric ranged compare matching as indicated by numeric and >, <, in parts
//     *) "xyz": [{"exists": true}] keyword indicates attribute xyz exists matching
//  3. RawMessageDelivery = When set to true, enables raw message delivery to Amazon SQS or HTTP/S endpoints.
//     This eliminates the need for the endpoints to process JSON formatting, which is otherwise created for Amazon SNS metadata
//  4. RedrivePolicy = When specified, sends undeliverable messages to the specified Amazon SQS dead-letter queue.
//     Messages that can't be delivered due to client errors (for example, when the subscribed endpoint is unreachable),
//     or server errors (for example, when the service that powers the subscribed endpoint becomes unavailable),
//     are held in the dead-letter queue for further analysis or reprocessing
//     *) example of RedrivePolicy to route failed messages to Dead Letter Queue (DLQ):
//     {
//     "deadLetterTargetArn": "dead letter sns queue arn such as arn:aws:sqs:us-east-2:12345678021:MyDeadLetterQueue"
//     }
//
// Subscription Confirmation Support:
//  1. Http / Https Endpoints Requires Subscription Confirmation Support, See Details in URL Below:
//     a) https://docs.aws.amazon.com/sns/latest/dg/sns-http-https-endpoint-as-subscriber.html
//  2. Once Subscribe action is performed, SNS sends confirmation notification to the HTTP/s Endpoint:
//     b) Client Upon Receipt of the SNS Notification, Retrieve Token and Respond via ConfirmSubscription method
func (s *SNS) Subscribe(topicArn string,
	protocol snsprotocol.SNSProtocol,
	endPoint string,
	attributes map[snssubscribeattribute.SNSSubscribeAttribute]string,
	timeOutDuration ...time.Duration) (subscriptionArn string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-Subscribe", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-Subscribe-TopicArn", topicArn)
			_ = seg.Seg.AddMetadata("SNS-Subscribe-Protocol", protocol)
			_ = seg.Seg.AddMetadata("SNS-Subscribe-Endpoint", endPoint)
			_ = seg.Seg.AddMetadata("SNS-Subscribe-Attributes", attributes)
			_ = seg.Seg.AddMetadata("SNS-Subscribe-Result-SubscriptionArn", subscriptionArn)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("Subscribe Failed: " + "SNS Client is Required")
		return "", err
	}

	if util.LenTrim(topicArn) <= 0 {
		err = errors.New("Subscribe Failed: " + "Topic ARN is Required")
		return "", err
	}

	if !protocol.Valid() || protocol == snsprotocol.UNKNOWN {
		err = errors.New("Subscribe Failed: " + "Protocol is Required")
		return "", err
	}

	if util.LenTrim(endPoint) <= 0 {
		err = errors.New("Subscribe Failed: " + "Endpoint is Required")
		return "", err
	}

	// create input object
	input := &sns.SubscribeInput{
		TopicArn: aws.String(topicArn),
		Protocol: aws.String(protocol.Key()),
		Endpoint: aws.String(endPoint),
	}

	if attributes != nil {
		input.Attributes = s.toAwsSubscribeAttributes(attributes)
	}

	// perform action
	var output *sns.SubscribeOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.SubscribeWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.SubscribeWithContext(segCtx, input)
		} else {
			output, err = client.Subscribe(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("Subscribe Failed: (Subscribe Action) " + err.Error())
		return "", err
	}

	subscriptionArn = aws.StringValue(output.SubscriptionArn)
	return subscriptionArn, nil
}

// Unsubscribe will remove a subscription in SNS via subscriptionArn,
// nil is returned if successful, otherwise err is filled with error info
//
// Parameters:
//  1. subscriptionArn = required, the subscription ARN to remove from SNS
//  2. timeOutDuration = optional, indicates timeout value for context
func (s *SNS) Unsubscribe(subscriptionArn string, timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-Unsubscribe", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-Unsubscribe-SubscriptionArn", subscriptionArn)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("Unsubscribe Failed: " + "SNS Client is Required")
		return err
	}

	if util.LenTrim(subscriptionArn) <= 0 {
		err = errors.New("Unsubscribe Failed: " + "Subscription ARN is Required")
		return err
	}

	// create input object
	input := &sns.UnsubscribeInput{
		SubscriptionArn: aws.String(subscriptionArn),
	}

	// perform action
	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		_, err = client.UnsubscribeWithContext(ctx, input)
	} else {
		if segCtxSet {
			_, err = client.UnsubscribeWithContext(segCtx, input)
		} else {
			_, err = client.Unsubscribe(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("Unsubscribe Failed: (Unsubscribe Action) " + err.Error())
		return err
	} else {
		return nil
	}
}

// ConfirmSubscription will confirm a pending subscription upon receive of SNS notification for subscription confirmation,
// the SNS subscription confirmation will contain a Token which is needed by ConfirmSubscription as input parameter in order to confirm,
//
// Parameters:
//  1. topicArn = required, the topic in SNS to confirm subscription for
//  2. token = required, the token from SNS confirmation notification receive upon call to Subscribe
//  3. timeOutDuration = optional, indicates timeout value for context
//
// Return Values:
//  1. subscriptionArn = upon confirmation, the subscription ARN attained
//  2. err = the error info if any
//
// Subscription Confirmation Support:
//  1. Http / Https / Email Endpoints Requires Subscription Confirmation Support, See Details in URL Below:
//     a) https://docs.aws.amazon.com/sns/latest/dg/sns-http-https-endpoint-as-subscriber.html
//  2. Once Subscribe action is performed, SNS sends confirmation notification to the HTTP/s Endpoint:
//     b) Client Upon Receipt of the SNS Notification, Retrieve Token and Respond via ConfirmSubscription method
func (s *SNS) ConfirmSubscription(topicArn string, token string, timeOutDuration ...time.Duration) (subscriptionArn string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-ConfirmSubscription", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-ConfirmSubscription-TopicArn", topicArn)
			_ = seg.Seg.AddMetadata("SNS-ConfirmSubscription-Token", token)
			_ = seg.Seg.AddMetadata("SNS-ConfirmSubscription-Result-SubscriptionArn", subscriptionArn)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("ConfirmSubscription Failed: " + "SNS Client is Required")
		return "", err
	}

	if util.LenTrim(topicArn) <= 0 {
		err = errors.New("ConfirmSubscription Failed: " + "Topic ARN is Required")
		return "", err
	}

	if util.LenTrim(token) <= 0 {
		err = errors.New("ConfirmSubscription Failed: " + "Token is Required (From Subscribe Action's SNS Confirmation Notification)")
		return "", err
	}

	// create input object
	input := &sns.ConfirmSubscriptionInput{
		TopicArn: aws.String(topicArn),
		Token:    aws.String(token),
	}

	// perform action
	var output *sns.ConfirmSubscriptionOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.ConfirmSubscriptionWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.ConfirmSubscriptionWithContext(segCtx, input)
		} else {
			output, err = client.ConfirmSubscription(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("ConfirmSubscription Failed: (ConfirmSubscription Action) " + err.Error())
		return "", err
	}

	subscriptionArn = aws.StringValue(output.SubscriptionArn)
	return subscriptionArn, nil
}

// ListSubscriptions will list SNS subscriptions, with optional nextToken for retrieving more list from a prior call
//
// Parameters:
//  1. nextToken = optional, if prior call returned more...token, pass in here to retrieve the related list
//  2. timeOutDuration = optional, indicates timeout value for context
//
// Return Values:
//  1. subscriptionsList = *SubscribedTopic slice containing subscriptions along with its related topic, nil if not set
//  2. moreSubscriptionsNextToken = if there are more subscriptions, this token is filled, to query more, use the token as input parameter, blank if no more
//  3. err = error info if any
func (s *SNS) ListSubscriptions(nextToken string, timeOutDuration ...time.Duration) (subscriptionsList []*SubscribedTopic, moreSubscriptionsNextToken string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-ListSubscriptions", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-ListSubscriptions-NextToken", nextToken)
			_ = seg.Seg.AddMetadata("SNS-ListSubscriptions-Result-SubscriptionsList", subscriptionsList)
			_ = seg.Seg.AddMetadata("SNS-ListSubscriptions-Result-NextToken", moreSubscriptionsNextToken)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("ListSubscriptions Failed: " + "SNS Client is Required")
		return nil, "", err
	}

	// create input object
	input := &sns.ListSubscriptionsInput{}

	if util.LenTrim(nextToken) > 0 {
		input.NextToken = aws.String(nextToken)
	}

	// perform action
	var output *sns.ListSubscriptionsOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.ListSubscriptionsWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.ListSubscriptionsWithContext(segCtx, input)
		} else {
			output, err = client.ListSubscriptions(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("ListSubscriptions Failed: (List Action) " + err.Error())
		return nil, "", err
	}

	moreSubscriptionsNextToken = aws.StringValue(output.NextToken)

	if len(output.Subscriptions) > 0 {
		var conv snsprotocol.SNSProtocol

		for _, v := range output.Subscriptions {
			if p, e := conv.ParseByKey(aws.StringValue(v.Protocol)); e == nil {
				subscriptionsList = append(subscriptionsList, &SubscribedTopic{
					SubscriptionArn: aws.StringValue(v.SubscriptionArn),
					TopicArn:        aws.StringValue(v.TopicArn),
					Endpoint:        aws.StringValue(v.Endpoint),
					Owner:           aws.StringValue(v.Owner),
					Protocol:        p,
				})
			}
		}
	}

	return subscriptionsList, moreSubscriptionsNextToken, nil
}

// ListSubscriptionsByTopic will list SNS subscriptions by a specific topic via topicArn,
// with optional nextToken for retrieving more list from a prior call
//
// Parameters:
//  1. topicArn = required, list subscriptions based on this topic ARN
//  2. nextToken = optional, if prior call returned more...token, pass in here to retrieve the related list
//  3. timeOutDuration = optional, indicates timeout value for context
//
// Return Values:
//  1. subscriptionsList = *SubscribedTopic slice containing subscriptions along with its related topic, nil if not set
//  2. moreSubscriptionsNextToken = if there are more subscriptions, this token is filled, to query more, use the token as input parameter, blank if no more
//  3. err = error info if any
func (s *SNS) ListSubscriptionsByTopic(topicArn string, nextToken string, timeOutDuration ...time.Duration) (subscriptionsList []*SubscribedTopic, moreSubscriptionsNextToken string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-ListSubscriptionsByTopic", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-ListSubscriptionsByTopic-TopicArn", topicArn)
			_ = seg.Seg.AddMetadata("SNS-ListSubscriptionsByTopic-NextToken", nextToken)
			_ = seg.Seg.AddMetadata("SNS-ListSubscriptionsByTopic-Result-SubscriptionsList", subscriptionsList)
			_ = seg.Seg.AddMetadata("SNS-ListSubscriptionsByTopic-Result-NextToken", moreSubscriptionsNextToken)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("ListSubscriptionsByTopic Failed: " + "SNS Client is Required")
		return nil, "", err
	}

	if util.LenTrim(topicArn) <= 0 {
		err = errors.New("ListSubscriptionsByTopic Failed: " + "Topic ARN is Required")
		return nil, "", err
	}

	// create input object
	input := &sns.ListSubscriptionsByTopicInput{
		TopicArn: aws.String(topicArn),
	}

	if util.LenTrim(nextToken) > 0 {
		input.NextToken = aws.String(nextToken)
	}

	// perform action
	var output *sns.ListSubscriptionsByTopicOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.ListSubscriptionsByTopicWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.ListSubscriptionsByTopicWithContext(segCtx, input)
		} else {
			output, err = client.ListSubscriptionsByTopic(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("ListSubscriptionsByTopic Failed: (List Action) " + err.Error())
		return nil, "", err
	}

	moreSubscriptionsNextToken = aws.StringValue(output.NextToken)

	if len(output.Subscriptions) > 0 {
		var conv snsprotocol.SNSProtocol

		for _, v := range output.Subscriptions {
			if p, e := conv.ParseByKey(aws.StringValue(v.Protocol)); e == nil {
				subscriptionsList = append(subscriptionsList, &SubscribedTopic{
					SubscriptionArn: aws.StringValue(v.SubscriptionArn),
					TopicArn:        aws.StringValue(v.TopicArn),
					Endpoint:        aws.StringValue(v.Endpoint),
					Owner:           aws.StringValue(v.Owner),
					Protocol:        p,
				})
			}
		}
	}

	return subscriptionsList, moreSubscriptionsNextToken, nil
}

// GetSubscriptionAttributes will retrieve all subscription attributes for a specific subscription based on subscriptionArn
//
// Parameters:
//  1. subscriptionArn = required, the subscriptionArn for which attributes are retrieved from
//  2. timeOutDuration = optional, indicates timeout value for context
//
// Return Values:
//  1. attributes = map of sns get subscription attributes in key value pairs
//  2. err = error info if any
//
// Subscription Attributes: (Key = Expected Value)
//  1. ConfirmationWasAuthenticated = true if the subscription confirmation request was authenticated
//  2. DeliveryPolicy = The JSON serialization of the subscription's delivery policy (See Subscribe Notes)
//  3. EffectiveDeliveryPolicy = The JSON serialization of the effective delivery policy that takes into account the topic delivery policy,
//     and account system defaults (See Subscribe Notes for DeliveryPolicy Json format)
//  4. FilterPolicy = The filter policy JSON that is assigned to the subscription (See Subscribe Notes)
//  5. Owner = The AWS account ID of the subscription's owner
//  6. PendingConfirmation = true if the subscription hasn't been confirmed,
//     To confirm a pending subscription, call the ConfirmSubscription action with a confirmation token
//  7. RawMessageDelivery = true if raw message delivery is enabled for the subscription.
//     Raw messages are free of JSON formatting and can be sent to HTTP/S and Amazon SQS endpoints
//  8. RedrivePolicy = When specified, sends undeliverable messages to the specified Amazon SQS dead-letter queue.
//     Messages that can't be delivered due to client errors (for example, when the subscribed endpoint is unreachable),
//     or server errors (for example, when the service that powers the subscribed endpoint becomes unavailable)
//     are held in the dead-letter queue for further analysis or reprocessing (See Subscribe Notes)
//  9. SubscriptionArn = The subscription's ARN
//  10. TopicArn = The topic ARN that the subscription is associated with
func (s *SNS) GetSubscriptionAttributes(subscriptionArn string, timeOutDuration ...time.Duration) (attributes map[snsgetsubscriptionattribute.SNSGetSubscriptionAttribute]string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-GetSubscriptionAttributes", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-GetSubscriptionAttributes-SubscriptionArn", subscriptionArn)
			_ = seg.Seg.AddMetadata("SNS-GetSubscriptionAttributes-Result-Attributes", attributes)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("GetSubscriptionAttributes Failed: " + "SNS Client is Required")
		return nil, err
	}

	if util.LenTrim(subscriptionArn) <= 0 {
		err = errors.New("GetSubscriptionAttributes Failed: " + "Subscription ARN is Required")
		return nil, err
	}

	// create input object
	input := &sns.GetSubscriptionAttributesInput{
		SubscriptionArn: aws.String(subscriptionArn),
	}

	// perform action
	var output *sns.GetSubscriptionAttributesOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.GetSubscriptionAttributesWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.GetSubscriptionAttributesWithContext(segCtx, input)
		} else {
			output, err = client.GetSubscriptionAttributes(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("GetSubscriptionAttributes Failed: (Get Action) " + err.Error())
		return nil, err
	}

	attributes = s.fromAwsGetSubscriptionAttributes(output.Attributes)
	return attributes, nil
}

// SetSubscriptionAttribute will set or update a subscription attribute,
// For attribute value or Json format, see corresponding notes in Subscribe where applicable
func (s *SNS) SetSubscriptionAttribute(subscriptionArn string,
	attributeName snssubscribeattribute.SNSSubscribeAttribute,
	attributeValue string,
	timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-SetSubscriptionAttribute", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-SetSubscriptionAttribute-SubscriptionArn", subscriptionArn)
			_ = seg.Seg.AddMetadata("SNS-SetSubscriptionAttribute-AttributeName", attributeName)
			_ = seg.Seg.AddMetadata("SNS-SetSubscriptionAttribute-AttributeValue", attributeValue)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("SetSubscriptionAttribute Failed: " + "SNS Client is Required")
		return err
	}

	if util.LenTrim(subscriptionArn) <= 0 {
		err = errors.New("SetSubscriptionAttribute Failed: " + "Subscription ARN is Required")
		return err
	}

	if !attributeName.Valid() || attributeName == snssubscribeattribute.UNKNOWN {
		err = errors.New("SetSubscriptionAttribute Failed: " + "Attribute Name is Required")
		return err
	}

	// create input object
	input := &sns.SetSubscriptionAttributesInput{
		SubscriptionArn: aws.String(subscriptionArn),
		AttributeName:   aws.String(attributeName.Key()),
		AttributeValue:  aws.String(attributeValue),
	}

	// perform action
	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		_, err = client.SetSubscriptionAttributesWithContext(ctx, input)
	} else {
		if segCtxSet {
			_, err = client.SetSubscriptionAttributesWithContext(segCtx, input)
		} else {
			_, err = client.SetSubscriptionAttributes(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("SetSubscriptionAttribute Failed: (Set Action) " + err.Error())
		return err
	} else {
		return nil
	}
}

// ----------------------------------------------------------------------------------------------------------------
// publisher methods
// ----------------------------------------------------------------------------------------------------------------

// Publish will publish a message to a topic or target via topicArn or targetArn respectively,
// upon publish completed, the messageId is returned
//
// Parameters:
//  1. topicArn = required but mutually exclusive, either topicArn or targetArn must be set (but NOT BOTH)
//  2. targetArn = required but mutually exclusive, either topicArn or targetArn must be set (but NOT BOTH)
//  3. message = required, the message to publish, up to 256KB
//  4. subject = optional, only for email endpoints, up to 100 characters
//  5. attributes = optional, message attributes
//     a) Other than defining Endpoint attributes as indicated in note below,
//     b) attributes can also contain Message specific attributes for use for Subscriber Filter Policy and etc,
//     *) For example, custom attribute name and value for the message can be defined in this map as metadata,
//     so that when Subscriber receives it can apply filter policy etc (See Subscribe method Filter Policy for more info)
//     i.e attributes["customer_interests"] = "rugby"
//     i.e attributes["price_usd"] = 100
//  6. timeOutDuration = optional, indicates timeout value for context
//
// Message Attribute Keys:
//  1. ADM
//     a) AWS.SNS.MOBILE.ADM.TTL
//  2. APNs
//     a) AWS.SNS.MOBILE.APNS_MDM.TTL
//     b) AWS.SNS.MOBILE.APNS_MDM_SANDBOX.TTL
//     c) AWS.SNS.MOBILE.APNS_PASSBOOK.TTL
//     d) AWS.SNS.MOBILE.APNS_PASSBOOK_SANDBOX.TTL
//     e) AWS.SNS.MOBILE.APNS_SANDBOX.TTL
//     f) AWS.SNS.MOBILE.APNS_VOIP.TTL
//     g) AWS.SNS.MOBILE.APNS_VOIP_SANDBOX.TTL
//     h) AWS.SNS.MOBILE.APNS.COLLAPSE_ID
//     i) AWS.SNS.MOBILE.APNS.PRIORITY
//     j) AWS.SNS.MOBILE.APNS.PUSH_TYPE
//     k) AWS.SNS.MOBILE.APNS.TOPIC
//     l) AWS.SNS.MOBILE.APNS.TTL
//     m) AWS.SNS.MOBILE.PREFERRED_AUTHENTICATION_METHOD
//  3. Custom Message Attribute Key Value Pairs
//     a) For Use Against Subscriber Filter Policy Matching
func (s *SNS) Publish(topicArn string,
	targetArn string,
	message string,
	subject string,
	attributes map[string]*sns.MessageAttributeValue,
	timeOutDuration ...time.Duration) (messageId string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-Publish", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-Publish-TopicArn", topicArn)
			_ = seg.Seg.AddMetadata("SNS-Publish-TargetArn", targetArn)
			_ = seg.Seg.AddMetadata("SNS-Publish-Message", message)
			_ = seg.Seg.AddMetadata("SNS-Publish-Subject", subject)
			_ = seg.Seg.AddMetadata("SNS-Publish-Attributes", attributes)
			_ = seg.Seg.AddMetadata("SNS-Publish-Result-MessageID", messageId)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("Publish Failed: " + "SNS Client is Required")
		return "", err
	}

	hasTopic := util.LenTrim(topicArn) > 0
	hasTarget := util.LenTrim(targetArn) > 0

	if !hasTopic && !hasTarget {
		err = errors.New("Publish Failed: " + "Either Topic ARN or Target ARN is Required")
		return "", err
	}

	if hasTopic && hasTarget {
		err = errors.New("Publish Failed: " + "Specify only one of Topic ARN or Target ARN, not both")
		return "", err
	}

	if util.LenTrim(message) <= 0 {
		err = errors.New("Publish Failed: " + "Message is Required")
		return "", err
	}

	if trimmedSubject := strings.TrimSpace(subject); len(trimmedSubject) > 0 {
		if utf8.RuneCountInString(trimmedSubject) > 100 {
			err = errors.New("Publish Failed: Subject Maximum Characters is 100")
			return "", err
		}
		subject = trimmedSubject
	}

	// create input object
	input := &sns.PublishInput{
		Message: aws.String(message),
	}

	if hasTopic {
		input.TopicArn = aws.String(topicArn)
	}

	if hasTarget {
		input.TargetArn = aws.String(targetArn)
	}

	if util.LenTrim(subject) > 0 {
		input.Subject = aws.String(subject)
	}

	if attributes != nil {
		input.MessageAttributes = attributes
	}

	// perform action
	var output *sns.PublishOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.PublishWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.PublishWithContext(segCtx, input)
		} else {
			output, err = client.Publish(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("Publish Failed: (Publish Action) " + err.Error())
		return "", err
	} else {
		messageId = aws.StringValue(output.MessageId)
		return messageId, nil
	}
}

// ----------------------------------------------------------------------------------------------------------------
// sms methods
// ----------------------------------------------------------------------------------------------------------------

// SendSMS will send a message to a specific SMS phone number, where phone number is in E.164 format (+12095551212 for example where +1 is country code),
// upon sms successfully sent, the messageId is returned
//
// Parameters:
//  1. phoneNumber = required, phone number to deliver an SMS message. Use E.164 format (+12095551212 where +1 is country code)
//  2. message = required, the message to publish; max is 140 ascii characters (70 characters when in UCS-2 encoding)
//  3. timeOutDuration = optional, indicates timeout value for context
//
// Fixed Message Attributes Explained:
//  1. AWS.SNS.SMS.SenderID = A custom ID that contains 3-11 alphanumeric characters, including at least one letter and no spaces.
//     The sender ID is displayed as the message sender on the receiving device.
//     For example, you can use your business brand to make the message source easier to recognize.
//     Support for sender IDs varies by country and/or region.
//     For example, messages delivered to U.S. phone numbers will not display the sender ID.
//     For the countries and regions that support sender IDs, see Supported Regions and countries.
//     If you do not specify a sender ID, the message will display a long code as the sender ID in supported countries and regions.
//     For countries or regions that require an alphabetic sender ID, the message displays NOTICE as the sender ID.
//     This message-level attribute overrides the account-level attribute DefaultSenderID, which you set using the SetSMSAttributes request.
//  2. AWS.SNS.SMS.SMSType = The type of message that you are sending:
//     a) Promotional = (default) – Noncritical messages, such as marketing messages.
//     Amazon SNS optimizes the message delivery to incur the lowest cost.
//     b) Transactional = Critical messages that support customer transactions,
//     such as one-time passcodes for multi-factor authentication.
//     Amazon SNS optimizes the message delivery to achieve the highest reliability.
//     This message-level attribute overrides the account-level attribute DefaultSMSType,
//     which you set using the SetSMSAttributes request.
func (s *SNS) SendSMS(phoneNumber string,
	message string,
	timeOutDuration ...time.Duration) (messageId string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-SendSMS", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-SendSMS-Phone", phoneNumber)
			_ = seg.Seg.AddMetadata("SNS-SendSMS-Message", message)
			_ = seg.Seg.AddMetadata("SNS-SendSMS-Result-MessageID", messageId)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("SendSMS Failed: " + "SNS Client is Required")
		return "", err
	}

	if util.LenTrim(phoneNumber) <= 0 {
		err = errors.New("SendSMS Failed: " + "SMS Phone Number is Required")
		return "", err
	}

	if err = validateE164Phone(phoneNumber); err != nil {
		err = errors.New("SendSMS Failed: " + err.Error())
		return "", err
	}

	if util.LenTrim(message) <= 0 {
		err = errors.New("SendSMS Failed: " + "Message is Required")
		return "", err
	}

	limit, used, encoding := smsLength(message)
	if used > limit {
		err = fmt.Errorf("SendSMS Failed: message length %d exceeds %d characters for %s encoding", used, limit, encoding)
		return "", err
	}

	if err = validateSenderID(s.getSMSSenderName()); err != nil {
		err = errors.New("SendSMS Failed: " + err.Error())
		return "", err
	}

	// fixed attributes
	m := make(map[string]*sns.MessageAttributeValue)

	if util.LenTrim(s.getSMSSenderName()) > 0 {
		m["AWS.SNS.SMS.SenderID"] = &sns.MessageAttributeValue{StringValue: aws.String(s.getSMSSenderName()), DataType: aws.String("String")}
	}

	smsTypeName := "Promotional"

	if s.getSMSTransactional() {
		smsTypeName = "Transactional"
	}

	m["AWS.SNS.SMS.SMSType"] = &sns.MessageAttributeValue{StringValue: aws.String(smsTypeName), DataType: aws.String("String")}

	// create input object
	input := &sns.PublishInput{
		PhoneNumber:       aws.String(phoneNumber),
		Message:           aws.String(message),
		MessageAttributes: m,
	}

	// perform action
	var output *sns.PublishOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.PublishWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.PublishWithContext(segCtx, input)
		} else {
			output, err = client.Publish(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("SendSMS Failed: (SMS Send Action) " + err.Error())
		return "", err
	} else {
		messageId = aws.StringValue(output.MessageId)
		return messageId, nil
	}
}

// OptInPhoneNumber will opt in a SMS phone number to SNS for receiving messages (explict allow),
// returns nil if successful, otherwise error info is returned
//
// Parameters:
//  1. phoneNumber = required, phone number to opt in. Use E.164 format (+12095551212 where +1 is country code)
//  2. timeOutDuration = optional, indicates timeout value for context
func (s *SNS) OptInPhoneNumber(phoneNumber string, timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-OptInPhoneNumber", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-OptInPhoneNumber-Phone", phoneNumber)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("OptInPhoneNumber Failed: " + "SNS Client is Required")
		return err
	}

	if util.LenTrim(phoneNumber) <= 0 {
		err = errors.New("OptInPhoneNumber Failed: " + "Phone Number is Required, in E.164 Format (i.e. +19255551212)")
		return err
	}

	if err = validateE164Phone(phoneNumber); err != nil {
		err = errors.New("OptInPhoneNumber Failed: " + err.Error())
		return err
	}

	// create input object
	input := &sns.OptInPhoneNumberInput{
		PhoneNumber: aws.String(phoneNumber),
	}

	// perform action
	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		_, err = client.OptInPhoneNumberWithContext(ctx, input)
	} else {
		if segCtxSet {
			_, err = client.OptInPhoneNumberWithContext(segCtx, input)
		} else {
			_, err = client.OptInPhoneNumber(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("OptInPhoneNumber Failed: (Action) " + err.Error())
		return err
	} else {
		return nil
	}
}

// CheckIfPhoneNumberIsOptedOut will verify if a phone number is opted out of message reception
//
// Parameters:
//  1. phoneNumber = required, phone number to check if opted out. Use E.164 format (+12095551212 where +1 is country code)
//  2. timeOutDuration = optional, indicates timeout value for context
//
// Return Values:
//  1. optedOut = bool indicating if the given phone via input parameter was opted out (true), or not (false)
//  2. err = error info if any
func (s *SNS) CheckIfPhoneNumberIsOptedOut(phoneNumber string, timeOutDuration ...time.Duration) (optedOut bool, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-CheckIfPhoneNumberIsOptedOutParentSegment", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-CheckIfPhoneNumberIsOptedOut-Phone", phoneNumber)
			_ = seg.Seg.AddMetadata("SNS-CheckIfPhoneNumberIsOptedOut-Result-OptedOut", optedOut)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("CheckIfPhoneNumberIsOptedOut Failed: " + "SNS Client is Required")
		return false, err
	}

	if util.LenTrim(phoneNumber) <= 0 {
		err = errors.New("CheckIfPhoneNumberIsOptedOut Failed: " + "Phone Number is Required, in E.164 Format (i.e. +19255551212)")
		return false, err
	}

	if err = validateE164Phone(phoneNumber); err != nil {
		err = errors.New("CheckIfPhoneNumberIsOptedOut Failed: " + err.Error())
		return false, err
	}

	// create input object
	input := &sns.CheckIfPhoneNumberIsOptedOutInput{
		PhoneNumber: aws.String(phoneNumber),
	}

	// perform action
	var output *sns.CheckIfPhoneNumberIsOptedOutOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.CheckIfPhoneNumberIsOptedOutWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.CheckIfPhoneNumberIsOptedOutWithContext(segCtx, input)
		} else {
			output, err = client.CheckIfPhoneNumberIsOptedOut(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("CheckIfPhoneNumberIsOptedOut Failed: (Action) " + err.Error())
		return false, err
	} else {
		optedOut = aws.BoolValue(output.IsOptedOut)
		return optedOut, nil
	}
}

// ListPhoneNumbersOptedOut will list opted out phone numbers, with optional nextToken for retrieving more list from a prior call
//
// Parameters:
//  1. nextToken = optional, if prior call returned more...token, pass in here to retrieve the related list
//  2. timeOutDuration = optional, indicates timeout value for context
//
// Return Values:
//  1. phonesList = string slice of opted out phone numbers, nil if not set
//  2. morePhonesNextToken = if there are more opted out phone numbers, this token is filled, to query more, use the token as input parameter, blank if no more
//  3. err = error info if any
func (s *SNS) ListPhoneNumbersOptedOut(nextToken string, timeOutDuration ...time.Duration) (phonesList []string, morePhonesNextToken string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-ListPhoneNumbersOptedOut", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-ListPhoneNumbersOptedOut-NextToken", nextToken)
			_ = seg.Seg.AddMetadata("SNS-ListPhoneNumbersOptedOut-Result-PhonesList", phonesList)
			_ = seg.Seg.AddMetadata("SNS-ListPhoneNumbersOptedOut-Result-NextToken", morePhonesNextToken)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("ListPhoneNumbersOptedOut Failed: " + "SNS Client is Required")
		return nil, "", err
	}

	// create input object
	input := &sns.ListPhoneNumbersOptedOutInput{}

	if util.LenTrim(nextToken) > 0 {
		input.NextToken = aws.String(nextToken)
	}

	// perform action
	var output *sns.ListPhoneNumbersOptedOutOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.ListPhoneNumbersOptedOutWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.ListPhoneNumbersOptedOutWithContext(segCtx, input)
		} else {
			output, err = client.ListPhoneNumbersOptedOut(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("ListPhoneNumbersOptedOut Failed: (Action) " + err.Error())
		return nil, "", err
	}

	morePhonesNextToken = aws.StringValue(output.NextToken)

	phonesList = aws.StringValueSlice(output.PhoneNumbers)
	return phonesList, morePhonesNextToken, nil
}

// ----------------------------------------------------------------------------------------------------------------
// application endpoint APNS (Apple Push Notification Service) / FCM (Firebase Cloud Messaging) methods
// ----------------------------------------------------------------------------------------------------------------

// CreatePlatformApplication will create a SNS platform application used for app notification via APNS, FCM, ADM etc.
// this method creates the application so that then Endpoint (devices that receives) for this application may be created to complete the setup.
//
// Once the application and endpoint is created, then for a device to Subscribe to a topic and receive SNS notifications
// via APNS, FCM, etc, the device will use the Subscribe's protocol as Application, and specify the Endpoint ARN accordingly.
//
// For the device to receive SNS notifications when provider Publish, the appropriate protocol specific setup is needed during
// Endpoint creation, for example, APNS requires to set private key and SSL certificate in Application Attributes' PlatformCredential and PlatformPrincipal (See notes below)
//
// In general, first create the Application via CreatePlatformApplication,
// Once application exists, then for each device that needs to receive SNS notification, create the appropriate Endpoint via CreatePlatformEndpoint
//
// Parameters:
//  1. name = required, platform application name
//  2. platform = required, the sns platform association with this application, such as APNS, FCM etc.
//  3. attributes = required, map of platform application attributes that defines specific values related to the chosen platform (see notes below)
//  4. timeOutDuration = optional, indicates timeout value for context
//
// Return Values:
//  1. platformApplicationArn = the created platform application's ARN
//  2. err = error info if any
//
// Platform Application Attributes: (Key = Expected Value)
//  1. PlatformCredential = The credential received from the notification service,
//     For APNS and APNS_SANDBOX, PlatformCredential is the private key
//     For GCM (Firebase Cloud Messaging), PlatformCredential is API key
//     For ADM, PlatformCredential is client secret
//  2. PlatformPrincipal = The principal received from the notification service,
//     For APNS and APNS_SANDBOX, PlatformPrincipal is SSL certificate
//     For GCM (Firebase Cloud Messaging), there is no PlatformPrincipal
//     For ADM, PlatformPrincipal is client id
//  3. EventEndpointCreated = Topic ARN to which EndpointCreated event notifications are sent
//  4. EventEndpointDeleted = Topic ARN to which EndpointDeleted event notifications are sent
//  5. EventEndpointUpdated = Topic ARN to which EndpointUpdate event notifications are sent
//  6. EventDeliveryFailure = Topic ARN to which DeliveryFailure event notifications are sent upon Direct Publish delivery failure (permanent) to one of the application's endpoints
//  7. SuccessFeedbackRoleArn = IAM role ARN used to give Amazon SNS write access to use CloudWatch Logs on your behalf
//  8. FailureFeedbackRoleArn = IAM role ARN used to give Amazon SNS write access to use CloudWatch Logs on your behalf
//  9. SuccessFeedbackSampleRate = Sample rate percentage (0-100) of successfully delivered messages
func (s *SNS) CreatePlatformApplication(name string,
	platform snsapplicationplatform.SNSApplicationPlatform,
	attributes map[snsplatformapplicationattribute.SNSPlatformApplicationAttribute]string,
	timeOutDuration ...time.Duration) (platformApplicationArn string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-CreatePlatformApplication", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-CreatePlatformApplication-Name", name)
			_ = seg.Seg.AddMetadata("SNS-CreatePlatformApplication-Platform", platform)
			_ = seg.Seg.AddMetadata("SNS-CreatePlatformApplication-Attributes", attributes)
			_ = seg.Seg.AddMetadata("SNS-CreatePlatformApplication-Result-PlatformApplicationArn", platformApplicationArn)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("CreatePlatformApplication Failed: " + "SNS Client is Required")
		return "", err
	}

	if util.LenTrim(name) <= 0 {
		err = errors.New("CreatePlatformApplication Failed: " + "Name is Required")
		return "", err
	}

	if !platform.Valid() || platform == snsapplicationplatform.UNKNOWN {
		err = errors.New("CreatePlatformApplication Failed: " + "Platform is Required")
		return "", err
	}

	if attributes == nil {
		err = errors.New("CreatePlatformApplication Failed: " + "Attributes Map is Required")
		return "", err
	}

	// create input object
	input := &sns.CreatePlatformApplicationInput{
		Name:       aws.String(name),
		Platform:   aws.String(platform.Key()),
		Attributes: s.toAwsPlatformApplicationAttributes(attributes),
	}

	// perform action
	var output *sns.CreatePlatformApplicationOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.CreatePlatformApplicationWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.CreatePlatformApplicationWithContext(segCtx, input)
		} else {
			output, err = client.CreatePlatformApplication(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("CreatePlatformApplication Failed: (Create Action) " + err.Error())
		return "", err
	} else {
		platformApplicationArn = aws.StringValue(output.PlatformApplicationArn)
		return platformApplicationArn, nil
	}
}

// DeletePlatformApplication will delete a platform application by platformApplicationArn,
// returns nil if successful, otherwise error info is returned
//
// Parameters:
//  1. platformApplicationArn = the platform application to delete via platform application ARN specified
//  2. timeOutDuration = optional, indicates timeout value for context
func (s *SNS) DeletePlatformApplication(platformApplicationArn string, timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-DeletePlatformApplication", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-DeletePlatformApplication-PlatformApplicationArn", platformApplicationArn)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("DeletePlatformApplication Failed: " + "SNS Client is Required")
		return err
	}

	if util.LenTrim(platformApplicationArn) <= 0 {
		err = errors.New("DeletePlatformApplication Failed: " + "Platform Application ARN is Required")
		return err
	}

	// create input object
	input := &sns.DeletePlatformApplicationInput{
		PlatformApplicationArn: aws.String(platformApplicationArn),
	}

	// perform action
	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		_, err = client.DeletePlatformApplicationWithContext(ctx, input)
	} else {
		if segCtxSet {
			_, err = client.DeletePlatformApplicationWithContext(segCtx, input)
		} else {
			_, err = client.DeletePlatformApplication(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("DeletePlatformApplication Failed: (Delete Action) " + err.Error())
		return err
	} else {
		return nil
	}
}

// ListPlatformApplications will list platform application ARNs, with optional nextToken for retrieving more list from a prior call
//
// Parameters:
//  1. nextToken = optional, if prior call returned more...token, pass in here to retrieve the related list
//  2. timeOutDuration = optional, indicates timeout value for context
//
// Return Values:
//  1. platformApplicationArnsList = string slice of platform application ARNs, nil if not set
//  2. moreAppArnsNextToken = if there are more platform application ARNs, this token is filled, to query more, use the token as input parameter, blank if no more
//  3. err = error info if any
func (s *SNS) ListPlatformApplications(nextToken string, timeOutDuration ...time.Duration) (platformApplicationArnsList []string, moreAppArnsNextToken string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-ListPlatformApplications", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-ListPlatformApplications-NextToken", nextToken)
			_ = seg.Seg.AddMetadata("SNS-ListPlatformApplications-Result-PlatformApplicationArnsList", platformApplicationArnsList)
			_ = seg.Seg.AddMetadata("SNS-ListPlatformApplications-Result-NextToken", moreAppArnsNextToken)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("ListPlatformApplications Failed: " + "SNS Client is Required")
		return nil, "", err
	}

	// create input object
	input := &sns.ListPlatformApplicationsInput{}

	if util.LenTrim(nextToken) > 0 {
		input.NextToken = aws.String(nextToken)
	}

	// perform action
	var output *sns.ListPlatformApplicationsOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.ListPlatformApplicationsWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.ListPlatformApplicationsWithContext(segCtx, input)
		} else {
			output, err = client.ListPlatformApplications(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("ListPlatformApplications Failed: (List Action) " + err.Error())
		return nil, "", err
	}

	moreAppArnsNextToken = aws.StringValue(output.NextToken)

	for _, v := range output.PlatformApplications {
		if v != nil {
			if v1 := aws.StringValue(v.PlatformApplicationArn); util.LenTrim(v1) > 0 {
				platformApplicationArnsList = append(platformApplicationArnsList, v1)
			}
		}
	}

	return platformApplicationArnsList, moreAppArnsNextToken, nil
}

// GetPlatformApplicationAttributes will retrieve application attributes based on a specific platform application ARN
//
// Parameters:
//  1. platformApplicationArn = required, the platform application ARN used to retrieve related application attributes
//  2. timeOutDuration = optional, indicates timeout value for context
//
// Return Values:
//  1. attributes = map of sns platform application attributes related to the given platform application ARN
//  2. err = error info if any
func (s *SNS) GetPlatformApplicationAttributes(platformApplicationArn string, timeOutDuration ...time.Duration) (attributes map[snsplatformapplicationattribute.SNSPlatformApplicationAttribute]string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-GetPlatformApplicationAttributes", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-GetPlatformApplicationAttributes-PlatformApplicationArn", platformApplicationArn)
			_ = seg.Seg.AddMetadata("SNS-GetPlatformApplicationAttributes-Result-Attributes", attributes)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("GetPlatformApplicationAttributes Failed: " + "SNS Client is Required")
		return nil, err
	}

	if util.LenTrim(platformApplicationArn) <= 0 {
		err = errors.New("GetPlatformApplicationAttributes Failed: " + "Platform Application ARN is Required")
		return nil, err
	}

	// create input object
	input := &sns.GetPlatformApplicationAttributesInput{
		PlatformApplicationArn: aws.String(platformApplicationArn),
	}

	// perform action
	var output *sns.GetPlatformApplicationAttributesOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.GetPlatformApplicationAttributesWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.GetPlatformApplicationAttributesWithContext(segCtx, input)
		} else {
			output, err = client.GetPlatformApplicationAttributes(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("GetPlatformApplicationAttributes Failed: (Get Action) " + err.Error())
		return nil, err
	}

	attributes = s.fromAwsPlatformApplicationAttributes(output.Attributes)
	return attributes, nil
}

// SetPlatformApplicationAttributes will set or update platform application attributes,
// For attribute value or Json format, see corresponding notes in CreatePlatformApplication where applicable
func (s *SNS) SetPlatformApplicationAttributes(platformApplicationArn string,
	attributes map[snsplatformapplicationattribute.SNSPlatformApplicationAttribute]string,
	timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-SetPlatformApplicationAttributes", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-SetPlatformApplicationAttributes-PlatformApplicationArn", platformApplicationArn)
			_ = seg.Seg.AddMetadata("SNS-SetPlatformApplicationAttributes-Attributes", attributes)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	client := s.getClient()
	if client == nil {
		err = errors.New("SetPlatformApplicationAttributes Failed: " + "SNS Client is Required")
		return err
	}

	if util.LenTrim(platformApplicationArn) <= 0 {
		err = errors.New("SetPlatformApplicationAttributes Failed: " + "Platform Application ARN is Required")
		return err
	}

	if attributes == nil {
		err = errors.New("SetPlatformApplicationAttributes Failed: " + "Attributes Map is Required")
		return err
	}

	// create input
	input := &sns.SetPlatformApplicationAttributesInput{
		PlatformApplicationArn: aws.String(platformApplicationArn),
		Attributes:             s.toAwsPlatformApplicationAttributes(attributes),
	}

	// perform action
	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		_, err = client.SetPlatformApplicationAttributesWithContext(ctx, input)
	} else {
		if segCtxSet {
			_, err = client.SetPlatformApplicationAttributesWithContext(segCtx, input)
		} else {
			_, err = client.SetPlatformApplicationAttributes(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("SetPlatformApplicationAttributes Failed: (Set Action) " + err.Error())
		return err
	} else {
		return nil
	}
}

// CreatePlatformEndpoint will create a device endpoint for a specific platform application,
// this is the endpoint that will receive SNS notifications via defined protocol such as APNS or FCM
//
// Parameters:
//  1. platformApplicationArn = required, Plaform application ARN that was created, endpoint is added to this platform application
//  2. token = Unique identifier created by the notification service for an app on a device,
//     The specific name for Token will vary, depending on which notification service is being used,
//     For example, when using APNS as the notification service, you need the device token,
//     Alternatively, when using FCM or ADM, the device token equivalent is called the registration ID
//  3. customUserData = optional, Arbitrary user data to associate with the endpoint,
//     Amazon SNS does not use this data. The data must be in UTF-8 format and less than 2KB
//  4. timeOutDuration = optional, indicates timeout value for context
//
// Return Values:
//  1. endpointArn = the created endpoint ARN
//  2. err = the error info if any
func (s *SNS) CreatePlatformEndpoint(platformApplicationArn string,
	token string,
	customUserData string,
	timeOutDuration ...time.Duration) (endpointArn string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-CreatePlatformEndpoint", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-CreatePlatformEndpoint-PlatformApplicationArn", platformApplicationArn)
			_ = seg.Seg.AddMetadata("SNS-CreatePlatformEndpoint-Token", token)
			_ = seg.Seg.AddMetadata("SNS-CreatePlatformEndpoint-CustomUserData", customUserData)
			_ = seg.Seg.AddMetadata("SNS-CreatePlatformEndpoint-Result-EndpointArn", endpointArn)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("CreatePlatformEndpoint Failed: " + "SNS Client is Required")
		return "", err
	}

	if util.LenTrim(platformApplicationArn) <= 0 {
		err = errors.New("CreatePlatformEndpoint Failed: " + "Platform Application ARN is Required")
		return "", err
	}

	if util.LenTrim(token) <= 0 {
		err = errors.New("CreatePlatformEndpoint Failed: " + "Token is Required")
		return "", err
	}

	// create input object
	input := &sns.CreatePlatformEndpointInput{
		PlatformApplicationArn: aws.String(platformApplicationArn),
		Token:                  aws.String(token),
	}

	if util.LenTrim(customUserData) > 0 {
		input.CustomUserData = aws.String(customUserData)
	}

	// perform action
	var output *sns.CreatePlatformEndpointOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.CreatePlatformEndpointWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.CreatePlatformEndpointWithContext(segCtx, input)
		} else {
			output, err = client.CreatePlatformEndpoint(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("CreatePlatformEndpoint Failed: (Create Action) " + err.Error())
		return "", err
	} else {
		endpointArn = aws.StringValue(output.EndpointArn)
		return endpointArn, nil
	}
}

// DeletePlatformEndpoint will delete an endpoint based on endpointArn,
// returns nil if successful, otherwise error info is returned
//
// Parameters:
//  1. endpointArn = required, the endpoint to delete
//  2. timeOutDuration = optional, indicates timeout value for context
func (s *SNS) DeletePlatformEndpoint(endpointArn string, timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-DeletePlatformEndpoint", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-DeletePlatformEndpoint-EndpointArn", endpointArn)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("DeletePlatformEndpoint Failed: " + "SNS Client is Required")
		return err
	}

	if util.LenTrim(endpointArn) <= 0 {
		err = errors.New("DeletePlatformEndpoint Failed: " + "Endpoint ARN is Required")
		return err
	}

	// create input object
	input := &sns.DeleteEndpointInput{
		EndpointArn: aws.String(endpointArn),
	}

	// perform action
	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		_, err = client.DeleteEndpointWithContext(ctx, input)
	} else {
		if segCtxSet {
			_, err = client.DeleteEndpointWithContext(segCtx, input)
		} else {
			_, err = client.DeleteEndpoint(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("DeletePlatformEndpoint Failed: (Delete Action) " + err.Error())
		return err
	} else {
		return nil
	}
}

// ListEndpointsByPlatformApplication will list endpoints by platform application, with optional nextToken for retrieving more list from a prior call
//
// Parameters:
//  1. platformApplicationArn = required, the platform application to filter for its related endpoints to retrieve
//  2. nextToken = optional, if prior call returned more...token, pass in here to retrieve the related list
//  3. timeOutDuration = optional, indicates timeout value for context
//
// Return Values:
//  1. endpointArnsList = string slice of endpoint ARNs under the given platform application ARN, nil if not set
//  2. moreEndpointArnsNextToken = if there are more endpoints to load, this token is filled, to query more, use the token as input parameter, blank if no more
//  3. err = error info if any
func (s *SNS) ListEndpointsByPlatformApplication(platformApplicationArn string,
	nextToken string,
	timeOutDuration ...time.Duration) (endpointArnsList []string, moreEndpointArnsNextToken string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-ListEndpointsByPlatformApplication", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-ListEndpointsByPlatformApplication-PlatformApplicationArn", platformApplicationArn)
			_ = seg.Seg.AddMetadata("SNS-ListEndpointsByPlatformApplication-NextToken", nextToken)
			_ = seg.Seg.AddMetadata("SNS-ListEndpointsByPlatformApplication-Result-EndpointArnsList", endpointArnsList)
			_ = seg.Seg.AddMetadata("SNS-ListEndpointsByPlatformApplication-Result-NextToken", moreEndpointArnsNextToken)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("ListEndpointsByPlatformApplication Failed: " + "SNS Client is Required")
		return nil, "", err
	}

	if util.LenTrim(platformApplicationArn) <= 0 {
		err = errors.New("ListEndpointsByPlatformApplication Failed: " + "Platform Application ARN is Required")
		return nil, "", err
	}

	// create input object
	input := &sns.ListEndpointsByPlatformApplicationInput{
		PlatformApplicationArn: aws.String(platformApplicationArn),
	}

	if util.LenTrim(nextToken) > 0 {
		input.NextToken = aws.String(nextToken)
	}

	// perform action
	var output *sns.ListEndpointsByPlatformApplicationOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.ListEndpointsByPlatformApplicationWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.ListEndpointsByPlatformApplicationWithContext(segCtx, input)
		} else {
			output, err = client.ListEndpointsByPlatformApplication(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("ListEndpointsByPlatformApplication Failed: (List Action) " + err.Error())
		return nil, "", err
	}

	moreEndpointArnsNextToken = aws.StringValue(output.NextToken)

	for _, v := range output.Endpoints {
		if v != nil {
			if v1 := aws.StringValue(v.EndpointArn); util.LenTrim(v1) > 0 {
				endpointArnsList = append(endpointArnsList, v1)
			}
		}
	}

	return endpointArnsList, moreEndpointArnsNextToken, nil
}

// GetPlatformEndpointAttributes will retrieve endpoint attributes based on a specific endpoint ARN
//
// Parameters:
//  1. endpointArn = required, the endpoint ARN used to retrieve related endpoint attributes
//  2. timeOutDuration = optional, indicates timeout value for context
//
// Return Values:
//  1. attributes = map of sns endpoint attributes related to the given endpoint ARN
//  2. err = error info if any
//
// Endpoint Attributes: (Key = Expected Value)
//  1. CustomUserData = arbitrary user data to associate with the endpoint.
//     Amazon SNS does not use this data.
//     The data must be in UTF-8 format and less than 2KB.
//  2. Enabled = flag that enables/disables delivery to the endpoint. Amazon
//     SNS will set this to false when a notification service indicates to Amazon SNS that the endpoint is invalid.
//     Users can set it back to true, typically after updating Token.
//  3. Token = device token, also referred to as a registration id, for an app and mobile device.
//     This is returned from the notification service when an app and mobile device are registered with the notification service.
//     The device token for the iOS platform is returned in lowercase.
func (s *SNS) GetPlatformEndpointAttributes(endpointArn string, timeOutDuration ...time.Duration) (attributes map[snsendpointattribute.SNSEndpointAttribute]string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-GetPlatformEndpointAttributes", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-GetPlatformEndpointAttributes-EndpointArn", endpointArn)
			_ = seg.Seg.AddMetadata("SNS-GetPlatformEndpointAttributes-Result-Attributes", attributes)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("GetPlatformEndpointAttributes Failed: " + "SNS Client is Required")
		return nil, err
	}

	if util.LenTrim(endpointArn) <= 0 {
		err = errors.New("GetPlatformEndpointAttributes Failed: " + "Endpoint ARN is Required")
		return nil, err
	}

	// create input object
	input := &sns.GetEndpointAttributesInput{
		EndpointArn: aws.String(endpointArn),
	}

	// perform action
	var output *sns.GetEndpointAttributesOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		output, err = client.GetEndpointAttributesWithContext(ctx, input)
	} else {
		if segCtxSet {
			output, err = client.GetEndpointAttributesWithContext(segCtx, input)
		} else {
			output, err = client.GetEndpointAttributes(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("GetPlatformEndpointAttributes Failed: (Get Action) " + err.Error())
		return nil, err
	} else {
		attributes = s.fromAwsEndpointAttributes(output.Attributes)
		return attributes, nil
	}
}

// SetPlatformEndpointAttributes will set or update platform endpoint attributes,
// For attribute value or Json format, see corresponding notes in CreatePlatformEndpoint where applicable
func (s *SNS) SetPlatformEndpointAttributes(endpointArn string,
	attributes map[snsendpointattribute.SNSEndpointAttribute]string,
	timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SNS-SetPlatformEndpointAttributes", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("SNS-SetPlatformEndpointAttributes-EndpointArn", endpointArn)
			_ = seg.Seg.AddMetadata("SNS-SetPlatformEndpointAttributes-Attributes", attributes)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validation
	client := s.getClient()
	if client == nil {
		err = errors.New("SetPlatformEndpointAttributes Failed: " + "SNS Client is Required")
		return err
	}

	if util.LenTrim(endpointArn) <= 0 {
		err = errors.New("SetPlatformEndpointAttributes Failed: " + "Endpoint ARN is Required")
		return err
	}

	if attributes == nil {
		err = errors.New("SetPlatformEndpointAttributes Failed: " + "Attributes Map is Required")
		return err
	}

	// create input
	input := &sns.SetEndpointAttributesInput{
		EndpointArn: aws.String(endpointArn),
		Attributes:  s.toAwsEndpointAttributes(attributes),
	}

	// perform action
	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(segCtx, timeOutDuration[0])
		defer cancel()

		_, err = client.SetEndpointAttributesWithContext(ctx, input)
	} else {
		if segCtxSet {
			_, err = client.SetEndpointAttributesWithContext(segCtx, input)
		} else {
			_, err = client.SetEndpointAttributes(input)
		}
	}

	// evaluate result
	if err != nil {
		err = errors.New("SetPlatformEndpointAttributes Failed: (Set Action) " + err.Error())
		return err
	} else {
		return nil
	}
}
