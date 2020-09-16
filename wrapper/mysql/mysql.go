package mysql

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

import (
	"database/sql"
	"errors"
	"strings"

	util "github.com/aldelo/common"
	"github.com/jmoiron/sqlx"

	// this package is used by database/sql as we are wrapping the sql access functionality in this utility package
	_ "github.com/go-sql-driver/mysql"
)

// ++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
// MySql struct Usage Hint
// ++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++

/*
	***************************************************************************************************************
	First, Create "../model/global.go"
	***************************************************************************************************************

	package model

	import (
			"errors"
			"time"
			data "github.com/aldelo/common/wrapper/mysql"
	)

	// package level accessible to the mysql server database object
	var db *data.MySql

	// SetDB allows code outside of package to set the mysql database reference
	func SetDB(dbx *data.MySql) {
		db = dbx
	}

	// BeginTran starts db transaction
	func BeginTran() {
		if db != nil {
			db.Begin()
		}
	}

	// CommitTran commits db transaction
	func CommitTran() {
		if db != nil {
			db.Commit()
		}
	}

	// RollbackTran rolls back db transaction
	func RollbackTran() {
		if db != nil {
			db.Rollback()
		}
	}

*/

/*
	***************************************************************************************************************
	Second, Prepare DB Object for Use in "../main.go"
	***************************************************************************************************************

	package main

	import (
			...
			data "github.com/aldelo/common/wrapper/mysql"
			"???/model" // ??? is path to model package
			...
	)

	...

	func main() {
		...

		// ========================================
		// setup database connection
		// ========================================

		//
		// declare mysql server object
		//
		s := new(data.MySql)

		//
		// set mysql dsn fields
		//
		s.Host = "" 	// from aws aurora endpoint
		s.Port = 0 		// custom port number if applicable (0 will ignore this field)
		s.Database = ""	// database name
		s.UserName = ""	// database server user name
		s.Password = ""	// database server user password

		//
		// open mysql server database connection
		//
		if err := s.Open(); err != nil {
			s.Close()
		} else {
			// add mysql db object to model global
			model.SetDB(&s)

			// defer db clean up upon execution ends
			defer model.SetDB(nil)
			defer s.Close()
		}

		...
	}

*/

/*
	***************************************************************************************************************
	Third, Using MySql Struct
	***************************************************************************************************************

	package model

	import (
		"bytes"
		"database/sql"	// this import is needed for db struct tags
		"errors"
		"time"
		util "github.com/aldelo/common"
	)

	// create a struct, and use db struct tags to identify parameter names
	// db struct tags can contain ,required ,size=# if string
	type Customer struct {
		CustomerID		int		`db:"customerID"`
		CompanyName		string	`db:"companyName"`
	}

	// when composing sql statements, if statement is long, use bytes.Buffer (or use /QueryBuilder.go)
	var b bytes.Buffer

	b.WriteString("xyz ")
	b.WriteString("123")

	v := b.String()		// v = xyz 123

	// for insert, update, logical delete, physical delete
	// use the appropriate functions from db struct, located in model/global.go
	// the db struct is global in scope for code files within model package
	db.GetStruct(...)
	db.GetSliceStruct(...)
	// etc

*/

// ================================================================================================================
// STRUCTS
// ================================================================================================================

// MySql struct encapsulates the MySql server database access functionality by wrapping Sqlx package with top level methods
//    	Charset = utf8, utf8mb4 (< Default)
//		Collation = utf8mb4_general_ci (< Default), utf8_general_ci
//    	...Timeout = must be decimal number with unit suffix (ms, s, m, h), such as "30s", "0.5m", "1m30s"
type MySql struct {
	// MySql server connection properties
	UserName  string
	Password  string

	Host      string
	Port      int
	Database  string

	Charset string			// utf8, utf8mb4
	Collation string		// utf8mb4_general_ci, utf8_general_ci
	ConnectTimeout  string	// must be decimal number with unit suffix (ms, s, m, h), such as "30s", "0.5m", "1m30s"
	ReadTimeout string		// must be decimal number with unit suffix (ms, s, m, h), such as "30s", "0.5m", "1m30s"
	WriteTimeout string		// must be decimal number with unit suffix (ms, s, m, h), such as "30s", "0.5m", "1m30s"
	RejectReadOnly bool

	// mysql server state object
	db *sqlx.DB
	tx *sqlx.Tx
}

// MySqlResult defines sql action query result info
// [ Notes ]
//		1) NewlyInsertedID = ONLY FOR INSERT, ONLY IF AUTO_INCREMENT PRIMARY KEY (Custom PK ID Will Have This Field as 0 Always)
type MySqlResult struct {
	RowsAffected    int64
	NewlyInsertedID int64	// ONLY FOR INSERT, ONLY IF AUTO_INCREMENT PRIMARY KEY (Custom PK ID Will Have This Field as 0 Always)
	Err             error
}

