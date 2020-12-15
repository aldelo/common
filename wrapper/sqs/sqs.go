package sqs

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
	"context"
	"errors"
	util "github.com/aldelo/common"
	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aldelo/common/wrapper/sqs/sqscreatequeueattribute"
	"github.com/aldelo/common/wrapper/sqs/sqsgetqueueattribute"
	"github.com/aldelo/common/wrapper/sqs/sqssetqueueattribute"
	"github.com/aldelo/common/wrapper/sqs/sqssystemattribute"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	awssqs "github.com/aws/aws-sdk-go/service/sqs"
	"net/http"
	"time"
)

// ================================================================================================================
// STRUCTS
// ================================================================================================================

// SQS struct encapsulates the AWS SQS access functionality
type SQS struct {
	// define the AWS region that SQS is located at
	AwsRegion awsregion.AWSRegion

	// custom http2 client options
	HttpOptions *awshttp2.HttpClientSettings

	// store SQS client object
	sqsClient *awssqs.SQS
}

// SQSMessageResult struct contains the message result via Send... operations
//
// MessageId = Message ID from SQS after send... operation
// MD5of... = MD5 of given data from SQS, PRIOR TO DATA URL-ENCODED by SQS,
//			  use this MD5 from SQS response to verify local value MD5 to check if SQS received data properly
// FifoSequenceNumber = For FIFO SQS Queue only
type SQSMessageResult struct {
	MessageId string
	MD5ofMessageBody string
	MD5ofMessageAttributes string
	FifoSequenceNumber string
}

// SQSStandardMessage struct represents a standard queue message to be used in Send... operations
//
// Id = up to 80 characters, alpha-numeric / dash, and unique, identifies the message within batch
type SQSStandardMessage struct {
	Id string
	MessageBody string
	MessageAttributes map[string]*awssqs.MessageAttributeValue
	DelaySeconds int64
}

// SQSFifoMessage struct represents a fifo queue message to be used in Send... operations
//
// Id = up to 80 characters, alpha-numeric / dash, and unique, identifies the message within batch
type SQSFifoMessage struct {
	Id string
	MessageDeduplicationId string
	MessageGroupId string
	MessageBody string
	MessageAttributes map[string]*awssqs.MessageAttributeValue
}

// SQSSuccessResult struct represents a Send... batch operation success result
//
// Id = up to 80 characters, alpha-numeric / dash, and unique, identifies the message within batch
type SQSSuccessResult struct {
	Id string
	MessageId string
	MD5ofMessageBody string
	MD5ofMessageAttributes string
	SequenceNumber string
}

// SQSFailResult struct represents a Send... batch operation failure result
//
// Id = up to 80 characters, alpha-numeric / dash, and unique, identifies the message within batch
type SQSFailResult struct {
	Id string
	Code string
	Message string
	SenderFault bool
}

// SQSReceivedMessage struct represents received message content elements
type SQSReceivedMessage struct {
	MessageId string
	Body string
	MessageAttributes map[string]*awssqs.MessageAttributeValue
	SystemAttributes map[sqssystemattribute.SQSSystemAttribute]string
	MD5ofBody string
	MD5ofMessageAttributes string
	ReceiptHandle string
}

// SQSChangeVisibilityRequest struct represents the request data for change visibility for a message in sqs queue
type SQSChangeVisibilityRequest struct {
	Id string
	ReceiptHandle string
	VisibilityTimeOutSeconds int64
}

// SQSDeleteMessageRequest struct represents the request data to delete a message from sqs queue
type SQSDeleteMessageRequest struct {
	Id string
	ReceiptHandle string
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the SQS service
func (s *SQS) Connect() error {
	// clean up prior sqs client reference
	s.sqsClient = nil

	if !s.AwsRegion.Valid() || s.AwsRegion == awsregion.UNKNOWN {
		return errors.New("Connect to SQS Failed: (AWS Session Error) " + "Region is Required")
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
		return errors.New("Connect to SQS Failed: (AWS Session Error) " + "Create Custom http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection
	if sess, err := session.NewSession(
		&aws.Config{
			Region:      aws.String(s.AwsRegion.Key()),
			HTTPClient:  httpCli,
		}); err != nil {
		// aws session error
		return errors.New("Connect to SQS Failed: (AWS Session Error) " + err.Error())
	} else {
		// create cached objects for shared use
		s.sqsClient = awssqs.New(sess)

		if s.sqsClient == nil {
			return errors.New("Connect to SQS Client Failed: (New SQS Client Connection) " + "Connection Object Nil")
		}

		// connect successful
		return nil
	}
}

// Disconnect will clear sqs client
func (s *SQS) Disconnect() {
	s.sqsClient = nil
}

// ----------------------------------------------------------------------------------------------------------------
// internal helper methods
// ----------------------------------------------------------------------------------------------------------------

// toAwsCreateQueueAttributes will convert from strongly typed to aws accepted map
func (s *SQS) toAwsCreateQueueAttributes(attributes map[sqscreatequeueattribute.SQSCreateQueueAttribute]string) (newMap map[string]*string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[string]*string)

	for k, v := range attributes {
		if k.Valid() && k != sqscreatequeueattribute.UNKNOWN {
			newMap[k.Key()] = aws.String(v)
		}
	}

	return newMap
}

// fromAwsCreateQueueAttributes will convert from aws map to strongly typed map
func (s *SQS) fromAwsCreateQueueAttributes(attributes map[string]*string) (newMap map[sqscreatequeueattribute.SQSCreateQueueAttribute]string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[sqscreatequeueattribute.SQSCreateQueueAttribute]string)
	var conv sqscreatequeueattribute.SQSCreateQueueAttribute

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

// toAwsGetQueueAttributes will convert from strongly typed to aws accepted map
func (s *SQS) toAwsGetQueueAttributes(attributes map[sqsgetqueueattribute.SQSGetQueueAttribute]string) (newMap map[string]*string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[string]*string)

	for k, v := range attributes {
		if k.Valid() && k != sqsgetqueueattribute.UNKNOWN {
			newMap[k.Key()] = aws.String(v)
		}
	}

	return newMap
}

// fromAwsGetQueueAttributes will convert from aws map to strongly typed map
func (s *SQS) fromAwsGetQueueAttributes(attributes map[string]*string) (newMap map[sqsgetqueueattribute.SQSGetQueueAttribute]string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[sqsgetqueueattribute.SQSGetQueueAttribute]string)
	var conv sqsgetqueueattribute.SQSGetQueueAttribute

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

// toAwsSetQueueAttributes will convert from strongly typed to aws accepted map
func (s *SQS) toAwsSetQueueAttributes(attributes map[sqssetqueueattribute.SQSSetQueueAttribute]string) (newMap map[string]*string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[string]*string)

	for k, v := range attributes {
		if k.Valid() && k != sqssetqueueattribute.UNKNOWN {
			newMap[k.Key()] = aws.String(v)
		}
	}

	return newMap
}

// fromAwsSetQueueAttributes will convert from aws map to strongly typed map
func (s *SQS) fromAwsSetQueueAttributes(attributes map[string]*string) (newMap map[sqssetqueueattribute.SQSSetQueueAttribute]string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[sqssetqueueattribute.SQSSetQueueAttribute]string)
	var conv sqssetqueueattribute.SQSSetQueueAttribute

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

// toAwsSystemAttributes will convert from strongly typed to aws accepted map
func (s *SQS) toAwsSystemAttributes(attributes map[sqssystemattribute.SQSSystemAttribute]string) (newMap map[string]*string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[string]*string)

	for k, v := range attributes {
		if k.Valid() && k != sqssystemattribute.UNKNOWN {
			newMap[k.Key()] = aws.String(v)
		}
	}

	return newMap
}

