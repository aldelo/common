package snsplatformapplicationattribute

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

// go:generate gen-enumer -type SNSPlatformApplicationAttribute

type SNSPlatformApplicationAttribute int

const (
	UNKNOWN                   SNSPlatformApplicationAttribute = 0
	PlatformCredential        SNSPlatformApplicationAttribute = 1
	PlatformPrincipal         SNSPlatformApplicationAttribute = 2
	EventEndpointCreated      SNSPlatformApplicationAttribute = 3
	EventEndpointDeleted      SNSPlatformApplicationAttribute = 4
	EventEndpointUpdated      SNSPlatformApplicationAttribute = 5
	EventDeliveryFailure      SNSPlatformApplicationAttribute = 6
	SuccessFeedbackRoleArn    SNSPlatformApplicationAttribute = 7
	FailureFeedbackRoleArn    SNSPlatformApplicationAttribute = 8
	SuccessFeedbackSampleRate SNSPlatformApplicationAttribute = 9
)

const (
	_SNSPlatformApplicationAttributeKey_0 = "UNKNOWN"
	_SNSPlatformApplicationAttributeKey_1 = "PlatformCredential"
	_SNSPlatformApplicationAttributeKey_2 = "PlatformPrincipal"
	_SNSPlatformApplicationAttributeKey_3 = "EventEndpointCreated"
	_SNSPlatformApplicationAttributeKey_4 = "EventEndpointDeleted"
	_SNSPlatformApplicationAttributeKey_5 = "EventEndpointUpdated"
	_SNSPlatformApplicationAttributeKey_6 = "EventDeliveryFailure"
	_SNSPlatformApplicationAttributeKey_7 = "SuccessFeedbackRoleArn"
	_SNSPlatformApplicationAttributeKey_8 = "FailureFeedbackRoleArn"
	_SNSPlatformApplicationAttributeKey_9 = "SuccessFeedbackSampleRate"
)

const (
	_SNSPlatformApplicationAttributeCaption_0 = "UNKNOWN"
	_SNSPlatformApplicationAttributeCaption_1 = "PlatformCredential"
	_SNSPlatformApplicationAttributeCaption_2 = "PlatformPrincipal"
	_SNSPlatformApplicationAttributeCaption_3 = "EventEndpointCreated"
	_SNSPlatformApplicationAttributeCaption_4 = "EventEndpointDeleted"
	_SNSPlatformApplicationAttributeCaption_5 = "EventEndpointUpdated"
	_SNSPlatformApplicationAttributeCaption_6 = "EventDeliveryFailure"
	_SNSPlatformApplicationAttributeCaption_7 = "SuccessFeedbackRoleArn"
	_SNSPlatformApplicationAttributeCaption_8 = "FailureFeedbackRoleArn"
	_SNSPlatformApplicationAttributeCaption_9 = "SuccessFeedbackSampleRate"
)

const (
	_SNSPlatformApplicationAttributeDescription_0 = "UNKNOWN"
	_SNSPlatformApplicationAttributeDescription_1 = "PlatformCredential"
	_SNSPlatformApplicationAttributeDescription_2 = "PlatformPrincipal"
	_SNSPlatformApplicationAttributeDescription_3 = "EventEndpointCreated"
	_SNSPlatformApplicationAttributeDescription_4 = "EventEndpointDeleted"
	_SNSPlatformApplicationAttributeDescription_5 = "EventEndpointUpdated"
	_SNSPlatformApplicationAttributeDescription_6 = "EventDeliveryFailure"
	_SNSPlatformApplicationAttributeDescription_7 = "SuccessFeedbackRoleArn"
	_SNSPlatformApplicationAttributeDescription_8 = "FailureFeedbackRoleArn"
	_SNSPlatformApplicationAttributeDescription_9 = "SuccessFeedbackSampleRate"
)
