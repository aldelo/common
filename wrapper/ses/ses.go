package ses

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
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
	"sync"
	"time"

	util "github.com/aldelo/common"
	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aldelo/common/wrapper/xray"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	awsxray "github.com/aws/aws-xray-sdk-go/xray"
)

// defaultSESCallTimeout bounds any single SES SDK call that would
// otherwise execute without a deadline. Applied in ensureSESCtx when
// the caller does not supply its own timeOutDuration. 30 seconds is
// consistent with wrapper/sns/sns.go defaultSNSCallTimeout and
// wrapper/kms/kms.go defaultKMSCallTimeout.
const defaultSESCallTimeout = 30 * time.Second

// ensureSESCtx normalizes the (segCtx, segCtxSet, timeOutDuration)
// triple into a single (ctx, cancel) pair so every SES SDK call has
// an upper-bound deadline:
//
//  1. Caller-supplied timeout: timeOutDuration[0] wins, applied to
//     segCtx (or context.Background() if segCtx is nil).
//  2. Xray segment ctx set (segCtxSet && segCtx != nil): segCtx
//     wrapped in WithTimeout(defaultSESCallTimeout).
//  3. Neither: context.Background() with defaultSESCallTimeout.
//
// Callers MUST invoke the returned cancel before returning.
func ensureSESCtx(segCtx context.Context, segCtxSet bool, timeOutDuration []time.Duration) (context.Context, context.CancelFunc) {
	if len(timeOutDuration) > 0 {
		parent := segCtx
		if parent == nil {
			parent = context.Background()
		}
		return context.WithTimeout(parent, timeOutDuration[0])
	}
	if segCtxSet && segCtx != nil {
		return context.WithTimeout(segCtx, defaultSESCallTimeout)
	}
	return context.WithTimeout(context.Background(), defaultSESCallTimeout)
}

// ================================================================================================================
// STRUCTS
// ================================================================================================================

// SES struct encapsulates the AWS SES access functionality
type SES struct {
	// define the AWS region that SES is located at
	AwsRegion awsregion.AWSRegion

	// custom http2 client options
	HttpOptions *awshttp2.HttpClientSettings

	// define configuration set to use if any
	ConfigurationSetName string

	// optional: override AWS endpoint URL for testing (e.g., LocalStack)
	CustomEndpoint string

	// store ses client object
	sesClient *ses.SES

	_parentSegment *xray.XRayParentSegment

	mu sync.RWMutex
}

// Email struct encapsulates the email message definition to send via AWS SES
type Email struct {
	// required
	From string

	// at least one element is required
	To []string

	// optional
	CC []string

	// optional
	BCC []string

	// required
	Subject string

	// either BodyHtml or BodyText is required
	BodyHtml string
	BodyText string

	// additional config info
	ReturnPath string
	ReplyTo    []string

	// defaults to UTF-8 if left blank
	Charset string
}

// SendQuota struct encapsulates the ses send quota definition
type SendQuota struct {
	Max24HourSendLimit    int
	MaxPerSecondSendLimit int
	SentLast24Hours       int
}

// EmailTarget defines a single email target contact for use with SendTemplateEmail or SendBulkTemplateEmail methods
type EmailTarget struct {
	// at least one element is required
	To []string

	// optional
	CC []string

	// optional
	BCC []string

	// required - template data in json
	TemplateDataJson string
}

// BulkEmailMessageResult contains result of a bulk email
type BulkEmailMessageResult struct {
	MessageId string
	Status    string
	ErrorInfo string
	Success   bool
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// internal helpers (thread-safe accessors)
// ----------------------------------------------------------------------------------------------------------------

// getClient returns the SES client in a thread-safe manner
func (s *SES) getClient() *ses.SES {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sesClient
}

// setClient sets the SES client in a thread-safe manner
func (s *SES) setClient(client *ses.SES) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sesClient = client
}

// clearClient clears the SES client in a thread-safe manner
func (s *SES) clearClient() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sesClient = nil
}

// maskEmailForXray returns a PII-reduced form of an email address suitable
// for xray metadata. The first 2 characters of the local part and the full
// domain are retained (enough to debug a sender/recipient mismatch); the
// remaining local-part characters are replaced with asterisks so the trace
// cannot be pivoted to a natural-person identity.
//
// SEC-001 (2026-04-16): used by SendEmail, SendRawEmail, and
// SendTemplateEmail xray emit sites. Mirrors the pattern established by
// maskPhoneForXray in the sibling sns package.
//
// Edge cases: inputs without "@" show the first 2 chars + "***". Inputs
// shorter than 2 runes are returned verbatim — there is nothing useful to
// mask. The helper walks runes, not bytes (lesson L18), to avoid splitting
// multi-byte code points in non-ASCII local-part addresses.
func maskEmailForXray(email string) string {
	runes := []rune(email)
	if len(runes) < 2 {
		return email
	}

	atIdx := strings.IndexRune(email, '@')
	if atIdx < 0 {
		// No "@" — show first 2 runes + "***"
		if len(runes) <= 2 {
			return email
		}
		return string(runes[:2]) + "***"
	}

	// Convert atIdx from byte offset to rune offset
	runeAtIdx := len([]rune(email[:atIdx]))
	if runeAtIdx <= 2 {
		// Local part is 2 or fewer runes — mask would reveal everything;
		// just show first rune + "***@domain"
		if runeAtIdx == 0 {
			return email
		}
		return string(runes[:1]) + "***@" + string(runes[runeAtIdx+1:])
	}
	return string(runes[:2]) + "***@" + string(runes[runeAtIdx+1:])
}

// maskEmailsForXray applies maskEmailForXray to each element of a slice.
func maskEmailsForXray(emails []string) []string {
	masked := make([]string, 0, len(emails))
	for _, e := range emails {
		masked = append(masked, maskEmailForXray(e))
	}
	return masked
}

// maskSubjectForXray returns a truncated subject line suitable for xray
// metadata. Full subject lines can contain sensitive context (account
// numbers, names, transaction details). The first 10 runes are retained
// for debugging; the remainder is replaced with "...".
//
// SEC-001 (2026-04-16): used by SendEmail and SendRawEmail xray emit
// sites.
func maskSubjectForXray(subject string) string {
	runes := []rune(subject)
	if len(runes) <= 10 {
		return subject
	}
	return string(runes[:10]) + "..."
}

// setParentSegment sets the parent segment in a thread-safe manner
func (s *SES) setParentSegment(seg *xray.XRayParentSegment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s._parentSegment = seg
}

