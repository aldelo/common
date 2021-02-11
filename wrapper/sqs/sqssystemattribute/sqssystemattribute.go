package sqssystemattribute

/*
 * Copyright 2020-2021 Aldelo, LP
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

// DO NOT EDIT - auto generated via tool

// go:generate gen-enumer -type SQSSystemAttribute

type SQSSystemAttribute int

const (
	UNKNOWN                          SQSSystemAttribute = 0
	All                              SQSSystemAttribute = 1
	ApproximateFirstReceiveTimestamp SQSSystemAttribute = 2
	ApproximateReceiveCount          SQSSystemAttribute = 3
	AWSTraceHeader                   SQSSystemAttribute = 4
	SenderId                         SQSSystemAttribute = 5
	SentTimestamp                    SQSSystemAttribute = 6
	MessageDeduplicationId           SQSSystemAttribute = 7
	MessageGroupId                   SQSSystemAttribute = 8
	SequenceNumber                   SQSSystemAttribute = 9
)

const (
	_SQSSystemAttributeKey_0 = "UNKNOWN"
	_SQSSystemAttributeKey_1 = "All"
	_SQSSystemAttributeKey_2 = "ApproximateFirstReceiveTimestamp"
	_SQSSystemAttributeKey_3 = "ApproximateReceiveCount"
	_SQSSystemAttributeKey_4 = "AWSTraceHeader"
	_SQSSystemAttributeKey_5 = "SenderId"
	_SQSSystemAttributeKey_6 = "SentTimestamp"
	_SQSSystemAttributeKey_7 = "MessageDeduplicationId"
	_SQSSystemAttributeKey_8 = "MessageGroupId"
	_SQSSystemAttributeKey_9 = "SequenceNumber"
)

const (
	_SQSSystemAttributeCaption_0 = "UNKNOWN"
	_SQSSystemAttributeCaption_1 = "All"
	_SQSSystemAttributeCaption_2 = "ApproximateFirstReceiveTimestamp"
	_SQSSystemAttributeCaption_3 = "ApproximateReceiveCount"
	_SQSSystemAttributeCaption_4 = "AWSTraceHeader"
	_SQSSystemAttributeCaption_5 = "SenderId"
	_SQSSystemAttributeCaption_6 = "SentTimestamp"
	_SQSSystemAttributeCaption_7 = "MessageDeduplicationId"
	_SQSSystemAttributeCaption_8 = "MessageGroupId"
	_SQSSystemAttributeCaption_9 = "SequenceNumber"
)

const (
	_SQSSystemAttributeDescription_0 = "UNKNOWN"
	_SQSSystemAttributeDescription_1 = "All"
	_SQSSystemAttributeDescription_2 = "ApproximateFirstReceiveTimestamp"
	_SQSSystemAttributeDescription_3 = "ApproximateReceiveCount"
	_SQSSystemAttributeDescription_4 = "AWSTraceHeader"
	_SQSSystemAttributeDescription_5 = "SenderId"
	_SQSSystemAttributeDescription_6 = "SentTimestamp"
	_SQSSystemAttributeDescription_7 = "MessageDeduplicationId"
	_SQSSystemAttributeDescription_8 = "MessageGroupId"
	_SQSSystemAttributeDescription_9 = "SequenceNumber"
)
