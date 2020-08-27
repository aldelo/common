package helper

import "time"

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

// ================================================================================================================
// Variadic Optional Value Helpers
// ================================================================================================================

// GetFirstOrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstOrDefault(defaultValue interface{}, paramValue ...interface{}) interface{} {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// GetFirstIntOrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstIntOrDefault(defaultValue int, paramValue ...int) int {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// GetFirstInt64OrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstInt64OrDefault(defaultValue int64, paramValue ...int64) int64 {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// GetFirstStringOrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstStringOrDefault(defaultValue string, paramValue ...string) string {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// GetFirstBoolOrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstBoolOrDefault(defaultValue bool, paramValue ...bool) bool {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// GetFirstFloat32OrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstFloat32OrDefault(defaultValue float32, paramValue ...float32) float32 {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// GetFirstFloat64OrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstFloat64OrDefault(defaultValue float64, paramValue ...float64) float64 {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// GetFirstTimeOrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstTimeOrDefault(defaultValue time.Time, paramValue ...time.Time) time.Time {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}

// GetFirstByteOrDefault will select the first variadic value from paramValue,
// if no paramValue variadic, then defaultValue is used as return value
func GetFirstByteOrDefault(defaultValue byte, paramValue ...byte) byte {
	if len(paramValue) > 0 {
		return paramValue[0]
	} else {
		// returning default
		return defaultValue
	}
}