// getParentSegment gets the parent segment in a thread-safe manner
func (s *SES) getParentSegment() *xray.XRayParentSegment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s._parentSegment
}

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the SES service
func (s *SES) Connect(parentSegment ...*xray.XRayParentSegment) (err error) {
	if s == nil {
		return errors.New("Connect To SES Failed: (SES Struct Nil)")
	}

	if len(parentSegment) > 0 {
		s.setParentSegment(parentSegment[0])
	}

	if xray.XRayServiceOn() {
		// snapshot AwsRegion under lock for the deferred closure
		s.mu.RLock()
		awsRegionSnap := s.AwsRegion
		s.mu.RUnlock()

		seg := xray.NewSegment("SES-Connect", s.getParentSegment())
		defer seg.Close()
		defer func() {
			// COMMON-R2-001: sampled xray failure logging instead of silent discard
			if e := seg.SafeAddMetadata("SES-AWS-Region", awsRegionSnap); e != nil {
				xray.LogXrayAddFailure("SES-Connect", e)
			}

			if err != nil {
				if e := seg.SafeAddError(err); e != nil {
					xray.LogXrayAddFailure("SES-Connect", e)
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

// connectInternal will establish a connection to the SES service
func (s *SES) connectInternal() error {
	s.mu.Lock()
	region := s.AwsRegion
	if s.HttpOptions == nil {
		s.HttpOptions = new(awshttp2.HttpClientSettings)
	}
	httpOpts := s.HttpOptions
	s.mu.Unlock()

	if !region.Valid() || region == awsregion.UNKNOWN {
		return errors.New("Connect To SES Failed: (AWS Session Error) " + "Region is Required")
	}

	// create custom http2 client if needed
	var httpCli *http.Client
	var httpErr error

	// use custom http2 client
	h2 := &awshttp2.AwsHttp2Client{
		Options: httpOpts,
	}

	if httpCli, httpErr = h2.NewHttp2Client(); httpErr != nil {
		return errors.New("Connect to SES Failed: (AWS Session Error) " + "Create Custom Http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection and keep session object in struct
	cfg := &aws.Config{
		Region:     aws.String(region.Key()),
		HTTPClient: httpCli,
	}

	s.mu.RLock()
	customEP := s.CustomEndpoint
	s.mu.RUnlock()

	if len(customEP) > 0 {
		cfg.Endpoint = aws.String(customEP)
	}

	sess, err := session.NewSession(cfg)
	if err != nil {
		// aws session error
		return errors.New("Connect To SES Failed: (AWS Session Error) " + err.Error())
	}

	// create cached objects for shared use
	cli := ses.New(sess)

	if cli == nil {
		return errors.New("Connect To SES Client Failed: (New SES Client Connection) " + "Connection Object Nil")
	}

	s.setClient(cli)
	return nil
}

// Disconnect will disjoin from aws session by clearing it
func (s *SES) Disconnect() {
	s.clearClient()
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (s *SES) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	s.setParentSegment(parentSegment)
}

// ----------------------------------------------------------------------------------------------------------------
// Email struct methods
// ----------------------------------------------------------------------------------------------------------------

// Initial sets new email message
//
// parameters:
//
//	from = email sender from address
//	to = email sender to address
//	subject = email subject text
//	bodyText = email body content in text (both bodyText and bodyHtml may be set at the same time)
//	bodyHtml = email body content in html (both bodyText and bodyHtml may be set at the same time)
func (e *Email) Initial(from string, to string, subject string, bodyText string, bodyHtml ...string) *Email {
	// set fields
	e.From = from
	e.To = append(e.To, to)
	e.Subject = subject
	e.BodyText = bodyText

	// set body html
	if len(bodyHtml) > 0 {
		e.BodyHtml = bodyHtml[0]
	}

	// defaults
	e.ReturnPath = from

	if util.LenTrim(from) > 0 {
		e.ReplyTo = append(e.ReplyTo, from)
	}

	e.Charset = "UTF-8"

	return e
}

// SetTo adds additional 'To' addresses to email
func (e *Email) SetTo(toAddress ...string) *Email {
	if len(toAddress) > 0 {
		for _, v := range toAddress {
			e.To = append(e.To, v)
		}
	}

	return e
}

// SetCC adds 'CC' addresses to email
func (e *Email) SetCC(ccAddress ...string) *Email {
	if len(ccAddress) > 0 {
		for _, v := range ccAddress {
			e.CC = append(e.CC, v)
		}
	}

	return e
}

// SetBCC adds 'BCC' addresses to email
func (e *Email) SetBCC(bccAddress ...string) *Email {
	if len(bccAddress) > 0 {
		for _, v := range bccAddress {
			e.BCC = append(e.BCC, v)
		}
	}

	return e
}

// SetReplyTo adds 'Reply-To' addresses to email
func (e *Email) SetReplyTo(replyToAddress ...string) *Email {
	if len(replyToAddress) > 0 {
		for _, v := range replyToAddress {
			e.ReplyTo = append(e.ReplyTo, v)
		}
	}

	return e
}

// validateEmailFields will verify if the email fields are satisfied
func (e *Email) validateEmailFields() error {
	if util.LenTrim(e.From) <= 0 {
		return errors.New("Verify Email Fields Failed: " + "From Address is Required")
	}

	if len(e.To) <= 0 {
		return errors.New("Verify Email Fields Failed: " + "To Address is Required")
	}

	if util.LenTrim(e.Subject) <= 0 {
		return errors.New("Verify Email Fields Failed: " + "Subject is Required")
	}

	if util.LenTrim(e.BodyHtml) <= 0 && util.LenTrim(e.BodyText) <= 0 {
		return errors.New("Verify Email Fields Failed: " + "Body (Html or Text) is Required")
	}

	if util.LenTrim(e.Charset) <= 0 {
		e.Charset = "UTF-8"
	}

	// validate successful
	return nil
}

// GenerateSendEmailInput will create the SendEmailInput object from the struct fields
func (e *Email) GenerateSendEmailInput() (*ses.SendEmailInput, error) {
	// validate
	if err := e.validateEmailFields(); err != nil {
		return nil, err
	}

	// assemble SendEmailInput object
	input := &ses.SendEmailInput{
		Source: aws.String(e.From),
		Destination: &ses.Destination{
			ToAddresses: aws.StringSlice(e.To),
		},
		Message: &ses.Message{
			Subject: &ses.Content{
				Data:    aws.String(e.Subject),
				Charset: aws.String(e.Charset),
			},
		},
	}

	// set cc and bcc if applicable
	if len(e.CC) > 0 {
		if input.Destination == nil {
			input.Destination = new(ses.Destination)
		}

		input.Destination.CcAddresses = aws.StringSlice(e.CC)
	}

	if len(e.BCC) > 0 {
		if input.Destination == nil {
			input.Destination = new(ses.Destination)
		}

		input.Destination.BccAddresses = aws.StringSlice(e.BCC)
	}

	// set message body
	if util.LenTrim(e.BodyHtml) > 0 {
		if input.Message == nil {
			input.Message = new(ses.Message)
		}

		if input.Message.Body == nil {
			input.Message.Body = new(ses.Body)
		}

		if input.Message.Body.Html == nil {
			input.Message.Body.Html = new(ses.Content)
		}

		input.Message.Body.Html.Data = aws.String(e.BodyHtml)
		input.Message.Body.Html.Charset = aws.String(e.Charset)
	}

	if util.LenTrim(e.BodyText) > 0 {
		if input.Message == nil {
			input.Message = new(ses.Message)
		}

		if input.Message.Body == nil {
			input.Message.Body = new(ses.Body)
		}

		if input.Message.Body.Text == nil {
			input.Message.Body.Text = new(ses.Content)
		}

		input.Message.Body.Text.Data = aws.String(e.BodyText)
		input.Message.Body.Text.Charset = aws.String(e.Charset)
	}

	if util.LenTrim(e.ReturnPath) > 0 {
		input.ReturnPath = aws.String(e.ReturnPath)
	}

	if len(e.ReplyTo) > 0 {
		input.ReplyToAddresses = aws.StringSlice(e.ReplyTo)
	}

	// return result
	return input, nil
}

// GenerateSendRawEmailInput will create the SendRawEmailInput object from the struct fields
//
// attachmentContentType = See: https://developer.mozilla.org/en-US/docs/Web/HTTP/Basics_of_HTTP/MIME_types/Common_types
func (e *Email) GenerateSendRawEmailInput(attachmentFileName string, attachmentContentType string, attachmentData []byte) (*ses.SendRawEmailInput, error) {
	// validate
	if err := e.validateEmailFields(); err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)

	// email main header
	h := make(textproto.MIMEHeader)
	h.Set("From", e.From)
	h.Set("To", util.SliceStringToCSVString(e.To, false))

	if len(e.CC) > 0 {
		h.Set("CC", util.SliceStringToCSVString(e.CC, false))
	}

	if util.LenTrim(e.ReturnPath) > 0 {
		h.Set("Return-Path", e.ReturnPath)
	}

	if len(e.ReplyTo) > 0 {
		h.Set("Reply-To", util.SliceStringToCSVString(e.ReplyTo, false))
	}

	h.Set("Subject", e.Subject)
	h.Set("Content-Language", "en-US")
	h.Set("Content-Type", "multipart/mixed; boundary=\""+writer.Boundary()+"\"")
	h.Set("MIME-Version", "1.0")

	if _, err := writer.CreatePart(h); err != nil {
		return nil, err
	}

	charset := "UTF-8"

	if util.LenTrim(e.Charset) > 0 {
		charset = e.Charset
	}

	// email body - plain text
	if util.LenTrim(e.BodyText) > 0 {
		h = make(textproto.MIMEHeader)
		h.Set("Content-Transfer-Encoding", "quoted-printable")
		h.Set("Content-Type", "text/plain; charset="+charset)
		part, err := writer.CreatePart(h)
		if err != nil {
			return nil, err
		}

		if _, err = part.Write([]byte(e.BodyText)); err != nil {
			return nil, err
		}
	}

	// email body - html text
	if util.LenTrim(e.BodyHtml) > 0 {
		h = make(textproto.MIMEHeader)
		h.Set("Content-Transfer-Encoding", "quoted-printable")
		h.Set("Content-Type", "text/html; charset="+charset)
		partH, err := writer.CreatePart(h)
		if err != nil {
			return nil, err
		}

		if _, err = partH.Write([]byte(e.BodyHtml)); err != nil {
			return nil, err
		}
	}

	// file attachment
	if util.LenTrim(attachmentFileName) > 0 {
		fn := filepath.Base(attachmentFileName)

		h = make(textproto.MIMEHeader)
		// h.Set("Content-Disposition", "attachment; filename="+fn)
		h.Set("Content-Type", attachmentContentType+"; name=\""+fn+"\"")
		h.Set("Content-Description", fn)
		h.Set("Content-Disposition", "attachment; filename=\""+fn+"\"; size="+util.Itoa(len(attachmentData))+";")
		h.Set("Content-Transfer-Encoding", "base64")

		partA, err := writer.CreatePart(h)
		if err != nil {
			return nil, err
		}

		// must base64 encode first before write
		if _, err = partA.Write([]byte(base64.StdEncoding.EncodeToString(attachmentData))); err != nil {
			return nil, err
		}
	}

	// clean up
	if err := writer.Close(); err != nil {
		return nil, err
	}

	// strip boundary line before header
	s := buf.String()

	if strings.Count(s, "\n") < 2 {
		return nil, fmt.Errorf("Email Content Not Valid")
	}

	s = strings.SplitN(s, "\n", 2)[1]

	raw := ses.RawMessage{
		Data: []byte(s),
	}

	destinations := make([]string, 0, len(e.To)+len(e.CC)+len(e.BCC))
	destinations = append(destinations, e.To...)
	destinations = append(destinations, e.CC...)
	destinations = append(destinations, e.BCC...)

	// assemble SendRawEmailInput object
	input := &ses.SendRawEmailInput{
		Destinations: aws.StringSlice(destinations),
		Source:       aws.String(e.From),
		RawMessage:   &raw,
	}

	// return result
	return input, nil
}

// ----------------------------------------------------------------------------------------------------------------
// EmailTarget struct methods
// ----------------------------------------------------------------------------------------------------------------

// Initial will set template data in json, and list of toAddresses variadic
func (t *EmailTarget) Initial(templateDataJson string, toAddress ...string) *EmailTarget {
	t.TemplateDataJson = templateDataJson

	if len(toAddress) > 0 {
		for _, v := range toAddress {
			t.To = append(t.To, v)
		}
	}

	return t
}

// SetCC will set list of ccAddress variadic to EmailTarget
func (t *EmailTarget) SetCC(ccAddress ...string) *EmailTarget {
	if len(ccAddress) > 0 {
		for _, v := range ccAddress {
			t.CC = append(t.CC, v)
		}
	}

	return t
}

// SetBCC will set list of bccAddress variadic to EmailTarget
func (t *EmailTarget) SetBCC(bccAddress ...string) *EmailTarget {
	if len(bccAddress) > 0 {
		for _, v := range bccAddress {
			t.BCC = append(t.BCC, v)
		}
	}

	return t
}

// ----------------------------------------------------------------------------------------------------------------
// SES general methods
// ----------------------------------------------------------------------------------------------------------------

// handleSESError will evaluate the error and return the revised error object
func (s *SES) handleSESError(err error) error {
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			// aws error
			return errors.New("[AWS] " + aerr.Code() + " - " + aerr.Message())
		} else {
			// other error
			return errors.New("[General] " + err.Error())
		}
	} else {
		return nil
	}
}

// GetSendQuota will get the ses send quota
func (s *SES) GetSendQuota(timeOutDuration ...time.Duration) (sq *SendQuota, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SES-GetSendQuota", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-GetSendQuota-Result-SendQuota", sq))

			if err != nil {
				xray.LogXrayAddFailure("SES", seg.SafeAddError(err))
			}
		}()
	}

	// validate
	client := s.getClient()
	if client == nil {
		err = errors.New("GetSendQuota Failed: " + "SES Client is Required")
		return nil, err
	}

	// compose input
	input := &ses.GetSendQuotaInput{}

	// SP-010 pass-6 A1-F1 (2026-04-16): bounded via ensureSESCtx —
	// same pattern as wrapper/sns/sns.go ensureSNSCtx.
	var output *ses.GetSendQuotaOutput

	callCtx, callCancel := ensureSESCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.GetSendQuotaWithContext(callCtx, input)
	callCancel()

	// evaluate result
	if err != nil {
		err = errors.New("GetSendQuota Failed: " + err.Error())
		return nil, err
	}

	if output == nil {
		return nil, errors.New("GetSendQuota Failed: nil output from AWS SDK")
	}

	sq = &SendQuota{
		Max24HourSendLimit:    util.Float64ToInt(aws.Float64Value(output.Max24HourSend)),
		MaxPerSecondSendLimit: util.Float64ToInt(aws.Float64Value(output.MaxSendRate)),
		SentLast24Hours:       util.Float64ToInt(aws.Float64Value(output.SentLast24Hours)),
	}

	return sq, nil
}

