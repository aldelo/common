package ses

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

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	util "github.com/aldelo/common"
	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
	"time"
)

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

	// store ses client object
	sesClient *ses.SES
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
	ReplyTo []string

	// defaults to UTF-8 if left blank
	Charset string
}

// SendQuota struct encapsulates the ses send quota definition
type SendQuota struct {
	Max24HourSendLimit int
	MaxPerSecondSendLimit int
	SentLast24Hours int
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
	Status string
	ErrorInfo string
	Success bool
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the SES service
func (s *SES) Connect() error {
	// clean up prior object
	s.sesClient = nil

	if !s.AwsRegion.Valid() || s.AwsRegion == awsregion.UNKNOWN {
		return errors.New("Connect To SES Failed: (AWS Session Error) " + "Region is Required")
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
		return errors.New("Connect to SES Failed: (AWS Session Error) " + "Create Custom Http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection and keep session object in struct
	if sess, err := session.NewSession(
		&aws.Config{
			Region:      aws.String(s.AwsRegion.Key()),
			HTTPClient:  httpCli,
		}); err != nil {
		// aws session error
		return errors.New("Connect To SES Failed: (AWS Session Error) " + err.Error())
	} else {
		// create cached objects for shared use
		s.sesClient = ses.New(sess)

		if s.sesClient == nil {
			return errors.New("Connect To SES Client Failed: (New SES Client Connection) " + "Connection Object Nil")
		}

		// session stored to struct
		return nil
	}
}

// Disconnect will disjoin from aws session by clearing it
func (s *SES) Disconnect() {
	s.sesClient = nil
}

// ----------------------------------------------------------------------------------------------------------------
// Email struct methods
// ----------------------------------------------------------------------------------------------------------------

// Initial sets new email message
//
// parameters:
//		from = email sender from address
//		to = email sender to address
// 		subject = email subject text
//		bodyText = email body content in text (both bodyText and bodyHtml may be set at the same time)
//		bodyHtml = email body content in html (both bodyText and bodyHtml may be set at the same time)
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
				Data: aws.String(e.Subject),
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

	if len(e.BCC) > 0 {
		h.Set("BCC", util.SliceStringToCSVString(e.BCC, false))
	}

	if util.LenTrim(e.ReturnPath) > 0 {
		h.Set("Return-Path", e.ReturnPath)
	}

	if len(e.ReplyTo) > 0 {
		h.Set("Reply-To", e.ReplyTo[0])
	}

	h.Set("Subject", e.Subject)
	h.Set("Content-Language", "en-US")
	h.Set("Content-Type", "multipart/mixed; boundary=\"" + writer.Boundary() + "\"")
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
		h.Set("Content-Type", "text/plain; charset=" + charset)
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
		h.Set("Content-Type", "text/html; charset=" + charset)
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
		h.Set("Content-Type", attachmentContentType + "; name=\"" + fn + "\"")
		h.Set("Content-Description", fn)
		h.Set("Content-Disposition", "attachment; filename=\"" + fn + "\"; size=" + util.Itoa(len(attachmentData)) + ";")
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

	// assemble SendRawEmailInput object
	input := &ses.SendRawEmailInput{
		Destinations: aws.StringSlice(e.To),
		Source: aws.String(e.From),
		RawMessage: &raw,
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
func (s *SES) GetSendQuota(timeOutDuration ...time.Duration) (*SendQuota, error) {
	// validate
	if s.sesClient == nil {
		return nil, errors.New("GetSendQuota Failed: " + "SES Client is Required")
	}

	// compose input
	input := &ses.GetSendQuotaInput{

	}

	// get send quota from ses
	var output *ses.GetSendQuotaOutput
	var err error

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sesClient.GetSendQuotaWithContext(ctx, input)
	} else {
		output, err = s.sesClient.GetSendQuota(input)
	}

	// evaluate result
	if err != nil {
		return nil, errors.New("GetSendQuota Failed: " + err.Error())
	}

	return &SendQuota{
		Max24HourSendLimit: util.Float64ToInt(*output.Max24HourSend),
		MaxPerSecondSendLimit: util.Float64ToInt(*output.MaxSendRate),
		SentLast24Hours: util.Float64ToInt(*output.SentLast24Hours),
	}, nil
}

// SendEmail will send an email message out via SES
func (s *SES) SendEmail(email *Email, timeOutDuration ...time.Duration) (messageId string, err error) {
	// validate
	if s.sesClient == nil {
		return "", errors.New("SendEmail Failed: " + "SES Client is Required")
	}

	if email == nil {
		return "", errors.New("SendEmail Failed: " + "Email Object is Required")
	}

	// generate email input
	var input *ses.SendEmailInput
	input, err = email.GenerateSendEmailInput()

	if err != nil {
		return "", errors.New("SendEmail Failed: " + err.Error())
	}

	if input == nil {
		return "", errors.New("SendEmail Failed: " + "SendEmailInput is Nil")
	}

	// set configurationSetName if applicable
	if util.LenTrim(s.ConfigurationSetName) > 0 {
		input.ConfigurationSetName = aws.String(s.ConfigurationSetName)
	}

	// send out email
	var output *ses.SendEmailOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sesClient.SendEmailWithContext(ctx, input)
	} else {
		output, err = s.sesClient.SendEmail(input)
	}

	// evaluate output
	if err != nil {
		if err = s.handleSESError(err); err != nil {
			return "", err
		}
	}

	// email sent successfully
	return aws.StringValue(output.MessageId), nil
}

// SendRawEmail will send an raw email message out via SES, supporting attachments
//
// attachmentContentType = See: https://developer.mozilla.org/en-US/docs/Web/HTTP/Basics_of_HTTP/MIME_types/Common_types
func (s *SES) SendRawEmail(email *Email, attachmentFileName string, attachmentContentType string,
						   timeOutDuration ...time.Duration) (messageId string, err error) {
	// validate
	if s.sesClient == nil {
		return "", errors.New("SendRawEmail Failed: " + "SES Client is Required")
	}

	if email == nil {
		return "", errors.New("SendRawEmail Failed: " + "Email Object is Required")
	}

	// load attachment info
	attachmentData := []byte{}

	if util.LenTrim(attachmentFileName) > 0 {
		if attachmentData, err = util.FileReadBytes(attachmentFileName); err != nil {
			return "", fmt.Errorf("SendRawEmail Failed: (Load Attachment File '" + attachmentFileName + "' Error)", err)
		} else {
			if len(attachmentData) == 0 {
				return "", fmt.Errorf("SendRawEmail Failed: ", "Attachment Data From File is Empty")
			}

			if util.LenTrim(attachmentContentType) == 0 {
				return "", fmt.Errorf("SendRawEmail Failed: ", "Attachment Content-Type is Required")
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
		return "", errors.New("SendRawEmail Failed: " + err.Error())
	}

	if input == nil {
		return "", errors.New("SendRawEmail Failed: " + "SendRawEmailInput is Nil")
	}

	// set configurationSetName if applicable
	if util.LenTrim(s.ConfigurationSetName) > 0 {
		input.ConfigurationSetName = aws.String(s.ConfigurationSetName)
	}

	// send out email
	var output *ses.SendRawEmailOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sesClient.SendRawEmailWithContext(ctx, input)
	} else {
		output, err = s.sesClient.SendRawEmail(input)
	}

	// evaluate output
	if err != nil {
		if err = s.handleSESError(err); err != nil {
			return "", err
		}
	}

	// email sent successfully
	return aws.StringValue(output.MessageId), nil
}

// ----------------------------------------------------------------------------------------------------------------
// SES template & bulk email methods
// ----------------------------------------------------------------------------------------------------------------

// CreateTemplate will create an email template for use with SendTemplateEmail and SendBulkTemplateEmail methods
//
// parameters:
//		1) templateName = name of the template, must be unique
//		2) subjectPart = subject text with tag name value replacements
//		3) textPart = body text with tag name value replacements
// 		4) htmlPart = body html with tag name value replacements
//		5) timeOutDuration = optional time out value to use in context
//
// template tokens:
//		1) subjectPart, textPart, htmlPart = may contain Tag Names for personalization
//		2) Tag Name = using {{tagName}} within the string defined above in #1
//		3) example:
//				a) Greetings, {{name}}
//				b) <h1>hello {{name}}, how are you</h1><p>Today is {{dayOfWeek}}</p>
//				c) <html><head></head><body><h1>hello {{name}}, how are you</h1><p>Today is {{dayOfWeek}}</p></body></html>
//		4) when sending template mail, the Tag Name values are replaced via key-Value pairs within TemplateData in SendTemplateEmail and SendBulkTemplateEmail methods:
//				a) { "name":"Harry", "dayOfWeek":"Monday" }
func (s *SES) CreateTemplate(templateName string, subjectPart string, textPart string, htmlPart string, timeOutDuration ...time.Duration) error {
	// validate
	if s.sesClient == nil {
		return errors.New("CreateTemplate Failed: " + "SES Client is Required")
	}

	if util.LenTrim(templateName) <= 0 {
		return errors.New("CreateTemplate Failed: " + "Template Name is Required")
	}

	if util.LenTrim(subjectPart) <= 0 {
		return errors.New("CreateTemplate Failed: " + "Subject Part is Required")
	}

	if util.LenTrim(textPart) <= 0 && util.LenTrim(htmlPart) <= 0 {
		return errors.New("CreateTemplate Failed: " + "Text or Html Part is Required")
	}

	// create input object
	input := &ses.CreateTemplateInput{
		Template: &ses.Template{
			TemplateName: aws.String(templateName),
			SubjectPart: aws.String(subjectPart),
		},
	}

	if util.LenTrim(textPart) > 0 {
		input.Template.TextPart = aws.String(textPart)
	}

	if util.LenTrim(htmlPart) > 0 {
		input.Template.HtmlPart = aws.String(htmlPart)
	}

	// create template action
	var err error

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		_, err = s.sesClient.CreateTemplateWithContext(ctx, input)
	} else {
		_, err = s.sesClient.CreateTemplate(input)
	}

	// evaluate result
	if err != nil {
		// failure
		return errors.New("CreateTemplate Failed: (Create Template Action) " + err.Error())
	} else {
		// success
		return nil
	}
}

// UpdateTemplate will update an existing email template for use with SendTemplateEmail and SendBulkTemplateEmail methods
//
// parameters:
//		1) templateName = name of the template, must be unique
//		2) subjectPart = subject text with tag name value replacements
//		3) textPart = body text with tag name value replacements
// 		4) htmlPart = body html with tag name value replacements
//		5) timeOutDuration = optional time out value to use in context
//
// template tokens:
//		1) subjectPart, textPart, htmlPart = may contain Tag Names for personalization
//		2) Tag Name = using {{tagName}} within the string defined above in #1
//		3) example:
//				a) Greetings, {{name}}
//				b) <h1>hello {{name}}, how are you</h1><p>Today is {{dayOfWeek}}</p>
//				c) <html><head></head><body><h1>hello {{name}}, how are you</h1><p>Today is {{dayOfWeek}}</p></body></html>
//		4) when sending template mail, the Tag Name values are replaced via key-Value pairs within TemplateData in SendTemplateEmail and SendBulkTemplateEmail methods:
//				a) { "name":"Harry", "dayOfWeek":"Monday" }
func (s *SES) UpdateTemplate(templateName string, subjectPart string, textPart string, htmlPart string, timeOutDuration ...time.Duration) error {
	// validate
	if s.sesClient == nil {
		return errors.New("UpdateTemplate Failed: " + "SES Client is Required")
	}

	if util.LenTrim(templateName) <= 0 {
		return errors.New("UpdateTemplate Failed: " + "Template Name is Required")
	}

	if util.LenTrim(subjectPart) <= 0 {
		return errors.New("UpdateTemplate Failed: " + "Subject Part is Required")
	}

	if util.LenTrim(textPart) <= 0 && util.LenTrim(htmlPart) <= 0 {
		return errors.New("UpdateTemplate Failed: " + "Text or Html Part is Required")
	}

	// create input object
	input := &ses.UpdateTemplateInput{
		Template: &ses.Template{
			TemplateName: aws.String(templateName),
			SubjectPart: aws.String(subjectPart),
		},
	}

	if util.LenTrim(textPart) > 0 {
		input.Template.TextPart = aws.String(textPart)
	}

	if util.LenTrim(htmlPart) > 0 {
		input.Template.HtmlPart = aws.String(htmlPart)
	}

	// update template action
	var err error

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		_, err = s.sesClient.UpdateTemplateWithContext(ctx, input)
	} else {
		_, err = s.sesClient.UpdateTemplate(input)
	}

	// evaluate result
	if err != nil {
		// failure
		return errors.New("UpdateTemplate Failed: (Update Template Action) " + err.Error())
	} else {
		// success
		return nil
	}
}

// DeleteTemplate will delete an existing email template
//
// parameters:
//		1) templateName = name of the template to delete
//		2) timeOutDuration = optional time out value to use in context
func (s *SES) DeleteTemplate(templateName string, timeOutDuration ...time.Duration) error {
	// validate
	if s.sesClient == nil {
		return errors.New("DeleteTemplate Failed: " + "SES Client is Required")
	}

	if util.LenTrim(templateName) <= 0 {
		return errors.New("DeleteTemplate Failed: " + "Template Name is Required")
	}

	// create input object
	input := &ses.DeleteTemplateInput{
		TemplateName: aws.String(templateName),
	}

	// update template action
	var err error

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		_, err = s.sesClient.DeleteTemplateWithContext(ctx, input)
	} else {
		_, err = s.sesClient.DeleteTemplate(input)
	}

	// evaluate result
	if err != nil {
		// failure
		return errors.New("DeleteTemplate Failed: (Delete Template Action) " + err.Error())
	} else {
		// success
		return nil
	}
}