// fromAwsSystemAttributes will convert from aws map to strongly typed map
func (s *SQS) fromAwsSystemAttributes(attributes map[string]*string) (newMap map[sqssystemattribute.SQSSystemAttribute]string) {
	// validate
	if attributes == nil {
		return nil
	}

	// make map
	newMap = make(map[sqssystemattribute.SQSSystemAttribute]string)
	var conv sqssystemattribute.SQSSystemAttribute

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
// queue management methods
// ----------------------------------------------------------------------------------------------------------------

// GetQueueArnFromQueueUrl encodes arn from url data
func (s *SQS) GetQueueArnFromQueue(queueUrl string, timeoutDuration ...time.Duration) (string, error) {
	if attr, err := s.GetQueueAttributes(queueUrl, []sqsgetqueueattribute.SQSGetQueueAttribute{
		sqsgetqueueattribute.QueueArn,
	}, timeoutDuration...); err != nil {
		// error
		return "", err
	} else {
		// found attribute
		return attr[sqsgetqueueattribute.QueueArn], nil
	}
}

// CreateQueue will create a queue in SQS,
// if Queue is a FIFO, then queueName must suffix with .fifo, otherwise name it without .fifo suffix
//
// Parameters:
//		1) queueName = required, up to 80 characters alpha-numeric and dash, case-sensitive
//					   if fifo queue, must suffix name with '.fifo' (fifo queue guarantees fifo and delivery once but only 300 / second rate)
//		2) attributes = optional, map of create queue attribute key value pairs
//		3) timeOutDuration = optional, time out duration to use for context if applicable
//
// Return Values:
//		1) queueUrl = the queue url after queue was created
//		2) err = error info if any
//
// Create Queue Attributes: (Key = Expected Value)
//   The following lists the names, descriptions, and values of the special request parameters that the CreateQueue action uses:
// 		1) DelaySeconds = The length of time, in seconds, for which the delivery of all messages in the queue is delayed.
//						  Valid values: An integer from 0 to 900 seconds (15 minutes). Default: 0.
//		2）MaximumMessageSize = The limit of how many bytes a message can contain before Amazon SQS rejects it.
//								Valid values: An integer from 1,024 bytes (1 KiB) to 262,144 bytes (256 KiB). Default: 262,144 (256 KiB).
//		3) MessageRetentionPeriod = The length of time, in seconds, for which Amazon SQS retains a message.
//									Valid values: An integer from 60 seconds (1 minute) to 1,209,600 seconds (14 days). Default: 345,600 (4 days).
// 		4) Policy = The queue's policy. A valid AWS policy.
//					For more information about policy structure,
//						see Overview of AWS IAM Policies (https://docs.aws.amazon.com/IAM/latest/UserGuide/PoliciesOverview.html)
//						in the Amazon IAM User Guide.
//		5) ReceiveMessageWaitTimeSeconds = The length of time, in seconds, for which a ReceiveMessage action waits for a message to arrive.
//										   Valid values: An integer from 0 to 20 (seconds). Default: 0.
//		6) RedrivePolicy = The string that includes the parameters for the dead-letter queue functionality of the source queue as a JSON object.
//						   For more information about the redrive policy and dead-letter queues,
//						   		see Using Amazon SQS Dead-Letter Queues (https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-dead-letter-queues.html)
//						   		in the Amazon Simple Queue Service Developer Guide.
//		7) deadLetterTargetArn = The Amazon Resource Name (ARN) of the dead-letter queue,
//								 to which Amazon SQS moves messages after the value of maxReceiveCount is exceeded.
// 		8) maxReceiveCount = The number of times a message is delivered to the source queue before being moved to the dead-letter queue.
//							 When the ReceiveCount for a message exceeds the maxReceiveCount for a queue,
//							 	Amazon SQS moves the message to the dead-letter-queue.
//							 The dead-letter queue of a FIFO queue must also be a FIFO queue.
//							 Similarly, the dead-letter queue of a standard queue must also be a standard queue.
//		9) VisibilityTimeout = The visibility timeout for the queue, in seconds.
//							   Valid values: An integer from 0 to 43,200 (12 hours). Default: 30.
//							   For more information about the visibility timeout,
//							   		see Visibility Timeout (https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-visibility-timeout.html)
// 							   		in the Amazon Simple Queue Service Developer Guide.
//
//   The following attributes apply only to server-side-encryption,
//   (https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-server-side-encryption.html):
//		10) KmsMasterKeyId = The ID of an AWS-managed customer master key (CMK) for Amazon SQS or a custom CMK.
//							 For more information, see Key Terms (https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-server-side-encryption.html#sqs-sse-key-terms).
//							 While the alias of the AWS-managed CMK for Amazon SQS is always alias/aws/sqs,
//							 	the alias of a custom CMK can, for example, be alias/MyAlias .
//							 For more examples, see KeyId (https://docs.aws.amazon.com/kms/latest/APIReference/API_DescribeKey.html#API_DescribeKey_RequestParameters)
//							 	in the AWS Key Management Service API Reference.
//		11）KmsDataKeyReusePeriodSeconds = The length of time, in seconds, for which Amazon SQS can reuse a data key (https://docs.aws.amazon.com/kms/latest/developerguide/concepts.html#data-keys)
//										   		to encrypt or decrypt messages before calling AWS KMS again.
//										   An integer representing seconds,
//											   between 60 seconds (1 minute) and 86,400 seconds (24 hours). Default: 300 (5 minutes).
//										   A shorter time period provides better security but results in more calls to KMS,
//										   		which might incur charges after Free Tier.
//										   For more information, see How Does the Data Key Reuse Period Work?
//											   (https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-server-side-encryption.html#sqs-how-does-the-data-key-reuse-period-work).
//
//   The following attributes apply only to FIFO (first-in-first-out) queues,
//   (https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/FIFO-queues.html):
//	 	12) FifoQueue = (AT NEW QUEUE CREATION ONLY) Designates a queue as FIFO. Valid values: true, false.
//						If you don't specify the FifoQueue attribute, Amazon SQS creates a standard queue.
//						You can provide this attribute only during queue creation. You can't change it for an existing queue.
//						When you set this attribute, you must also provide the MessageGroupId for your messages explicitly.
//						For more information, see FIFO Queue Logic
//							(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/FIFO-queues.html#FIFO-queues-understanding-logic)
// 							in the Amazon Simple Queue Service Developer Guide.
//		13) ContentBasedDeduplication = Enables content-based deduplication. Valid values: true, false.
//										For more information, see Exactly-Once Processing
//											(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/FIFO-queues.html#FIFO-queues-exactly-once-processing)
//											in the Amazon Simple Queue Service Developer Guide.
//										Every message must have a unique MessageDeduplicationId, You may provide a MessageDeduplicationId explicitly.
//										If you aren't able to provide a MessageDeduplicationId
//											and you enable ContentBasedDeduplication for your queue,
//											Amazon SQS uses a SHA-256 hash to generate the MessageDeduplicationId
//											using the body of the message (but not the attributes of the message).
//										If you don't provide a MessageDeduplicationId and the queue doesn't have ContentBasedDeduplication set,
//											the action fails with an error.
//										If the queue has ContentBasedDeduplication set, your MessageDeduplicationId overrides the generated one.
//										When ContentBasedDeduplication is in effect,
//											messages with identical content sent within the deduplication interval are treated as duplicates,
//											and only one copy of the message is delivered.
//										If you send one message with ContentBasedDeduplication enabled,
//											and then another message with a MessageDeduplicationId that is the same as the one generated,
//											for the first MessageDeduplicationId, the two messages are treated as duplicates,
//											and only one copy of the message is delivered.
func (s *SQS) CreateQueue(queueName string,
						  attributes map[sqscreatequeueattribute.SQSCreateQueueAttribute]string,
						  timeOutDuration ...time.Duration) (queueUrl string, err error) {
	// validate
	if s.sqsClient == nil {
		return "", errors.New("CreateQueue Failed: " + "SQS Client is Required")
	}

	if util.LenTrim(queueName) <= 0 {
		return "", errors.New("CreateQueue Failed: " + "Queue Name is Required")
	}

	// create input object
	input := &awssqs.CreateQueueInput{
		QueueName: aws.String(queueName),
	}

	if attributes != nil {
		input.Attributes = s.toAwsCreateQueueAttributes(attributes)
	}

	// perform action
	var output *awssqs.CreateQueueOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sqsClient.CreateQueueWithContext(ctx, input)
	} else {
		output, err = s.sqsClient.CreateQueue(input)
	}

	// evaluate result
	if err != nil {
		return "", errors.New("CreateQueue Failed: (Create Action) " + err.Error())
	} else {
		return aws.StringValue(output.QueueUrl), nil
	}
}

// GetQueueUrl returns the queue url for specific SQS queue
//
// queueName = required, case-sensitive
func (s *SQS) GetQueueUrl(queueName string, timeOutDuration ...time.Duration) (queueUrl string, notFound bool, err error) {
	// validate
	if s.sqsClient == nil {
		return "", true, errors.New("GetQueueUrl Failed: " + "SQS Client is Required")
	}

	if util.LenTrim(queueName) <= 0 {
		return "", true, errors.New("GetQueueUrl Failed: " + "Queue Name is Required")
	}

	// create input object
	input := &awssqs.GetQueueUrlInput{
		QueueName: aws.String(queueName),
	}

	// perform action
	var output *awssqs.GetQueueUrlOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sqsClient.GetQueueUrlWithContext(ctx, input)
	} else {
		output, err = s.sqsClient.GetQueueUrl(input)
	}

	// evaluate result
	if err != nil {
		if awshttp2.ToAwsError(err).Code() == awssqs.ErrCodeQueueDoesNotExist {
			// queue does not exist
			return "", true, nil
		} else {
			// error
			return "", true, errors.New("GetQueueUrl Failed: (Get Action) " + err.Error())
		}
	} else {
		return aws.StringValue(output.QueueUrl), false,nil
	}
}

