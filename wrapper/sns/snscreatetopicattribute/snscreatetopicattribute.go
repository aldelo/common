package snscreatetopicattribute

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

// go:generate gen-enumer -type SNSCreateTopicAttribute

type SNSCreateTopicAttribute int

const (
	UNKNOWN        SNSCreateTopicAttribute = 0
	DeliveryPolicy SNSCreateTopicAttribute = 1
	DisplayName    SNSCreateTopicAttribute = 2
	Policy         SNSCreateTopicAttribute = 3
	KmsMasterKeyId SNSCreateTopicAttribute = 4
)

const (
	_SNSCreateTopicAttributeKey_0 = "UNKNOWN"
	_SNSCreateTopicAttributeKey_1 = "DeliveryPolicy"
	_SNSCreateTopicAttributeKey_2 = "DisplayName"
	_SNSCreateTopicAttributeKey_3 = "Policy"
	_SNSCreateTopicAttributeKey_4 = "KmsMasterKeyId"
)

const (
	_SNSCreateTopicAttributeCaption_0 = "UNKNOWN"
	_SNSCreateTopicAttributeCaption_1 = "DeliveryPolicy"
	_SNSCreateTopicAttributeCaption_2 = "DisplayName"
	_SNSCreateTopicAttributeCaption_3 = "Policy"
	_SNSCreateTopicAttributeCaption_4 = "KmsMasterKeyId"
)

const (
	_SNSCreateTopicAttributeDescription_0 = "UNKNOWN"
	_SNSCreateTopicAttributeDescription_1 = "DeliveryPolicy"
	_SNSCreateTopicAttributeDescription_2 = "DisplayName"
	_SNSCreateTopicAttributeDescription_3 = "Policy"
	_SNSCreateTopicAttributeDescription_4 = "KmsMasterKeyId"
)
