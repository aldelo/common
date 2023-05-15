package mysql

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

import (
	"bytes"
	util "github.com/aldelo/common"
)

// QueryBuilder struct to help build sql queries (use named parameters instead of ordinal)
type QueryBuilder struct {
	buf           bytes.Buffer           // internal buffer holder
	output        string                 // internal output data based on buffer string()
	paramsNamed   map[string]interface{} // internal map of named parameters
	paramsOrdinal []interface{}          // internal slice of ordinal parameters
}

// ClearAll will reset the query builder internal fields to init status
func (q *QueryBuilder) ClearAll() {
	q.buf.Reset()
	q.output = ""
	q.paramsNamed = make(map[string]interface{})
	q.paramsOrdinal = make([]interface{}, 0)
}

// ClearSQL will clear the sql buffer and output only, leaving named map params intact
func (q *QueryBuilder) ClearSQL() {
	q.buf.Reset()
	q.output = ""
}

// ClearParams will clear the parameters map in reset state
func (q *QueryBuilder) ClearParams() {
	q.paramsNamed = make(map[string]interface{})
	q.paramsOrdinal = make([]interface{}, 0)
}

// Set will append a sqlPart to the query builder buffer
// [ notes ]
//  1. Ordinal Params
//     a) MySql, SQLite
//     Params = ?
//     b) SQLServer
//     Params = @p1, @p2, ...@pN
//  2. Named Params
//     a) MySql, SQLite, SQLServer
//     Params = :xyz1, :xyz2, ...:xyzN
func (q *QueryBuilder) Set(sqlPart string) {
	q.buf.WriteString(sqlPart)
}

// Named will add or update a named parameter and its value into named params map
func (q *QueryBuilder) Named(paramName string, paramValue interface{}) {
	if util.LenTrim(paramName) == 0 {
		return
	}

	if q.paramsNamed == nil {
		q.paramsNamed = make(map[string]interface{})
	}

	q.paramsNamed[paramName] = paramValue
}

// Ordinal will add an ordinal parameter value into ordinal params slice
func (q *QueryBuilder) Ordinal(ordinalParamValue interface{}) {
	q.paramsOrdinal = append(q.paramsOrdinal, ordinalParamValue)
}

// Build will create the output query string based on the bytes buffer appends
func (q *QueryBuilder) Build() {
	q.output = q.buf.String()
}

// SQL will return the built query string
func (q *QueryBuilder) SQL() string {
	if util.LenTrim(q.output) == 0 {
		q.output = q.buf.String()
	}

	return q.output
}

// ParamsMap returns the parameters map for use as input argument to the appropriate mysql query or exec actions
func (q *QueryBuilder) ParamsMap() map[string]interface{} {
	return q.paramsNamed
}

// ParamsSlice returns parameters slice for use as input argument to appropriate mysql, sqlite, sqlserver as its ordinal parameters
func (q *QueryBuilder) ParamsSlice() []interface{} {
	return q.paramsOrdinal
}