// ================================================================================================================
// STRUCT FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// GetDsn serializes MySql server dsn to connection string, for use in database connectivity
func (svr *MySql) GetDsn() (string, error) {
	//
	// first validate input
	//
	if len(svr.UserName) == 0 {
		return "", errors.New("User Name is Required")
	}

	if len(svr.Password) == 0 {
		return "", errors.New("Password is Required")
	}

	if len(svr.Host) == 0 {
		return "", errors.New("MySQL Host Address is Required")
	}

	if len(svr.Database) == 0 {
		return "", errors.New("MySQL Database Name is Required")
	}

	//
	// now create mysql connection string
	// format = [username[:password]@][protocol[(address)]]/dbname[?param1=value1&...&paramN=valueN]
	//
	str := svr.UserName + ":" + svr.Password
	str += "@(" + svr.Host

	if svr.Port > 0 {
		str += ":" + util.Itoa(svr.Port)
	}

	str += ")/" + svr.Database

	if util.LenTrim(svr.Charset) > 0 {
		str += "?charset=" + svr.Charset
	} else {
		str += "?charset=utf8mb4"
	}

	if util.LenTrim(svr.Collation) > 0 {
		str += "&collation=" + svr.Collation
	} else {
		str += "&collation=utf8mb4_general_ci"
	}

	str += "&parseTime=true"

	if util.LenTrim(svr.ConnectTimeout) > 0 {
		str += "&timeout=" + svr.ConnectTimeout
	}

	if util.LenTrim(svr.ReadTimeout) > 0 {
		str += "&readTimeout=" + svr.ReadTimeout
	}

	if util.LenTrim(svr.WriteTimeout) > 0 {
		str += "&writeTimeout=" + svr.WriteTimeout
	}

	if svr.RejectReadOnly {
		str += "&rejectReadOnly=true"
	}

	// return to caller
	return str, nil
}

// Open a database by connecting to it, using the dsn properties defined in the struct fields
func (svr *MySql) Open() error {
	// declare
	var str string
	var err error

	// get connect string
	str, err = svr.GetDsn()

	if err != nil {
		svr.tx = nil
		svr.db = nil
		return err
	}

	// validate connection string
	if len(str) == 0 {
		svr.tx = nil
		svr.db = nil
		return errors.New("MySQL Server Connect String Generated Cannot Be Empty")
	}

	// now ready to open mysql database
	svr.db, err = sqlx.Open("mysql", str)

	if err != nil {
		svr.tx = nil
		svr.db = nil
		return err
	}

	// test mysql server state object
	if err = svr.db.Ping(); err != nil {
		svr.tx = nil
		svr.db = nil
		return err
	}

	// upon open, transaction object already nil
	svr.tx = nil

	// mysql server state object successfully opened
	return nil
}

// Close will close the database connection and set db to nil
func (svr *MySql) Close() error {
	if svr.db != nil {
		if err := svr.db.Close(); err != nil {
			return err
		}

		// clean up
		svr.tx = nil
		svr.db = nil
		return nil
	}

	return nil
}

// Ping tests if current database connection is still active and ready
func (svr *MySql) Ping() error {
	if svr.db == nil {
		return errors.New("MySQL Server Not Connected")
	}

	if err := svr.db.Ping(); err != nil {
		return err
	}

	// database ok
	return nil
}

// Begin starts a database transaction, and stores the transaction object until commit or rollback
func (svr *MySql) Begin() error {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return err
	}

	// does transaction already exist
	if svr.tx != nil {
		return errors.New("Transaction Already Started")
	}

	// begin transaction on database
	tx, err := svr.db.Beginx()

	if err != nil {
		return err
	}

	// transaction begin successful,
	// store tx into svr.tx field
	svr.tx = tx

	// return nil as success
	return nil
}

// Commit finalizes a database transaction, and commits changes to database
func (svr *MySql) Commit() error {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return err
	}

	// does transaction already exist
	if svr.tx == nil {
		return errors.New("Transaction Does Not Exist")
	}

	// perform tx commit
	if err := svr.tx.Commit(); err != nil {
		return err
	}

	// commit successful
	svr.tx = nil
	return nil
}

// Rollback cancels pending database changes for the current transaction and clears out transaction object
func (svr *MySql) Rollback() error {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return err
	}

	// does transaction already exist
	if svr.tx == nil {
		return errors.New("Transaction Does Not Exist")
	}

	// perform tx commit
	if err := svr.tx.Rollback(); err != nil {
		svr.tx = nil
		return err
	}

	// commit successful
	svr.tx = nil
	return nil
}