// SendEmail will send an email message out via SES
func (s *SES) SendEmail(email *Email, timeOutDuration ...time.Duration) (messageId string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SES-SendEmail", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			// COMMON-R2-001: sampled xray failure logging instead of silent discard
			if email != nil {
				// SEC-001 (2026-04-16): mask email PII in xray metadata —
				// email addresses and subject lines can contain or constitute
				// personally identifiable information.
				// NOTE (A1-F4): CC and BCC are intentionally NOT emitted to xray
				// metadata — they contain PII (email addresses) and are not needed
				// for debugging. If CC/BCC logging is added in the future, apply
				// maskEmailsForXray before emitting.
				if e := seg.SafeAddMetadata("SES-SendEmail-Email-From", maskEmailForXray(email.From)); e != nil {
					xray.LogXrayAddFailure("SES-SendEmail", e)
				}
				if e := seg.SafeAddMetadata("SES-SendEmail-Email-To", maskEmailsForXray(email.To)); e != nil {
					xray.LogXrayAddFailure("SES-SendEmail", e)
				}
				if e := seg.SafeAddMetadata("SES-SendEmail-Email-Subject", maskSubjectForXray(email.Subject)); e != nil {
					xray.LogXrayAddFailure("SES-SendEmail", e)
				}
				if e := seg.SafeAddMetadata("SES-SendEmail-Result-MessageID", messageId); e != nil {
					xray.LogXrayAddFailure("SES-SendEmail", e)
				}
			} else {
				if e := seg.SafeAddMetadata("SES-SendEmail-Email-Fail", "Email Object Nil"); e != nil {
					xray.LogXrayAddFailure("SES-SendEmail", e)
				}
			}

			if err != nil {
				if e := seg.SafeAddError(err); e != nil {
					xray.LogXrayAddFailure("SES-SendEmail", e)
				}
			}
		}()
	}

	// validate
	client := s.getClient()
	if client == nil {
		err = errors.New("SendEmail Failed: " + "SES Client is Required")
		return "", err
	}

	if email == nil {
		err = errors.New("SendEmail Failed: " + "Email Object is Required")
		return "", err
	}

	// generate email input
	var input *ses.SendEmailInput
	input, err = email.GenerateSendEmailInput()

	if err != nil {
		err = errors.New("SendEmail Failed: " + err.Error())
		return "", err
	}

	if input == nil {
		err = errors.New("SendEmail Failed: " + "SendEmailInput is Nil")
		return "", err
	}

	// set configurationSetName if applicable
	s.mu.RLock()
	cfgSetName := s.ConfigurationSetName
	s.mu.RUnlock()
	if util.LenTrim(cfgSetName) > 0 {
		input.ConfigurationSetName = aws.String(cfgSetName)
	}

	// send out email
	var output *ses.SendEmailOutput

	callCtx, callCancel := ensureSESCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.SendEmailWithContext(callCtx, input)
	callCancel()

	// evaluate output
	if err != nil {
		if err = s.handleSESError(err); err != nil {
			return "", err
		}
	}

	if output == nil || output.MessageId == nil {
		// CHANGED: correct function name in error for clearer debugging
		return "", errors.New("SendEmail Failed: nil output or MessageId from AWS SDK")
	}

	// email sent successfully
	messageId = aws.StringValue(output.MessageId)
	return messageId, nil
}