// PurgeQueue will delete all messages within the SQS queue indicated by the queueUrl,
// deletion of messages will take up to 60 seconds,
// it is advisable to wait 60 seconds before attempting to use the queue again (to ensure purge completion)
//
// queueUrl = required, case-sensitive
func (s *SQS) PurgeQueue(queueUrl string, timeOutDuration ...time.Duration) error {
	// validate
	if s.sqsClient == nil {
		return errors.New("PurgeQueue Failed: " + "SQS Client is Required")
	}

	if util.LenTrim(queueUrl) <= 0 {
		return errors.New("PurgeQueue Failed: " + "Queue Url is Required")
	}

	// create input object
	input := &awssqs.PurgeQueueInput{
		QueueUrl: aws.String(queueUrl),
	}

	// perform action
	var err error

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		_, err = s.sqsClient.PurgeQueueWithContext(ctx, input)
	} else {
		_, err = s.sqsClient.PurgeQueue(input)
	}

	// evaluate result
	if err != nil {
		return errors.New("PurgeQueue Failed: (Purge Action) " + err.Error())
	} else {
		return nil
	}
}

// DeleteQueue will delete the given SQS queue based on the queueUrl,
// there is a 60 second backoff time for the queue deletion to complete,
// do not send more messages to the queue within the 60 second deletion window of time
//
// queueUrl = required, case-sensitive
func (s *SQS) DeleteQueue(queueUrl string, timeOutDuration ...time.Duration) error {
	// validate
	if s.sqsClient == nil {
		return errors.New("DeleteQueue Failed: " + "SQS Client is Required")
	}

	if util.LenTrim(queueUrl) <= 0 {
		return errors.New("DeleteQueue Failed: " + "Queue Url is Required")
	}

	// create input object
	input := &awssqs.DeleteQueueInput{
		QueueUrl: aws.String(queueUrl),
	}

	// perform action
	var err error

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		_, err = s.sqsClient.DeleteQueueWithContext(ctx, input)
	} else {
		_, err = s.sqsClient.DeleteQueue(input)
	}

	// evaluate result
	if err != nil {
		return errors.New("DeleteQueue Failed: (Delete Action) " + err.Error())
	} else {
		return nil
	}
}

// ListQueues will return a string slice of queue Urls
//
// Parameters:
//		1) queueNamePrefix = optional, match queues based on the queue name that starts with the specified prefix value, case-sensitive
//		2) nextToken = optional, if the prior call to ListQueues expected more values, a more...token is returned, to be used in this parameter
//		3) maxResults = optional, if > 0, the maximum results limited to
//		4) timeOutDuration = optional, timeout value to use within context
//
// Return Values:
//		1) queueUrlsList = string slice of queue urls
//		2) moreQueueUrlsNextToken = if more queue urls expected, use this token in the next method call by passing into nextToken parameter
//		3) err = error info if any
func (s *SQS) ListQueues(queueNamePrefix string,
						 nextToken string,
						 maxResults int64,
						 timeOutDuration ...time.Duration) (queueUrlsList []string, moreQueueUrlsNextToken string, err error) {
	// validate
	if s.sqsClient == nil {
		return nil, "", errors.New("ListQueues Failed: " + "SQS Client is Required")
	}

	// create input object
	input := &awssqs.ListQueuesInput{}

	if util.LenTrim(queueNamePrefix) > 0 {
		input.QueueNamePrefix = aws.String(queueNamePrefix)
	}

	if util.LenTrim(nextToken) > 0 {
		input.NextToken = aws.String(nextToken)
	}

	if maxResults > 0 {
		input.MaxResults = aws.Int64(maxResults)
	}

	// perform action
	var output *awssqs.ListQueuesOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sqsClient.ListQueuesWithContext(ctx, input)
	} else {
		output, err = s.sqsClient.ListQueues(input)
	}

	// evaluate result
	if err != nil {
		return nil, "", errors.New("ListQueues Failed: (List Action) " + err.Error())
	} else {
		moreQueueUrlsNextToken = aws.StringValue(output.NextToken)
		return aws.StringValueSlice(output.QueueUrls), moreQueueUrlsNextToken, nil
	}
}

// ListDeadLetterSourceQueues will retrieve a list of source queue urls,
// that have its RedrivePolicy set to the given Dead Letter Queue as specified in queueUrl parameter
//
// Parameters:
//		1) queueUrl= required, the dead letter queue url for which to list the dead letter source queues, case-sensitive
//		2) nextToken = optional, if the prior call to ListDeadLetterSourceQueues expected more values, a more...token is returned, to be used in this parameter
//		3) maxResults = optional, if > 0, the maximum results limited to
//		4) timeOutDuration = optional, timeout value to use within context
//
// Return Values:
//		1) queueUrlsList = string slice of source queue urls that points to the dead letter queue as indicated in queueUrl parameter
//		2) moreQueueUrlsNextToken = if more queue urls expected, use this token in the next method call by passing into nextToken parameter
//		3) err = error info if any
func (s *SQS) ListDeadLetterSourceQueues(queueUrl string,
										 nextToken string,
										 maxResults int64,
										 timeOutDuration ...time.Duration) (queueUrlsList []string, moreQueueUrlsNextToken string, err error) {
	// validate
	if s.sqsClient == nil {
		return nil, "", errors.New("ListDeadLetterSourceQueues Failed: " + "SQS Client is Required")
	}

	if util.LenTrim(queueUrl) <= 0 {
		return nil, "", errors.New("ListDeadLetterSourceQueues Failed: " + "Dead Letter Queue Url is Required")
	}

	// create input object
	input := &awssqs.ListDeadLetterSourceQueuesInput{
		QueueUrl: aws.String(queueUrl),
	}

	if util.LenTrim(nextToken) > 0 {
		input.NextToken = aws.String(nextToken)
	}

	if maxResults > 0 {
		input.MaxResults = aws.Int64(maxResults)
	}

	// perform action
	var output *awssqs.ListDeadLetterSourceQueuesOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sqsClient.ListDeadLetterSourceQueuesWithContext(ctx, input)
	} else {
		output, err = s.sqsClient.ListDeadLetterSourceQueues(input)
	}

	// evaluate result
	if err != nil {
		return nil, "", errors.New("ListDeadLetterSourceQueues Failed: (List Action) " + err.Error())
	} else {
		moreQueueUrlsNextToken = aws.StringValue(output.NextToken)
		return aws.StringValueSlice(output.QueueUrls), moreQueueUrlsNextToken, nil
	}
}