// ----------------------------------------------------------------------------------------------------------------
// query and marshal to 'struct slice' or 'struct' helpers
// ----------------------------------------------------------------------------------------------------------------

// GetStructSlice performs query with optional variadic parameters, and unmarshal result rows into target struct slice,
// in essence, each row of data is marshaled into the given struct, and multiple struct form the slice,
// such as: []Customer where each row represent a customer, and multiple customers being part of the slice
// [ Parameters ]
//		dest = pointer to the struct slice or address of struct slice, this is the result of rows to be marshaled into struct slice
//		query = sql query, optionally having parameters marked as ?, where each represents a parameter position
// 		args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
// [ Return Values ]
//		1) notFound = indicates no rows found in query (aka sql.ErrNoRows), if error is detected, notFound is always false
//		2) if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is nil)
// [ Notes ]
//		1) if error == nil, and len(dest struct slice) == 0 then zero struct slice result
func (svr *MySql) GetStructSlice(dest interface{}, query string, args ...interface{}) (notFound bool, retErr error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return false, err
	}

	// perform select action, and unmarshal result rows into target struct slice
	var err error

	if svr.tx == nil {
		// not in transaction mode
		// query using db object
		err = svr.db.Select(dest, query, args...)
	} else {
		// in transaction mode
		// query using tx object
		err = svr.tx.Select(dest, query, args...)
	}

	// if err is sql.ErrNoRows then treat as no error
	if err != nil && err == sql.ErrNoRows {
		notFound = true
		dest = nil
		err = nil
	} else {
		notFound = false
	}

	// return error
	return notFound, err
}

// GetStruct performs query with optional variadic parameters, and unmarshal single result row into single target struct,
// such as: Customer struct where one row of data represent a customer
// [ Parameters ]
//		dest = pointer to struct or address of struct, this is the result of row to be marshaled into this struct
//		query = sql query, optionally having parameters marked as ?, where each represents a parameter position
//		args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
// [ Return Values ]
//		1) notFound = indicates no rows found in query (aka sql.ErrNoRows), if error is detected, notFound is always false
//		2) if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is nil)
func (svr *MySql) GetStruct(dest interface{}, query string, args ...interface{}) (notFound bool, retErr error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return false, err
	}

	// perform select action, and unmarshal result row (single row) into target struct (single object)
	var err error

	if svr.tx == nil {
		// not in transaction mode
		// query using db object
		err = svr.db.Get(dest, query, args...)
	} else {
		// in transaction mode
		// query using tx object
		err = svr.tx.Get(dest, query, args...)
	}

	// if err is sql.ErrNoRows then treat as no error
	if err != nil && err == sql.ErrNoRows {
		notFound = true
		dest = nil
		err = nil
	} else {
		notFound = false
	}

	// return error
	return notFound, err
}

// ----------------------------------------------------------------------------------------------------------------
// query and get rows helpers
// ----------------------------------------------------------------------------------------------------------------

// GetRowsByOrdinalParams performs query with optional variadic parameters to get ROWS of result, and returns *sqlx.Rows
// [ Parameters ]
//		query = sql query, optionally having parameters marked as ?, where each represents a parameter position
//		args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
// [ Return Values ]
//		1) *sqlx.Rows = pointer to sqlx.Rows; or nil if no rows yielded
// 		2) if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and sqlx.Rows is returned as nil)
// [ Ranged Loop & Scan ]
//		1) to loop, use: for _, r := range rows
//		2) to scan, use: r.Scan(&x, &y, ...), where r is the row struct in loop, where &x &y etc are the scanned output value (scan in order of select columns sequence)
// [ Continuous Loop & Scan ]
//		1) Continuous loop until endOfRows = true is yielded from ScanSlice() or ScanStruct()
//		2) ScanSlice(): accepts *sqlx.Rows, scans rows result into target pointer slice (if no error, endOfRows = true is returned)
//		3) ScanStruct(): accepts *sqlx.Rows, scans current single row result into target pointer struct, returns endOfRows as true of false; if endOfRows = true, loop should stop
func (svr *MySql) GetRowsByOrdinalParams(query string, args ...interface{}) (*sqlx.Rows, error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return nil, err
	}

	// perform select action, and return sqlx rows
	var rows *sqlx.Rows
	var err error

	if svr.tx == nil {
		// not in transaction mode
		// query using db object
		rows, err = svr.db.Queryx(query, args...)
	} else {
		// in transaction mode
		// query using tx object
		rows, err = svr.tx.Queryx(query, args...)
	}

	// if err is sql.ErrNoRows then treat as no error
	if err != nil && err == sql.ErrNoRows {
		rows = nil
		err = nil
	}

	// return result
	return rows, err
}