// SendTemplateEmail will send out email via ses using template
//
// parameters:
//		1) senderEmail = sender's email address
//		2) returnPath = optional, email return path address
//		3) replyTo = optional, email reply to address
//		4) templateName = the name of the template to use for sending out template emails
//		5) emailTarget = pointer to email target object defining where the email is sent to, along with template data key value pairs in json
//		6) timeOutDuration = optional, time out value to use in context
//
// template tokens:
//		1) subjectPart, textPart, htmlPart = may contain Tag Names for personalization
//		2) Tag Name = using {{tagName}} within the string defined above in #1
//		3) example:
//				a) Greetings, {{name}}
//				b) <h1>hello {{name}}, how are you</h1><p>Today is {{dayOfWeek}}</p>
//				c) <html><head></head><body><h1>hello {{name}}, how are you</h1><p>Today is {{dayOfWeek}}</p></body></html>
//		4) when sending template mail, the Tag Name values are replaced via key-Value pairs within TemplateData in SendTemplateEmail and SendBulkTemplateEmail methods:
//				a) { "name":"Harry", "dayOfWeek":"Monday" }
//
// notes:
//		a) Template = name of template to apply to the email
//		b) TemplateData = json string holding key-value pair of Tag Name and Tag Value (see below about template tokens)
//		c) Source = email address of sender
//		d) ReturnPath = return email address (usually same as source)
//		e) ReplyToAddresses = list of email addresses for email reply to
//		f) Destination = email send to destination
//		g) ConfigurationSetName = if ses has a set defined, and need to be used herein
func (s *SES) SendTemplateEmail(senderEmail string,
								returnPath string,
								replyTo string,
								templateName string,
								emailTarget *EmailTarget,
								timeOutDuration ...time.Duration) (messageId string, err error) {
	// validate
	if s.sesClient == nil {
		return "", errors.New("SendTemplateEmail Failed: " + "SES Client is Required")
	}

	if util.LenTrim(senderEmail) <= 0 {
		return "", errors.New("SendTemplateEmail Failed: " + "Sender Email is Required")
	}

	if util.LenTrim(templateName) <= 0 {
		return "", errors.New("SendTemplateEmail Failed: " + "Template Name is Required")
	}

	if emailTarget == nil {
		return "", errors.New("SendTemplateEmail Failed: " + "Email Target is Required (Nil)")
	}

	if len(emailTarget.To) <= 0 {
		return "", errors.New("SendTemplateEmail Failed: " + "Email Target's To-Address is Required")
	}

	// create input object
	input := &ses.SendTemplatedEmailInput{
		Template: aws.String(templateName),
		Source: aws.String(senderEmail),
	}

	// set additional input values
	if util.LenTrim(returnPath) > 0 {
		input.ReturnPath = aws.String(returnPath)
	} else {
		input.ReturnPath = aws.String(senderEmail)
	}

	if util.LenTrim(replyTo) > 0 {
		input.ReplyToAddresses = []*string{ aws.String(replyTo) }
	} else {
		input.ReplyToAddresses = []*string{ aws.String(senderEmail) }
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

	if util.LenTrim(s.ConfigurationSetName) > 0 {
		input.ConfigurationSetName = aws.String(s.ConfigurationSetName)
	}

	// perform action
	var output *ses.SendTemplatedEmailOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sesClient.SendTemplatedEmailWithContext(ctx, input)
	} else {
		output, err = s.sesClient.SendTemplatedEmail(input)
	}

	// evaluate result
	if err != nil {
		return "", errors.New("SendTemplateEmailFailed: (Send Action) " + err.Error())
	} else {
		return *output.MessageId, nil
	}
}