// GetQueueAttributes will retrieve queue attributes for a given queueUrl, filtered by the attributeNames slice parameter
//
// Parameters:
//		1) queueUrl = required, the queue url for which queue attributes are retrieved against, case-sensitive
//		2) attributeNames = required, slice of attribute names to filter attributes for (see notes below for allowed attribute name values)
//		3) timeOutDuration = optional, timeout duration for context if applicable
//
// Return Values:
//		1) attributes = map of get queue attributes key value pairs retrieved
//		2) err = error info if any
//
// Get Queue Attributes: (Key = Expected Value)
//		1) All = Returns all values.
//		2) ApproximateNumberOfMessages = Returns the approximate number of messages available for retrieval from the queue.
// 		3) ApproximateNumberOfMessagesDelayed = Returns the approximate number of messages in the queue that are delayed
//													and not available for reading immediately.
//												This can happen when the queue is configured as a delay queue
//													or when a message has been sent with a delay parameter.
//		4) ApproximateNumberOfMessagesNotVisible = Returns the approximate number of messages that are in flight.
//												   Messages are considered to be in flight if they have been sent to a client
//													   but have not yet been deleted, or have not yet reached the end of their visibility window.
// 		5) CreatedTimestamp = Returns the time when the queue was created in seconds (epoch time (http://en.wikipedia.org/wiki/Unix_time)).
// 		6) DelaySeconds = Returns the default delay on the queue in seconds.
//		7) LastModifiedTimestamp = Returns the time when the queue was last changed in seconds (epoch time (http://en.wikipedia.org/wiki/Unix_time)).
//		8) MaximumMessageSize = Returns the limit of how many bytes a message can contain before Amazon SQS rejects it.
//		9) MessageRetentionPeriod = Returns the length of time, in seconds, for which Amazon SQS retains a message.
// 		10) Policy = Returns the policy of the queue.
//		11) QueueArn = Returns the Amazon resource name (ARN) of the queue.
// 		12) ReceiveMessageWaitTimeSeconds = Returns the length of time, in seconds,
//											for which the ReceiveMessage action waits for a message to arrive.
// 		13) RedrivePolicy = The string that includes the parameters for the dead-letter queue functionality of the source queue as a JSON object.
//							For more information about the redrive policy and dead-letter queues,
//								see Using Amazon SQS Dead-Letter Queues (https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-dead-letter-queues.html)
//								in the Amazon Simple Queue Service Developer Guide.
//		14) deadLetterTargetArn = The Amazon Resource Name (ARN) of the dead-letter queue to which
//								  Amazon SQS moves messages after the value of maxReceiveCount is exceeded.
//		15) maxReceiveCount = The number of times a message is delivered to the source queue before being moved to the dead-letter queue.
//							  When the ReceiveCount for a message exceeds the maxReceiveCount for a queue,
//							  Amazon SQS moves the message to the dead-letter-queue.
// 		16) VisibilityTimeout = Returns the visibility timeout for the queue.
//								For more information about the visibility timeout, see Visibility Timeout
//									(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-visibility-timeout.html)
//									in the Amazon Simple Queue Service Developer Guide.
//
// 		The following attributes apply only to server-side-encryption
//		(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-server-side-encryption.html):
//		17) KmsMasterKeyId = Returns the ID of an AWS-managed customer master key (CMK) for Amazon SQS or a custom CMK.
//							 For more information, see Key Terms
//							 (https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-server-side-encryption.html#sqs-sse-key-terms).
//		18）KmsDataKeyReusePeriodSeconds = Returns the length of time, in seconds,
//											   for which Amazon SQS can reuse a data key to encrypt or decrypt messages before calling AWS KMS again.
//										   For more information, see How Does the Data Key Reuse Period Work?
//											   (https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-server-side-encryption.html#sqs-how-does-the-data-key-reuse-period-work).
//
// 		The following attributes apply only to FIFO (first-in-first-out) queues
//		(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/FIFO-queues.html):
//		19) FifoQueue = Returns whether the queue is FIFO.
//						For more information, see FIFO Queue Logic
//							(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/FIFO-queues.html#FIFO-queues-understanding-logic)
//							in the Amazon Simple Queue Service Developer Guide.
//						To determine whether a queue is FIFO
//							(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/FIFO-queues.html),
//							you can check whether QueueName ends with the .fifo suffix.
//		20) ContentBasedDeduplication = Returns whether content-based deduplication is enabled for the queue.
//										For more information, see Exactly-Once Processing
//											(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/FIFO-queues.html#FIFO-queues-exactly-once-processing)
//											in the Amazon Simple Queue Service Developer Guide.
func (s *SQS) GetQueueAttributes(queueUrl string,
								 attributeNames []sqsgetqueueattribute.SQSGetQueueAttribute,
								 timeOutDuration ...time.Duration) (attributes map[sqsgetqueueattribute.SQSGetQueueAttribute]string, err error) {
	// validate
	if s.sqsClient == nil {
		return nil, errors.New("GetQueueAttributes Failed: " + "SQS Client is Required")
	}

	if util.LenTrim(queueUrl) <= 0 {
		return nil, errors.New("GetQueueAttributes Failed: " + "Queue Url is Required")
	}

	if attributeNames == nil {
		return nil, errors.New("GetQueueAttributes Failed: " + "Attribute Names Map is Required (nil)")
	}

	if len(attributeNames) <= 0 {
		return nil, errors.New("GetQueueAttributes Failed: " + "Attribute Names Map is Required (len = 0)")
	}

	// create input object
	var keys []*string

	for _, v := range attributeNames {
		if v.Valid() && v != sqsgetqueueattribute.UNKNOWN {
			keys = append(keys, aws.String(v.Key()))
		}
	}

	input := &awssqs.GetQueueAttributesInput{
		QueueUrl: aws.String(queueUrl),
		AttributeNames: keys,
	}

	// perform action
	var output *awssqs.GetQueueAttributesOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sqsClient.GetQueueAttributesWithContext(ctx, input)
	} else {
		output, err = s.sqsClient.GetQueueAttributes(input)
	}

	// evaluate result
	if err != nil {
		return nil, errors.New("GetQueueAttributes Failed: (Get Action) " + err.Error())
	} else {
		return s.fromAwsGetQueueAttributes(output.Attributes), nil
	}
}

// SetQueueAttributes will set attributes for a given queueUrl
//
// Parameters:
//		1) queueUrl = required, the queue to set attributes for, case-sensitive
//		2) attributes = required, map of attribute key value pairs to set for a queue based on the queueUrl
//		3) timeOutDuration = optional, timeout duration for context if applicable
//
// Return Values:
//		1) if successful, nil is returned
//		2) otherwise, error info is returned
//
// Set Queue Attributes: (Key = Expected Value)
// 		1) DelaySeconds = The length of time, in seconds, for which the delivery of all messages in the queue is delayed.
//						  Valid values: An integer from 0 to 900 (15 minutes). Default: 0.
// 		2) MaximumMessageSize = The limit of how many bytes a message can contain before Amazon SQS rejects it.
//								Valid values: An integer from 1,024 bytes (1 KiB) up to 262,144 bytes (256 KiB). Default: 262,144 (256 KiB).
//   	3) MessageRetentionPeriod = The length of time, in seconds, for which Amazon SQS retains a message.
//  								Valid values: An integer representing seconds,
// 										from 60 (1 minute) to 1,209,600 (14 days). Default: 345,600 (4 days).
//   	4) Policy = The queue's policy. A valid AWS policy. For more information about policy structure,
//  				see Overview of AWS IAM Policies
// 						(https://docs.aws.amazon.com/IAM/latest/UserGuide/PoliciesOverview.html)
//						in the Amazon IAM User Guide.
//   	5) ReceiveMessageWaitTimeSeconds = The length of time, in seconds, for which a ReceiveMessage action waits for a message to arrive.
//  									   Valid values: An integer from 0 to 20 (seconds). Default: 0.
//   	6) RedrivePolicy = The string that includes the parameters for the dead-letter queue functionality of the source queue as a JSON object.
//  					   For more information about the redrive policy and dead-letter queues, see Using Amazon SQS Dead-Letter Queues
// 					   			(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-dead-letter-queues.html )
//   							in the Amazon Simple Queue Service Developer Guide.
// 		7) deadLetterTargetArn = The Amazon Resource Name (ARN) of the dead-letter queue to which Amazon SQS moves messages
//									after the value of maxReceiveCount is exceeded.
//  	8) maxReceiveCount = The number of times a message is delivered to the source queue before being moved to the dead-letter queue.
// 							 When the ReceiveCount for a message exceeds the maxReceiveCount for a queue,
//							 	Amazon SQS moves the message to the dead-letter-queue.
//							 The dead-letter queue of a FIFO queue must also be a FIFO queue.
//							 Similarly, the dead-letter queue of a standard queue must also be a standard queue.
//   	9) VisibilityTimeout = The visibility timeout for the queue, in seconds.
//							   Valid values: An integer from 0 to 43,200 (12 hours). Default: 30.
//							   For more information about the visibility timeout, see Visibility Timeout
//				 				    (https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-visibility-timeout.html)
//   								in the Amazon Simple Queue Service Developer Guide.
//
//		The following attributes apply only to server-side-encryption
//		(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-server-side-encryption.html):
//   	10) KmsMasterKeyId = The ID of an AWS-managed customer master key (CMK) for Amazon SQS or a custom CMK.
//  						 For more information, see Key Terms
// 								(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-server-side-encryption.html#sqs-sse-key-terms).
//   						 While the alias of the AWS-managed CMK for Amazon SQS is always alias/aws/sqs,
//   							the alias of a custom CMK can, for example, be alias/MyAlias.
//  						 For more examples, see KeyId (https://docs.aws.amazon.com/kms/latest/APIReference/API_DescribeKey.html#API_DescribeKey_RequestParameters)
//   							in the AWS Key Management Service API Reference.
//   	11) KmsDataKeyReusePeriodSeconds = The length of time, in seconds, for which Amazon SQS can reuse a data key,
//  											to encrypt or decrypt messages before calling AWS KMS again.
//  									   See more info at (https://docs.aws.amazon.com/kms/latest/developerguide/concepts.html#data-keys)
//   									   Valid Value is an integer representing seconds,
//  				 						    between 60 seconds (1 minute) and 86,400 seconds (24 hours).
//  									   		Default: 300 (5 minutes).
// 		  						   		   A shorter time period provides better security but results in more calls to KMS
//		  				 			   		    which might incur charges after Free Tier.
//		  						   		   For more information, see How Does the Data Key Reuse Period Work?
//		  						   		   		(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-server-side-encryption.html#sqs-how-does-the-data-key-reuse-period-work).
//
// 		The following attribute applies only to FIFO (first-in-first-out) queues
//		(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/FIFO-queues.html):
//   	12) ContentBasedDeduplication = Enables content-based deduplication.
//  									For more information, see Exactly-Once Processing
// 											(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/FIFO-queues.html#FIFO-queues-exactly-once-processing)
//											in the Amazon Simple Queue Service Developer Guide.
//										Every message must have a unique MessageDeduplicationId,
//											You may provide a MessageDeduplicationId explicitly.
//										If you aren't able to provide a MessageDeduplicationId and you enable ContentBasedDeduplication
//											for your queue, Amazon SQS uses a SHA-256 hash to generate the MessageDeduplicationId
//											using the body of the message (but not the attributes of the message).
//										If you don't provide a MessageDeduplicationId and the queue doesn't have ContentBasedDeduplication set,
//											the action fails with an error.
//										If the queue has ContentBasedDeduplication set, your MessageDeduplicationId overrides the generated one.
//										When ContentBasedDeduplication is in effect,
//											messages with identical content sent within the deduplication interval
//											are treated as duplicates and only one copy of the message is delivered.
//										If you send one message with ContentBasedDeduplication enabled
//											and then another message with a MessageDeduplicationId that is the same as the one generated
//											for the first MessageDeduplicationId, the two messages are treated as duplicates
//											and only one copy of the message is delivered.
func (s *SQS) SetQueueAttributes(queueUrl string,
								 attributes map[sqssetqueueattribute.SQSSetQueueAttribute]string,
								 timeOutDuration ...time.Duration) error {
	// validate
	if s.sqsClient == nil {
		return errors.New("SetQueueAttributes Failed: " + "SQS Client is Required")
	}

	if util.LenTrim(queueUrl) <= 0 {
		return errors.New("SetQueueAttributes Failed: " + "Queue Url is Required")
	}

	if attributes == nil {
		return errors.New("SetQueueAttributes Failed: " + "Attributes Map is Required")
	}

	// create input object
	input := &awssqs.SetQueueAttributesInput{
		QueueUrl: aws.String(queueUrl),
		Attributes: s.toAwsSetQueueAttributes(attributes),
	}

	// perform action
	var err error

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		_, err = s.sqsClient.SetQueueAttributesWithContext(ctx, input)
	} else {
		_, err = s.sqsClient.SetQueueAttributes(input)
	}

	// evaluate result
	if err != nil {
		return errors.New("SetQueueAttributes Failed: (Set Action) " + err.Error())
	} else {
		return nil
	}
}

