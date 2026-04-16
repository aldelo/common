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
	"sort"
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

// defaultSNSCallTimeout bounds any single SNS SDK call that would
// otherwise have no deadline.
//
// SP-008 P2-CMN-3 (2026-04-15): Publish and SendSMS previously fell
// into `client.Publish(input)` when the caller supplied no
// timeOutDuration and xray was disabled. That path uses
// context.Background() internally, so a hung AWS endpoint could block
// the caller indefinitely. 30s matches the KMS default (see
// wrapper/kms/kms.go defaultKMSCallTimeout): long enough for any
// legitimate SNS publish to complete (real-world P99 is <2s) and
// short enough to fail fast when the endpoint is unreachable.
const defaultSNSCallTimeout = 30 * time.Second

// ensureSNSCtx normalizes the (segCtx, segCtxSet, timeOutDuration)
// trilemma into a single bounded context. Precedence:
//  1. If timeOutDuration is set, the caller-supplied timeout wins
//     (context is derived from segCtx so xray parents stay attached).
//  2. Else if segCtxSet (xray active), the xray segment ctx is
//     wrapped in WithTimeout(defaultSNSCallTimeout). Xray segment
//     lineage is preserved via the segCtx parent, and a deadline is
//     enforced so hung AWS endpoints cannot block indefinitely.
//  3. Else a fresh context.Background() is wrapped in
//     WithTimeout(defaultSNSCallTimeout) so unsupervised calls cannot
//     block indefinitely on a hung AWS endpoint.
//
// Callers MUST invoke the returned cancel before returning from the
// enclosing method (use defer or an explicit call after the SDK
// invocation). All three branches return a real cancel func — no-op
// cancels are no longer possible after SP-008 pass-5 A1-F1.
func ensureSNSCtx(segCtx context.Context, segCtxSet bool, timeOutDuration []time.Duration) (context.Context, context.CancelFunc) {
	// SP-008 re-eval follow-up (2026-04-15): defensive nil-segCtx guard
	// for case 1. Every current caller passes a non-nil segCtx derived
	// from ensureSegCtx/segment, but the signature accepts plain
	// context.Context, so "trust the caller" is fragile: a future caller
	// that constructs a partial SNS wrapper state with no xray parent
	// could legally pass nil + a non-empty timeOutDuration, at which
	// point context.WithTimeout(nil, ...) panics. Fall back to Background
	// so the deadline still applies and the call-site shape stays uniform.
	if len(timeOutDuration) > 0 {
		parent := segCtx
		if parent == nil {
			parent = context.Background()
		}
		return context.WithTimeout(parent, timeOutDuration[0])
	}
	if segCtxSet && segCtx != nil {
		// SP-008 pass-5 A1-F1 (2026-04-15): wrap the xray segment ctx
		// with defaultSNSCallTimeout. Prior to this fix, branch 2
		// returned the xray ctx as-is with a no-op cancel, which left
		// SNS calls deadline-less under xray-on because
		// xray.BeginSegment derives its child ctx from
		// context.Background() (see wrapper/xray/xray.go:405). A hung
		// AWS endpoint under xray-on therefore parked goroutines
		// indefinitely. WithTimeout preserves the segment lineage via
		// the segCtx parent while enforcing the same 30s deadline as
		// branch 3.
		return context.WithTimeout(segCtx, defaultSNSCallTimeout)
	}
	return context.WithTimeout(context.Background(), defaultSNSCallTimeout)
}

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
	if s == nil {
		return awsregion.UNKNOWN
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.AwsRegion
}

func (s *SNS) getSMSSenderName() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SMSSenderName
}

func (s *SNS) getSMSTransactional() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.SMSTransactional
}

func (s *SNS) getClient() *sns.SNS {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snsClient
}

func (s *SNS) setClient(cli *sns.SNS) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snsClient = cli
}

func (s *SNS) clearClient() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snsClient = nil
}

func (s *SNS) ensureHttpOptions() *awshttp2.HttpClientSettings {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.HttpOptions == nil {
		s.HttpOptions = new(awshttp2.HttpClientSettings)
	}
	return s.HttpOptions
}