// GetRowsByNamedMapParam performs query with named map containing parameters to get ROWS of result, and returns *sqlx.Rows
// [ Syntax ]
//		1) in sql = instead of defining ordinal parameters ?, each parameter in sql does not need to be ordinal, rather define with :xyz (must have : in front of param name), where xyz is name of parameter, such as :customerID
//		2) in go = setup a map variable: var p = make(map[string]interface{})
//		3) in go = to set values into map variable: p["xyz"] = abc
//				   where xyz is the parameter name matching the sql :xyz (do not include : in go map "xyz")
//				   where abc is the value of the parameter value, whether string or other data types
//				   note: in using map, just add additional map elements using the p["xyz"] = abc syntax
//				   note: if parameter value can be a null, such as nullint, nullstring, use util.ToNullTime(), ToNullInt(), ToNullString(), etc.
//		4) in go = when calling this function passing the map variable, simply pass the map variable p into the args parameter
// [ Parameters ]
//		query = sql query, optionally having parameters marked as :xyz for each parameter name, where each represents a named parameter
//		args = required, the map variable of the named parameters
// [ Return Values ]
//		1) *sqlx.Rows = pointer to sqlx.Rows; or nil if no rows yielded
// 		2) if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and sqlx.Rows is returned as nil)
// [ Ranged Loop & Scan ]
//		1) to loop, use: for _, r := range rows
//		2) to scan, use: r.Scan(&x, &y, ...), where r is the row struct in loop, where &x &y etc are the scanned output value (scan in order of select columns sequence)
// [ Continuous Loop & Scan ]
//		1) Continuous loop until endOfRows = true is yielded from ScanSlice() or ScanStruct()
//		2) ScanSlice(): accepts *sqlx.Rows, scans rows result into target pointer slice (if no error, endOfRows = true is returned)
//		3) ScanStruct(): accepts *sqlx.Rows, scans current single row result into target pointer struct, returns endOfRows as true of false; if endOfRows = true, loop should stop
func (svr *MySql) GetRowsByNamedMapParam(query string, args map[string]interface{}) (*sqlx.Rows, error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return nil, err
	}

	// perform select action, and return sqlx rows
	var rows *sqlx.Rows
	var err error

	if svr.tx == nil {
		// not in transaction mode
		// query using db object
		rows, err = svr.db.NamedQuery(query, args)
	} else {
		// in transaction mode
		// query using tx object
		rows, err = svr.tx.NamedQuery(query, args)
	}

	if err != nil && err == sql.ErrNoRows {
		// no rows
		rows = nil
		err = nil
	}

	// return result
	return rows, err
}

// GetRowsByStructParam performs query with a struct as parameter input to get ROWS of result, and returns *sqlx.Rows
// [ Syntax ]
//		1) in sql = instead of defining ordinal parameters ?, each parameter in sql does not need to be ordinal, rather define with :xyz (must have : in front of param name), where xyz is name of parameter, such as :customerID
//		2) in sql = important: the :xyz defined where xyz portion of parameter name must batch the struct tag's `db:"xyz"`
//		3) in go = a struct containing struct tags that matches the named parameters will be set with values, and passed into this function's args parameter input
//		4) in go = when calling this function passing the struct variable, simply pass the struct variable into the args parameter
// [ Parameters ]
//		query = sql query, optionally having parameters marked as :xyz for each parameter name, where each represents a named parameter
//		args = required, the struct variable where struct fields' struct tags match to the named parameters
// [ Return Values ]
//		1) *sqlx.Rows = pointer to sqlx.Rows; or nil if no rows yielded
// 		2) if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and sqlx.Rows is returned as nil)
// [ Ranged Loop & Scan ]
//		1) to loop, use: for _, r := range rows
//		2) to scan, use: r.Scan(&x, &y, ...), where r is the row struct in loop, where &x &y etc are the scanned output value (scan in order of select columns sequence)
// [ Continuous Loop & Scan ]
//		1) Continuous loop until endOfRows = true is yielded from ScanSlice() or ScanStruct()
//		2) ScanSlice(): accepts *sqlx.Rows, scans rows result into target pointer slice (if no error, endOfRows = true is returned)
//		3) ScanStruct(): accepts *sqlx.Rows, scans current single row result into target pointer struct, returns endOfRows as true of false; if endOfRows = true, loop should stop
func (svr *MySql) GetRowsByStructParam(query string, args interface{}) (*sqlx.Rows, error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return nil, err
	}

	// perform select action, and return sqlx rows
	var rows *sqlx.Rows
	var err error

	if svr.tx == nil {
		// not in transaction mode
		// query using db object
		rows, err = svr.db.NamedQuery(query, args)
	} else {
		// in transaction mode
		// query using tx object
		rows, err = svr.tx.NamedQuery(query, args)
	}

	if err != nil && err == sql.ErrNoRows {
		// no rows
		rows = nil
		err = nil
	}

	// return result
	return rows, err
}