// ----------------------------------------------------------------------------------------------------------------
// messaging related methods
// ----------------------------------------------------------------------------------------------------------------

// SendMessage will send a standard message to SQS standard queue via queue url
//
// Parameters:
//		1) queueUrl = required, the queue to send message to
//		2) messageBody = required, the message to send to the queue
//		3) messageAttributes = optional, map of message attribute key value pairs.
//							   Each message attribute consists of a Name, Type, and Value.
//							   For more information, see Amazon SQS Message Attributes:
//							   		(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-message-attributes.html)
//							   		in the Amazon Simple Queue Service Developer Guide.
//							   Example: use message attributes to define meta data about the message being sent:
//									a) {
//											"Title": { DataType: "String", StringValue: "The Whistler"},
//											"Author": { DataType: "String", StringValue: "John Grisham"},
//											"WeeksOn": { DataType: "Number", StringValue: "6"}
//									   }
// 		4) delaySeconds = optional, if greater than 0, indicates how many seconds to delay before message is available to consumers
//		5) timeOutDuration = optional, timeout value for context if any
//
// Return Values:
//		1) result = struct containing Send... action message result
//		2) err = error info if any
func (s *SQS) SendMessage(queueUrl string,
						  messageBody string,
						  messageAttributes map[string]*awssqs.MessageAttributeValue,
						  delaySeconds int64,
						  timeOutDuration ...time.Duration) (result *SQSMessageResult, err error) {
	// validate
	if s.sqsClient == nil {
		return nil, errors.New("SendMessage Failed: " + "SQS Client is Required")
	}

	if util.LenTrim(queueUrl) <= 0 {
		return nil, errors.New("SendMessage Failed: " + "Queue Url is Required")
	}

	if util.LenTrim(messageBody) <= 0 {
		return nil, errors.New("SendMessage Failed: " + "Message Body is Required")
	}

	// create input object
	input := &awssqs.SendMessageInput{
		QueueUrl: aws.String(queueUrl),
		MessageBody: aws.String(messageBody),
	}

	if messageAttributes != nil {
		input.MessageAttributes = messageAttributes
	}

	if delaySeconds > 0 {
		input.DelaySeconds = aws.Int64(delaySeconds)
	}

	// perform action
	var output *awssqs.SendMessageOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sqsClient.SendMessageWithContext(ctx, input)
	} else {
		output, err = s.sqsClient.SendMessage(input)
	}

	// evaluate result
	if err != nil {
		return nil, errors.New("SendMessage Failed: (Send Action) " + err.Error())
	} else {
		return &SQSMessageResult{
			MessageId: aws.StringValue(output.MessageId),
			MD5ofMessageBody: aws.StringValue(output.MD5OfMessageBody),
			MD5ofMessageAttributes: aws.StringValue(output.MD5OfMessageAttributes),
			FifoSequenceNumber: "",
		}, nil
	}
}

// SendMessageFifo will send a fifo message to SQS fifo queue via queue url
//
// Parameters:
//		1) queueUrl = required, the queue to send message to
// 		2) messageDeduplicationId = required, The token used for deduplication of sent messages.
//									If a message with a particular MessageDeduplicationId is sent successfully,
//										any messages sent with the same MessageDeduplicationId are accepted successfully
//										but aren't delivered during the 5-minute deduplication interval.
//									For more information, see Exactly-Once Processing
//										(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/FIFO-queues.html#FIFO-queues-exactly-once-processing)
//										in the Amazon Simple Queue Service Developer Guide.
//   								Every message must have a unique MessageDeduplicationId, You may provide a MessageDeduplicationId explicitly.
//  								If you aren't able to provide a MessageDeduplicationId,
// 										and you enable ContentBasedDeduplication for your queue,
//										Amazon SQS uses a SHA-256 hash to generate the MessageDeduplicationId,
//										using the body of the message (but not the attributes of the message).
//									If you don't provide a MessageDeduplicationId and the queue doesn't have ContentBasedDeduplication set,
//										the action fails with an error.
//									If the queue has ContentBasedDeduplication set, your MessageDeduplicationId overrides the generated one.
//   								When ContentBasedDeduplication is in effect,
//  									messages with identical content sent within the deduplication interval are treated as duplicates
//   									and only one copy of the message is delivered.
//  								If you send one message with ContentBasedDeduplication enabled，
// 										and then another message with a MessageDeduplicationId that is
//										the same as the one generated for the first MessageDeduplicationId,
//										the two messages are treated as duplicates and only one copy of the message is delivered.
//									The MessageDeduplicationId is available to the consumer of the message，
//										(this can be useful for troubleshooting delivery issues).
//									If a message is sent successfully but the acknowledgement is lost and the message is resent
//										with the same MessageDeduplicationId after the deduplication interval,
//										Amazon SQS can't detect duplicate messages.
//									Amazon SQS continues to keep track of the message deduplication ID，
//										even after the message is received and deleted.
//									The maximum length of MessageDeduplicationId is 128 characters.
//									MessageDeduplicationId can contain alphanumeric characters (a-z, A-Z, 0-9)
//										and punctuation (!"#$%&'()*+,-./:;<=>?@[\]^_`{|}~).
//									For best practices of using MessageDeduplicationId,
//										see Using the MessageDeduplicationId Property
//										(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/using-messagededuplicationid-property.html)
//										in the Amazon Simple Queue Service Developer Guide.
//		3) messageGroupId = required, The tag that specifies that a message belongs to a specific message group.
//							Messages that belong to the same message group are processed in a FIFO manner,
//								however, messages in different message groups might be processed out of order.
//							To interleave multiple ordered streams within a single queue,
//								use MessageGroupId values (for example, session data for multiple users).
//							In this scenario, multiple consumers can process the queue,
//								but the session data of each user is processed in a FIFO fashion.
//   						You must associate a non-empty MessageGroupId with a message,
//  							if you don't provide a MessageGroupId, the action fails.
//   						ReceiveMessage might return messages with multiple MessageGroupId values,
//   							For each MessageGroupId, the messages are sorted by time sent,
//		  						The caller can't specify a MessageGroupId.
//							The length of MessageGroupId is 128 characters,
//								Valid values: alphanumeric characters and punctuation (!"#$%&'()*+,-./:;<=>?@[\]^_`{|}~).
//							For best practices of using MessageGroupId,
//								see Using the MessageGroupId Property
//								(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/using-messagegroupid-property.html)
//								in the Amazon Simple Queue Service Developer Guide.
//							MessageGroupId is required for FIFO queues. You can't use it for Standard queues.
//		4) messageBody = required, the message to send to the queue
//		5) messageAttributes = optional, map of message attribute key value pairs.
//							   Each message attribute consists of a Name, Type, and Value.
//							   For more information, see Amazon SQS Message Attributes:
//							   		(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-message-attributes.html)
//							   		in the Amazon Simple Queue Service Developer Guide.
//							   Example: use message attributes to define meta data about the message being sent:
//									a) {
//											"Title": { DataType: "String", StringValue: "The Whistler"},
//											"Author": { DataType: "String", StringValue: "John Grisham"},
//											"WeeksOn": { DataType: "Number", StringValue: "6"}
//									   }
//		6) timeOutDuration = optional, timeout value for context if any
//
// Return Values:
//		1) result = struct containing Send... action message result
//		2) err = error info if any
func (s *SQS) SendMessageFifo(queueUrl string,
							  messageDeduplicationId string,
							  messageGroupId string,
							  messageBody string,
							  messageAttributes map[string]*awssqs.MessageAttributeValue,
							  timeOutDuration ...time.Duration) (result *SQSMessageResult, err error) {
	// validate
	if s.sqsClient == nil {
		return nil, errors.New("SendMessageFifo Failed: " + "SQS Client is Required")
	}

	if util.LenTrim(queueUrl) <= 0 {
		return nil, errors.New("SendMessageFifo Failed: " + "Queue Url is Required")
	}

	if util.LenTrim(messageDeduplicationId) <= 0 {
		return nil, errors.New("SendMessageFifo Failed: " + "Message Deduplication Id is Required")
	}

	if util.LenTrim(messageGroupId) <= 0 {
		return nil, errors.New("SendMessageFifo Failed: " + "Message Group Id is Required")
	}

	if util.LenTrim(messageBody) <= 0 {
		return nil, errors.New("SendMessageFifo Failed: " + "Message Body is Required")
	}

	// create input object
	input := &awssqs.SendMessageInput{
		QueueUrl: aws.String(queueUrl),
		MessageDeduplicationId: aws.String(messageDeduplicationId),
		MessageGroupId: aws.String(messageGroupId),
		MessageBody: aws.String(messageBody),
	}

	if messageAttributes != nil {
		input.MessageAttributes = messageAttributes
	}

	// perform action
	var output *awssqs.SendMessageOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sqsClient.SendMessageWithContext(ctx, input)
	} else {
		output, err = s.sqsClient.SendMessage(input)
	}

	// evaluate result
	if err != nil {
		return nil, errors.New("SendMessageFifo Failed: (Send Action) " + err.Error())
	} else {
		return &SQSMessageResult{
			MessageId: aws.StringValue(output.MessageId),
			MD5ofMessageBody: aws.StringValue(output.MD5OfMessageBody),
			MD5ofMessageAttributes: aws.StringValue(output.MD5OfMessageAttributes),
			FifoSequenceNumber: aws.StringValue(output.SequenceNumber),
		}, nil
	}
}

