package sdoperationfilter

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

// go:generate gen-enumer -type SdOperationFilter

type SdOperationFilter int

const (
	UNKNOWN            SdOperationFilter = 0
	EQ_NameSpaceID     SdOperationFilter = 1
	EQ_ServiceID       SdOperationFilter = 2
	EQ_Status          SdOperationFilter = 3
	EQ_Type            SdOperationFilter = 4
	IN_Status          SdOperationFilter = 5
	IN_Type            SdOperationFilter = 6
	BETWEEN_UpdateDate SdOperationFilter = 7
)

const (
	_SdOperationFilterKey_0 = "UNKNOWN"
	_SdOperationFilterKey_1 = "EQ_NameSpaceID"
	_SdOperationFilterKey_2 = "EQ_ServiceID"
	_SdOperationFilterKey_3 = "EQ_Status"
	_SdOperationFilterKey_4 = "EQ_Type"
	_SdOperationFilterKey_5 = "IN_Status"
	_SdOperationFilterKey_6 = "IN_Type"
	_SdOperationFilterKey_7 = "BETWEEN_UpdateDate"
)

const (
	_SdOperationFilterCaption_0 = "UNKNOWN"
	_SdOperationFilterCaption_1 = "EQ_NameSpaceID"
	_SdOperationFilterCaption_2 = "EQ_ServiceID"
	_SdOperationFilterCaption_3 = "EQ_Status"
	_SdOperationFilterCaption_4 = "EQ_Type"
	_SdOperationFilterCaption_5 = "IN_Status"
	_SdOperationFilterCaption_6 = "IN_Type"
	_SdOperationFilterCaption_7 = "BETWEEN_UpdateDate"
)

const (
	_SdOperationFilterDescription_0 = "UNKNOWN"
	_SdOperationFilterDescription_1 = "EQ_NameSpaceID"
	_SdOperationFilterDescription_2 = "EQ_ServiceID"
	_SdOperationFilterDescription_3 = "EQ_Status"
	_SdOperationFilterDescription_4 = "EQ_Type"
	_SdOperationFilterDescription_5 = "IN_Status"
	_SdOperationFilterDescription_6 = "IN_Type"
	_SdOperationFilterDescription_7 = "BETWEEN_UpdateDate"
)