// ----------------------------------------------------------------------------------------------------------------
// scan row data and marshal to 'slice' or 'struct' helpers
// ----------------------------------------------------------------------------------------------------------------

// ScanSlice takes in *sqlx.Rows as parameter, will invoke the rows.Next() to advance to next row position,
// and marshals current row's column values into a pointer reference to a slice,
// this enables us to quickly retrieve a slice of current row column values without knowing how many columns or names or columns (columns appear in select columns sequence),
// to loop thru all rows, use range, and loop until endOfRows = true; the dest is nil if no columns found; the dest is pointer of slice when columns exists
// [ Parameters ]
//		rows = *sqlx.Rows
//		dest = pointer or address to slice, such as: variable to "*[]string", or variable to "&cList for declaration cList []string"
// [ Return Values ]
//		1) endOfRows = true if this action call yielded end of rows, meaning stop further processing of current loop
//		2) if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is set as nil)
func (svr *MySql) ScanSlice(rows *sqlx.Rows, dest []interface{}) (endOfRows bool, err error) {
	// ensure rows pointer is set
	if rows == nil {
		return true, nil
	}

	// call rows.Next() first to position the row
	if rows.Next() {
		// now slice scan
		dest, err = rows.SliceScan()

		// if err is sql.ErrNoRows then treat as no error
		if err != nil && err == sql.ErrNoRows {
			endOfRows = true
			dest = nil
			err = nil
			return
		}

		if err != nil {
			// has error
			endOfRows = false	// although error but may not be at end of rows
			dest = nil
			return
		}

		// slice scan success, but may not be at end of rows
		return false, nil
	}

	// no more rows
	return true, nil
}

// ScanStruct takes in *sqlx.Rows, will invoke the rows.Next() to advance to next row position,
// and marshals current row's column values into a pointer reference to a struct,
// the struct fields and row columns must match for both name and sequence position,
// this enables us to quickly convert the row's columns into a defined struct automatically,
// to loop thru all rows, use range, and loop until endOfRows = true; the dest is nil if no columns found; the dest is pointer of struct when mapping is complete
// [ Parameters ]
//		rows = *sqlx.Rows
//		dest = pointer or address to struct, such as: variable to "*Customer", or variable to "&c for declaration c Customer"
// [ Return Values ]
//		1) endOfRows = true if this action call yielded end of rows, meaning stop further processing of current loop
//		2) if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is set as nil)
func (svr *MySql) ScanStruct(rows *sqlx.Rows, dest interface{}) (endOfRows bool, err error) {
	// ensure rows pointer is set
	if rows == nil {
		return true, nil
	}

	// call rows.Next() first to position the row
	//if rows.Next() {
	//	// now struct scan
	//	err = rows.StructScan(dest)
	//
	//	// if err is sql.ErrNoRows then treat as no error
	//	if err != nil && err == sql.ErrNoRows {
	//		endOfRows = true
	//		dest = nil
	//		err = nil
	//		return
	//	}
	//
	//	if err != nil {
	//		// has error
	//		endOfRows = false	// although error but may not be at end of rows
	//		dest = nil
	//		return
	//	}
	//
	//	// struct scan successful, but may not be at end of rows
	//	return false, nil
	//}

	err = sqlx.StructScan(rows, dest)
	if err != nil {
		return false, err
	}

	// no more rows
	return true, nil
}

// ----------------------------------------------------------------------------------------------------------------
// query for single row helper
// ----------------------------------------------------------------------------------------------------------------

// GetSingleRow performs query with optional variadic parameters to get a single ROW of result, and returns *sqlx.Row (This function returns SINGLE ROW)
// [ Parameters ]
//		query = sql query, optionally having parameters marked as ?, where each represents a parameter position
//		args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
// [ Return Values ]
//		1) *sqlx.Row = pointer to sqlx.Row; or nil if no row yielded
//		2) if error != nil, then error is encountered (if error = sql.ErrNoRows, then error is treated as nil, and sqlx.Row is returned as nil)
// [ Scan Values ]
//		1) Use row.Scan() and pass in pointer or address of variable to receive scanned value outputs (Scan is in the order of column sequences in select statement)
// [ WARNING !!! ]
//		WHEN USING Scan(), MUST CHECK Scan Result Error for sql.ErrNoRow status
//		SUGGESTED TO USE ScanColumnsByRow() Instead of Scan()
func (svr *MySql) GetSingleRow(query string, args ...interface{}) (*sqlx.Row, error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return nil, err
	}

	// perform select action, and return sqlx row
	var row *sqlx.Row
	var err error

	if svr.tx == nil {
		// not in transaction mode
		// query using db object
		row = svr.db.QueryRowx(query, args...)
	} else {
		// in transaction mode
		// query using tx object
		row = svr.tx.QueryRowx(query, args...)
	}

	if row == nil {
		err = errors.New("No Row Data Found From Query")
	} else {
		err = row.Err()

		if err != nil {
			if err == sql.ErrNoRows {
				// no rows
				row = nil
				err = nil
			} else {
				// has error
				row = nil
			}
		}
	}

	// return result
	return row, err
}

