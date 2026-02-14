package sqlite

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

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	util "github.com/aldelo/common"
	"github.com/jmoiron/sqlx"

	// this package is used by database/sql as we are wrapping the sql access functionality in this utility package
	_ "github.com/mattn/go-sqlite3"
)

// ++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
// SQLite struct Usage Guide
// ++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++

/*
	***************************************************************************************************************
	First, Create "../model/global.go"
	***************************************************************************************************************

	package model

	import (
			"errors"
			"time"
			data "github.com/aldelo/common/wrapper/sqlite"
	)

	// package level accessible to the sqlite database object
	var db *data.SQLite

	// SetDB allows code outside of package to set the sqlite database reference
	func SetDB(dbx *data.SQLite) {
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
			data "github.com/aldelo/common/wrapper/sqlite"
			"???/model" // ??? represents path to the model package
			...
	)

	...

	func main() {
		...

		// ========================================
		// setup database connection
		// ========================================

		//
		// declare sqlite database object
		//
		s := new(data.SQLite)

		//
		// set sqlite dsn fields
		//
		s.DatabasePath = ""	// database full path and file name with extension (typically .db extension)

		//
		// open sqlite database connection
		//
		if err := s.Open(); err != nil {
			s.Close()
		} else {
			// add sqlite db object to model global
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
	Third, Using SQLite Struct
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

	// when composing sql statements, if statement is long, use bytes.Buffer (or use data/QueryBuilder.go)
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

// SQLite struct encapsulates the SQLite database access functionality (using sqlx package)
//
//	DatabasePath = full path to the sqlite db file with file name and extension
//	Mode = ro (ReadOnly), rw (ReadWrite), rwc (ReadWriteCreate < Default), memory (In-Memory)
//	JournalMode = DELETE, MEMORY, WAL (< Default)
//	Synchronous = 0 (OFF), 1 (NORMAL < Default), 2 (FULL), 3 (EXTRA)
//	BusyTimeoutMS = 0 if not specified; > 0 if specified
type SQLite struct {
	// SQLite database connection properties
	DatabasePath string // including path, file name, and extension

	Mode          string // mode=ro: readOnly; rw: readwrite; rwc: readWriteCreate; memory: inMemoryOnly (set default to rwc)
	JournalMode   string // _journal_mode=DELETE, MEMORY, WAL (set default to WAL)
	Synchronous   string // _synchronous=0: OFF; 1: NORMAL; 2: FULL; 3: EXTRA (set default to NORMAL)
	BusyTimeoutMS int    // _busy_timeout=milliseconds
	LockingMode   string // _locking_mode=EXCLUSIVE (default), NORMAL

	// Connection pool configuration
	MaxOpenConns    int           // 0 = default (1 for SQLite single-writer)
	MaxIdleConns    int           // 0 = default
	MaxConnIdleTime time.Duration // 0 = no limit

	// Ping cache configuration
	PingFrequencySec int // 0 = default 30 seconds; <0 = ping every time (old behavior)

	// sqlite database state object
	db *sqlx.DB
	tx *sqlx.Tx

	// named transaction map for concurrent transaction support
	txMap map[string]*SQLiteTransaction

	// ping cache state
	lastPing time.Time

	mu sync.RWMutex
}

// SQLiteResult defines sql action query result info
// [ Notes ]
//
//	NewlyInsertedID = ONLY FOR INSERT, ONLY IF AUTO_INCREMENT PRIMARY KEY (Custom PK ID Will Have This Field as 0 Always)
type SQLiteResult struct {
	RowsAffected    int64
	NewlyInsertedID int64 // ONLY FOR INSERT, ONLY IF AUTO_INCREMENT PRIMARY KEY (Custom PK ID Will Have This Field as 0 Always)
	Err             error
}

// resetDest clears caller-owned pointer destinations on not-found results.
func resetDest(dest interface{}) {
	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return
	}
	ev := rv.Elem()
	if ev.CanSet() {
		ev.Set(reflect.Zero(ev.Type()))
	}
}

// ================================================================================================================
// STRUCT FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// GetDsn serializes SQLite database dsn to connection string, for use in database connectivity
func (svr *SQLite) GetDsn() (string, error) {
	if svr == nil {
		return "", errors.New("SQLite GetDsn Failed: SQLite receiver is nil")
	}

	//
	// first validate input
	//
	if len(svr.DatabasePath) == 0 {
		return "", errors.New("SQLite Database Path is Required")
	}

	//
	// now create sqlite database connection string
	// format = test.db?cache=private&mode=rwc
	//
	str := svr.DatabasePath + "?" + "cache=private"
	if util.LenTrim(svr.LockingMode) == 0 {
		str += "&_locking_mode=EXCLUSIVE"
	} else {
		str += "&_locking_mode=" + svr.LockingMode
	}
	str += "&_txlock=immediate"
	str += "&_foreign_keys=true"

	if util.LenTrim(svr.Mode) == 0 {
		str += "&mode=rwc"
	} else {
		str += "&mode=" + svr.Mode
	}

	if util.LenTrim(svr.JournalMode) == 0 {
		str += "&_journal_mode=WAL"
	} else {
		str += "&_journal_mode=" + svr.JournalMode
	}

	if util.LenTrim(svr.Synchronous) == 0 {
		str += "&_synchronous=1" // NORMAL
	} else {
		str += "&_synchronous=" + svr.Synchronous
	}

	if svr.BusyTimeoutMS > 0 {
		str += "&_busy_timeout=" + util.Itoa(svr.BusyTimeoutMS)
	}

	// return to caller
	return str, nil
}

// Open a database by connecting to it, using the dsn properties defined in the struct fields
func (svr *SQLite) Open() error {
	if svr == nil {
		return errors.New("SQLite Open Failed: SQLite receiver is nil")
	}

	// declare
	var str string
	var err error

	// get connect string
	str, err = svr.GetDsn()

	if err != nil {
		return err
	}
	if util.LenTrim(str) == 0 {
		return errors.New("SQLite Database Connect String Generated Cannot Be Empty")
	}

	svr.mu.Lock() // protect existing db/tx state during open
	if svr.db != nil {
		svr.mu.Unlock()
		return errors.New("SQLite Database is Already Connected") // CHANGED
	}
	svr.mu.Unlock()

	// now ready to open sqlite database
	db, e1 := sqlx.Open("sqlite3", str)
	if e1 != nil {
		return e1
	}
	if e1 = db.Ping(); e1 != nil {
		_ = db.Close()
		return e1
	}

	svr.mu.Lock()
	defer svr.mu.Unlock()

	if svr.db != nil { // close the just-opened handle if someone else connected meanwhile
		_ = db.Close()
		return errors.New("SQLite Database is Already Connected")
	}

	svr.db = db
	svr.tx = nil

	// Connection pool defaults for SQLite (single-writer)
	if svr.MaxOpenConns > 0 {
		db.SetMaxOpenConns(svr.MaxOpenConns)
	} else {
		db.SetMaxOpenConns(1) // SQLite single-writer default
	}
	if svr.MaxIdleConns > 0 {
		db.SetMaxIdleConns(svr.MaxIdleConns)
	}
	if svr.MaxConnIdleTime > 0 {
		db.SetConnMaxIdleTime(svr.MaxConnIdleTime)
	}
	svr.lastPing = time.Now()

	return nil
}

// Close will close the database connection and set db to nil
func (svr *SQLite) Close() error {
	if svr == nil {
		return nil
	}

	svr.mu.Lock()
	defer svr.mu.Unlock()

	if svr.tx != nil {
		_ = svr.tx.Rollback() // best-effort rollback
		svr.tx = nil
	}

	// rollback any outstanding named transactions
	for id, t := range svr.txMap {
		t.mu.Lock()
		if !t.closed && t.tx != nil {
			_ = t.tx.Rollback()
			t.closed = true
		}
		t.mu.Unlock()
		delete(svr.txMap, id)
	}

	if svr.db != nil {
		if err := svr.db.Close(); err != nil {
			return err
		}

		// clean up
		svr.db = nil
	}

	return nil
}

// Ping tests if current database connection is still active and ready.
// It supports cached pings to reduce overhead: by default pings are skipped
// if the last successful ping was within 30 seconds. Set PingFrequencySec
// to a positive value to change the interval, or to a negative value to
// always ping (backward-compatible behavior).
func (svr *SQLite) Ping() error {
	if svr == nil {
		return errors.New("SQLite Ping Failed: SQLite Receiver is Nil")
	}

	svr.mu.RLock()
	db := svr.db
	lastPing := svr.lastPing
	svr.mu.RUnlock()

	if db == nil {
		return errors.New("SQLite Database is Not Connected")
	}

	// Determine ping frequency
	freq := 30 * time.Second // default
	if svr.PingFrequencySec > 0 {
		freq = time.Duration(svr.PingFrequencySec) * time.Second
	} else if svr.PingFrequencySec < 0 {
		freq = 0 // always ping (backward compat mode)
	}

	if freq > 0 && time.Since(lastPing) < freq {
		return nil // skip ping, within cache window
	}

	if err := db.Ping(); err != nil {
		return err
	}

	svr.mu.Lock()
	svr.lastPing = time.Now()
	svr.mu.Unlock()
	return nil
}

// Begin starts a database transaction, and stores the transaction object until commit or rollback
func (svr *SQLite) Begin() error {
	if svr == nil {
		return errors.New("SQLite Begin Failed: SQLite receiver is nil")
	}

	svr.mu.Lock()
	defer svr.mu.Unlock()

	if svr.db == nil {
		return errors.New("SQLite Database is Not Connected")
	}

	// does transaction already exist
	if svr.tx != nil {
		return errors.New("Transaction Already Started")
	}

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
func (svr *SQLite) Commit() error {
	if svr == nil {
		return errors.New("SQLite Commit Failed: SQLite receiver is nil")
	}

	// verify if the database connection is good
	svr.mu.Lock()
	defer svr.mu.Unlock()

	if svr.db == nil {
		return errors.New("SQLite Database is Not Connected")
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
func (svr *SQLite) Rollback() error {
	if svr == nil {
		return errors.New("SQLite Rollback Failed: SQLite receiver is nil")
	}

	svr.mu.Lock()
	defer svr.mu.Unlock()

	if svr.db == nil {
		return errors.New("SQLite Database is Not Connected")
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
//
//	dest = pointer to the struct slice or address of struct slice, this is the result of rows to be marshaled into struct slice
//	query = sql query, optionally having parameters marked as ?, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. notFound = indicates no rows found in query (aka sql.ErrNoRows), if error is detected, notFound is always false
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is nil)
//
// [ Notes ]
//  1. if error == nil, and len(dest struct slice) == 0 then zero struct slice result
func (svr *SQLite) GetStructSlice(dest interface{}, query string, args ...interface{}) (notFound bool, retErr error) {
	if svr == nil {
		return false, errors.New("SQLite GetStructSlice Failed: SQLite receiver is nil")
	}

	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return false, err
	}

	svr.mu.RLock()
	defer svr.mu.RUnlock()

	if svr.db == nil {
		return false, errors.New("SQLite Database is Not Connected")
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
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		resetDest(dest)
		return true, nil
	}

	// return error
	return false, err
}

// GetStruct performs query with optional variadic parameters, and unmarshal single result row into single target struct,
// such as: Customer struct where one row of data represent a customer
// [ Parameters ]
//
//	dest = pointer to struct or address of struct, this is the result of row to be marshaled into this struct
//	query = sql query, optionally having parameters marked as ?, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. notFound = indicates no rows found in query (aka sql.ErrNoRows), if error is detected, notFound is always false
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is nil)
func (svr *SQLite) GetStruct(dest interface{}, query string, args ...interface{}) (notFound bool, retErr error) {
	if svr == nil {
		return false, errors.New("SQLite GetStruct Failed: SQLite receiver is nil")
	}

	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return false, err
	}

	svr.mu.RLock()
	defer svr.mu.RUnlock()

	if svr.db == nil {
		return false, errors.New("SQLite Database is Not Connected")
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
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		resetDest(dest)
		return true, nil
	}

	// return error
	return false, err
}

// ----------------------------------------------------------------------------------------------------------------
// query and get rows helpers
// ----------------------------------------------------------------------------------------------------------------

// GetRowsByOrdinalParams performs query with optional variadic parameters to get ROWS of result, and returns *sqlx.Rows
// [ Parameters ]
//
//	query = sql query, optionally having parameters marked as ?, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. *sqlx.Rows = pointer to sqlx.Rows; or nil if no rows yielded
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and sqlx.Rows is returned as nil)
//
// [ Ranged Loop & Scan ]
//  1. to loop, use: for _, r := range rows
//  2. to scan, use: r.Scan(&x, &y, ...), where r is the row struct in loop, where &x &y etc are the scanned output value (scan in order of select columns sequence)
//
// [ Continuous Loop & Scan ]
//  1. Continuous loop until endOfRows = true is yielded from ScanSlice() or ScanStruct()
//  2. ScanSlice(): accepts *sqlx.Rows, scans rows result into target pointer slice (if no error, endOfRows = true is returned)
//  3. ScanStruct(): accepts *sqlx.Rows, scans current single row result into target pointer struct, returns endOfRows as true of false; if endOfRows = true, loop should stop
func (svr *SQLite) GetRowsByOrdinalParams(query string, args ...interface{}) (*sqlx.Rows, error) {
	if svr == nil {
		return nil, errors.New("SQLite GetRowsByOrdinalParams Failed: SQLite receiver is nil")
	}

	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return nil, err
	}

	svr.mu.RLock()
	defer svr.mu.RUnlock()

	if svr.db == nil {
		return nil, errors.New("SQLite Database is Not Connected")
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
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		rows = nil
		err = nil
	}

	// return result
	return rows, err
}

// GetRowsByNamedMapParam performs query with named map containing parameters to get ROWS of result, and returns *sqlx.Rows
// [ Syntax ]
//  1. in sql = instead of defining ordinal parameters ?, each parameter in sql does not need to be ordinal, rather define with :xyz (must have : in front of param name), where xyz is name of parameter, such as :customerID
//  2. in go = setup a map variable: var p = make(map[string]interface{})
//  3. in go = to set values into map variable: p["xyz"] = abc
//     where xyz is the parameter name matching the sql :xyz (do not include : in go map "xyz")
//     where abc is the value of the parameter value, whether string or other data types
//     note: in using map, just add additional map elements using the p["xyz"] = abc syntax
//     note: if parameter value can be a null, such as nullint, nullstring, use util.ToNullTime(), ToNullInt(), ToNullString(), etc.
//  4. in go = when calling this function passing the map variable, simply pass the map variable p into the args parameter
//
// [ Parameters ]
//
//	query = sql query, optionally having parameters marked as :xyz for each parameter name, where each represents a named parameter
//	args = required, the map variable of the named parameters
//
// [ Return Values ]
//  1. *sqlx.Rows = pointer to sqlx.Rows; or nil if no rows yielded
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and sqlx.Rows is returned as nil)
//
// [ Ranged Loop & Scan ]
//  1. to loop, use: for _, r := range rows
//  2. to scan, use: r.Scan(&x, &y, ...), where r is the row struct in loop, where &x &y etc are the scanned output value (scan in order of select columns sequence)
//
// [ Continuous Loop & Scan ]
//  1. Continuous loop until endOfRows = true is yielded from ScanSlice() or ScanStruct()
//  2. ScanSlice(): accepts *sqlx.Rows, scans rows result into target pointer slice (if no error, endOfRows = true is returned)
//  3. ScanStruct(): accepts *sqlx.Rows, scans current single row result into target pointer struct, returns endOfRows as true of false; if endOfRows = true, loop should stop
func (svr *SQLite) GetRowsByNamedMapParam(query string, args map[string]interface{}) (*sqlx.Rows, error) {
	if svr == nil {
		return nil, errors.New("SQLite GetRowsByNamedMapParam Failed: SQLite receiver is nil")
	}

	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return nil, err
	}

	svr.mu.RLock()
	defer svr.mu.RUnlock()

	if svr.db == nil {
		return nil, errors.New("SQLite Database is Not Connected")
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

	if err != nil && errors.Is(err, sql.ErrNoRows) {
		// no rows
		rows = nil
		err = nil
	}

	// return result
	return rows, err
}

// GetRowsByStructParam performs query with a struct as parameter input to get ROWS of result, and returns *sqlx.Rows
// [ Syntax ]
//  1. in sql = instead of defining ordinal parameters ?, each parameter in sql does not need to be ordinal, rather define with :xyz (must have : in front of param name), where xyz is name of parameter, such as :customerID
//  2. in sql = important: the :xyz defined where xyz portion of parameter name must batch the struct tag's `db:"xyz"`
//  3. in go = a struct containing struct tags that matches the named parameters will be set with values, and passed into this function's args parameter input
//  4. in go = when calling this function passing the struct variable, simply pass the struct variable into the args parameter
//
// [ Parameters ]
//
//	query = sql query, optionally having parameters marked as :xyz for each parameter name, where each represents a named parameter
//	args = required, the struct variable where struct fields' struct tags match to the named parameters
//
// [ Return Values ]
//  1. *sqlx.Rows = pointer to sqlx.Rows; or nil if no rows yielded
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and sqlx.Rows is returned as nil)
//
// [ Ranged Loop & Scan ]
//  1. to loop, use: for _, r := range rows
//  2. to scan, use: r.Scan(&x, &y, ...), where r is the row struct in loop, where &x &y etc are the scanned output value (scan in order of select columns sequence)
//
// [ Continuous Loop & Scan ]
//  1. Continuous loop until endOfRows = true is yielded from ScanSlice() or ScanStruct()
//  2. ScanSlice(): accepts *sqlx.Rows, scans rows result into target pointer slice (if no error, endOfRows = true is returned)
//  3. ScanStruct(): accepts *sqlx.Rows, scans current single row result into target pointer struct, returns endOfRows as true of false; if endOfRows = true, loop should stop
func (svr *SQLite) GetRowsByStructParam(query string, args interface{}) (*sqlx.Rows, error) {
	if svr == nil {
		return nil, errors.New("SQLite GetRowsByStructParam Failed: SQLite receiver is nil")
	}

	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return nil, err
	}

	svr.mu.RLock()
	defer svr.mu.RUnlock()

	if svr.db == nil {
		return nil, errors.New("SQLite Database is Not Connected")
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

	if err != nil && errors.Is(err, sql.ErrNoRows) {
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
//
//	rows = *sqlx.Rows
//	dest = pointer or address to slice, such as: variable to "*[]string", or variable to "&cList for declaration cList []string"
//
// [ Return Values ]
//  1. endOfRows = true if this action call yielded end of rows, meaning stop further processing of current loop
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is set as nil)
func (svr *SQLite) ScanSlice(rows *sqlx.Rows, dest *[]interface{}) (endOfRows bool, err error) {
	if svr == nil {
		return false, errors.New("SQLite ScanSlice Failed: SQLite receiver is nil")
	}

	// ensure rows pointer is set
	if rows == nil {
		return true, nil
	}

	// call rows.Next() first to position the row
	if rows.Next() {
		var out []interface{}
		out, err = rows.SliceScan()
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return true, nil
			}
			return false, err
		}
		*dest = out
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
//
//	rows = *sqlx.Rows
//	dest = pointer or address to struct, such as: variable to "*Customer", or variable to "&c for declaration c Customer"
//
// [ Return Values ]
//  1. endOfRows = true if this action call yielded end of rows, meaning stop further processing of current loop
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is set as nil)
func (svr *SQLite) ScanStruct(rows *sqlx.Rows, dest interface{}) (endOfRows bool, err error) {
	if svr == nil {
		return false, errors.New("SQLite ScanStruct Failed: SQLite receiver is nil")
	}

	// ensure rows pointer is set
	if rows == nil {
		return true, nil
	}

	// call rows.Next() first to position the row
	if rows.Next() {
		// now struct scan
		err = rows.StructScan(dest)

		// if err is sql.ErrNoRows then treat as no error
		if err != nil && errors.Is(err, sql.ErrNoRows) {
			endOfRows = true
			dest = nil
			err = nil
			return
		}

		if err != nil {
			// has error
			endOfRows = false // although error but may not be at end of rows
			dest = nil
			return
		}

		// struct scan successful, but may not be at end of rows
		return false, nil
	}

	// no more rows
	return true, nil
}

// ----------------------------------------------------------------------------------------------------------------
// query for single row helper
// ----------------------------------------------------------------------------------------------------------------

// GetSingleRow performs query with optional variadic parameters to get a single ROW of result, and returns *sqlx.Row (This function returns SINGLE ROW)
// [ Parameters ]
//
//	query = sql query, optionally having parameters marked as ?, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. *sqlx.Row = pointer to sqlx.Row; or nil if no row yielded
//  2. if error != nil, then error is encountered (if error = sql.ErrNoRows, then error is treated as nil, and sqlx.Row is returned as nil)
//
// [ Scan Values ]
//  1. Use row.Scan() and pass in pointer or address of variable to receive scanned value outputs (Scan is in the order of column sequences in select statement)
//
// [ WARNING !!! ]
//
//	WHEN USING Scan(), MUST CHECK Scan Result Error for sql.ErrNoRow status
//	SUGGESTED TO USE ScanColumnsByRow() Instead of Scan()
func (svr *SQLite) GetSingleRow(query string, args ...interface{}) (*sqlx.Row, error) {
	if svr == nil {
		return nil, errors.New("SQLite GetSingleRow Failed: SQLite receiver is nil")
	}

	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return nil, err
	}

	svr.mu.RLock()
	defer svr.mu.RUnlock()

	if svr.db == nil {
		return nil, errors.New("SQLite Database is Not Connected")
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
			if errors.Is(err, sql.ErrNoRows) {
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
//
//	row = *sqlx.Row
//	dest = pointer or address to slice, such as: variable to "*[]string", or variable to "&cList for declaration cList []string"
//
// [ Return Values ]
//  1. notFound = true if no row is found in current scan
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is set as nil and notFound is true)
func (svr *SQLite) ScanSliceByRow(row *sqlx.Row, dest *[]interface{}) (notFound bool, err error) {
	if svr == nil {
		return false, errors.New("SQLite ScanSliceByRow Failed: SQLite receiver is nil")
	}

	// if row is nil, treat as no row and not an error
	if row == nil {
		return true, nil
	}

	var out []interface{}
	out, err = row.SliceScan()
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true, nil
		}
		return false, err
	}

	*dest = out // assign back to caller
	return false, nil
}

// ScanStructByRow takes in *sqlx.Row, and marshals current row's column values into a pointer reference to a struct,
// the struct fields and row columns must match for both name and sequence position,
// this enables us to quickly convert the row's columns into a defined struct automatically,
// the dest is nil if no columns found; the dest is pointer of struct when mapping is complete
// [ Parameters ]
//
//	row = *sqlx.Row
//	dest = pointer or address to struct, such as: variable to "*Customer", or variable to "&c for declaration c Customer"
//
// [ Return Values ]
//  1. notFound = true if no row is found in current scan
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is set as nil and notFound is true)
func (svr *SQLite) ScanStructByRow(row *sqlx.Row, dest interface{}) (notFound bool, err error) {
	if svr == nil {
		return false, errors.New("SQLite ScanStructByRow Failed: SQLite receiver is nil")
	}

	// if row is nil, treat as no row and not an error
	if row == nil {
		dest = nil
		return true, nil
	}

	// now struct scan
	err = row.StructScan(dest)

	// if err is sql.ErrNoRows then treat as no error
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		dest = nil
		return true, nil
	}

	if err != nil {
		// has error
		dest = nil
		return false, err // although error but may not be not found
	}

	// struct scan successful
	return false, nil
}

// ScanColumnsByRow accepts a *sqlx row, and scans specific columns into dest outputs,
// this is different than ScanSliceByRow or ScanStructByRow because this function allows specific extraction of column values into target fields，
// (note: this function must extra all row column values to dest variadic parameters as present in the row parameter)
// [ Parameters ]
//
//	row = *sqlx.Row representing the row containing columns to extract, note that this function MUST extract all columns from this row
//	dest = MUST BE pointer (or &variable) to target variable to receive the column value, data type must match column data type value, and sequence of dest must be in the order of columns sequence
//
// [ Return Values ]
//  1. notFound = true if no row is found in current scan
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is set as nil and notFound is true)
//
// [ Example ]
//  1. assuming: Select CustomerID, CustomerName, Address FROM Customer Where CustomerPhone='123';
//  2. assuming: row // *sqlx.Row derived from GetSingleRow() or specific row from GetRowsByOrdinalParams() / GetRowsByNamedMapParam() / GetRowsByStructParam()
//  3. assuming: var CustomerID int64
//     var CustomerName string
//     var Address string
//  4. notFound, err := svr.ScanColumnsByRow(row, &CustomerID, &CustomerName, &Address)
func (svr *SQLite) ScanColumnsByRow(row *sqlx.Row, dest ...interface{}) (notFound bool, err error) {
	if svr == nil {
		return false, errors.New("SQLite ScanColumnsByRow Failed: SQLite receiver is nil")
	}

	// if row is nil, treat as no row and not an error
	if row == nil {
		return true, nil
	}

	// now scan columns from row
	err = row.Scan(dest...)

	// if err is sql.ErrNoRows then treat as no error
	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}

	if err != nil {
		// has error
		return false, err // although error but may not be not found
	}

	// scan columns successful
	return false, nil
}

// ----------------------------------------------------------------------------------------------------------------
// query for single value in single row helpers
// ----------------------------------------------------------------------------------------------------------------

// GetScalarString performs query with optional variadic parameters, and returns the first row and first column value in string data type
// [ Parameters ]
//
//	query = sql query, optionally having parameters marked as ?, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. retVal = string value of scalar result, if no value, blank is returned
//  2. retNotFound = now row found
//  3. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and retVal is returned as blank)
func (svr *SQLite) GetScalarString(query string, args ...interface{}) (retVal string, retNotFound bool, retErr error) {
	if svr == nil {
		return "", true, errors.New("SQLite GetScalarString Failed: SQLite receiver is nil")
	}

	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return "", false, err
	}

	svr.mu.RLock()
	defer svr.mu.RUnlock()

	if svr.db == nil {
		return "", false, errors.New("SQLite Database is Not Connected")
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
			if errors.Is(retErr, sql.ErrNoRows) {
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

	if errors.Is(retErr, sql.ErrNoRows) {
		// no rows
		return "", true, nil
	} else {
		// return value
		return retVal, false, retErr
	}
}

// GetScalarNullString performs query with optional variadic parameters, and returns the first row and first column value in sql.NullString{} data type
// [ Parameters ]
//
//	query = sql query, optionally having parameters marked as ?, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. retVal = string value of scalar result, if no value, sql.NullString{} is returned
//  2. retNotFound = now row found
//  3. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and retVal is returned as sql.NullString{})
func (svr *SQLite) GetScalarNullString(query string, args ...interface{}) (retVal sql.NullString, retNotFound bool, retErr error) {
	if svr == nil {
		return sql.NullString{}, true, errors.New("SQLite GetScalarNullString Failed: SQLite receiver is nil")
	}

	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return sql.NullString{}, false, err
	}

	svr.mu.RLock()
	defer svr.mu.RUnlock()

	if svr.db == nil {
		return sql.NullString{}, false, errors.New("SQLite Database is Not Connected")
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
			if errors.Is(retErr, sql.ErrNoRows) {
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

	if errors.Is(retErr, sql.ErrNoRows) {
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
//
//	actionQuery = sql action query, optionally having parameters marked as ?, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. SQLiteResult = represents the sql action result received (including error info if applicable)
func (svr *SQLite) ExecByOrdinalParams(actionQuery string, args ...interface{}) SQLiteResult {
	if svr == nil {
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("SQLite ExecByOrdinalParams Failed: SQLite receiver is nil")}
	}

	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	trimmed := strings.TrimSpace(strings.ToUpper(actionQuery))
	isInsert := strings.HasPrefix(trimmed, "INSERT")

	svr.mu.Lock()
	defer svr.mu.Unlock()

	if svr.db == nil {
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("SQLite Database is Not Connected")}
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
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// if inserted, get last id if known
	var newID int64
	if isInsert {
		newID, err = result.LastInsertId()
		if err != nil {
			err = errors.New("ExecByOrdinalParams() Get LastInsertId() Error: " + err.Error())
			return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
		}
	}

	// get rows affected by this action
	var affected int64
	affected = 0

	affected, err = result.RowsAffected()

	if err != nil {
		err = errors.New("ExecByOrdinalParams() Get RowsAffected() Error: " + err.Error())
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// return result
	return SQLiteResult{RowsAffected: affected, NewlyInsertedID: newID, Err: nil}
}

// ExecByNamedMapParam executes action query string with named map containing parameters to return result, if error, returns error object within result
// [ Syntax ]
//  1. in sql = instead of defining ordinal parameters ?, each parameter in sql does not need to be ordinal, rather define with :xyz (must have : in front of param name), where xyz is name of parameter, such as :customerID
//  2. in go = setup a map variable: var p = make(map[string]interface{})
//  3. in go = to set values into map variable: p["xyz"] = abc
//     where xyz is the parameter name matching the sql :xyz (do not include : in go map "xyz")
//     where abc is the value of the parameter value, whether string or other data types
//     note: in using map, just add additional map elements using the p["xyz"] = abc syntax
//     note: if parameter value can be a null, such as nullint, nullstring, use util.ToNullTime(), ToNullInt(), ToNullString(), etc.
//  4. in go = when calling this function passing the map variable, simply pass the map variable p into the args parameter
//
// [ Parameters ]
//
//	actionQuery = sql action query, with named parameters using :xyz syntax
//	args = required, the map variable of the named parameters
//
// [ Return Values ]
//  1. SQLiteResult = represents the sql action result received (including error info if applicable)
func (svr *SQLite) ExecByNamedMapParam(actionQuery string, args map[string]interface{}) SQLiteResult {
	if svr == nil {
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("SQLite ExecByNamedMapParam Failed: SQLite receiver is nil")}
	}

	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}
	if args == nil {
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("ExecByNamedMapParam() Error: args map cannot be nil")}
	}

	trimmed := strings.TrimSpace(strings.ToUpper(actionQuery))
	isInsert := strings.HasPrefix(trimmed, "INSERT")

	svr.mu.Lock()
	defer svr.mu.Unlock()

	if svr.db == nil {
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("SQLite Database is Not Connected")}
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
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// if inserted, get last id if known
	var newID int64
	if isInsert {
		newID, err = result.LastInsertId()

		if err != nil {
			err = errors.New("ExecByNamedMapParam() Get LastInsertId() Error: " + err.Error())
			return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
		}
	}

	// get rows affected by this action
	var affected int64
	affected = 0

	affected, err = result.RowsAffected()

	if err != nil {
		err = errors.New("ExecByNamedMapParam() Get RowsAffected() Error: " + err.Error())
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// return result
	return SQLiteResult{RowsAffected: affected, NewlyInsertedID: newID, Err: nil}
}

// ExecByStructParam executes action query string with struct containing parameters to return result, if error, returns error object within result,
// the struct fields' struct tags must match the parameter names, such as: struct tag `db:"customerID"` must match parameter name in sql as ":customerID"
// [ Syntax ]
//  1. in sql = instead of defining ordinal parameters ?, each parameter in sql does not need to be ordinal, rather define with :xyz (must have : in front of param name), where xyz is name of parameter, such as :customerID
//  2. in go = using a struct to contain fields to match parameters, make sure struct tags match to the sql parameter names, such as struct tag `db:"customerID"` must match parameter name in sql as ":customerID" (the : is not part of the match)
//
// [ Parameters ]
//
//	actionQuery = sql action query, with named parameters using :xyz syntax
//	args = required, the struct variable, whose fields having struct tags matching sql parameter names
//
// [ Return Values ]
//  1. SQLiteResult = represents the sql action result received (including error info if applicable)
func (svr *SQLite) ExecByStructParam(actionQuery string, args interface{}) SQLiteResult {
	if svr == nil {
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("SQLite ExecByStructParam Failed: SQLite receiver is nil")}
	}

	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}
	if args == nil {
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("ExecByStructParam() Error: args struct cannot be nil")}
	}

	trimmed := strings.TrimSpace(strings.ToUpper(actionQuery))
	isInsert := strings.HasPrefix(trimmed, "INSERT")

	svr.mu.Lock()
	defer svr.mu.Unlock()

	if svr.db == nil {
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("SQLite Database is Not Connected")}
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
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// if inserted, get last id if known
	var newID int64
	if isInsert {
		newID, err = result.LastInsertId()

		if err != nil {
			err = errors.New("ExecByStructParam() Get LastInsertId() Error: " + err.Error())
			return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
		}
	}

	// get rows affected by this action
	var affected int64
	affected = 0

	affected, err = result.RowsAffected()

	if err != nil {
		err = errors.New("ExecByStructParam() Get RowsAffected() Error: " + err.Error())
		return SQLiteResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// return result
	return SQLiteResult{RowsAffected: affected, NewlyInsertedID: newID, Err: nil}
}

// ----------------------------------------------------------------------------------------------------------------
// context-aware query and exec helpers
// ----------------------------------------------------------------------------------------------------------------

// GetStructContext is the context-aware version of GetStruct.
func (svr *SQLite) GetStructContext(ctx context.Context, dest interface{}, query string, args ...interface{}) (notFound bool, retErr error) {
	if svr == nil {
		return false, errors.New("SQLite GetStructContext Failed: SQLite Receiver is Nil")
	}
	if err := svr.Ping(); err != nil {
		return false, err
	}
	svr.mu.RLock()
	defer svr.mu.RUnlock()
	if svr.db == nil {
		return false, errors.New("SQLite Database is Not Connected")
	}
	resetDest(dest)
	var err error
	if svr.tx == nil {
		err = svr.db.GetContext(ctx, dest, query, args...)
	} else {
		err = svr.tx.GetContext(ctx, dest, query, args...)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			resetDest(dest)
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// GetStructSliceContext is the context-aware version of GetStructSlice.
func (svr *SQLite) GetStructSliceContext(ctx context.Context, dest interface{}, query string, args ...interface{}) (notFound bool, retErr error) {
	if svr == nil {
		return false, errors.New("SQLite GetStructSliceContext Failed: SQLite Receiver is Nil")
	}
	if err := svr.Ping(); err != nil {
		return false, err
	}
	svr.mu.RLock()
	defer svr.mu.RUnlock()
	if svr.db == nil {
		return false, errors.New("SQLite Database is Not Connected")
	}
	resetDest(dest)
	var err error
	if svr.tx == nil {
		err = svr.db.SelectContext(ctx, dest, query, args...)
	} else {
		err = svr.tx.SelectContext(ctx, dest, query, args...)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			resetDest(dest)
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// ExecByOrdinalParamsContext is the context-aware version of ExecByOrdinalParams.
func (svr *SQLite) ExecByOrdinalParamsContext(ctx context.Context, actionQuery string, args ...interface{}) SQLiteResult {
	if svr == nil {
		return SQLiteResult{Err: errors.New("SQLite ExecByOrdinalParamsContext Failed: SQLite Receiver is Nil")}
	}
	if err := svr.Ping(); err != nil {
		return SQLiteResult{Err: err}
	}

	isInsert := strings.ToUpper(util.Left(strings.TrimSpace(actionQuery), 6)) == "INSERT"

	svr.mu.Lock()
	defer svr.mu.Unlock()
	if svr.db == nil {
		return SQLiteResult{Err: errors.New("SQLite Database is Not Connected")}
	}

	var result sql.Result
	var err error
	if svr.tx == nil {
		result, err = svr.db.ExecContext(ctx, actionQuery, args...)
	} else {
		result, err = svr.tx.ExecContext(ctx, actionQuery, args...)
	}
	if err != nil {
		return SQLiteResult{Err: err}
	}

	rtn := SQLiteResult{}
	rtn.RowsAffected, _ = result.RowsAffected()
	if isInsert {
		rtn.NewlyInsertedID, _ = result.LastInsertId()
	}
	return rtn
}

// GetScalarStringContext is the context-aware version of GetScalarString.
func (svr *SQLite) GetScalarStringContext(ctx context.Context, query string, args ...interface{}) (retVal string, retNotFound bool, retErr error) {
	if svr == nil {
		return "", false, errors.New("SQLite GetScalarStringContext Failed: SQLite Receiver is Nil")
	}
	if err := svr.Ping(); err != nil {
		return "", false, err
	}
	svr.mu.RLock()
	defer svr.mu.RUnlock()
	if svr.db == nil {
		return "", false, errors.New("SQLite Database is Not Connected")
	}
	var result string
	var err error
	if svr.tx == nil {
		err = svr.db.GetContext(ctx, &result, query, args...)
	} else {
		err = svr.tx.GetContext(ctx, &result, query, args...)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", true, nil
		}
		return "", false, err
	}
	return result, false, nil
}

// ----------------------------------------------------------------------------------------------------------------
// concurrent named transaction support
// ----------------------------------------------------------------------------------------------------------------

// SQLiteTransaction represents a named database transaction for concurrent transaction support.
type SQLiteTransaction struct {
	id     string
	parent *SQLite
	tx     *sqlx.Tx
	closed bool
	mu     sync.Mutex
}

// BeginTx starts a new named transaction. The tag is used as an identifier.
// Multiple transactions can coexist with different tags.
// Call Commit() or Rollback() on the returned transaction when done.
func (svr *SQLite) BeginTx(tag string) (*SQLiteTransaction, error) {
	if svr == nil {
		return nil, errors.New("SQLite BeginTx Failed: SQLite Receiver is Nil")
	}
	if err := svr.Ping(); err != nil {
		return nil, err
	}
	svr.mu.Lock()
	defer svr.mu.Unlock()
	if svr.db == nil {
		return nil, errors.New("SQLite Database is Not Connected")
	}
	tx, err := svr.db.Beginx()
	if err != nil {
		return nil, err
	}
	id := tag
	if id == "" {
		id = fmt.Sprintf("tx_%d", time.Now().UnixNano())
	}
	t := &SQLiteTransaction{id: id, parent: svr, tx: tx, closed: false}
	if svr.txMap == nil {
		svr.txMap = make(map[string]*SQLiteTransaction)
	}
	svr.txMap[id] = t
	return t, nil
}

// ID returns the transaction identifier.
func (t *SQLiteTransaction) ID() string {
	return t.id
}

// Commit commits the named transaction.
func (t *SQLiteTransaction) Commit() error {
	if t == nil {
		return errors.New("SQLiteTransaction Commit Failed: Receiver is Nil")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return errors.New("SQLiteTransaction Already Closed")
	}
	if err := t.tx.Commit(); err != nil {
		return err
	}
	t.closed = true
	// Remove from parent txMap
	if t.parent != nil {
		t.parent.mu.Lock()
		delete(t.parent.txMap, t.id)
		t.parent.mu.Unlock()
	}
	return nil
}

// Rollback rolls back the named transaction.
func (t *SQLiteTransaction) Rollback() error {
	if t == nil {
		return errors.New("SQLiteTransaction Rollback Failed: Receiver is Nil")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil // already closed, no-op
	}
	err := t.tx.Rollback()
	t.closed = true
	// Remove from parent txMap
	if t.parent != nil {
		t.parent.mu.Lock()
		delete(t.parent.txMap, t.id)
		t.parent.mu.Unlock()
	}
	return err
}

// GetStruct queries a single row within the transaction.
func (t *SQLiteTransaction) GetStruct(dest interface{}, query string, args ...interface{}) (notFound bool, err error) {
	if t == nil {
		return false, errors.New("SQLiteTransaction GetStruct Failed: Receiver is Nil")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return false, errors.New("SQLiteTransaction Already Closed")
	}
	resetDest(dest)
	if err := t.tx.Get(dest, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			resetDest(dest)
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// GetStructSlice queries multiple rows within the transaction.
func (t *SQLiteTransaction) GetStructSlice(dest interface{}, query string, args ...interface{}) (notFound bool, err error) {
	if t == nil {
		return false, errors.New("SQLiteTransaction GetStructSlice Failed: Receiver is Nil")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return false, errors.New("SQLiteTransaction Already Closed")
	}
	resetDest(dest)
	if err := t.tx.Select(dest, query, args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			resetDest(dest)
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// ExecByOrdinalParams executes a write query within the transaction.
func (t *SQLiteTransaction) ExecByOrdinalParams(actionQuery string, args ...interface{}) SQLiteResult {
	if t == nil {
		return SQLiteResult{Err: errors.New("SQLiteTransaction ExecByOrdinalParams Failed: Receiver is Nil")}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return SQLiteResult{Err: errors.New("SQLiteTransaction Already Closed")}
	}

	isInsert := strings.ToUpper(util.Left(strings.TrimSpace(actionQuery), 6)) == "INSERT"

	result, err := t.tx.Exec(actionQuery, args...)
	if err != nil {
		return SQLiteResult{Err: err}
	}

	rtn := SQLiteResult{}
	rtn.RowsAffected, _ = result.RowsAffected()
	if isInsert {
		rtn.NewlyInsertedID, _ = result.LastInsertId()
	}
	return rtn
}