// SendRawEmail will send an raw email message out via SES, supporting attachments
//
// attachmentContentType = See: https://developer.mozilla.org/en-US/docs/Web/HTTP/Basics_of_HTTP/MIME_types/Common_types
func (s *SES) SendRawEmail(email *Email, attachmentFileName string, attachmentContentType string,
	timeOutDuration ...time.Duration) (messageId string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SES-SendRawEmail", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			if email != nil {
				// SEC-001 (2026-04-16): mask email PII in xray metadata —
				// email addresses and subject lines can contain or constitute
				// personally identifiable information.
				// NOTE (A1-F4): CC and BCC are intentionally NOT emitted to xray
				// metadata — same rationale as SendEmail above.
				xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendRawEmail-Email-From", maskEmailForXray(email.From)))
				xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendRawEmail-Email-To", maskEmailsForXray(email.To)))
				xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendRawEmail-Email-Subject", maskSubjectForXray(email.Subject)))
				xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendRawEmail-Email-Attachment-FileName", attachmentFileName))
				xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendRawEmail-Email-Attachment-ContentType", attachmentContentType))
				xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendRawEmail-Result-MessageID", messageId))
			} else {
				xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendRawEmail-Email-Fail", "Email Object Nil"))
			}

			if err != nil {
				xray.LogXrayAddFailure("SES", seg.SafeAddError(err))
			}
		}()
	}

	// validate
	client := s.getClient()
	if client == nil {
		err = errors.New("SendRawEmail Failed: " + "SES Client is Required")
		return "", err
	}

	if email == nil {
		err = errors.New("SendRawEmail Failed: " + "Email Object is Required")
		return "", err
	}

	// load attachment info
	attachmentData := []byte{}

	if util.LenTrim(attachmentFileName) > 0 {
		if attachmentData, err = util.FileReadBytes(attachmentFileName); err != nil {
			err = fmt.Errorf("SendRawEmail Failed: Load Attachment File '%s' Error: %w", attachmentFileName, err)
			return "", err
		} else {
			if len(attachmentData) == 0 {
				err = fmt.Errorf("SendRawEmail Failed: Attachment Data From File is Empty")
				return "", err
			}

			if util.LenTrim(attachmentContentType) == 0 {
				err = fmt.Errorf("SendRawEmail Failed: Attachment Content-Type is Required")
				return "", err
			}
		}
	} else {
		attachmentFileName = ""
		attachmentContentType = ""
	}

	// generate email input
	var input *ses.SendRawEmailInput
	input, err = email.GenerateSendRawEmailInput(attachmentFileName, attachmentContentType, attachmentData)

	if err != nil {
		err = errors.New("SendRawEmail Failed: " + err.Error())
		return "", err
	}

	if input == nil {
		err = errors.New("SendRawEmail Failed: " + "SendRawEmailInput is Nil")
		return "", err
	}

	// set configurationSetName if applicable
	s.mu.RLock()
	cfgSetName := s.ConfigurationSetName
	s.mu.RUnlock()
	if util.LenTrim(cfgSetName) > 0 {
		input.ConfigurationSetName = aws.String(cfgSetName)
	}

	// send out email
	var output *ses.SendRawEmailOutput

	callCtx, callCancel := ensureSESCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.SendRawEmailWithContext(callCtx, input)
	callCancel()

	// evaluate output
	if err != nil {
		if err = s.handleSESError(err); err != nil {
			return "", err
		}
	}

	if output == nil || output.MessageId == nil {
		return "", errors.New("SendRawEmail Failed: nil output or MessageId from AWS SDK")
	}

	// email sent successfully
	messageId = aws.StringValue(output.MessageId)
	return messageId, nil
}

// ----------------------------------------------------------------------------------------------------------------
// SES template & bulk email methods
// ----------------------------------------------------------------------------------------------------------------