// SendBulkTemplateEmail will send out bulk email via ses using template
//
// parameters:
//		1) senderEmail = sender's email address
//		2) returnPath = optional, email return path address
//		3) replyTo = optional, email reply to address
//		4) templateName = the name of the template to use for sending out template emails
//		5) defaultTemplateDataJson = the default template data key value pairs in json, to be used if emailTarget did not define a template data json
//		6) emailTargetList = slice of pointer to email target object defining where the email is sent to, along with template data key value pairs in json
//		7) timeOutDuration = optional, time out value to use in context
//
// template tokens:
//		1) subjectPart, textPart, htmlPart = may contain Tag Names for personalization
//		2) Tag Name = using {{tagName}} within the string defined above in #1
//		3) example:
//				a) Greetings, {{name}}
//				b) <h1>hello {{name}}, how are you</h1><p>Today is {{dayOfWeek}}</p>
//				c) <html><head></head><body><h1>hello {{name}}, how are you</h1><p>Today is {{dayOfWeek}}</p></body></html>
//		4) when sending template mail, the Tag Name values are replaced via key-Value pairs within TemplateData in SendTemplateEmail and SendBulkTemplateEmail methods:
//				a) { "name":"Harry", "dayOfWeek":"Monday" }
//
// notes:
//		a) Template = name of template to apply to the email
//		b) DefaultTemplateData = json string holding key-value pair of Tag Name and Tag Value (see below about template tokens)
//   							 acting as a catch all default in case destination level replacement template data is not set
//		c) Source = email address of sender
//		d) ReturnPath = return email address (usually same as source)
//		e) ReplyToAddresses = list of email addresses for email reply to
//		f) Destinations = one or more Destinations, each defining target, and replacement template data for that specific destination
//		g) ConfigurationSetName = if ses has a set defined, and need to be used herein
//
// limits:
//		1) bulk is limited to 50 destinations per single call (subject to aws ses limits on account)
func (s *SES) SendBulkTemplateEmail(senderEmail string,
									returnPath string,
									replyTo string,
									templateName string,
									defaultTemplateDataJson string,
									emailTargetList []*EmailTarget,
									timeOutDuration ...time.Duration) (resultList []*BulkEmailMessageResult, failedCount int, err error) {
	// validate
	if s.sesClient == nil {
		return nil, 0, errors.New("SendBulkTemplateEmail Failed: " + "SES Client is Required")
	}

	if util.LenTrim(senderEmail) <= 0 {
		return nil, 0, errors.New("SendBulkTemplateEmail Failed: " + "Sender Email is Required")
	}

	if util.LenTrim(templateName) <= 0 {
		return nil, 0, errors.New("SendBulkTemplateEmail Failed: " + "Template Name is Required")
	}

	if len(emailTargetList) <= 0 {
		return nil, 0, errors.New("SendBulkTemplateEmail Failed: " + "Email Target List is Required")
	}

	// create input object
	input := &ses.SendBulkTemplatedEmailInput{
		Template: aws.String(templateName),
		Source: aws.String(senderEmail),
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
		input.ReplyToAddresses = []*string{ aws.String(replyTo) }
	} else {
		input.ReplyToAddresses = []*string{ aws.String(senderEmail) }
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

	if util.LenTrim(s.ConfigurationSetName) > 0 {
		input.ConfigurationSetName = aws.String(s.ConfigurationSetName)
	}

	// perform action
	var output *ses.SendBulkTemplatedEmailOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sesClient.SendBulkTemplatedEmailWithContext(ctx, input)
	} else {
		output, err = s.sesClient.SendBulkTemplatedEmail(input)
	}

	// evaluate result
	if err != nil {
		return nil, 0, errors.New("SendBulkTemplateEmail Failed: (Send Action) " + err.Error())
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
			Status: aws.StringValue(v.Status),
			ErrorInfo: aws.StringValue(v.Error),
			Success: success,
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
//		1) templateName = the name of the template, must be unique
//		2) templateSubject = the subject of the verification email (note that there is no tag name value replacement here)
//		3) templateContent = the body of the email, can be html or text (note that there is no tag name value replacement here)
//		4) fromEmailAddress = the email address that the verification email is sent from (must be email address)
// 		5) successRedirectionURL = the url that users are sent to if their email addresses are successfully verified
//		6) failureRedirectionURL = the url that users are sent to if their email addresses are not successfully verified
//		7) timeOutDuration = optional time out value to use in context
func (s *SES) CreateCustomVerificationEmailTemplate(templateName string,
													templateSubject string,
													templateContent string,
													fromEmailAddress string,
													successRedirectionURL string,
													failureRedirectionURL string,
												    timeOutDuration ...time.Duration) error {
	// validate
	if s.sesClient == nil {
		return errors.New("CreateCustomVerificationEmailTemplate Failed: " + "SES Client is Required")
	}

	if util.LenTrim(templateName) <= 0 {
		return errors.New("CreateCustomVerificationEmailTemplate Failed: " + "Template Name is Required")
	}

	if util.LenTrim(templateSubject) <= 0 {
		return errors.New("CreateCustomVerificationEmailTemplate Failed: " + "Template Subject is Required")
	}

	if util.LenTrim(templateContent) <= 0 {
		return errors.New("CreateCustomVerificationEmailTemplate Failed: " + "Template Content is Required")
	}

	if util.LenTrim(fromEmailAddress) <= 0 {
		return errors.New("CreateCustomVerificationEmailTemplate Failed: " + "From Email Address is Required")
	}

	if util.LenTrim(successRedirectionURL) <= 0 {
		return errors.New("CreateCustomVerificationEmailTemplate Failed: " + "Success Redirection URL is Required")
	}

	if util.LenTrim(failureRedirectionURL) <= 0 {
		return errors.New("CreateCustomVerificationEmailTemplate Failed: " + "Failure Redirection URL is Required")
	}

	// create input object
	input := &ses.CreateCustomVerificationEmailTemplateInput{
		TemplateName: aws.String(templateName),
		TemplateSubject: aws.String(templateSubject),
		TemplateContent: aws.String(templateContent),
		FromEmailAddress: aws.String(fromEmailAddress),
		SuccessRedirectionURL: aws.String(successRedirectionURL),
		FailureRedirectionURL: aws.String(failureRedirectionURL),
	}

	// perform action
	var err error

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		_, err = s.sesClient.CreateCustomVerificationEmailTemplateWithContext(ctx, input)
	} else {
		_, err = s.sesClient.CreateCustomVerificationEmailTemplate(input)
	}

	// evaluate result
	if err != nil {
		// error
		return errors.New("CreateCustomVerificationEmailTemplate Failed: (Create Template Action) " + err.Error())
	} else {
		// success
		return nil
	}
}

// UpdateCustomVerificationEmailTemplate will update a template for use with custom verification of email address
//
// parameters:
//		1) templateName = the name of the template, must be unique
//		2) templateSubject = the subject of the verification email (note that there is no tag name value replacement here)
//		3) templateContent = the body of the email, can be html or text (note that there is no tag name value replacement here)
//		4) fromEmailAddress = the email address that the verification email is sent from (must be email address)
// 		5) successRedirectionURL = the url that users are sent to if their email addresses are successfully verified
//		6) failureRedirectionURL = the url that users are sent to if their email addresses are not successfully verified
//		7) timeOutDuration = optional time out value to use in context
func (s *SES) UpdateCustomVerificationEmailTemplate(templateName string,
													templateSubject string,
													templateContent string,
													fromEmailAddress string,
													successRedirectionURL string,
													failureRedirectionURL string,
													timeOutDuration ...time.Duration) error {
	// validate
	if s.sesClient == nil {
		return errors.New("UpdateCustomVerificationEmailTemplate Failed: " + "SES Client is Required")
	}

	if util.LenTrim(templateName) <= 0 {
		return errors.New("UpdateCustomVerificationEmailTemplate Failed: " + "Template Name is Required")
	}

	if util.LenTrim(templateSubject) <= 0 {
		return errors.New("UpdateCustomVerificationEmailTemplate Failed: " + "Template Subject is Required")
	}

	if util.LenTrim(templateContent) <= 0 {
		return errors.New("UpdateCustomVerificationEmailTemplate Failed: " + "Template Content is Required")
	}

	if util.LenTrim(fromEmailAddress) <= 0 {
		return errors.New("UpdateCustomVerificationEmailTemplate Failed: " + "From Email Address is Required")
	}

	if util.LenTrim(successRedirectionURL) <= 0 {
		return errors.New("UpdateCustomVerificationEmailTemplate Failed: " + "Success Redirection URL is Required")
	}

	if util.LenTrim(failureRedirectionURL) <= 0 {
		return errors.New("UpdateCustomVerificationEmailTemplate Failed: " + "Failure Redirection URL is Required")
	}

	// create input object
	input := &ses.UpdateCustomVerificationEmailTemplateInput{
		TemplateName: aws.String(templateName),
		TemplateSubject: aws.String(templateSubject),
		TemplateContent: aws.String(templateContent),
		FromEmailAddress: aws.String(fromEmailAddress),
		SuccessRedirectionURL: aws.String(successRedirectionURL),
		FailureRedirectionURL: aws.String(failureRedirectionURL),
	}

	// perform action
	var err error

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		_, err = s.sesClient.UpdateCustomVerificationEmailTemplateWithContext(ctx, input)
	} else {
		_, err = s.sesClient.UpdateCustomVerificationEmailTemplate(input)
	}

	// evaluate result
	if err != nil {
		// error
		return errors.New("UpdateCustomVerificationEmailTemplate Failed: (Update Template Action) " + err.Error())
	} else {
		// success
		return nil
	}
}