// SendMessageBatch will send up to 10 standard messages in a batch to SQS standard queue as indicated by queueUrl
func (s *SQS) SendMessageBatch(queueUrl string,
							   messageEntries []*SQSStandardMessage,
							   timeOutDuration ...time.Duration) (successList []*SQSSuccessResult, failedList []*SQSFailResult, err error) {
	// validate
	if s.sqsClient == nil {
		return nil, nil, errors.New("SendMessageBatch Failed: " + "SQS Client is Required")
	}

	if util.LenTrim(queueUrl) <= 0 {
		return nil, nil, errors.New("SendMessageBatch Failed: " + "Queue Url is Required")
	}

	if messageEntries == nil {
		return nil, nil, errors.New("SendMessageBatch Failed: " + "Message Entries Required (nil)")
	}

	if len(messageEntries) <= 0 {
		return nil, nil, errors.New("SendMessageBatch Failed: " + "Message Entries Required (count = 0)")
	}

	if len(messageEntries) > 10 {
		return nil, nil, errors.New("SendMessageBatch Failed: " + "Message Entries Per Batch Limited to 10")
	}

	// create input object
	var entries []*awssqs.SendMessageBatchRequestEntry

	for _, v := range messageEntries {
		if util.LenTrim(v.Id) > 0 && util.LenTrim(v.MessageBody) > 0 {
			entries = append(entries, &awssqs.SendMessageBatchRequestEntry{
				Id: aws.String(v.Id),
				MessageBody: aws.String(v.MessageBody),
				MessageAttributes: v.MessageAttributes,
				DelaySeconds: aws.Int64(v.DelaySeconds),
			})
		}
	}

	if len(entries) <= 0 {
		return nil, nil, errors.New("SendMessageBatch Failed: " + "Message Entries Elements Count Must Not Be Zero")
	}

	input := &awssqs.SendMessageBatchInput{
		QueueUrl: aws.String(queueUrl),
		Entries: entries,
	}

	// perform action
	var output *awssqs.SendMessageBatchOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sqsClient.SendMessageBatchWithContext(ctx, input)
	} else {
		output, err = s.sqsClient.SendMessageBatch(input)
	}

	// evaluate result
	if err != nil {
		return nil, nil, errors.New("SendMessageBatch Failed: (Send Action) " + err.Error())
	} else {
		if len(output.Successful) > 0 {
			for _, v := range output.Successful {
				successList = append(successList, &SQSSuccessResult{
					Id: aws.StringValue(v.Id),
					MessageId: aws.StringValue(v.MessageId),
					MD5ofMessageBody: aws.StringValue(v.MD5OfMessageBody),
					MD5ofMessageAttributes: aws.StringValue(v.MD5OfMessageAttributes),
				})
			}
		}

		if len(output.Failed) > 0 {
			for _, v := range output.Failed {
				failedList = append(failedList, &SQSFailResult{
					Id: aws.StringValue(v.Id),
					Code: aws.StringValue(v.Code),
					Message: aws.StringValue(v.Message),
					SenderFault: aws.BoolValue(v.SenderFault),
				})
			}
		}

		return successList, failedList, nil
	}
}

// SendMessageBatchFifo will send up to 10 fifo messages in a batch to SQS fifo queue as indicated by queueUrl
func (s *SQS) SendMessageBatchFifo(queueUrl string,
								   messageEntries []*SQSFifoMessage,
								   timeOutDuration ...time.Duration) (successList []*SQSSuccessResult, failedList []*SQSFailResult, err error) {
	// validate
	if s.sqsClient == nil {
		return nil, nil, errors.New("SendMessageBatchFifo Failed: " + "SQS Client is Required")
	}

	if util.LenTrim(queueUrl) <= 0 {
		return nil, nil, errors.New("SendMessageBatchFifo Failed: " + "Queue Url is Required")
	}

	if messageEntries == nil {
		return nil, nil, errors.New("SendMessageBatchFifo Failed: " + "Message Entries Required (nil)")
	}

	if len(messageEntries) <= 0 {
		return nil, nil, errors.New("SendMessageBatchFifo Failed: " + "Message Entries Required (count = 0)")
	}

	if len(messageEntries) > 10 {
		return nil, nil, errors.New("SendMessageBatchFifo Failed: " + "Message Entries Per Batch Limited to 10")
	}

	// create input object
	var entries []*awssqs.SendMessageBatchRequestEntry

	for _, v := range messageEntries {
		if util.LenTrim(v.Id) > 0 && util.LenTrim(v.MessageBody) > 0 && util.LenTrim(v.MessageGroupId) > 0 && util.LenTrim(v.MessageDeduplicationId) > 0 {
			entries = append(entries, &awssqs.SendMessageBatchRequestEntry{
				Id: aws.String(v.Id),
				MessageDeduplicationId: aws.String(v.MessageDeduplicationId),
				MessageGroupId: aws.String(v.MessageGroupId),
				MessageBody: aws.String(v.MessageBody),
				MessageAttributes: v.MessageAttributes,
			})
		}
	}

	if len(entries) <= 0 {
		return nil, nil, errors.New("SendMessageBatchFifo Failed: " + "Message Entries Elements Count Must Not Be Zero")
	}

	input := &awssqs.SendMessageBatchInput{
		QueueUrl: aws.String(queueUrl),
		Entries: entries,
	}

	// perform action
	var output *awssqs.SendMessageBatchOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sqsClient.SendMessageBatchWithContext(ctx, input)
	} else {
		output, err = s.sqsClient.SendMessageBatch(input)
	}

	// evaluate result
	if err != nil {
		return nil, nil, errors.New("SendMessageBatchFifo Failed: (Send Action) " + err.Error())
	} else {
		if len(output.Successful) > 0 {
			for _, v := range output.Successful {
				successList = append(successList, &SQSSuccessResult{
					Id: aws.StringValue(v.Id),
					MessageId: aws.StringValue(v.MessageId),
					MD5ofMessageBody: aws.StringValue(v.MD5OfMessageBody),
					MD5ofMessageAttributes: aws.StringValue(v.MD5OfMessageAttributes),
					SequenceNumber: aws.StringValue(v.SequenceNumber),
				})
			}
		}

		if len(output.Failed) > 0 {
			for _, v := range output.Failed {
				failedList = append(failedList, &SQSFailResult{
					Id: aws.StringValue(v.Id),
					Code: aws.StringValue(v.Code),
					Message: aws.StringValue(v.Message),
					SenderFault: aws.BoolValue(v.SenderFault),
				})
			}
		}

		return successList, failedList, nil
	}
}

