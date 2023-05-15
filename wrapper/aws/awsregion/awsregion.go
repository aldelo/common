package awsregion

import "strings"

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

// DO NOT EDIT - auto generated via tool

// go:generate gen-enumer -type AWSRegion

type AWSRegion int

const (
	UNKNOWN AWSRegion = 0

	AWS_us_west_2_oregon         AWSRegion = 1
	AWS_us_east_1_nvirginia      AWSRegion = 2
	AWS_eu_west_2_london         AWSRegion = 3
	AWS_eu_central_1_frankfurt   AWSRegion = 4
	AWS_ap_southeast_1_singapore AWSRegion = 5
	AWS_ap_east_1_hongkong       AWSRegion = 6
	AWS_ap_northeast_1_tokyo     AWSRegion = 7
	AWS_ap_southeast_2_sydney    AWSRegion = 8
)

// https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.RegionsAndAvailabilityZones.html
func GetAwsRegion(regionStr string) AWSRegion {
	switch strings.ToLower(regionStr) {
	case "us-west-2":
		return AWS_us_west_2_oregon
	case "us-east-1":
		return AWS_us_east_1_nvirginia
	case "eu-west-2":
		return AWS_eu_west_2_london
	case "eu-central-1":
		return AWS_eu_central_1_frankfurt
	case "ap-southeast-1":
		return AWS_ap_southeast_1_singapore
	case "ap-east-1":
		return AWS_ap_east_1_hongkong
	case "ap-northeast-1":
		return AWS_ap_northeast_1_tokyo
	case "ap-southeast-2":
		return AWS_ap_southeast_2_sydney
	default:
		return UNKNOWN
	}
}

const (
	_AWSRegionKey_0 = "UNKNOWN"
	_AWSRegionKey_1 = "us-west-2"
	_AWSRegionKey_2 = "us-east-1"
	_AWSRegionKey_3 = "eu-west-2"
	_AWSRegionKey_4 = "eu-central-1"
	_AWSRegionKey_5 = "ap-southeast-1"
	_AWSRegionKey_6 = "ap-east-1"
	_AWSRegionKey_7 = "ap-northeast-1"
	_AWSRegionKey_8 = "ap-southeast-2"
)

const (
	_AWSRegionCaption_0 = "UNKNOWN"
	_AWSRegionCaption_1 = "AWS_us_west_2_oregon"
	_AWSRegionCaption_2 = "AWS_us_east_1_nvirginia"
	_AWSRegionCaption_3 = "AWS_eu_west_2_london"
	_AWSRegionCaption_4 = "AWS_eu_central_1_frankfurt"
	_AWSRegionCaption_5 = "AWS_ap_southeast_1_singapore"
	_AWSRegionCaption_6 = "AWS_ap_east_1_hongkong"
	_AWSRegionCaption_7 = "AWS_ap_northeast_1_tokyo"
	_AWSRegionCaption_8 = "AWS_ap_southeast_2_sydney"
)

const (
	_AWSRegionDescription_0 = "UNKNOWN"
	_AWSRegionDescription_1 = "AWS_us_west_2_oregon"
	_AWSRegionDescription_2 = "AWS_us_east_1_nvirginia"
	_AWSRegionDescription_3 = "AWS_eu_west_2_london"
	_AWSRegionDescription_4 = "AWS_eu_central_1_frankfurt"
	_AWSRegionDescription_5 = "AWS_ap_southeast_1_singapore"
	_AWSRegionDescription_6 = "AWS_ap_east_1_hongkong"
	_AWSRegionDescription_7 = "AWS_ap_northeast_1_tokyo"
	_AWSRegionDescription_8 = "AWS_ap_southeast_2_sydney"
)