// CreateTemplate will create an email template for use with SendTemplateEmail and SendBulkTemplateEmail methods
//
// parameters:
//  1. templateName = name of the template, must be unique
//  2. subjectPart = subject text with tag name value replacements
//  3. textPart = body text with tag name value replacements
//  4. htmlPart = body html with tag name value replacements
//  5. timeOutDuration = optional time out value to use in context
//
// template tokens:
//  1. subjectPart, textPart, htmlPart = may contain Tag Names for personalization
//  2. Tag Name = using {{tagName}} within the string defined above in #1
//  3. example:
//     a) Greetings, {{name}}
//     b) <h1>hello {{name}}, how are you</h1><p>Today is {{dayOfWeek}}</p>
//     c) <html><head></head><body><h1>hello {{name}}, how are you</h1><p>Today is {{dayOfWeek}}</p></body></html>
//  4. when sending template mail, the Tag Name values are replaced via key-Value pairs within TemplateData in SendTemplateEmail and SendBulkTemplateEmail methods:
//     a) { "name":"Harry", "dayOfWeek":"Monday" }
func (s *SES) CreateTemplate(templateName string, subjectPart string, textPart string, htmlPart string, timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SES-CreateTemplate", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-CreateTemplate-TemplateName", templateName))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-CreateTemplate-SubjectPart", maskSubjectForXray(subjectPart)))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-CreateTemplate-TextPart", textPart))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-CreateTemplate-HtmlPart", htmlPart))

			if err != nil {
				xray.LogXrayAddFailure("SES", seg.SafeAddError(err))
			}
		}()
	}

	// validate
	client := s.getClient()
	if client == nil {
		err = errors.New("CreateTemplate Failed: " + "SES Client is Required")
		return err
	}

	if util.LenTrim(templateName) <= 0 {
		err = errors.New("CreateTemplate Failed: " + "Template Name is Required")
		return err
	}

	if util.LenTrim(subjectPart) <= 0 {
		err = errors.New("CreateTemplate Failed: " + "Subject Part is Required")
		return err
	}

	if util.LenTrim(textPart) <= 0 && util.LenTrim(htmlPart) <= 0 {
		err = errors.New("CreateTemplate Failed: " + "Text or Html Part is Required")
		return err
	}

	// create input object
	input := &ses.CreateTemplateInput{
		Template: &ses.Template{
			TemplateName: aws.String(templateName),
			SubjectPart:  aws.String(subjectPart),
		},
	}

	if util.LenTrim(textPart) > 0 {
		input.Template.TextPart = aws.String(textPart)
	}

	if util.LenTrim(htmlPart) > 0 {
		input.Template.HtmlPart = aws.String(htmlPart)
	}

	// create template action
	callCtx, callCancel := ensureSESCtx(segCtx, segCtxSet, timeOutDuration)
	_, err = client.CreateTemplateWithContext(callCtx, input)
	callCancel()

	// evaluate result
	if err != nil {
		// failure
		err = errors.New("CreateTemplate Failed: (Create Template Action) " + err.Error())
		return err
	} else {
		// success
		return nil
	}
}

// UpdateTemplate will update an existing email template for use with SendTemplateEmail and SendBulkTemplateEmail methods
//
// parameters:
//  1. templateName = name of the template, must be unique
//  2. subjectPart = subject text with tag name value replacements
//  3. textPart = body text with tag name value replacements
//  4. htmlPart = body html with tag name value replacements
//  5. timeOutDuration = optional time out value to use in context
//
// template tokens:
//  1. subjectPart, textPart, htmlPart = may contain Tag Names for personalization
//  2. Tag Name = using {{tagName}} within the string defined above in #1
//  3. example:
//     a) Greetings, {{name}}
//     b) <h1>hello {{name}}, how are you</h1><p>Today is {{dayOfWeek}}</p>
//     c) <html><head></head><body><h1>hello {{name}}, how are you</h1><p>Today is {{dayOfWeek}}</p></body></html>
//  4. when sending template mail, the Tag Name values are replaced via key-Value pairs within TemplateData in SendTemplateEmail and SendBulkTemplateEmail methods:
//     a) { "name":"Harry", "dayOfWeek":"Monday" }
func (s *SES) UpdateTemplate(templateName string, subjectPart string, textPart string, htmlPart string, timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SES-UpdateTemplate", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-UpdateTemplate-TemplateName", templateName))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-UpdateTemplate-SubjectPart", maskSubjectForXray(subjectPart)))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-UpdateTemplate-TextPart", textPart))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-UpdateTemplate-HtmlPart", htmlPart))

			if err != nil {
				xray.LogXrayAddFailure("SES", seg.SafeAddError(err))
			}
		}()
	}

	// validate
	client := s.getClient()
	if client == nil {
		err = errors.New("UpdateTemplate Failed: " + "SES Client is Required")
		return err
	}

	if util.LenTrim(templateName) <= 0 {
		err = errors.New("UpdateTemplate Failed: " + "Template Name is Required")
		return err
	}

	if util.LenTrim(subjectPart) <= 0 {
		err = errors.New("UpdateTemplate Failed: " + "Subject Part is Required")
		return err
	}

	if util.LenTrim(textPart) <= 0 && util.LenTrim(htmlPart) <= 0 {
		err = errors.New("UpdateTemplate Failed: " + "Text or Html Part is Required")
		return err
	}

	// create input object
	input := &ses.UpdateTemplateInput{
		Template: &ses.Template{
			TemplateName: aws.String(templateName),
			SubjectPart:  aws.String(subjectPart),
		},
	}

	if util.LenTrim(textPart) > 0 {
		input.Template.TextPart = aws.String(textPart)
	}

	if util.LenTrim(htmlPart) > 0 {
		input.Template.HtmlPart = aws.String(htmlPart)
	}

	// update template action
	callCtx, callCancel := ensureSESCtx(segCtx, segCtxSet, timeOutDuration)
	_, err = client.UpdateTemplateWithContext(callCtx, input)
	callCancel()

	// evaluate result
	if err != nil {
		// failure
		err = errors.New("UpdateTemplate Failed: (Update Template Action) " + err.Error())
		return err
	} else {
		// success
		return nil
	}
}

// DeleteTemplate will delete an existing email template
//
// parameters:
//  1. templateName = name of the template to delete
//  2. timeOutDuration = optional time out value to use in context
func (s *SES) DeleteTemplate(templateName string, timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SES-DeleteTemplate", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-DeleteTemplate-TemplateName", templateName))

			if err != nil {
				xray.LogXrayAddFailure("SES", seg.SafeAddError(err))
			}
		}()
	}

	// validate
	client := s.getClient()
	if client == nil {
		err = errors.New("DeleteTemplate Failed: " + "SES Client is Required")
		return err
	}

	if util.LenTrim(templateName) <= 0 {
		err = errors.New("DeleteTemplate Failed: " + "Template Name is Required")
		return err
	}

	// create input object
	input := &ses.DeleteTemplateInput{
		TemplateName: aws.String(templateName),
	}

	// delete template action
	callCtx, callCancel := ensureSESCtx(segCtx, segCtxSet, timeOutDuration)
	_, err = client.DeleteTemplateWithContext(callCtx, input)
	callCancel()

	// evaluate result
	if err != nil {
		// failure
		err = errors.New("DeleteTemplate Failed: (Delete Template Action) " + err.Error())
		return err
	} else {
		// success
		return nil
	}
}

