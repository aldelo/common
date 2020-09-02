package ginhttpmethod

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

// go:generate gen-enumer -type GinHttpMethod

type GinHttpMethod int

const (
	UNKNOWN GinHttpMethod = 0
	GET     GinHttpMethod = 1
	POST    GinHttpMethod = 2
	PUT     GinHttpMethod = 3
	DELETE  GinHttpMethod = 4
)

const (
	_GinHttpMethodKey_0 = "UNKNOWN"
	_GinHttpMethodKey_1 = "GET"
	_GinHttpMethodKey_2 = "POST"
	_GinHttpMethodKey_3 = "PUT"
	_GinHttpMethodKey_4 = "DELETE"
)

const (
	_GinHttpMethodCaption_0 = "UNKNOWN"
	_GinHttpMethodCaption_1 = "GET"
	_GinHttpMethodCaption_2 = "POST"
	_GinHttpMethodCaption_3 = "PUT"
	_GinHttpMethodCaption_4 = "DELETE"
)

const (
	_GinHttpMethodDescription_0 = "UNKNOWN"
	_GinHttpMethodDescription_1 = "GET"
	_GinHttpMethodDescription_2 = "POST"
	_GinHttpMethodDescription_3 = "PUT"
	_GinHttpMethodDescription_4 = "DELETE"
)