// ReceiveMessage will retrieve 1 or more (up to 10) messages from the SQS (standard or fifo) queue indicated by queueUrl,
// note that this method retrieves the message(s), and blocks them from other consumer view for a short time, but does not delete such messages,
// to delete the messages, consumer must call the DeleteMessage methods accordingly
//
// Parameters:
//		1) queueUrl = required, retrieve message(s) from this queue as indicated by queueUrl, either standard or fifo queue, case-sensitive
//		2) maxNumberOfMessages = required, valid value is 1 - 10, indicates the maximum number of messages to retrieve during the method call
//		3) messageAttributeNames = optional, string slice of message attribute names to retrieve as related to the received messages.
//								   Attribute names are case-sensitive, alphanumeric, -, _, . are allowed.
//								   To receive all message attributes, use filter name as 'All' or '.*'.
//								   To receive all message attributes with a prefix pattern, use filter name as 'xyz.*',
//									   where attribute names prefix with xyz are included in the return list.
// 		4) systemAttributeNames = optional, string slice of system attribute names to retrieve as related to the received messages.
//								  Attribute names are case-sensitive, alphanumeric, -, _, . are allowed.
//								  Key-Value Pairs:
//										a) All = Return all system attributes and values.
//										b) ApproximateFirstReceiveTimestamp = Returns the time the message was first received
//										   									  from the queue in milliseconds.
//										   									  (epoch time (http://en.wikipedia.org/wiki/Unix_time)
//										c) ApproximateReceiveCount = Returns the number of times a message has been received
//																	 across all queues but not deleted.
//										d) AWSTraceHeader = Returns the AWS X-Ray trace header string.
//										e) SenderId = For an IAM user, returns the IAM user ID, for example ABCDEFGHI1JKLMNOPQ23R.
//													  For an IAM role, returns the IAM role ID, for example ABCDE1F2GH3I4JK5LMNOP:i-a123b456.
// 										f) SentTimestamp = Returns the time the message was sent to the queue,
//														   Epoch time = http://en.wikipedia.org/wiki/Unix_time in milliseconds.
// 										g) MessageDeduplicationId = Returns the value provided by the producer that calls the SendMessage action.
//										h) MessageGroupId = Returns the value provided by the producer that calls the SendMessage action.
//															Messages with the same MessageGroupId are returned in sequence.
// 										i) SequenceNumber = Returns the value provided by Amazon SQS.
//		5) visibilityTimeOutSeconds = optional, The duration (in seconds) that the received messages are hidden
//									  from subsequent retrieve requests after being retrieved by a ReceiveMessage request.
//		6) waitTimeSeconds = optional, The duration (in seconds) for which the call waits for a message to arrive in the queue before returning.
//							 If a message is available, the call returns sooner than WaitTimeSeconds.
//							 If no messages are available and the wait time expires,
//								 the call returns successfully with an empty list of messages.
//							 To avoid HTTP errors, ensure that the HTTP response timeout
//								 for ReceiveMessage requests is longer than the WaitTimeSeconds parameter.
//		7) receiveRequestAttemptId = optional, applies to FIFO queue ONLY. The token used for deduplication of ReceiveMessage calls.
//									 If a networking issue occurs after a ReceiveMessage action,
//										 and instead of a response you receive a generic error.
//									 It is possible to retry the same action with an identical ReceiveRequestAttemptId,
//									 	to retrieve the same set of messages, even if their visibility timeout has not yet expired.
//
// More About FIFO's receiveRequestAttemptId:
//		a) You can use ReceiveRequestAttemptId only for 5 minutes after a ReceiveMessage action.
//		b) When you set FifoQueue, a caller of the ReceiveMessage action can provide a ReceiveRequestAttemptId explicitly.
// 		c) If a caller of the ReceiveMessage action doesn't provide a ReceiveRequestAttemptId, Amazon SQS generates a ReceiveRequestAttemptId.
//		d) It is possible to retry the ReceiveMessage action with the same ReceiveRequestAttemptId,
//				if none of the messages have been modified (deleted or had their visibility changes).
//		e) During a visibility timeout, subsequent calls with the same ReceiveRequestAttemptId return the same messages and receipt handles.
//				If a retry occurs within the deduplication interval, it resets the visibility timeout.
//				For more information, see Visibility Timeout in the Amazon Simple Queue Service Developer Guide.
//					(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-visibility-timeout.html)
//				If a caller of the ReceiveMessage action still processes messages when the visibility timeout expires
//					and messages become visible, another worker consuming from the same queue can receive the same messages
//					and therefore process duplicates.
//				Also, if a consumer whose message processing time is longer than the visibility timeout tries to delete the processed messages,
//					the action fails with an error. To mitigate this effect, ensure that your application observes
//					a safe threshold before the visibility timeout expires and extend the visibility timeout as necessary.
//		f) While messages with a particular MessageGroupId are invisible,
//				no more messages belonging to the same MessageGroupId are returned until the visibility timeout expires.
//				You can still receive messages with another MessageGroupId as long as it is also visible.
//		g) If a caller of ReceiveMessage can't track the ReceiveRequestAttemptId, no retries work until the original visibility timeout expires.
//				As a result, delays might occur but the messages in the queue remain in a strict order.
//		h) The maximum length of ReceiveRequestAttemptId is 128 characters.
//				ReceiveRequestAttemptId can contain alphanumeric characters (a-z, A-Z, 0-9) and punctuation (!"#$%&'()*+,-./:;<=>?@[\]^_`{|}~).
//		i) For best practices of using ReceiveRequestAttemptId,
//				see Using the ReceiveRequestAttemptId Request Parameter in the Amazon Simple Queue Service Developer Guide.
//				(https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/using-receiverequestattemptid-request-parameter.html)
func (s *SQS) ReceiveMessage(queueUrl string,
							 maxNumberOfMessages int64,
							 messageAttributeNames []string,
							 systemAttributeNames []sqssystemattribute.SQSSystemAttribute,
							 visibilityTimeOutSeconds int64,
							 waitTimeSeconds int64,
							 receiveRequestAttemptId string,
							 timeOutDuration ...time.Duration) (messagesList []*SQSReceivedMessage, err error) {
	// validate
	if s.sqsClient == nil {
		return nil, errors.New("ReceiveMessage Failed: " + "SQS Client is Required")
	}

	if util.LenTrim(queueUrl) <= 0 {
		return nil, errors.New("ReceiveMessage Failed: " + "Queue Url is Required")
	}

	if maxNumberOfMessages <= 0 || maxNumberOfMessages > 10 {
		return nil, errors.New("ReceiveMessage Failed: " + "Max Number of Messages Must Be 1 to 10")
	}

	if visibilityTimeOutSeconds < 0 {
		visibilityTimeOutSeconds = 0
	}

	if waitTimeSeconds < 0 {
		waitTimeSeconds = 0
	}

	// create input object
	input := &awssqs.ReceiveMessageInput{
		QueueUrl: aws.String(queueUrl),
		MaxNumberOfMessages: aws.Int64(maxNumberOfMessages),
	}

	if len(messageAttributeNames) > 0 {
		input.MessageAttributeNames = aws.StringSlice(messageAttributeNames)
	}

	if len(systemAttributeNames) > 0 {
		var keys []*string

		for _, v := range systemAttributeNames {
			if v.Valid() && v != sqssystemattribute.UNKNOWN {
				keys = append(keys, aws.String(v.Key()))
			}
		}

		input.AttributeNames = keys
	}

	if visibilityTimeOutSeconds > 0 {
		input.VisibilityTimeout = aws.Int64(visibilityTimeOutSeconds)
	}

	if waitTimeSeconds > 0 {
		input.WaitTimeSeconds = aws.Int64(waitTimeSeconds)
	}

	if util.LenTrim(receiveRequestAttemptId) > 0 {
		input.ReceiveRequestAttemptId = aws.String(receiveRequestAttemptId)
	}

	// perform action
	var output *awssqs.ReceiveMessageOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sqsClient.ReceiveMessageWithContext(ctx, input)
	} else {
		output, err = s.sqsClient.ReceiveMessage(input)
	}

	// evaluate result
	if err != nil {
		return nil, errors.New("ReceiveMessage Failed: (Receive Action) " + err.Error())
	} else {
		for _, v := range output.Messages {
			if v != nil {
				messagesList = append(messagesList, &SQSReceivedMessage{
					MessageId: aws.StringValue(v.MessageId),
					Body: aws.StringValue(v.Body),
					MessageAttributes: v.MessageAttributes,
					SystemAttributes: s.fromAwsSystemAttributes(v.Attributes),
					MD5ofBody: aws.StringValue(v.MD5OfBody),
					MD5ofMessageAttributes: aws.StringValue(v.MD5OfMessageAttributes),
					ReceiptHandle: aws.StringValue(v.ReceiptHandle),
				})
			}
		}

		return messagesList, nil
	}
}

