package snssubscribeattribute

/*
 * Copyright 2020-2023 Aldelo, LP
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

// go:generate gen-enumer -type SNSSubscribeAttribute

type SNSSubscribeAttribute int

const (
	UNKNOWN            SNSSubscribeAttribute = 0
	DeliveryPolicy     SNSSubscribeAttribute = 1
	FilterPolicy       SNSSubscribeAttribute = 2
	RawMessageDelivery SNSSubscribeAttribute = 3
	RedrivePolicy      SNSSubscribeAttribute = 4
)

const (
	_SNSSubscribeAttributeKey_0 = "UNKNOWN"
	_SNSSubscribeAttributeKey_1 = "DeliveryPolicy"
	_SNSSubscribeAttributeKey_2 = "FilterPolicy"
	_SNSSubscribeAttributeKey_3 = "RawMessageDelivery"
	_SNSSubscribeAttributeKey_4 = "RedrivePolicy"
)

const (
	_SNSSubscribeAttributeCaption_0 = "UNKNOWN"
	_SNSSubscribeAttributeCaption_1 = "DeliveryPolicy"
	_SNSSubscribeAttributeCaption_2 = "FilterPolicy"
	_SNSSubscribeAttributeCaption_3 = "RawMessageDelivery"
	_SNSSubscribeAttributeCaption_4 = "RedrivePolicy"
)

const (
	_SNSSubscribeAttributeDescription_0 = "UNKNOWN"
	_SNSSubscribeAttributeDescription_1 = "DeliveryPolicy"
	_SNSSubscribeAttributeDescription_2 = "FilterPolicy"
	_SNSSubscribeAttributeDescription_3 = "RawMessageDelivery"
	_SNSSubscribeAttributeDescription_4 = "RedrivePolicy"
)
