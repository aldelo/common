package snsapplicationplatform

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

// go:generate gen-enumer -type SNSApplicationPlatform

type SNSApplicationPlatform int

const (
	UNKNOWN                           SNSApplicationPlatform = 0
	ADM_AmazonDeviceMessaging         SNSApplicationPlatform = 1
	APNS_ApplePushNotificationService SNSApplicationPlatform = 2
	APNS_Sandbox                      SNSApplicationPlatform = 3
	FCM_FirebaseCloudMessaging        SNSApplicationPlatform = 4
)

const (
	_SNSApplicationPlatformKey_0 = "UNKNOWN"
	_SNSApplicationPlatformKey_1 = "ADM"
	_SNSApplicationPlatformKey_2 = "APNS"
	_SNSApplicationPlatformKey_3 = "APNS_SANDBOX"
	_SNSApplicationPlatformKey_4 = "FCM"
)

const (
	_SNSApplicationPlatformCaption_0 = "UNKNOWN"
	_SNSApplicationPlatformCaption_1 = "ADM_AmazonDeviceMessaging"
	_SNSApplicationPlatformCaption_2 = "APNS_ApplePushNotificationService"
	_SNSApplicationPlatformCaption_3 = "APNS_Sandbox"
	_SNSApplicationPlatformCaption_4 = "FCM_FirebaseCloudMessaging"
)

const (
	_SNSApplicationPlatformDescription_0 = "UNKNOWN"
	_SNSApplicationPlatformDescription_1 = "ADM_AmazonDeviceMessaging"
	_SNSApplicationPlatformDescription_2 = "APNS_ApplePushNotificationService"
	_SNSApplicationPlatformDescription_3 = "APNS_Sandbox"
	_SNSApplicationPlatformDescription_4 = "FCM_FirebaseCloudMessaging"
)