// SendTemplateEmail will send out email via ses using template
//
// parameters:
//  1. senderEmail = sender's email address
//  2. returnPath = optional, email return path address
//  3. replyTo = optional, email reply to address
//  4. templateName = the name of the template to use for sending out template emails
//  5. emailTarget = pointer to email target object defining where the email is sent to, along with template data key value pairs in json
//  6. timeOutDuration = optional, time out value to use in context
//
// template tokens:
//  1. subjectPart, textPart, htmlPart = may contain Tag Names for personalization
//  2. Tag Name = using {{tagName}} within the string defined above in #1
//  3. example:
//     a) Greetings, {{name}}
//     b) <h1>hello {{name}}, how are you</h1><p>Today is {{dayOfWeek}}</p>
//     c) <html><head></head><body><h1>hello {{name}}, how are you</h1><p>Today is {{dayOfWeek}}</p></body></html>
//  4. when sending template mail, the Tag Name values are replaced via key-Value pairs within TemplateData in SendTemplateEmail and SendBulkTemplateEmail methods:
//     a) { "name":"Harry", "dayOfWeek":"Monday" }
//
// notes:
//
//	a) Template = name of template to apply to the email
//	b) TemplateData = json string holding key-value pair of Tag Name and Tag Value (see below about template tokens)
//	c) Source = email address of sender
//	d) ReturnPath = return email address (usually same as source)
//	e) ReplyToAddresses = list of email addresses for email reply to
//	f) Destination = email send to destination
//	g) ConfigurationSetName = if ses has a set defined, and need to be used herein
func (s *SES) SendTemplateEmail(senderEmail string,
	returnPath string,
	replyTo string,
	templateName string,
	emailTarget *EmailTarget,
	timeOutDuration ...time.Duration) (messageId string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SES-SendTemplateEmail", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			// SEC-001 (2026-04-16): mask email PII in xray metadata —
			// senderEmail, returnPath, replyTo, and emailTarget.To are
			// all email addresses containing personally identifiable
			// information.
			// NOTE (A1-F4): emailTarget.CC and emailTarget.BCC are intentionally
			// NOT emitted to xray metadata — same rationale as SendEmail above.
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendTemplateEmail-Email-From", maskEmailForXray(senderEmail)))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendTemplateEmail-Email-ReturnPath", maskEmailForXray(returnPath)))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendTemplateEmail-Email-ReplyTo", maskEmailForXray(replyTo)))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendTemplateEmail-TemplateName", templateName))

			if emailTarget != nil {
				xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendTemplateEmail-Email-To", maskEmailsForXray(emailTarget.To)))
				xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendTemplateEmail-Result-MessageID", messageId))
			} else {
				xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendTemplateEmail-Email-Fail", "Email Target Nil"))
			}

			if err != nil {
				xray.LogXrayAddFailure("SES", seg.SafeAddError(err))
			}
		}()
	}

	// validate
	client := s.getClient()
	if client == nil {
		err = errors.New("SendTemplateEmail Failed: " + "SES Client is Required")
		return "", err
	}

	if util.LenTrim(senderEmail) <= 0 {
		err = errors.New("SendTemplateEmail Failed: " + "Sender Email is Required")
		return "", err
	}

	if util.LenTrim(templateName) <= 0 {
		err = errors.New("SendTemplateEmail Failed: " + "Template Name is Required")
		return "", err
	}

	if emailTarget == nil {
		err = errors.New("SendTemplateEmail Failed: " + "Email Target is Required (Nil)")
		return "", err
	}

	if len(emailTarget.To) <= 0 {
		err = errors.New("SendTemplateEmail Failed: " + "Email Target's To-Address is Required")
		return "", err
	}

	// create input object
	input := &ses.SendTemplatedEmailInput{
		Template: aws.String(templateName),
		Source:   aws.String(senderEmail),
	}

	// set additional input values
	if util.LenTrim(returnPath) > 0 {
		input.ReturnPath = aws.String(returnPath)
	} else {
		input.ReturnPath = aws.String(senderEmail)
	}

	if util.LenTrim(replyTo) > 0 {
		input.ReplyToAddresses = []*string{aws.String(replyTo)}
	} else {
		input.ReplyToAddresses = []*string{aws.String(senderEmail)}
	}

	if input.Destination == nil {
		input.Destination = new(ses.Destination)
	}

	if len(emailTarget.To) > 0 {
		input.Destination.ToAddresses = aws.StringSlice(emailTarget.To)
	}

	if len(emailTarget.CC) > 0 {
		input.Destination.CcAddresses = aws.StringSlice(emailTarget.CC)
	}

	if len(emailTarget.BCC) > 0 {
		input.Destination.BccAddresses = aws.StringSlice(emailTarget.BCC)
	}

	if util.LenTrim(emailTarget.TemplateDataJson) > 0 {
		input.TemplateData = aws.String(emailTarget.TemplateDataJson)
	}

	s.mu.RLock()
	cfgSetName := s.ConfigurationSetName
	s.mu.RUnlock()
	if util.LenTrim(cfgSetName) > 0 {
		input.ConfigurationSetName = aws.String(cfgSetName)
	}

	// perform action
	var output *ses.SendTemplatedEmailOutput

	callCtx, callCancel := ensureSESCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.SendTemplatedEmailWithContext(callCtx, input)
	callCancel()

	// evaluate result
	if err != nil {
		err = errors.New("SendTemplateEmailFailed: (Send Action) " + err.Error())
		return "", err
	}

	if output == nil || output.MessageId == nil {
		return "", errors.New("SendTemplateEmailFailed: nil output or MessageId from AWS SDK")
	}

	messageId = *output.MessageId
	return messageId, nil
}

// SendBulkTemplateEmail will send out bulk email via ses using template
//
// parameters:
//  1. senderEmail = sender's email address
//  2. returnPath = optional, email return path address
//  3. replyTo = optional, email reply to address
//  4. templateName = the name of the template to use for sending out template emails
//  5. defaultTemplateDataJson = the default template data key value pairs in json, to be used if emailTarget did not define a template data json
//  6. emailTargetList = slice of pointer to email target object defining where the email is sent to, along with template data key value pairs in json
//  7. timeOutDuration = optional, time out value to use in context
//
// template tokens:
//  1. subjectPart, textPart, htmlPart = may contain Tag Names for personalization
//  2. Tag Name = using {{tagName}} within the string defined above in #1
//  3. example:
//     a) Greetings, {{name}}
//     b) <h1>hello {{name}}, how are you</h1><p>Today is {{dayOfWeek}}</p>
//     c) <html><head></head><body><h1>hello {{name}}, how are you</h1><p>Today is {{dayOfWeek}}</p></body></html>
//  4. when sending template mail, the Tag Name values are replaced via key-Value pairs within TemplateData in SendTemplateEmail and SendBulkTemplateEmail methods:
//     a) { "name":"Harry", "dayOfWeek":"Monday" }
//
// notes:
//
//			a) Template = name of template to apply to the email
//			b) DefaultTemplateData = json string holding key-value pair of Tag Name and Tag Value (see below about template tokens)
//	  							 acting as a catch all default in case destination level replacement template data is not set
//			c) Source = email address of sender
//			d) ReturnPath = return email address (usually same as source)
//			e) ReplyToAddresses = list of email addresses for email reply to
//			f) Destinations = one or more Destinations, each defining target, and replacement template data for that specific destination
//			g) ConfigurationSetName = if ses has a set defined, and need to be used herein
//
// limits:
//  1. bulk is limited to 50 destinations per single call (subject to aws ses limits on account)
func (s *SES) SendBulkTemplateEmail(senderEmail string,
	returnPath string,
	replyTo string,
	templateName string,
	defaultTemplateDataJson string,
	emailTargetList []*EmailTarget,
	timeOutDuration ...time.Duration) (resultList []*BulkEmailMessageResult, failedCount int, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SES-SendBulkTemplateEmail", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			// NOTE (A1-F4): per-target CC and BCC are intentionally NOT emitted
			// to xray metadata — same rationale as SendEmail above.
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendBulkTemplateEmail-Email-From", maskEmailForXray(senderEmail)))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendBulkTemplateEmail-ReturnPath", maskEmailForXray(returnPath)))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendBulkTemplateEmail-ReplyTo", maskEmailForXray(replyTo)))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendBulkTemplateEmail-TemplateName", templateName))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendBulkTemplateEmail-DefaultTemplateDataJson", defaultTemplateDataJson))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendBulkTemplateEmail-EmailTargetList-Count", len(emailTargetList)))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendBulkTemplateEmail-Result-FailCount", failedCount))

			if err != nil {
				xray.LogXrayAddFailure("SES", seg.SafeAddError(err))
			}
		}()
	}

	// validate
	client := s.getClient()
	if client == nil {
		err = errors.New("SendBulkTemplateEmail Failed: " + "SES Client is Required")
		return nil, 0, err
	}

	if util.LenTrim(senderEmail) <= 0 {
		err = errors.New("SendBulkTemplateEmail Failed: " + "Sender Email is Required")
		return nil, 0, err
	}

	if util.LenTrim(templateName) <= 0 {
		err = errors.New("SendBulkTemplateEmail Failed: " + "Template Name is Required")
		return nil, 0, err
	}

	if len(emailTargetList) <= 0 {
		err = errors.New("SendBulkTemplateEmail Failed: " + "Email Target List is Required")
		return nil, 0, err
	}

	if len(emailTargetList) > 50 {
		err = fmt.Errorf("SendBulkTemplateEmail Failed: AWS SES bulk send is limited to 50 destinations per call (got %d). Please batch the requests.", len(emailTargetList))
		return nil, 0, err
	}

	// create input object
	input := &ses.SendBulkTemplatedEmailInput{
		Template: aws.String(templateName),
		Source:   aws.String(senderEmail),
	}

	if util.LenTrim(defaultTemplateDataJson) > 0 {
		input.DefaultTemplateData = aws.String(defaultTemplateDataJson)
	}

	if util.LenTrim(returnPath) > 0 {
		input.ReturnPath = aws.String(returnPath)
	} else {
		input.ReturnPath = aws.String(senderEmail)
	}

	if util.LenTrim(replyTo) > 0 {
		input.ReplyToAddresses = []*string{aws.String(replyTo)}
	} else {
		input.ReplyToAddresses = []*string{aws.String(senderEmail)}
	}

	if input.Destinations == nil {
		for _, v := range emailTargetList {
			dest := new(ses.BulkEmailDestination)

			if dest.Destination == nil {
				dest.Destination = new(ses.Destination)
			}

			isSet := false

			if len(v.To) > 0 {
				isSet = true
				dest.Destination.ToAddresses = aws.StringSlice(v.To)
			}

			if len(v.CC) > 0 {
				isSet = true
				dest.Destination.CcAddresses = aws.StringSlice(v.CC)
			}

			if len(v.BCC) > 0 {
				isSet = true
				dest.Destination.BccAddresses = aws.StringSlice(v.BCC)
			}

			if util.LenTrim(v.TemplateDataJson) > 0 {
				dest.ReplacementTemplateData = aws.String(v.TemplateDataJson)
			}

			if isSet {
				input.Destinations = append(input.Destinations, dest)
			}
		}
	}

	if len(input.Destinations) == 0 {
		return nil, 0, errors.New("SendBulkTemplateEmail Failed: no valid destinations after validation")
	}

	s.mu.RLock()
	cfgSetName2 := s.ConfigurationSetName
	s.mu.RUnlock()
	if util.LenTrim(cfgSetName2) > 0 {
		input.ConfigurationSetName = aws.String(cfgSetName2)
	}

	// perform action
	var output *ses.SendBulkTemplatedEmailOutput

	callCtx, callCancel := ensureSESCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.SendBulkTemplatedEmailWithContext(callCtx, input)
	callCancel()

	// evaluate result
	if err != nil {
		err = errors.New("SendBulkTemplateEmail Failed: (Send Action) " + err.Error())
		return nil, 0, err
	}

	if output == nil {
		return nil, 0, errors.New("SendBulkTemplateEmail Failed: nil output from AWS SDK")
	}

	for _, v := range output.Status {
		success := false

		if strings.ToUpper(aws.StringValue(v.Status)) == "SUCCESS" {
			success = true
		} else {
			failedCount++
		}

		resultList = append(resultList, &BulkEmailMessageResult{
			MessageId: aws.StringValue(v.MessageId),
			Status:    aws.StringValue(v.Status),
			ErrorInfo: aws.StringValue(v.Error),
			Success:   success,
		})
	}

	return resultList, failedCount, nil
}