// ----------------------------------------------------------------------------------------------------------------
// scan single row data and marshal to 'slice' or 'struct' or specific fields, or scan columns helpers
// ----------------------------------------------------------------------------------------------------------------

// ScanSliceByRow takes in *sqlx.Row as parameter, and marshals current row's column values into a pointer reference to a slice,
// this enables us to quickly retrieve a slice of current row column values without knowing how many columns or names or columns (columns appear in select columns sequence)
// [ Parameters ]
//		row = *sqlx.Row
//		dest = pointer or address to slice, such as: variable to "*[]string", or variable to "&cList for declaration cList []string"
// [ Return Values ]
//		1) notFound = true if no row is found in current scan
//		2) if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is set as nil and notFound is true)
func (svr *MySql) ScanSliceByRow(row *sqlx.Row, dest []interface{}) (notFound bool, err error) {
	// if row is nil, treat as no row and not an error
	if row == nil {
		dest = nil
		return true, nil
	}

	// perform slice scan on the given row
	dest, err = row.SliceScan()

	// if err is sql.ErrNoRows then treat as no error
	if err != nil && err == sql.ErrNoRows {
		dest = nil
		return true, nil
	}

	if err != nil {
		// has error
		dest = nil
		return false, err	// although error but may not be not found
	}

	// slice scan success
	return false, nil
}

// ScanStructByRow takes in *sqlx.Row, and marshals current row's column values into a pointer reference to a struct,
// the struct fields and row columns must match for both name and sequence position,
// this enables us to quickly convert the row's columns into a defined struct automatically,
// the dest is nil if no columns found; the dest is pointer of struct when mapping is complete
// [ Parameters ]
//		row = *sqlx.Row
//		dest = pointer or address to struct, such as: variable to "*Customer", or variable to "&c for declaration c Customer"
// [ Return Values ]
//		1) notFound = true if no row is found in current scan
//		2) if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is set as nil and notFound is true)
func (svr *MySql) ScanStructByRow(row *sqlx.Row, dest interface{}) (notFound bool, err error) {
	// if row is nil, treat as no row and not an error
	if row == nil {
		dest = nil
		return true, nil
	}

	// now struct scan
	err = row.StructScan(dest)

	// if err is sql.ErrNoRows then treat as no error
	if err != nil && err == sql.ErrNoRows {
		dest = nil
		return true, nil
	}

	if err != nil {
		// has error
		dest = nil
		return false, err	// although error but may not be not found
	}

	// struct scan successful
	return false, nil
}

// ScanColumnsByRow accepts a *sqlx row, and scans specific columns into dest outputs,
// this is different than ScanSliceByRow or ScanStructByRow because this function allows specific extraction of column values into target fields，
// (note: this function must extra all row column values to dest variadic parameters as present in the row parameter)
// [ Parameters ]
//		row = *sqlx.Row representing the row containing columns to extract, note that this function MUST extract all columns from this row
//		dest = MUST BE pointer (or &variable) to target variable to receive the column value, data type must match column data type value, and sequence of dest must be in the order of columns sequence
// [ Return Values ]
//		1) notFound = true if no row is found in current scan
// 		2) if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is set as nil and notFound is true)
// [ Example ]
//		1) assuming: Select CustomerID, CustomerName, Address FROM Customer Where CustomerPhone='123';
//		2) assuming: row // *sqlx.Row derived from GetSingleRow() or specific row from GetRowsByOrdinalParams() / GetRowsByNamedMapParam() / GetRowsByStructParam()
//		3) assuming: var CustomerID int64
//					 var CustomerName string
//					 var Address string
//		4) notFound, err := svr.ScanColumnsByRow(row, &CustomerID, &CustomerName, &Address)
func (svr *MySql) ScanColumnsByRow(row *sqlx.Row, dest ...interface{}) (notFound bool, err error) {
	// if row is nil, treat as no row and not an error
	if row == nil {
		return true, nil
	}

	// now scan columns from row
	err = row.Scan(dest...)

	// if err is sql.ErrNoRows then treat as no error
	if err != nil && err == sql.ErrNoRows {
		return true, nil
	}

	if err != nil {
		// has error
		return false, err	// although error but may not be not found
	}

	// scan columns successful
	return false, nil
}

