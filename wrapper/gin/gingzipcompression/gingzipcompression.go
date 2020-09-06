package gingzipcompression

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

// go:generate gen-enumer -type GinGZipCompression

type GinGZipCompression int

const (
	UNKNOWN         GinGZipCompression = 0
	Default         GinGZipCompression = 1
	BestSpeed       GinGZipCompression = 2
	BestCompression GinGZipCompression = 3
)

const (
	_GinGZipCompressionKey_0 = "UNKNOWN"
	_GinGZipCompressionKey_1 = "Default"
	_GinGZipCompressionKey_2 = "BestSpeed"
	_GinGZipCompressionKey_3 = "BestCompression"
)

const (
	_GinGZipCompressionCaption_0 = "UNKNOWN"
	_GinGZipCompressionCaption_1 = "Default"
	_GinGZipCompressionCaption_2 = "BestSpeed"
	_GinGZipCompressionCaption_3 = "BestCompression"
)

const (
	_GinGZipCompressionDescription_0 = "UNKNOWN"
	_GinGZipCompressionDescription_1 = "Default"
	_GinGZipCompressionDescription_2 = "BestSpeed"
	_GinGZipCompressionDescription_3 = "BestCompression"
)