// ----------------------------------------------------------------------------------------------------------------
// SES custom verification email methods
// ----------------------------------------------------------------------------------------------------------------

// CreateCustomVerificationEmailTemplate will create a template for use with custom verification of email address
//
// parameters:
//  1. templateName = the name of the template, must be unique
//  2. templateSubject = the subject of the verification email (note that there is no tag name value replacement here)
//  3. templateContent = the body of the email, can be html or text (note that there is no tag name value replacement here)
//  4. fromEmailAddress = the email address that the verification email is sent from (must be email address)
//  5. successRedirectionURL = the url that users are sent to if their email addresses are successfully verified
//  6. failureRedirectionURL = the url that users are sent to if their email addresses are not successfully verified
//  7. timeOutDuration = optional time out value to use in context
func (s *SES) CreateCustomVerificationEmailTemplate(templateName string,
	templateSubject string,
	templateContent string,
	fromEmailAddress string,
	successRedirectionURL string,
	failureRedirectionURL string,
	timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SES-CreateCustomVerificationEmailTemplate", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-CreateCustomVerificationEmailTemplate-TemplateName", templateName))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-CreateCustomVerificationEmailTemplate-TemplateSubject", maskSubjectForXray(templateSubject)))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-CreateCustomVerificationEmailTemplate-TemplateContent", templateContent))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-CreateCustomVerificationEmailTemplate-Email-From", maskEmailForXray(fromEmailAddress)))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-CreateCustomVerificationEmailTemplate-Success-RedirectURL", successRedirectionURL))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-CreateCustomVerificationEmailTemplate-Failure-RedirectURL", failureRedirectionURL))

			if err != nil {
				xray.LogXrayAddFailure("SES", seg.SafeAddError(err))
			}
		}()
	}

	// validate
	client := s.getClient()
	if client == nil {
		err = errors.New("CreateCustomVerificationEmailTemplate Failed: " + "SES Client is Required")
		return err
	}

	if util.LenTrim(templateName) <= 0 {
		err = errors.New("CreateCustomVerificationEmailTemplate Failed: " + "Template Name is Required")
		return err
	}

	if util.LenTrim(templateSubject) <= 0 {
		err = errors.New("CreateCustomVerificationEmailTemplate Failed: " + "Template Subject is Required")
		return err
	}

	if util.LenTrim(templateContent) <= 0 {
		err = errors.New("CreateCustomVerificationEmailTemplate Failed: " + "Template Content is Required")
		return err
	}

	if util.LenTrim(fromEmailAddress) <= 0 {
		err = errors.New("CreateCustomVerificationEmailTemplate Failed: " + "From Email Address is Required")
		return err
	}

	if util.LenTrim(successRedirectionURL) <= 0 {
		err = errors.New("CreateCustomVerificationEmailTemplate Failed: " + "Success Redirection URL is Required")
		return err
	}

	if util.LenTrim(failureRedirectionURL) <= 0 {
		err = errors.New("CreateCustomVerificationEmailTemplate Failed: " + "Failure Redirection URL is Required")
		return err
	}

	// create input object
	input := &ses.CreateCustomVerificationEmailTemplateInput{
		TemplateName:          aws.String(templateName),
		TemplateSubject:       aws.String(templateSubject),
		TemplateContent:       aws.String(templateContent),
		FromEmailAddress:      aws.String(fromEmailAddress),
		SuccessRedirectionURL: aws.String(successRedirectionURL),
		FailureRedirectionURL: aws.String(failureRedirectionURL),
	}

	// perform action
	callCtx, callCancel := ensureSESCtx(segCtx, segCtxSet, timeOutDuration)
	_, err = client.CreateCustomVerificationEmailTemplateWithContext(callCtx, input)
	callCancel()

	// evaluate result
	if err != nil {
		// error
		err = errors.New("CreateCustomVerificationEmailTemplate Failed: (Create Template Action) " + err.Error())
		return err
	} else {
		// success
		return nil
	}
}