// ----------------------------------------------------------------------------------------------------------------
// query for single value in single row helpers
// ----------------------------------------------------------------------------------------------------------------

// GetScalarString performs query with optional variadic parameters, and returns the first row and first column value in string data type
// [ Parameters ]
//		query = sql query, optionally having parameters marked as ?, where each represents a parameter position
//		args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
// [ Return Values ]
//		1) retVal = string value of scalar result, if no value, blank is returned
//		2) retNotFound = now row found
// 		3) if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and retVal is returned as blank)
func (svr *MySql) GetScalarString(query string, args ...interface{}) (retVal string, retNotFound bool, retErr error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return "", false, err
	}

	// get row using query string and parameters
	var row *sqlx.Row

	if svr.tx == nil {
		// not in transaction
		// use db object
		row = svr.db.QueryRowx(query, args...)
	} else {
		// in transaction
		// use tx object
		row = svr.tx.QueryRowx(query, args...)
	}

	if row == nil {
		return "", false, errors.New("Scalar Query Yielded Empty Row")
	} else {
		retErr = row.Err()

		if retErr != nil {
			if retErr == sql.ErrNoRows {
				// no rows
				return "", true, nil
			} else {
				// has error
				return "", false, retErr
			}
		}
	}

	// get value via scan
	retErr = row.Scan(&retVal)

	if retErr == sql.ErrNoRows {
		// no rows
		return "", true, nil
	} else {
		// return value
		return retVal, false, retErr
	}
}

// GetScalarNullString performs query with optional variadic parameters, and returns the first row and first column value in sql.NullString{} data type
// [ Parameters ]
//		query = sql query, optionally having parameters marked as ?, where each represents a parameter position
//		args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
// [ Return Values ]
//		1) retVal = string value of scalar result, if no value, sql.NullString{} is returned
//		2) retNotFound = now row found
// 		3) if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and retVal is returned as sql.NullString{})
func (svr *MySql) GetScalarNullString(query string, args ...interface{}) (retVal sql.NullString, retNotFound bool, retErr error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return sql.NullString{}, false, err
	}

	// get row using query string and parameters
	var row *sqlx.Row

	if svr.tx == nil {
		// not in transaction
		// use db object
		row = svr.db.QueryRowx(query, args...)
	} else {
		// in transaction
		// use tx object
		row = svr.tx.QueryRowx(query, args...)
	}

	if row == nil {
		return sql.NullString{}, false, errors.New("Scalar Query Yielded Empty Row")
	} else {
		retErr = row.Err()

		if retErr != nil {
			if retErr == sql.ErrNoRows {
				// no rows
				return sql.NullString{}, true, nil
			} else {
				// has error
				return sql.NullString{}, false, retErr
			}
		}
	}

	// get value via scan
	retErr = row.Scan(&retVal)

	if retErr == sql.ErrNoRows {
		// no rows
		return sql.NullString{}, true, nil
	} else {
		// return value
		return retVal, false, retErr
	}
}

// ----------------------------------------------------------------------------------------------------------------
// execute helpers
// ----------------------------------------------------------------------------------------------------------------