// DeleteCustomVerificationEmailTemplate will delete a custom verification email template
//
// parameters:
//		1) templateName = name of the template to delete
//		2) timeOutDuration = optional time out value to use in context
func (s *SES) DeleteCustomVerificationEmailTemplate(templateName string, timeOutDuration ...time.Duration) error {
	// validate
	if s.sesClient == nil {
		return errors.New("DeleteCustomVerificationEmailTemplate Failed: " + "SES Client is Required")
	}

	if util.LenTrim(templateName) <= 0 {
		return errors.New("DeleteCustomVerificationEmailTemplate Failed: " + "Template Name is Required")
	}

	// create input object
	input := &ses.DeleteCustomVerificationEmailTemplateInput{
		TemplateName: aws.String(templateName),
	}

	// perform action
	var err error

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		_, err = s.sesClient.DeleteCustomVerificationEmailTemplateWithContext(ctx, input)
	} else {
		_, err = s.sesClient.DeleteCustomVerificationEmailTemplate(input)
	}

	// evaluate result
	if err != nil {
		return errors.New("DeleteCustomVerificationEmailTemplate Failed: (Delete Template Action) " + err.Error())
	} else {
		return nil
	}
}

// SendCustomVerificationEmail will send out a custom verification email to recipient
//
// parameters:
//		1) templateName = name of the template to use for the verification email
//		2) toEmailAddress = the email address to send verification to
//		3) timeOutDuration = optional time out value to use in context
func (s *SES) SendCustomVerificationEmail(templateName string, toEmailAddress string, timeOutDuration ...time.Duration) (messageId string, err error) {
	// validate
	if s.sesClient == nil {
		return "", errors.New("SendCustomVerificationEmail Failed: " + "SES Client is Required")
	}

	if util.LenTrim(templateName) <= 0 {
		return "", errors.New("SendCustomVerificationEmail Failed: " + "Template Name is Required")
	}

	if util.LenTrim(toEmailAddress) <= 0 {
		return "", errors.New("SendCustomVerificationEmail Failed: " + "To-Email Address is Required")
	}

	// create input object
	input := &ses.SendCustomVerificationEmailInput{
		TemplateName: aws.String(templateName),
		EmailAddress: aws.String(toEmailAddress),
	}

	if util.LenTrim(s.ConfigurationSetName) > 0 {
		input.ConfigurationSetName = aws.String(s.ConfigurationSetName)
	}

	// perform action
	var output *ses.SendCustomVerificationEmailOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sesClient.SendCustomVerificationEmailWithContext(ctx, input)
	} else {
		output, err = s.sesClient.SendCustomVerificationEmail(input)
	}

	// evaluate result
	if err != nil {
		// failure
		return "", errors.New("SendCustomVerificationEmail Failed: (Send Action) " + err.Error())
	} else {
		// success
		return *output.MessageId, nil
	}
}