// UpdateCustomVerificationEmailTemplate will update a template for use with custom verification of email address
//
// parameters:
//  1. templateName = the name of the template, must be unique
//  2. templateSubject = the subject of the verification email (note that there is no tag name value replacement here)
//  3. templateContent = the body of the email, can be html or text (note that there is no tag name value replacement here)
//  4. fromEmailAddress = the email address that the verification email is sent from (must be email address)
//  5. successRedirectionURL = the url that users are sent to if their email addresses are successfully verified
//  6. failureRedirectionURL = the url that users are sent to if their email addresses are not successfully verified
//  7. timeOutDuration = optional time out value to use in context
func (s *SES) UpdateCustomVerificationEmailTemplate(templateName string,
	templateSubject string,
	templateContent string,
	fromEmailAddress string,
	successRedirectionURL string,
	failureRedirectionURL string,
	timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SES-UpdateCustomVerificationEmailTemplate", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-UpdateCustomVerificationEmailTemplate-TemplateName", templateName))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-UpdateCustomVerificationEmailTemplate-TemplateSubject", maskSubjectForXray(templateSubject)))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-UpdateCustomVerificationEmailTemplate-TemplateContent", templateContent))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-UpdateCustomVerificationEmailTemplate-Email-From", maskEmailForXray(fromEmailAddress)))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-UpdateCustomVerificationEmailTemplate-Success-RedirectURL", successRedirectionURL))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-UpdateCustomVerificationEmailTemplate-Failure-RedirectURL", failureRedirectionURL))

			if err != nil {
				xray.LogXrayAddFailure("SES", seg.SafeAddError(err))
			}
		}()
	}

	// validate
	client := s.getClient()
	if client == nil {
		err = errors.New("UpdateCustomVerificationEmailTemplate Failed: " + "SES Client is Required")
		return err
	}

	if util.LenTrim(templateName) <= 0 {
		err = errors.New("UpdateCustomVerificationEmailTemplate Failed: " + "Template Name is Required")
		return err
	}

	if util.LenTrim(templateSubject) <= 0 {
		err = errors.New("UpdateCustomVerificationEmailTemplate Failed: " + "Template Subject is Required")
		return err
	}

	if util.LenTrim(templateContent) <= 0 {
		err = errors.New("UpdateCustomVerificationEmailTemplate Failed: " + "Template Content is Required")
		return err
	}

	if util.LenTrim(fromEmailAddress) <= 0 {
		err = errors.New("UpdateCustomVerificationEmailTemplate Failed: " + "From Email Address is Required")
		return err
	}

	if util.LenTrim(successRedirectionURL) <= 0 {
		err = errors.New("UpdateCustomVerificationEmailTemplate Failed: " + "Success Redirection URL is Required")
		return err
	}

	if util.LenTrim(failureRedirectionURL) <= 0 {
		err = errors.New("UpdateCustomVerificationEmailTemplate Failed: " + "Failure Redirection URL is Required")
		return err
	}

	// create input object
	input := &ses.UpdateCustomVerificationEmailTemplateInput{
		TemplateName:          aws.String(templateName),
		TemplateSubject:       aws.String(templateSubject),
		TemplateContent:       aws.String(templateContent),
		FromEmailAddress:      aws.String(fromEmailAddress),
		SuccessRedirectionURL: aws.String(successRedirectionURL),
		FailureRedirectionURL: aws.String(failureRedirectionURL),
	}

	// perform action
	callCtx, callCancel := ensureSESCtx(segCtx, segCtxSet, timeOutDuration)
	_, err = client.UpdateCustomVerificationEmailTemplateWithContext(callCtx, input)
	callCancel()

	// evaluate result
	if err != nil {
		// error
		err = errors.New("UpdateCustomVerificationEmailTemplate Failed: (Update Template Action) " + err.Error())
		return err
	} else {
		// success
		return nil
	}
}

// DeleteCustomVerificationEmailTemplate will delete a custom verification email template
//
// parameters:
//  1. templateName = name of the template to delete
//  2. timeOutDuration = optional time out value to use in context
func (s *SES) DeleteCustomVerificationEmailTemplate(templateName string, timeOutDuration ...time.Duration) (err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SES-DeleteCustomVerificationEmailTemplate", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-DeleteCustomVerificationEmailTemplate-TemplateName", templateName))

			if err != nil {
				xray.LogXrayAddFailure("SES", seg.SafeAddError(err))
			}
		}()
	}

	// validate
	client := s.getClient()
	if client == nil {
		err = errors.New("DeleteCustomVerificationEmailTemplate Failed: " + "SES Client is Required")
		return err
	}

	if util.LenTrim(templateName) <= 0 {
		err = errors.New("DeleteCustomVerificationEmailTemplate Failed: " + "Template Name is Required")
		return err
	}

	// create input object
	input := &ses.DeleteCustomVerificationEmailTemplateInput{
		TemplateName: aws.String(templateName),
	}

	// perform action
	callCtx, callCancel := ensureSESCtx(segCtx, segCtxSet, timeOutDuration)
	_, err = client.DeleteCustomVerificationEmailTemplateWithContext(callCtx, input)
	callCancel()

	// evaluate result
	if err != nil {
		err = errors.New("DeleteCustomVerificationEmailTemplate Failed: (Delete Template Action) " + err.Error())
		return err
	} else {
		return nil
	}
}

// SendCustomVerificationEmail will send out a custom verification email to recipient
//
// parameters:
//  1. templateName = name of the template to use for the verification email
//  2. toEmailAddress = the email address to send verification to
//  3. timeOutDuration = optional time out value to use in context
func (s *SES) SendCustomVerificationEmail(templateName string, toEmailAddress string, timeOutDuration ...time.Duration) (messageId string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("SES-SendCustomVerificationEmail", s.getParentSegment())

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendCustomVerificationEmail-TemplateName", templateName))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendCustomVerificationEmail-Email-To", maskEmailForXray(toEmailAddress)))
			xray.LogXrayAddFailure("SES", seg.SafeAddMetadata("SES-SendCustomVerificationEmail-Result-MessageID", messageId))

			if err != nil {
				xray.LogXrayAddFailure("SES", seg.SafeAddError(err))
			}
		}()
	}

	// validate
	client := s.getClient()
	if client == nil {
		err = errors.New("SendCustomVerificationEmail Failed: " + "SES Client is Required")
		return "", err
	}

	if util.LenTrim(templateName) <= 0 {
		err = errors.New("SendCustomVerificationEmail Failed: " + "Template Name is Required")
		return "", err
	}

	if util.LenTrim(toEmailAddress) <= 0 {
		err = errors.New("SendCustomVerificationEmail Failed: " + "To-Email Address is Required")
		return "", err
	}

	// create input object
	input := &ses.SendCustomVerificationEmailInput{
		TemplateName: aws.String(templateName),
		EmailAddress: aws.String(toEmailAddress),
	}

	s.mu.RLock()
	cfgSetName3 := s.ConfigurationSetName
	s.mu.RUnlock()
	if util.LenTrim(cfgSetName3) > 0 {
		input.ConfigurationSetName = aws.String(cfgSetName3)
	}

	// perform action
	var output *ses.SendCustomVerificationEmailOutput

	callCtx, callCancel := ensureSESCtx(segCtx, segCtxSet, timeOutDuration)
	output, err = client.SendCustomVerificationEmailWithContext(callCtx, input)
	callCancel()

	// evaluate result
	if err != nil {
		// failure
		err = errors.New("SendCustomVerificationEmail Failed: (Send Action) " + err.Error())
		return "", err
	}

	if output == nil || output.MessageId == nil {
		return "", errors.New("SendCustomVerificationEmail Failed: nil output or MessageId from AWS SDK")
	}

	// success
	messageId = *output.MessageId
	return messageId, nil
}