// ExecByOrdinalParams executes action query string and parameters to return result, if error, returns error object within result
// [ Parameters ]
//		actionQuery = sql action query, optionally having parameters marked as ?1, ?2 .. ?N, where each represents a parameter position
//		args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
// [ Return Values ]
//		1) MySqlResult = represents the sql action result received (including error info if applicable)
func (svr *MySql) ExecByOrdinalParams(actionQuery string, args ...interface{}) MySqlResult {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// is new insertion?
	var isInsert bool

	if strings.ToUpper(util.Left(actionQuery, 6)) == "INSERT" {
		isInsert = true
	} else {
		isInsert = false
	}

	// perform exec action, and return to caller
	var result sql.Result
	var err error

	if svr.tx == nil {
		// not in transaction mode,
		// action using db object
		result, err = svr.db.Exec(actionQuery, args...)
	} else {
		// in transaction mode,
		// action using tx object
		result, err = svr.tx.Exec(actionQuery, args...)
	}

	if err != nil {
		err = errors.New("ExecByOrdinalParams() Error: " + err.Error())
		return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// if inserted, get last id if known
	var newID int64
	newID = 0

	if isInsert {
		newID, err = result.LastInsertId()

		if err != nil {
			err = errors.New("ExecByOrdinalParams() Get LastInsertId() Error: " + err.Error())
			return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
		}
	}

	// get rows affected by this action
	var affected int64
	affected = 0

	affected, err = result.RowsAffected()

	if err != nil {
		err = errors.New("ExecByOrdinalParams() Get RowsAffected() Error: " + err.Error())
		return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// return result
	return MySqlResult{RowsAffected: affected, NewlyInsertedID: newID, Err: nil}
}

// ExecByNamedMapParam executes action query string with named map containing parameters to return result, if error, returns error object within result
// [ Syntax ]
//		1) in sql = instead of defining ordinal parameters ?, each parameter in sql does not need to be ordinal, rather define with :xyz (must have : in front of param name), where xyz is name of parameter, such as :customerID
//		2) in go = setup a map variable: var p = make(map[string]interface{})
//		3) in go = to set values into map variable: p["xyz"] = abc
//				   where xyz is the parameter name matching the sql :xyz (do not include : in go map "xyz")
//				   where abc is the value of the parameter value, whether string or other data types
//				   note: in using map, just add additional map elements using the p["xyz"] = abc syntax
//				   note: if parameter value can be a null, such as nullint, nullstring, use util.ToNullTime(), ToNullInt(), ToNullString(), etc.
//		4) in go = when calling this function passing the map variable, simply pass the map variable p into the args parameter
// [ Parameters ]
//		actionQuery = sql action query, with named parameters using :xyz syntax
//		args = required, the map variable of the named parameters
// [ Return Values ]
//		1) MySqlResult = represents the sql action result received (including error info if applicable)
func (svr *MySql) ExecByNamedMapParam(actionQuery string, args map[string]interface{}) MySqlResult {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// is new insertion?
	var isInsert bool

	if strings.ToUpper(util.Left(actionQuery, 6)) == "INSERT" {
		isInsert = true
	} else {
		isInsert = false
	}

	// perform exec action, and return to caller
	var result sql.Result
	var err error

	if svr.tx == nil {
		// not in transaction mode,
		// action using db object
		result, err = svr.db.NamedExec(actionQuery, args)
	} else {
		// in transaction mode,
		// action using tx object
		result, err = svr.tx.NamedExec(actionQuery, args)
	}

	if err != nil {
		err = errors.New("ExecByNamedMapParam() Error: " + err.Error())
		return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// if inserted, get last id if known
	var newID int64
	newID = 0

	if isInsert {
		newID, err = result.LastInsertId()

		if err != nil {
			err = errors.New("ExecByNamedMapParam() Get LastInsertId() Error: " + err.Error())
			return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
		}
	}

	// get rows affected by this action
	var affected int64
	affected = 0

	affected, err = result.RowsAffected()

	if err != nil {
		err = errors.New("ExecByNamedMapParam() Get RowsAffected() Error: " + err.Error())
		return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// return result
	return MySqlResult{RowsAffected: affected, NewlyInsertedID: newID, Err: nil}
}

// ExecByStructParam executes action query string with struct containing parameters to return result, if error, returns error object within result,
// the struct fields' struct tags must match the parameter names, such as: struct tag `db:"customerID"` must match parameter name in sql as ":customerID"
// [ Syntax ]
//		1) in sql = instead of defining ordinal parameters ?, each parameter in sql does not need to be ordinal, rather define with :xyz (must have : in front of param name), where xyz is name of parameter, such as :customerID
//		2) in go = using a struct to contain fields to match parameters, make sure struct tags match to the sql parameter names, such as struct tag `db:"customerID"` must match parameter name in sql as ":customerID" (the : is not part of the match)
// [ Parameters ]
//		actionQuery = sql action query, with named parameters using :xyz syntax
//		args = required, the struct variable, whose fields having struct tags matching sql parameter names
// [ Return Values ]
//		1) MySqlResult = represents the sql action result received (including error info if applicable)
func (svr *MySql) ExecByStructParam(actionQuery string, args interface{}) MySqlResult {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// is new insertion?
	var isInsert bool

	if strings.ToUpper(util.Left(actionQuery, 6)) == "INSERT" {
		isInsert = true
	} else {
		isInsert = false
	}

	// perform exec action, and return to caller
	var result sql.Result
	var err error

	if svr.tx == nil {
		// not in transaction mode,
		// action using db object
		result, err = svr.db.NamedExec(actionQuery, args)
	} else {
		// in transaction mode,
		// action using tx object
		result, err = svr.tx.NamedExec(actionQuery, args)
	}

	if err != nil {
		err = errors.New("ExecByStructParam() Error: " + err.Error())
		return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// if inserted, get last id if known
	var newID int64
	newID = 0

	if isInsert {
		newID, err = result.LastInsertId()

		if err != nil {
			err = errors.New("ExecByStructParam() Get LastInsertId() Error: " + err.Error())
			return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
		}
	}

	// get rows affected by this action
	var affected int64
	affected = 0

	affected, err = result.RowsAffected()

	if err != nil {
		err = errors.New("ExecByStructParam() Get RowsAffected() Error: " + err.Error())
		return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// return result
	return MySqlResult{RowsAffected: affected, NewlyInsertedID: newID, Err: nil}
}