// ChangeMessageVisibility will update a message's visibility time out seconds,
// the message's receiptHandle is used along with queueUrl to identify which message in which queue is to be updated the new timeout value
func (s *SQS) ChangeMessageVisibility(queueUrl string,
									  receiptHandle string,
									  visibilityTimeOutSeconds int64,
									  timeOutDuration ...time.Duration) error {
	// validate
	if s.sqsClient == nil {
		return errors.New("ChangeMessageVisibility Failed: " + "SQS Client is Required")
	}

	if util.LenTrim(queueUrl) <= 0 {
		return errors.New("ChangeMessageVisibility Failed: " + "Queue Url is Required")
	}

	if util.LenTrim(receiptHandle) <= 0 {
		return errors.New("ChangeMessageVisibility Failed: " + "Receipt Handle is Required")
	}

	if visibilityTimeOutSeconds < 0 {
		visibilityTimeOutSeconds = 0
	}

	// create input object
	input := &awssqs.ChangeMessageVisibilityInput{
		QueueUrl: aws.String(queueUrl),
		ReceiptHandle: aws.String(receiptHandle),
	}

	if visibilityTimeOutSeconds > 0 {
		input.VisibilityTimeout = aws.Int64(visibilityTimeOutSeconds)
	}

	// perform action
	var err error

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		_, err = s.sqsClient.ChangeMessageVisibilityWithContext(ctx, input)
	} else {
		_, err = s.sqsClient.ChangeMessageVisibility(input)
	}

	// evaluate result
	if err != nil {
		return errors.New("ChangeMessageVisibility Failed: (Change Action) " + err.Error())
	} else {
		return nil
	}
}

// ChangeMessageVisibilityBatch will update up to 10 messages' visibility time out values in a batch
func (s *SQS) ChangeMessageVisibilityBatch(queueUrl string,
										   messageEntries []*SQSChangeVisibilityRequest,
										   timeOutDuration ...time.Duration) (successIDsList []string, failedList []*SQSFailResult, err error) {
	// validate
	if s.sqsClient == nil {
		return nil, nil, errors.New("ChangeMessageVisibilityBatch Failed: " + "SQS Client is Required")
	}

	if util.LenTrim(queueUrl) <= 0 {
		return nil, nil, errors.New("ChangeMessageVisibilityBatch Failed: " + "Queue Url is Required")
	}

	if messageEntries == nil {
		return nil, nil, errors.New("ChangeMessageVisibilityBatch Failed: " + "Message Entries Required (nil)")
	}

	if len(messageEntries) <= 0 {
		return nil, nil, errors.New("ChangeMessageVisibilityBatch Failed: " + "Message Entries Required (count = 0")
	}

	if len(messageEntries) > 10 {
		return nil, nil, errors.New("ChangeMessageVisibilityBatch Failed: " + "Message Entries Per Batch Limited to 10")
	}

	// create input object
	var entries []*awssqs.ChangeMessageVisibilityBatchRequestEntry

	for _, v := range messageEntries {
		if v != nil {
			if util.LenTrim(v.Id) > 0 && util.LenTrim(v.ReceiptHandle) > 0 {
				timeout := v.VisibilityTimeOutSeconds

				if timeout < 0 {
					timeout = 0
				}

				entries = append(entries, &awssqs.ChangeMessageVisibilityBatchRequestEntry{
					Id: aws.String(v.Id),
					ReceiptHandle: aws.String(v.ReceiptHandle),
					VisibilityTimeout: aws.Int64(timeout),
				})
			}
		}
	}

	if len(entries) <= 0 {
		return nil, nil, errors.New("ChangeMessageVisibilityBatch Failed: " + "Message Entries Elements Count Must Not Be Zero")
	}

	input := &awssqs.ChangeMessageVisibilityBatchInput{
		QueueUrl: aws.String(queueUrl),
		Entries: entries,
	}

	// perform action
	var output *awssqs.ChangeMessageVisibilityBatchOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sqsClient.ChangeMessageVisibilityBatchWithContext(ctx, input)
	} else {
		output, err = s.sqsClient.ChangeMessageVisibilityBatch(input)
	}

	// evaluate result
	if err != nil {
		return nil, nil, errors.New("ChangeMessageVisibilityBatch Failed: (Change Action) " + err.Error())
	} else {
		if len(output.Successful) > 0 {
			for _, v := range output.Successful {
				if v != nil {
					successIDsList = append(successIDsList, aws.StringValue(v.Id))
				}
			}
		}

		if len(output.Failed) > 0 {
			for _, v := range output.Failed {
				if v != nil {
					failedList = append(failedList, &SQSFailResult{
						Id: aws.StringValue(v.Id),
						Code: aws.StringValue(v.Code),
						Message: aws.StringValue(v.Message),
						SenderFault: aws.BoolValue(v.SenderFault),
					})
				}
			}
		}

		return successIDsList, failedList, nil
	}
}

// DeleteMessage will delete a message from sqs queue based on receiptHandle and queueUrl specified
func (s *SQS) DeleteMessage(queueUrl string,
							receiptHandle string,
							timeOutDuration ...time.Duration) error {
	// validate
	if s.sqsClient == nil {
		return errors.New("DeleteMessage Failed: " + "SQS Client is Required")
	}

	if util.LenTrim(queueUrl) <= 0 {
		return errors.New("DeleteMessage Failed: " + "Queue Url is Required")
	}

	if util.LenTrim(receiptHandle) <= 0 {
		return errors.New("DeleteMessage Failed: " + "Receipt Handle is Required")
	}

	// create input object
	input := &awssqs.DeleteMessageInput{
		QueueUrl: aws.String(queueUrl),
		ReceiptHandle: aws.String(receiptHandle),
	}

	// perform action
	var err error

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		_, err = s.sqsClient.DeleteMessageWithContext(ctx, input)
	} else {
		_, err = s.sqsClient.DeleteMessage(input)
	}

	// evaluate result
	if err != nil {
		return errors.New("DeleteMessage Failed: (Delete Action) " + err.Error())
	} else {
		return nil
	}
}

// DeleteMessageBatch will delete up to 10 messages from given queue by queueUrl and message receiptHandles
func (s *SQS) DeleteMessageBatch(queueUrl string,
								 messageEntries []*SQSDeleteMessageRequest,
								 timeOutDuration ...time.Duration) (successIDsList []string, failedList []*SQSFailResult, err error) {
	// validate
	if s.sqsClient == nil {
		return nil, nil, errors.New("DeleteMessageBatch Failed: " + "SQS Client is Required")
	}

	if util.LenTrim(queueUrl) <= 0 {
		return nil, nil, errors.New("DeleteMessageBatch Failed: " + "Queue Url is Required")
	}

	if messageEntries == nil {
		return nil, nil, errors.New("DeleteMessageBatch Failed: " + "Message Entries Required (nil)")
	}

	if len(messageEntries) <= 0 {
		return nil, nil, errors.New("DeleteMessageBatch Failed: " + "Message Entries Required (count = 0)")
	}

	if len(messageEntries) > 10 {
		return nil, nil, errors.New("DeleteMessageBatch Failed: " + "Message Entries Per Batch Limited to 10")
	}

	// create input object
	var entries []*awssqs.DeleteMessageBatchRequestEntry

	for _, v := range messageEntries {
		if v != nil {
			if util.LenTrim(v.Id) > 0 && util.LenTrim(v.ReceiptHandle) > 0 {
				entries = append(entries, &awssqs.DeleteMessageBatchRequestEntry{
					Id: aws.String(v.Id),
					ReceiptHandle: aws.String(v.ReceiptHandle),
				})
			}
		}
	}

	if len(entries) <= 0 {
		return nil, nil, errors.New("DeleteMessagseBatch Failed: " + "Message Entries Elements Must Not Be Zero")
	}

	input := &awssqs.DeleteMessageBatchInput{
		QueueUrl: aws.String(queueUrl),
		Entries: entries,
	}

	// perform action
	var output *awssqs.DeleteMessageBatchOutput

	if len(timeOutDuration) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), timeOutDuration[0])
		defer cancel()

		output, err = s.sqsClient.DeleteMessageBatchWithContext(ctx, input)
	} else {
		output, err = s.sqsClient.DeleteMessageBatch(input)
	}

	// evaluate result
	if err != nil {
		return nil, nil, errors.New("DeleteMessageBatch Failed: (Delete Action) " + err.Error())
	} else {
		if len(output.Successful) > 0 {
			for _, v := range output.Successful {
				if v != nil {
					successIDsList = append(successIDsList, aws.StringValue(v.Id))
				}
			}
		}

		if len(output.Failed) > 0 {
			for _, v := range output.Failed {
				if v != nil {
					failedList = append(failedList, &SQSFailResult{
						Id: aws.StringValue(v.Id),
						Code: aws.StringValue(v.Code),
						Message: aws.StringValue(v.Message),
						SenderFault: aws.BoolValue(v.SenderFault),
					})
				}
			}
		}

		return successIDsList, failedList, nil
	}
}
