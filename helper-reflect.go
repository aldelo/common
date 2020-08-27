package helper

import (
	"reflect"
)

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
// Custom Struct Tag Reflect Helpers
// ================================================================================================================

// GetStructTagValueByObject will accept a struct object, struct field name, and struct tag name,
// and return the found tag value and reflect type,
// if reflect type or struct tag is not found, a notFound is returned
// [ Parameters ]
//		structObj = struct object variable
// 		structFieldName = struct's field name (CASE SENSITIVE)
//		structTagName = struct's tag name (the left side of struct tag - the key portion) (CASE SENSITIVE)
func GetStructTagValueByObject(structObj interface{}, structFieldName string, structTagName string) (notFound bool, tagValue string, t reflect.Type) {
	// get reflect type from struct object
	t = reflect.TypeOf(structObj)

	if t == nil {
		// no reflect type found
		return true, "", nil
	}

	// get field
	field, ok := t.FieldByName(structFieldName)

	if !ok {
		// struct field not found
		return true, "", t
	} else {
		// struct field found
		return false, field.Tag.Get(structTagName), t
	}
}

// GetStructTagValueByType will accept a prior obtained reflect type, struct field name, and struct tag name,
// and return the found tag value,
// if struct tag value is not found, a notFound is returned,
// if the reflect type is nil, then not found is returned too
// [ Parameters ]
//		t = reflect type of a struct object (obtained via GetStructTagValueByObject)
// 		structFieldName = struct's field name (CASE SENSITIVE)
//		structTagName = struct's tag name (the left side of struct tag - the key portion) (CASE SENSITIVE)
func GetStructTagValueByType(t reflect.Type, structFieldName string, structTagName string) (notFound bool, tagValue string) {
	// check if reflect type is valid
	if t == nil {
		return true, ""
	}

	// get field
	field, ok := t.FieldByName(structFieldName)

	if !ok {
		// struct field not found
		return true, ""
	} else {
		// struct field found
		return false, field.Tag.Get(structTagName)
	}
}