func (s *SNS) setParentSegment(seg *xray.XRayParentSegment) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s._parentSegment = seg
}

func (s *SNS) getParentSegment() *xray.XRayParentSegment {
	if s == nil {
		return nil
	}
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

// maskPhoneForXray returns a PII-reduced form of a phone number suitable
// for xray metadata. The country-code prefix and the last 4 digits are
// retained (enough to debug an ARN/endpoint mismatch); the middle digits
// are replaced with asterisks so the metadata cannot be pivoted to a
// natural-person identity by a reader of the xray trace.
//
// SP-008 P1-COMMON-SNS-01 (2026-04-15): used by OptInPhoneNumber,
// CheckIfPhoneNumberIsOptedOut, and ListPhoneNumbersOptedOut xray emit
// sites. SP-010 pass-5 A1-F3 (2026-04-15) extended usage to SendSMS —
// the three "admin" phone APIs were masking while SendSMS (the
// highest-volume delivery method) left the full E.164 in xray metadata.
// The asymmetry had no principled basis; pass-5 closed it so every
// phone arg that reaches an xray emit is masked to country-code +
// last-4-subscriber. Last-4 is still sufficient for correlating "did
// this device get the SMS?" during an incident.
//
// Edge cases: inputs shorter than 7 runes (which would not validate as
// E.164 anyway — the mask needs "+X" + middle + "NNNN") are returned
// verbatim. Anything without a "+" prefix is also returned verbatim
// since it cannot be interpreted as E.164 in the first place. A 7-rune
// input leaves exactly one rune masked ("+1*3456"), which is the
// minimum useful output.
//
// SP-010 pass-5 A1-F4 (2026-04-15): the helper walks the input as
// runes, not bytes. The prior byte-slice form (phone[:2],
// phone[len(phone)-4:]) was incidentally correct only because valid
// E.164 is ASCII. maskPhoneForXray is called from deferred xray-emit
// closures which fire regardless of whether validateE164Phone accepted
// or rejected the input, so a non-ASCII string CAN reach the helper in
// practice. Byte-slicing such an input would split a rune mid-codepoint
// and emit invalid UTF-8 bytes into xray metadata, which downstream
// xray sanitizers may escape as U+FFFD or reject outright. Walking as
// runes eliminates the class at the cost of one []rune allocation per
// call — the helper is not on a measured hot path, so the allocation
// is immaterial.
func maskPhoneForXray(phone string) string {
	// E.164 is always "+<country><subscriber>"; keep the leading "+"
	// plus the country code's first digit (head = 2 runes) and the
	// final 4 subscriber digits (tail = 4 runes), replace the rest
	// with asterisks. Length threshold 7 = len(head) + len(tail) + 1.
	runes := []rune(phone)
	if len(runes) < 7 || runes[0] != '+' {
		return phone
	}
	const headLen, tailLen = 2, 4
	head := string(runes[:headLen])
	tail := string(runes[len(runes)-tailLen:])
	return head + strings.Repeat("*", len(runes)-headLen-tailLen) + tail
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
	if s == nil {
		return errors.New("cannot connect: SNS instance is nil")
	}
	if len(parentSegment) > 0 {
		s.setParentSegment(parentSegment[0])
	}

	if xray.XRayServiceOn() {
		seg := xray.NewSegment("SNS-Connect", s.getParentSegment())
		defer seg.Close()
		defer func() {
			// COMMON-R2-001: sampled xray failure logging instead of silent discard
			if e := seg.SafeAddMetadata("SNS-AWS-Region", s.getAwsRegion()); e != nil {
				xray.LogXrayAddFailure("SNS-Connect", e)
			}
			if e := seg.SafeAddMetadata("SNS-SMS-Sender-Name", s.getSMSSenderName()); e != nil {
				xray.LogXrayAddFailure("SNS-Connect", e)
			}
			if e := seg.SafeAddMetadata("SNS-SMS-Transactional", s.getSMSTransactional()); e != nil {
				xray.LogXrayAddFailure("SNS-Connect", e)
			}

			if err != nil {
				if e := seg.SafeAddError(err); e != nil {
					xray.LogXrayAddFailure("SNS-Connect", e)
				}
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
	region := s.getAwsRegion()
	if !region.Valid() || region == awsregion.UNKNOWN {
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
			Region:     aws.String(region.Key()),
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
	if s == nil {
		return
	}
	s.clearClient()
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (s *SNS) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	if s == nil {
		return
	}
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
			// COMMON-R2-001: sampled xray failure logging instead of silent discard
			if e := seg.SafeAddMetadata("SNS-CreateTopic-TopicName", topicName); e != nil {
				xray.LogXrayAddFailure("SNS-CreateTopic", e)
			}
			if e := seg.SafeAddMetadata("SNS-CreateTopic-Attributes", attributes); e != nil {
				xray.LogXrayAddFailure("SNS-CreateTopic", e)
			}
			if e := seg.SafeAddMetadata("SNS-CreateTopic-Result-TopicArn", topicArn); e != nil {
				xray.LogXrayAddFailure("SNS-CreateTopic", e)
			}

			if err != nil {
				if e := seg.SafeAddError(err); e != nil {
					xray.LogXrayAddFailure("SNS-CreateTopic", e)
				}
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

	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.CreateTopicWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-DeleteTopic-TopicArn", topicArn)

			if err != nil {
				_ = seg.SafeAddError(err)
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
	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	_, err = client.DeleteTopicWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-ListTopics-NextToken", nextToken)
			_ = seg.SafeAddMetadata("SNS-ListTopics-Result-TopicArnsList", topicArnsList)
			_ = seg.SafeAddMetadata("SNS-ListTopics-Result-NextToken", moreTopicArnsNextToken)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.ListTopicsWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-GetTopicAttributes-TopicArn", topicArn)
			_ = seg.SafeAddMetadata("SNS-GetTopicAttributes-Result-Attributes", attributes)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.GetTopicAttributesWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-SetTopicAttribute-TopicArn", topicArn)
			_ = seg.SafeAddMetadata("SNS-SetTopicAttribute-AttributeName", attributeName)
			_ = seg.SafeAddMetadata("SNS-SetTopicAttribute-AttributeValue", attributeValue)

			if err != nil {
				_ = seg.SafeAddError(err)
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
	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	_, err = client.SetTopicAttributesWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-Subscribe-TopicArn", topicArn)
			_ = seg.SafeAddMetadata("SNS-Subscribe-Protocol", protocol)
			_ = seg.SafeAddMetadata("SNS-Subscribe-Endpoint", endPoint)
			_ = seg.SafeAddMetadata("SNS-Subscribe-Attributes", attributes)
			_ = seg.SafeAddMetadata("SNS-Subscribe-Result-SubscriptionArn", subscriptionArn)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.SubscribeWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-Unsubscribe-SubscriptionArn", subscriptionArn)

			if err != nil {
				_ = seg.SafeAddError(err)
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
	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	_, err = client.UnsubscribeWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-ConfirmSubscription-TopicArn", topicArn)
			// SEC-002 (2026-04-16): the SNS confirmation token is a
			// security credential — never emit the raw value to xray.
			// Length is sufficient for debugging (was a token present?
			// how long was it?).
			_ = seg.SafeAddMetadata("SNS-ConfirmSubscription-Token-Len", len(token))
			_ = seg.SafeAddMetadata("SNS-ConfirmSubscription-Result-SubscriptionArn", subscriptionArn)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.ConfirmSubscriptionWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-ListSubscriptions-NextToken", nextToken)
			_ = seg.SafeAddMetadata("SNS-ListSubscriptions-Result-SubscriptionsList", subscriptionsList)
			_ = seg.SafeAddMetadata("SNS-ListSubscriptions-Result-NextToken", moreSubscriptionsNextToken)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.ListSubscriptionsWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-ListSubscriptionsByTopic-TopicArn", topicArn)
			_ = seg.SafeAddMetadata("SNS-ListSubscriptionsByTopic-NextToken", nextToken)
			_ = seg.SafeAddMetadata("SNS-ListSubscriptionsByTopic-Result-SubscriptionsList", subscriptionsList)
			_ = seg.SafeAddMetadata("SNS-ListSubscriptionsByTopic-Result-NextToken", moreSubscriptionsNextToken)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.ListSubscriptionsByTopicWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-GetSubscriptionAttributes-SubscriptionArn", subscriptionArn)
			_ = seg.SafeAddMetadata("SNS-GetSubscriptionAttributes-Result-Attributes", attributes)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.GetSubscriptionAttributesWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-SetSubscriptionAttribute-SubscriptionArn", subscriptionArn)
			_ = seg.SafeAddMetadata("SNS-SetSubscriptionAttribute-AttributeName", attributeName)
			_ = seg.SafeAddMetadata("SNS-SetSubscriptionAttribute-AttributeValue", attributeValue)

			if err != nil {
				_ = seg.SafeAddError(err)
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
	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	_, err = client.SetSubscriptionAttributesWithContext(callCtx, input)
	callCancel()

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
			// F4 (pass-3 contrarian, 2026-04-14): xray segment metadata is
			// a PII-exposure surface. Publish may carry arbitrary payloads
			// (including PII or credentials) and arbitrary attribute
			// values. Record only length-plus-key-list so traces never
			// contain raw message bodies or attribute values.
			// Observable contract change per workspace rule #10 — prior
			// consumers saw full message/attributes; now see length +
			// sorted key list. No known alarm depends on metadata content.
			_ = seg.SafeAddMetadata("SNS-Publish-TopicArn", topicArn)
			_ = seg.SafeAddMetadata("SNS-Publish-TargetArn", targetArn)
			_ = seg.SafeAddMetadata("SNS-Publish-Message-Length", len(message))
			_ = seg.SafeAddMetadata("SNS-Publish-Subject", subject)
			_ = seg.SafeAddMetadata("SNS-Publish-Attribute-Keys", sortedAttributeKeys(attributes))
			_ = seg.SafeAddMetadata("SNS-Publish-Result-MessageID", messageId)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	const maxSNSMessageBytes = 256 * 1024
	// SP-008 P3-CMN-2: len(s) on a Go string returns byte count directly
	// without the []byte allocation — avoids copying up to 256KiB per call.
	if len(message) > maxSNSMessageBytes {
		err = fmt.Errorf("Publish Failed: message exceeds %d bytes SNS limit", maxSNSMessageBytes)
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

	// SP-008 P2-CMN-3 + pass-5 A1-F1 (2026-04-15): always bound via
	// ensureSNSCtx. Caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No Publish can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	pubCtx, pubCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.PublishWithContext(pubCtx, input)
	pubCancel()

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
			// F5 (pass-3, 2026-04-14): SMS bodies commonly carry OTP /
			// MFA codes and other auth secrets — record length only,
			// never content.
			//
			// A1-F3 (pass-5, 2026-04-15): phone number is now masked
			// via maskPhoneForXray, matching OptInPhoneNumber,
			// CheckIfPhoneNumberIsOptedOut, and ListPhoneNumbersOptedOut.
			// Pass-3 originally rationalized the unmasked emit here as
			// "delivery destination needed for operational debugging"
			// but pass-5 showed that the three masked siblings log
			// phone numbers for the same debugging reason and there
			// is no principled distinction — SendSMS is the highest-
			// volume phone API in this package, so leaving it unmasked
			// left the head of the PII distribution exposed while the
			// long tail was already masked. The masked form retains
			// country code + last 4 subscriber digits which remains
			// sufficient for "did this device get the SMS?" correlation
			// during an incident.
			_ = seg.SafeAddMetadata("SNS-SendSMS-Phone", maskPhoneForXray(phoneNumber))
			_ = seg.SafeAddMetadata("SNS-SendSMS-Message-Length", len(message))
			_ = seg.SafeAddMetadata("SNS-SendSMS-Result-MessageID", messageId)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	// enforce AWS SMS segment caps (approx. 10 segments) to avoid runtime InvalidParameter errors
	const maxGSM7Chars = 1600 // 10 * 160 (single) or 153/segment w/ UDH; AWS documented soft cap
	const maxUCS2Chars = 670  // ~10 segments * 67 chars
	switch encoding {
	case "GSM-7":
		if used > maxGSM7Chars {
			err = fmt.Errorf("SendSMS Failed: message length %d exceeds AWS SMS limit of %d GSM-7 characters (~10 segments)", used, maxGSM7Chars)
			return "", err
		}
	case "UCS-2":
		if used > maxUCS2Chars {
			err = fmt.Errorf("SendSMS Failed: message length %d exceeds AWS SMS limit of %d UCS-2 characters (~10 segments)", used, maxUCS2Chars)
			return "", err
		}
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

	// SP-008 P2-CMN-3 + pass-5 A1-F1 (2026-04-15): always bound via
	// ensureSNSCtx. Caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No Publish can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	pubCtx, pubCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.PublishWithContext(pubCtx, input)
	pubCancel()

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
			// SP-008 P1-COMMON-SNS-01 (2026-04-15): mask phone PII in
			// xray metadata — retain country code + last 4 digits for
			// debugging, redact the rest so the trace cannot be pivoted
			// back to a natural-person identity.
			_ = seg.SafeAddMetadata("SNS-OptInPhoneNumber-Phone", maskPhoneForXray(phoneNumber))

			if err != nil {
				_ = seg.SafeAddError(err)
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
	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	_, err = client.OptInPhoneNumberWithContext(callCtx, input)
	callCancel()

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
			// SP-008 P1-COMMON-SNS-01 (2026-04-15): mask phone PII in
			// xray metadata — retain country code + last 4 digits for
			// debugging, redact the rest so the trace cannot be pivoted
			// back to a natural-person identity.
			_ = seg.SafeAddMetadata("SNS-CheckIfPhoneNumberIsOptedOut-Phone", maskPhoneForXray(phoneNumber))
			_ = seg.SafeAddMetadata("SNS-CheckIfPhoneNumberIsOptedOut-Result-OptedOut", optedOut)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.CheckIfPhoneNumberIsOptedOutWithContext(callCtx, input)
	callCancel()

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
			// SP-008 P1-COMMON-SNS-01 (2026-04-15): mask phone PII in
			// xray metadata — retain country code + last 4 digits for
			// debugging, redact the rest so the trace cannot be pivoted
			// back to a natural-person identity. Count is retained so
			// operators can still see how many opted-out numbers came
			// back in one page.
			maskedPhones := make([]string, 0, len(phonesList))
			for _, p := range phonesList {
				maskedPhones = append(maskedPhones, maskPhoneForXray(p))
			}
			_ = seg.SafeAddMetadata("SNS-ListPhoneNumbersOptedOut-NextToken", nextToken)
			_ = seg.SafeAddMetadata("SNS-ListPhoneNumbersOptedOut-Result-PhonesList", maskedPhones)
			_ = seg.SafeAddMetadata("SNS-ListPhoneNumbersOptedOut-Result-PhonesList-Count", len(phonesList))
			_ = seg.SafeAddMetadata("SNS-ListPhoneNumbersOptedOut-Result-NextToken", morePhonesNextToken)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.ListPhoneNumbersOptedOutWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-CreatePlatformApplication-Name", name)
			_ = seg.SafeAddMetadata("SNS-CreatePlatformApplication-Platform", platform)
			_ = seg.SafeAddMetadata("SNS-CreatePlatformApplication-Attributes", attributes)
			_ = seg.SafeAddMetadata("SNS-CreatePlatformApplication-Result-PlatformApplicationArn", platformApplicationArn)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.CreatePlatformApplicationWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-DeletePlatformApplication-PlatformApplicationArn", platformApplicationArn)

			if err != nil {
				_ = seg.SafeAddError(err)
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
	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	_, err = client.DeletePlatformApplicationWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-ListPlatformApplications-NextToken", nextToken)
			_ = seg.SafeAddMetadata("SNS-ListPlatformApplications-Result-PlatformApplicationArnsList", platformApplicationArnsList)
			_ = seg.SafeAddMetadata("SNS-ListPlatformApplications-Result-NextToken", moreAppArnsNextToken)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.ListPlatformApplicationsWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-GetPlatformApplicationAttributes-PlatformApplicationArn", platformApplicationArn)
			_ = seg.SafeAddMetadata("SNS-GetPlatformApplicationAttributes-Result-Attributes", attributes)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.GetPlatformApplicationAttributesWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-SetPlatformApplicationAttributes-PlatformApplicationArn", platformApplicationArn)
			_ = seg.SafeAddMetadata("SNS-SetPlatformApplicationAttributes-Attributes", attributes)

			if err != nil {
				_ = seg.SafeAddError(err)
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
	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	_, err = client.SetPlatformApplicationAttributesWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-CreatePlatformEndpoint-PlatformApplicationArn", platformApplicationArn)
			_ = seg.SafeAddMetadata("SNS-CreatePlatformEndpoint-Token", token)
			_ = seg.SafeAddMetadata("SNS-CreatePlatformEndpoint-CustomUserData", customUserData)
			_ = seg.SafeAddMetadata("SNS-CreatePlatformEndpoint-Result-EndpointArn", endpointArn)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	// SP-008 P3-CMN-2: len(s) avoids the []byte copy allocation.
	if util.LenTrim(customUserData) > 0 && len(customUserData) > 2048 {
		err = errors.New("CreatePlatformEndpoint Failed: CustomUserData must be < 2KB")
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

	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.CreatePlatformEndpointWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-DeletePlatformEndpoint-EndpointArn", endpointArn)

			if err != nil {
				_ = seg.SafeAddError(err)
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
	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	_, err = client.DeleteEndpointWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-ListEndpointsByPlatformApplication-PlatformApplicationArn", platformApplicationArn)
			_ = seg.SafeAddMetadata("SNS-ListEndpointsByPlatformApplication-NextToken", nextToken)
			_ = seg.SafeAddMetadata("SNS-ListEndpointsByPlatformApplication-Result-EndpointArnsList", endpointArnsList)
			_ = seg.SafeAddMetadata("SNS-ListEndpointsByPlatformApplication-Result-NextToken", moreEndpointArnsNextToken)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.ListEndpointsByPlatformApplicationWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-GetPlatformEndpointAttributes-EndpointArn", endpointArn)
			_ = seg.SafeAddMetadata("SNS-GetPlatformEndpointAttributes-Result-Attributes", attributes)

			if err != nil {
				_ = seg.SafeAddError(err)
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

	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.GetEndpointAttributesWithContext(callCtx, input)
	callCancel()

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
			_ = seg.SafeAddMetadata("SNS-SetPlatformEndpointAttributes-EndpointArn", endpointArn)
			_ = seg.SafeAddMetadata("SNS-SetPlatformEndpointAttributes-Attributes", attributes)

			if err != nil {
				_ = seg.SafeAddError(err)
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
	// SP-008 P1-COMMON-SNS-01 + pass-5 A1-F1 (2026-04-15): bounded via
	// ensureSNSCtx — caller timeout wins when supplied; otherwise
	// defaultSNSCallTimeout (30s) is applied to BOTH the xray-segment-
	// parented and Background-parented branches. No SNS call can block
	// forever on a hung AWS endpoint. See wrapper/sns/sns.go:100 for
	// the full precedence contract.
	callCtx, callCancel := ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)
	_, err = client.SetEndpointAttributesWithContext(callCtx, input)
	callCancel()

	// evaluate result
	if err != nil {
		err = errors.New("SetPlatformEndpointAttributes Failed: (Set Action) " + err.Error())
		return err
	} else {
		return nil
	}
}

// sortedAttributeKeys returns the keys of an SNS message-attribute map
// as a deterministic comma-separated string suitable for xray segment
// metadata (F4 pass-3 contrarian redaction).
//
// Only keys are returned — attribute values are deliberately omitted
// because consumers may pass PII, credentials, or routing tokens as
// attribute values, none of which should land in trace storage. The
// key list remains useful for operational debugging (verifying which
// filter-policy attributes were sent) without the exposure surface of
// the values. Empty or nil input returns the empty string.
func sortedAttributeKeys(attributes map[string]*sns.MessageAttributeValue) string {
	if len(attributes) == 0 {
		return ""
	}
	keys := make([]string, 0, len(attributes))
	for k := range attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}
