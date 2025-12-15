package mysql

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
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aldelo/common/wrapper/xray"
	awsxray "github.com/aws/aws-xray-sdk-go/xray"

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
//
//	   	Charset = utf8, utf8mb4 (< Default)
//			Collation = utf8mb4_general_ci (< Default), utf8_general_ci
//	   	...Timeout = must be decimal number with unit suffix (ms, s, m, h), such as "30s", "0.5m", "1m30s"
//
//			MaxOpenConns = maximum open and idle connections for the connection pool, default is 0, which is unlimited (db *sqlx.DB internally is a connection pool object)
//			MaxIdleConns = maximum number of idle connections to keep in the connection pool, default is 0 which is unlimited, this number should be equal or less than MaxOpenConns
//			MaxConnIdleTime = maximum duration that an idle connection be kept in the connection pool, default is 0 which has no time limit, suggest 5 - 10 minutes if set
type MySql struct {
	// MySql server connection properties
	UserName string
	Password string

	Host     string
	Port     int
	Database string

	Charset        string // utf8, utf8mb4
	Collation      string // utf8mb4_general_ci, utf8_general_ci
	ConnectTimeout string // must be decimal number with unit suffix (ms, s, m, h), such as "30s", "0.5m", "1m30s"
	ReadTimeout    string // must be decimal number with unit suffix (ms, s, m, h), such as "30s", "0.5m", "1m30s"
	WriteTimeout   string // must be decimal number with unit suffix (ms, s, m, h), such as "30s", "0.5m", "1m30s"
	RejectReadOnly bool

	MaxOpenConns    int
	MaxIdleConns    int
	MaxConnIdleTime time.Duration

	// mysql server state object
	db    *sqlx.DB
	txMap map[string]*MySqlTransaction
	mux   sync.RWMutex

	lastPing       time.Time
	_parentSegment *xray.XRayParentSegment
}

// MySqlTransaction represents an instance of the sqlx.Tx.
// Since sqlx.DB is actually a connection pool container, it may initiate multiple sql transactions,
// therefore, independent sqlx.Tx must be managed individually throughout its lifecycle.
//
// each mysql transaction created will also register with MySql struct, so that during Close/Cleanup, all the related outstanding Transactions can rollback
type MySqlTransaction struct {
	id         string
	parent     *MySql
	tx         *sqlx.Tx
	closed     bool
	_xrayTxSeg *xray.XSegment
}

// Commit will commit the current mysql transaction and close off from further uses (if commit was successful).
// on commit error, transaction will not close off
func (t *MySqlTransaction) Commit() (err error) {
	if t._xrayTxSeg != nil {
		defer func() {
			if err != nil {
				_ = t._xrayTxSeg.Seg.AddError(err)
			}
			if t.closed {
				t._xrayTxSeg.Close()
			}
		}()
	}

	if t.closed {
		err = fmt.Errorf("MySql Commit Transaction Not Valid, Transaction Already Closed")
		return err
	} else if util.LenTrim(t.id) == 0 {
		err = fmt.Errorf("MySql Commit Transaction Not Valid, ID is Missing")
		return err
	} else if t.parent == nil {
		err = fmt.Errorf("MySql Commit Transaction Not Valid, Parent is Missing")
		return err
	} else if t.tx == nil {
		err = fmt.Errorf("MySql Commit Transaction Not Valid, Transaction Object Nil")
		return err
	}

	if err = t.parent.Ping(); err != nil {
		// ping failed
		if t._xrayTxSeg != nil {
			_ = t._xrayTxSeg.Seg.AddError(err)
			t._xrayTxSeg.Close()
			t._xrayTxSeg = nil
		}
		return err
	}

	if t._xrayTxSeg == nil {
		// not using xray
		if e := t.tx.Commit(); e != nil {
			// on commit error, return error, don't close off transaction
			err = fmt.Errorf("MySql Commit Transaction Failed, %s", e)
			return err
		} else {
			// transaction commit success
			t.closed = true

			// remove this transaction from mysql parent txMap
			t.parent.mux.Lock()
			delete(t.parent.txMap, t.id)
			t.parent.mux.Unlock()

			// success response
			return nil
		}
	} else {
		// using xray
		subSeg := t._xrayTxSeg.NewSubSegment("Commit-Transaction")
		defer subSeg.Close()

		subSeg.Capture("Commit-Transaction-Do", func() error {
			if e := t.tx.Commit(); e != nil {
				// on commit error, return error, don't close off transaction
				err = fmt.Errorf("MySql Commit Transaction Failed, %s", e)
				_ = subSeg.Seg.AddError(err)
				return err
			} else {
				// transaction commit success
				t.closed = true

				// remove this transaction from mysql parent txMap
				t.parent.mux.Lock()
				delete(t.parent.txMap, t.id)
				t.parent.mux.Unlock()

				// success response
				return nil
			}
		})

		return err
	}
}

// Rollback will rollback the current mysql transaction and close off from further uses (whether rollback succeeds or failures)
func (t *MySqlTransaction) Rollback() (err error) {
	if t._xrayTxSeg != nil {
		defer func() {
			if err != nil {
				_ = t._xrayTxSeg.Seg.AddError(err)
			}
			t._xrayTxSeg.Close()
		}()
	}

	if t.closed {
		err = fmt.Errorf("MySql Rollback Transaction Not Valid, Transaction Already Closed")
		return err
	} else if util.LenTrim(t.id) == 0 {
		err = fmt.Errorf("MySql Rollback Transaction Not Valid, ID is Missing")
		return err
	} else if t.parent == nil {
		err = fmt.Errorf("MySql Rollback Transaction Not Valid, Parent is Missing")
		return err
	} else if t.tx == nil {
		err = fmt.Errorf("MySql Rollback Transaction Not Valid, Transaction Object Nil")
		return err
	}

	if err = t.parent.Ping(); err != nil {
		// ping failed
		if t._xrayTxSeg != nil {
			_ = t._xrayTxSeg.Seg.AddError(err)
			t._xrayTxSeg.Close()
			t._xrayTxSeg = nil
		}
		return err
	}

	// perform rollback
	if t._xrayTxSeg == nil {
		if e := t.tx.Rollback(); e != nil {
			err = fmt.Errorf("MySql Rollback Transaction Failed, %s", e)
		}
	} else {
		subSeg := t._xrayTxSeg.NewSubSegment("Rollback-Transaction")
		defer subSeg.Close()

		subSeg.Capture("Rollback-Transaction-Do", func() error {
			if e := t.tx.Rollback(); e != nil {
				err = fmt.Errorf("MySql Rollback Transaction Failed, %s", e)
				_ = subSeg.Seg.AddError(err)
				return err
			} else {
				return nil
			}
		})
	}

	// transaction rollback success or failure, always close off transaction
	t.closed = true

	// remove this transaction from mysql parent txMap
	t.parent.mux.Lock()
	delete(t.parent.txMap, t.id)
	t.parent.mux.Unlock()

	// return response
	return err
}

// ready checks if MySqlTransaction
func (t *MySqlTransaction) ready() error {
	if t.closed {
		return fmt.Errorf("MySql Transaction Not Valid, Transaction Already Closed")
	} else if util.LenTrim(t.id) == 0 {
		return fmt.Errorf("MySql Transaction Not Valid, ID is Missing")
	} else if t.parent == nil {
		return fmt.Errorf("MySql Transaction Not Valid, Parent is Missing")
	} else if t.tx == nil {
		return fmt.Errorf("MySql Transaction Not Valid, Transaction Object Nil")
	}

	return nil
}

// MySqlResult defines sql action query result info
// [ Notes ]
//  1. NewlyInsertedID = ONLY FOR INSERT, ONLY IF AUTO_INCREMENT PRIMARY KEY (Custom PK ID Will Have This Field as 0 Always)
type MySqlResult struct {
	RowsAffected    int64
	NewlyInsertedID int64 // ONLY FOR INSERT, ONLY IF AUTO_INCREMENT PRIMARY KEY (Custom PK ID Will Have This Field as 0 Always)
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

// cleanUpAllSqlTransactions is a helper that cleans up (rolls back) any outstanding sql transactions in the txMap
func (svr *MySql) cleanUpAllSqlTransactions() {
	svr.mux.Lock()
	if svr.txMap != nil && len(svr.txMap) > 0 {
		for k, v := range svr.txMap {
			if v != nil && !v.closed && v.tx != nil {
				_ = v.tx.Rollback()
				if v._xrayTxSeg != nil {
					v._xrayTxSeg.Close()
				}
			}
			delete(svr.txMap, k)
		}
		svr.txMap = make(map[string]*MySqlTransaction)
	}
	svr.mux.Unlock()
}

// Open will open a database either as normal or with xray tracing,
// Open uses the dsn properties defined in the struct fields
func (svr *MySql) Open(parentSegment ...*xray.XRayParentSegment) error {
	if !xray.XRayServiceOn() {
		return svr.openNormal()
	} else {
		if len(parentSegment) > 0 {
			svr.mux.Lock()
			svr._parentSegment = parentSegment[0]
			svr.mux.Unlock()
		}

		return svr.openWithXRay()
	}
}

// openNormal opens a database by connecting to it, using the dsn properties defined in the struct fields
func (svr *MySql) openNormal() error {
	// clean up first
	// clear _parentSegment since XRay is not in use
	svr.mux.Lock()
	svr.db = nil
	svr._parentSegment = nil
	svr.mux.Unlock()
	svr.cleanUpAllSqlTransactions()

	// declare
	var str string
	var err error

	// get connect string
	str, err = svr.GetDsn()

	if err != nil {
		return err
	}

	// validate connection string
	if len(str) == 0 {
		return errors.New("MySQL Server Connect String Generated Cannot Be Empty")
	}

	// now ready to open mysql database
	newDB, err := sqlx.Open("mysql", str)

	if err != nil {
		return err
	}

	// test mysql server state object
	if err = newDB.Ping(); err != nil {
		return err
	}

	if svr.MaxOpenConns > 0 {
		newDB.SetMaxOpenConns(svr.MaxOpenConns)
	}

	if svr.MaxIdleConns > 0 {
		if svr.MaxIdleConns <= svr.MaxOpenConns || svr.MaxOpenConns == 0 {
			newDB.SetMaxIdleConns(svr.MaxIdleConns)
		} else {
			newDB.SetMaxIdleConns(svr.MaxOpenConns)
		}
	}

	if svr.MaxConnIdleTime > 0 {
		newDB.SetConnMaxIdleTime(svr.MaxConnIdleTime)
	}

	newDB.SetConnMaxLifetime(0)

	// set db and lastPing under lock
	svr.mux.Lock()
	svr.db = newDB
	svr.lastPing = time.Now()
	svr.mux.Unlock()

	// mysql server state object successfully opened
	return nil
}

// openWithXRay opens a database by connecting to it, wrap with XRay tracing, using the dsn properties defined in the struct fields
func (svr *MySql) openWithXRay() (err error) {
	svr.mux.RLock()
	parentSeg := svr._parentSegment
	svr.mux.RUnlock()

	trace := xray.NewSegment("MySql-Open-Entry", parentSeg)
	defer trace.Close()
	defer func() {
		_ = trace.Seg.AddMetadata("DB-Host", svr.Host)
		_ = trace.Seg.AddMetadata("DB-Database", svr.Database)
		_ = trace.Seg.AddMetadata("DB-UserName", svr.UserName)
		_ = trace.Seg.AddMetadata("DB-Charset", svr.Charset)
		_ = trace.Seg.AddMetadata("DB-Collation", svr.Collation)
		_ = trace.Seg.AddMetadata("DB-ConnectTimeout", svr.ConnectTimeout)
		_ = trace.Seg.AddMetadata("DB-ReadTimeout", svr.ReadTimeout)
		_ = trace.Seg.AddMetadata("DB-WriteTimeout", svr.WriteTimeout)

		if err != nil {
			_ = trace.Seg.AddError(err)
		}
	}()

	// clean up first
	// note: _parentSegment is NOT cleared here - it was set by Open() and will be used by all query methods
	svr.mux.Lock()
	svr.db = nil
	svr.mux.Unlock()
	svr.cleanUpAllSqlTransactions()

	// declare
	var str string

	trace.Capture("Get-DSN", func() error {
		// get connect string
		str, err = svr.GetDsn()
		return err
	})

	if err != nil {
		return err
	}

	// validate connection string
	if len(str) == 0 {
		err = errors.New("MySQL Server Connect String Generated Cannot Be Empty")
		return err
	}

	var newDB *sqlx.DB
	trace.Capture("Get-SQL-Context", func() error {
		// now ready to open mysql database
		baseDb, e := awsxray.SQLContext("mysql", str)

		if e != nil {
			err = fmt.Errorf("openWithXRay() Failed During xray.SQLContext(): %s", e.Error())
			return err
		}

		newDB = sqlx.NewDb(baseDb, "mysql")
		return nil
	})

	if err != nil {
		return err
	}

	// test mysql server state object
	subTrace := trace.NewSubSegment("MySql-Open-Ping")
	defer subTrace.Close()
	defer func() {
		if err != nil {
			_ = subTrace.Seg.AddError(err)
		}
	}()

	subTrace.Capture("Ping", func() error {
		if err = newDB.PingContext(trace.Ctx); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return err
	}

	if svr.MaxOpenConns > 0 {
		newDB.SetMaxOpenConns(svr.MaxOpenConns)
	}

	if svr.MaxIdleConns > 0 {
		if svr.MaxIdleConns <= svr.MaxOpenConns || svr.MaxOpenConns == 0 {
			newDB.SetMaxIdleConns(svr.MaxIdleConns)
		} else {
			newDB.SetMaxIdleConns(svr.MaxOpenConns)
		}
	}

	if svr.MaxConnIdleTime > 0 {
		newDB.SetConnMaxIdleTime(svr.MaxConnIdleTime)
	}

	newDB.SetConnMaxLifetime(0)

	// set db and lastPing under lock
	svr.mux.Lock()
	svr.db = newDB
	svr.lastPing = time.Now()
	svr.mux.Unlock()

	// mysql server state object successfully opened
	return nil
}

// Close will close the database connection and set db to nil
func (svr *MySql) Close() error {
	svr.cleanUpAllSqlTransactions()

	svr.mux.Lock()
	db := svr.db
	svr.db = nil
	svr._parentSegment = nil
	svr.lastPing = time.Now()
	svr.mux.Unlock()

	if db != nil {
		if err := db.Close(); err != nil {
			return err
		}
	}

	return nil
}

// Ping tests if current database connection is still active and ready
func (svr *MySql) Ping() (err error) {
	svr.mux.RLock()
	db := svr.db
	lastPing := svr.lastPing
	parentSeg := svr._parentSegment
	svr.mux.RUnlock()

	if db == nil {
		return errors.New("MySQL Server Not Connected")
	}

	if time.Since(lastPing) < 90*time.Second {
		return nil
	}

	if !xray.XRayServiceOn() {
		if err := db.Ping(); err != nil {
			return err
		}
	} else {
		trace := xray.NewSegment("MySql-Ping", parentSeg)
		defer trace.Close()
		defer func() {
			_ = trace.Seg.AddMetadata("Ping-Timestamp-UTC", time.Now().UTC())

			if err != nil {
				_ = trace.Seg.AddError(err)
			}
		}()

		if err = db.PingContext(trace.Ctx); err != nil {
			return err
		}
	}

	// database ok
	svr.mux.Lock()
	svr.lastPing = time.Now()
	svr.mux.Unlock()
	return nil
}

// Begin starts a mysql transaction, and stores the transaction object into txMap until commit or rollback.
// ensure that transaction related actions are executed from the MySqlTransaction object.
func (svr *MySql) Begin() (*MySqlTransaction, error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		// begin failed
		return nil, err
	}

	svr.mux.RLock()
	db := svr.db
	parentSeg := svr._parentSegment
	svr.mux.RUnlock()

	if !xray.XRayServiceOn() {
		// begin transaction on database
		if tx, err := db.Beginx(); err != nil {
			// begin failed
			return nil, err
		} else {
			// begin succeeded, create MySqlTransaction and return result
			myTx := &MySqlTransaction{
				id:         util.NewULID(),
				parent:     svr,
				tx:         tx,
				closed:     false,
				_xrayTxSeg: nil,
			}
			svr.mux.Lock()
			if svr.txMap == nil {
				svr.txMap = make(map[string]*MySqlTransaction)
			}
			svr.txMap[myTx.id] = myTx
			svr.mux.Unlock()
			return myTx, nil
		}
	} else {
		// begin transaction on database
		xseg := xray.NewSegment("MySql-Transaction", parentSeg)
		subXSeg := xseg.NewSubSegment("Begin-Transaction")

		if tx, err := db.BeginTxx(subXSeg.Ctx, &sql.TxOptions{Isolation: 0, ReadOnly: false}); err != nil {
			// begin failed
			_ = subXSeg.Seg.AddError(err)
			_ = xseg.Seg.AddError(err)
			subXSeg.Close()
			xseg.Close()
			return nil, err
		} else {
			// begin succeeded, create MySqlTransaction and return result
			myTx := &MySqlTransaction{
				id:         util.NewULID(),
				parent:     svr,
				tx:         tx,
				closed:     false,
				_xrayTxSeg: xseg,
			}
			svr.mux.Lock()
			if svr.txMap == nil {
				svr.txMap = make(map[string]*MySqlTransaction)
			}
			svr.txMap[myTx.id] = myTx
			svr.mux.Unlock()

			subXSeg.Close()
			return myTx, nil
		}
	}
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
func (svr *MySql) GetStructSlice(dest interface{}, query string, args ...interface{}) (notFound bool, retErr error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return false, err
	}

	svr.mux.RLock()
	db := svr.db
	parentSeg := svr._parentSegment
	svr.mux.RUnlock()

	// perform select action, and unmarshal result rows into target struct slice
	var err error

	// not in transaction mode
	// query using db object
	if !xray.XRayServiceOn() {
		err = db.Select(dest, query, args...)
	} else {
		trace := xray.NewSegment("MySql-Select-GetStructSlice", parentSeg)
		defer trace.Close()
		defer func() {
			_ = trace.Seg.AddMetadata("SQL-Query", query)
			_ = trace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = trace.Seg.AddMetadata("Struct-Slice-Result", dest)
			if err != nil {
				_ = trace.Seg.AddError(err)
			}
		}()

		err = db.SelectContext(trace.Ctx, dest, query, args...)
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
func (t *MySqlTransaction) GetStructSlice(dest interface{}, query string, args ...interface{}) (notFound bool, retErr error) {
	if retErr = t.ready(); retErr != nil {
		return false, retErr
	}

	// verify if the database connection is good
	if retErr = t.parent.Ping(); retErr != nil {
		return false, retErr
	}

	// perform select action, and unmarshal result rows into target struct slice
	var err error

	// in transaction mode
	// query using tx object
	if t._xrayTxSeg == nil {
		err = t.tx.Select(dest, query, args...)
	} else {
		subTrace := t._xrayTxSeg.NewSubSegment("Transaction-Select-GetStructSlice")
		defer subTrace.Close()
		defer func() {
			_ = subTrace.Seg.AddMetadata("SQL-Query", query)
			_ = subTrace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = subTrace.Seg.AddMetadata("Struct-Slice-Result", dest)
			if err != nil {
				_ = subTrace.Seg.AddError(err)
			}
		}()

		err = t.tx.SelectContext(subTrace.Ctx, dest, query, args...)
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
//
//	dest = pointer to struct or address of struct, this is the result of row to be marshaled into this struct
//	query = sql query, optionally having parameters marked as ?, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. notFound = indicates no rows found in query (aka sql.ErrNoRows), if error is detected, notFound is always false
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is nil)
func (svr *MySql) GetStruct(dest interface{}, query string, args ...interface{}) (notFound bool, retErr error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return false, err
	}

	svr.mux.RLock()
	db := svr.db
	parentSeg := svr._parentSegment
	svr.mux.RUnlock()

	// perform select action, and unmarshal result row (single row) into target struct (single object)
	var err error

	// not in transaction mode
	// query using db object
	if !xray.XRayServiceOn() {
		err = db.Get(dest, query, args...)
	} else {
		trace := xray.NewSegment("MySql-Select-GetStruct", parentSeg)
		defer trace.Close()
		defer func() {
			_ = trace.Seg.AddMetadata("SQL-Query", query)
			_ = trace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = trace.Seg.AddMetadata("Struct-Result", dest)
			if err != nil {
				_ = trace.Seg.AddError(err)
			}
		}()

		err = db.GetContext(trace.Ctx, dest, query, args...)
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
//
//	dest = pointer to struct or address of struct, this is the result of row to be marshaled into this struct
//	query = sql query, optionally having parameters marked as ?, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. notFound = indicates no rows found in query (aka sql.ErrNoRows), if error is detected, notFound is always false
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is nil)
func (t *MySqlTransaction) GetStruct(dest interface{}, query string, args ...interface{}) (notFound bool, retErr error) {
	if retErr = t.ready(); retErr != nil {
		return false, retErr
	}

	// verify if the database connection is good
	if retErr = t.parent.Ping(); retErr != nil {
		return false, retErr
	}

	// perform select action, and unmarshal result row (single row) into target struct (single object)
	var err error

	// in transaction mode
	// query using tx object
	if t._xrayTxSeg == nil {
		err = t.tx.Get(dest, query, args...)
	} else {
		subTrace := t._xrayTxSeg.NewSubSegment("Transaction-Select-GetStruct")
		defer subTrace.Close()
		defer func() {
			_ = subTrace.Seg.AddMetadata("SQL-Query", query)
			_ = subTrace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = subTrace.Seg.AddMetadata("Struct-Result", dest)
			if err != nil {
				_ = subTrace.Seg.AddError(err)
			}
		}()

		err = t.tx.GetContext(subTrace.Ctx, dest, query, args...)
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
//
//	query = sql query, optionally having parameters marked as ?, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. *sqlx.Rows = pointer to sqlx.Rows; or nil if no rows yielded
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and sqlx.Rows is returned as nil)
//
// [ Ranged Loop & Scan ]
//  1. to loop, use:
//     for {
//     if !rows.Next() { break }
//     rows.Scan(&x, &y, etc)
//     }
//  2. to scan, use: r.Scan(&x, &y, ...), where r is the row struct in loop, where &x &y etc are the scanned output value (scan in order of select columns sequence)
//
// [ Continuous Loop & Scan ]
//  1. Continuous loop until endOfRows = true is yielded from ScanSlice() or ScanStruct()
//  2. ScanSlice(): accepts *sqlx.Rows, scans rows result into target pointer slice (if no error, endOfRows = true is returned)
//  3. ScanStruct(): accepts *sqlx.Rows, scans current single row result into target pointer struct, returns endOfRows as true of false; if endOfRows = true, loop should stop
func (svr *MySql) GetRowsByOrdinalParams(query string, args ...interface{}) (*sqlx.Rows, error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return nil, err
	}

	svr.mux.RLock()
	db := svr.db
	parentSeg := svr._parentSegment
	svr.mux.RUnlock()

	// perform select action, and return sqlx rows
	var rows *sqlx.Rows
	var err error

	// not in transaction mode
	// query using db object
	if !xray.XRayServiceOn() {
		rows, err = db.Queryx(query, args...)
	} else {
		trace := xray.NewSegment("MySql-Select-GetRowsByOrdinalParams", parentSeg)
		defer trace.Close()
		defer func() {
			_ = trace.Seg.AddMetadata("SQL-Query", query)
			_ = trace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = trace.Seg.AddMetadata("Rows-Result", rows)
			if err != nil {
				_ = trace.Seg.AddError(err)
			}
		}()

		rows, err = db.QueryxContext(trace.Ctx, query, args...)
	}

	// if err is sql.ErrNoRows then treat as no error
	if err != nil && err == sql.ErrNoRows {
		rows = nil
		err = nil
	}

	// return result
	return rows, err
}

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
//  1. to loop, use:
//     for {
//     if !rows.Next() { break }
//     rows.Scan(&x, &y, etc)
//     }
//  2. to scan, use: r.Scan(&x, &y, ...), where r is the row struct in loop, where &x &y etc are the scanned output value (scan in order of select columns sequence)
//
// [ Continuous Loop & Scan ]
//  1. Continuous loop until endOfRows = true is yielded from ScanSlice() or ScanStruct()
//  2. ScanSlice(): accepts *sqlx.Rows, scans rows result into target pointer slice (if no error, endOfRows = true is returned)
//  3. ScanStruct(): accepts *sqlx.Rows, scans current single row result into target pointer struct, returns endOfRows as true of false; if endOfRows = true, loop should stop
func (t *MySqlTransaction) GetRowsByOrdinalParams(query string, args ...interface{}) (*sqlx.Rows, error) {
	if err := t.ready(); err != nil {
		return nil, err
	}

	// verify if the database connection is good
	if err := t.parent.Ping(); err != nil {
		return nil, err
	}

	// perform select action, and return sqlx rows
	var rows *sqlx.Rows
	var err error

	// in transaction mode
	// query using tx object
	if t._xrayTxSeg == nil {
		rows, err = t.tx.Queryx(query, args...)
	} else {
		subTrace := t._xrayTxSeg.NewSubSegment("Transaction-Select-GetRowsByOrdinalParams")
		defer subTrace.Close()
		defer func() {
			_ = subTrace.Seg.AddMetadata("SQL-Query", query)
			_ = subTrace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = subTrace.Seg.AddMetadata("Rows-Result", rows)
			if err != nil {
				_ = subTrace.Seg.AddError(err)
			}
		}()

		rows, err = t.tx.QueryxContext(subTrace.Ctx, query, args...)
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
//  1. to loop, use:
//     for {
//     if !rows.Next() { break }
//     rows.Scan(&x, &y, etc)
//     }
//  2. to scan, use: r.Scan(&x, &y, ...), where r is the row struct in loop, where &x &y etc are the scanned output value (scan in order of select columns sequence)
//
// [ Continuous Loop & Scan ]
//  1. Continuous loop until endOfRows = true is yielded from ScanSlice() or ScanStruct()
//  2. ScanSlice(): accepts *sqlx.Rows, scans rows result into target pointer slice (if no error, endOfRows = true is returned)
//  3. ScanStruct(): accepts *sqlx.Rows, scans current single row result into target pointer struct, returns endOfRows as true of false; if endOfRows = true, loop should stop
func (svr *MySql) GetRowsByNamedMapParam(query string, args map[string]interface{}) (*sqlx.Rows, error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return nil, err
	}

	svr.mux.RLock()
	db := svr.db
	parentSeg := svr._parentSegment
	svr.mux.RUnlock()

	// perform select action, and return sqlx rows
	var rows *sqlx.Rows
	var err error

	// not in transaction mode
	// query using db object
	if !xray.XRayServiceOn() {
		rows, err = db.NamedQuery(query, args)
	} else {
		trace := xray.NewSegment("MySql-Select-GetRowsByNamedMapParam", parentSeg)
		defer trace.Close()
		defer func() {
			_ = trace.Seg.AddMetadata("SQL-Query", query)
			_ = trace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = trace.Seg.AddMetadata("Rows-Result", rows)
			if err != nil {
				_ = trace.Seg.AddError(err)
			}
		}()

		rows, err = db.NamedQueryContext(trace.Ctx, query, args)
	}

	if err != nil && err == sql.ErrNoRows {
		// no rows
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
//  1. to loop, use:
//     for {
//     if !rows.Next() { break }
//     rows.Scan(&x, &y, etc)
//     }
//  2. to scan, use: r.Scan(&x, &y, ...), where r is the row struct in loop, where &x &y etc are the scanned output value (scan in order of select columns sequence)
//
// [ Continuous Loop & Scan ]
//  1. Continuous loop until endOfRows = true is yielded from ScanSlice() or ScanStruct()
//  2. ScanSlice(): accepts *sqlx.Rows, scans rows result into target pointer slice (if no error, endOfRows = true is returned)
//  3. ScanStruct(): accepts *sqlx.Rows, scans current single row result into target pointer struct, returns endOfRows as true of false; if endOfRows = true, loop should stop
func (t *MySqlTransaction) GetRowsByNamedMapParam(query string, args map[string]interface{}) (*sqlx.Rows, error) {
	if err := t.ready(); err != nil {
		return nil, err
	}

	// verify if the database connection is good
	if err := t.parent.Ping(); err != nil {
		return nil, err
	}

	// perform select action, and return sqlx rows
	var rows *sqlx.Rows
	var err error

	// in transaction mode
	// query using tx object
	if t._xrayTxSeg == nil {
		rows, err = t.tx.NamedQuery(query, args)
	} else {
		subTrace := t._xrayTxSeg.NewSubSegment("Transaction-Select-GetRowsByNamedMapParam")
		defer subTrace.Close()
		defer func() {
			_ = subTrace.Seg.AddMetadata("SQL-Query", query)
			_ = subTrace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = subTrace.Seg.AddMetadata("Rows-Result", rows)
			if err != nil {
				_ = subTrace.Seg.AddError(err)
			}
		}()

		rows, err = sqlx.NamedQueryContext(subTrace.Ctx, t.tx, query, args)
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
//  1. to loop, use:
//     for {
//     if !rows.Next() { break }
//     rows.Scan(&x, &y, etc)
//     }
//  2. to scan, use: r.Scan(&x, &y, ...), where r is the row struct in loop, where &x &y etc are the scanned output value (scan in order of select columns sequence)
//
// [ Continuous Loop & Scan ]
//  1. Continuous loop until endOfRows = true is yielded from ScanSlice() or ScanStruct()
//  2. ScanSlice(): accepts *sqlx.Rows, scans rows result into target pointer slice (if no error, endOfRows = true is returned)
//  3. ScanStruct(): accepts *sqlx.Rows, scans current single row result into target pointer struct, returns endOfRows as true of false; if endOfRows = true, loop should stop
func (svr *MySql) GetRowsByStructParam(query string, args interface{}) (*sqlx.Rows, error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return nil, err
	}

	svr.mux.RLock()
	db := svr.db
	parentSeg := svr._parentSegment
	svr.mux.RUnlock()

	// perform select action, and return sqlx rows
	var rows *sqlx.Rows
	var err error

	// not in transaction mode
	// query using db object
	if !xray.XRayServiceOn() {
		rows, err = db.NamedQuery(query, args)
	} else {
		trace := xray.NewSegment("MySql-Select-GetRowsByStructParam", parentSeg)
		defer trace.Close()
		defer func() {
			_ = trace.Seg.AddMetadata("SQL-Query", query)
			_ = trace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = trace.Seg.AddMetadata("Rows-Result", rows)
			if err != nil {
				_ = trace.Seg.AddError(err)
			}
		}()

		rows, err = db.NamedQueryContext(trace.Ctx, query, args)
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
//  1. to loop, use:
//     for {
//     if !rows.Next() { break }
//     rows.Scan(&x, &y, etc)
//     }
//  2. to scan, use: r.Scan(&x, &y, ...), where r is the row struct in loop, where &x &y etc are the scanned output value (scan in order of select columns sequence)
//
// [ Continuous Loop & Scan ]
//  1. Continuous loop until endOfRows = true is yielded from ScanSlice() or ScanStruct()
//  2. ScanSlice(): accepts *sqlx.Rows, scans rows result into target pointer slice (if no error, endOfRows = true is returned)
//  3. ScanStruct(): accepts *sqlx.Rows, scans current single row result into target pointer struct, returns endOfRows as true of false; if endOfRows = true, loop should stop
func (t *MySqlTransaction) GetRowsByStructParam(query string, args interface{}) (*sqlx.Rows, error) {
	if err := t.ready(); err != nil {
		return nil, err
	}

	// verify if the database connection is good
	if err := t.parent.Ping(); err != nil {
		return nil, err
	}

	// perform select action, and return sqlx rows
	var rows *sqlx.Rows
	var err error

	// in transaction mode
	// query using tx object
	if t._xrayTxSeg == nil {
		rows, err = t.tx.NamedQuery(query, args)
	} else {
		subTrace := t._xrayTxSeg.NewSubSegment("Transaction-Select-GetRowsByStructParam")
		defer subTrace.Close()
		defer func() {
			_ = subTrace.Seg.AddMetadata("SQL-Query", query)
			_ = subTrace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = subTrace.Seg.AddMetadata("Rows-Result", rows)
			if err != nil {
				_ = subTrace.Seg.AddError(err)
			}
		}()

		rows, err = sqlx.NamedQueryContext(subTrace.Ctx, t.tx, query, args)
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
//
//	rows = *sqlx.Rows
//	dest = pointer to slice, such as: variable to "*[]interface{}", this will receive the scanned data
//
// [ Return Values ]
//  1. endOfRows = true if this action call yielded end of rows, meaning stop further processing of current loop (rows will be closed automatically)
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is set as nil)
func (svr *MySql) ScanSlice(rows *sqlx.Rows, dest *[]interface{}) (endOfRows bool, err error) {
	// ensure rows pointer is set
	if rows == nil {
		return true, nil
	}

	// call rows.Next() first to position the row
	if !xray.XRayServiceOn() {
		if rows.Next() {
			// now slice scan
			var scanned []interface{}
			scanned, err = rows.SliceScan()

			// if err is sql.ErrNoRows then treat as no error
			if err != nil && err == sql.ErrNoRows {
				endOfRows = true
				if dest != nil {
					*dest = nil
				}
				err = nil
				_ = rows.Close()
				return
			}

			if err != nil {
				// has error
				endOfRows = false // although error but may not be at end of rows
				if dest != nil {
					*dest = nil
				}
				_ = rows.Close()
				return
			}

			// slice scan success, but may not be at end of rows
			// assign scanned data to caller's destination
			if dest != nil {
				*dest = scanned
			}
			return false, nil
		} else {
			endOfRows = true
			if dest != nil {
				*dest = nil
			}
			err = nil
			_ = rows.Close()
		}
	} else {
		svr.mux.RLock()
		parentSeg := svr._parentSegment
		svr.mux.RUnlock()

		trace := xray.NewSegment("MySql-InMemory-ScanSlice", parentSeg)
		defer trace.Close()
		defer func() {
			if err != nil {
				_ = trace.Seg.AddError(err)
			}
		}()

		trace.Capture("ScanSlice-Do", func() error {
			if rows.Next() {
				// now slice scan
				var scanned []interface{}
				scanned, err = rows.SliceScan()

				// if err is sql.ErrNoRows then treat as no error
				if err != nil && err == sql.ErrNoRows {
					endOfRows = true
					if dest != nil {
						*dest = nil
					}
					err = nil
					_ = rows.Close()
					return nil
				}

				if err != nil {
					// has error
					endOfRows = false // although error but may not be at end of rows
					if dest != nil {
						*dest = nil
					}
					_ = rows.Close()
					return err
				}

				// slice scan success, but may not be at end of rows
				// assign scanned data to caller's destination
				if dest != nil {
					*dest = scanned
				}
				endOfRows = false
				return nil
			} else {
				endOfRows = true
				if dest != nil {
					*dest = nil
				}
				err = nil
				_ = rows.Close()
				return nil
			}
		}, &xray.XTraceData{
			Meta: map[string]interface{}{
				"Rows-To-Scan": rows,
				"Slice-Result": dest,
				"End-Of-Rows":  endOfRows,
			},
		})
	}

	// no more rows
	return endOfRows, err
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
//  1. endOfRows = true if this action call yielded end of rows, meaning stop further processing of current loop (rows will be closed automatically)
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is set as nil)
func (svr *MySql) ScanStruct(rows *sqlx.Rows, dest interface{}) (endOfRows bool, err error) {
	// ensure rows pointer is set
	if rows == nil {
		return true, nil
	}

	// call rows.Next() first to position the row
	if !xray.XRayServiceOn() {
		if rows.Next() {
			// now struct scan
			err = rows.StructScan(dest)

			// if err is sql.ErrNoRows then treat as no error
			if err != nil && err == sql.ErrNoRows {
				endOfRows = true
				err = nil
				_ = rows.Close()
				return
			}

			if err != nil {
				// has error
				endOfRows = false // although error but may not be at end of rows
				_ = rows.Close()
				return
			}

			// struct scan successful, but may not be at end of rows
			return false, nil
		} else {
			endOfRows = true
			err = nil
			_ = rows.Close()
		}
	} else {
		svr.mux.RLock()
		parentSeg := svr._parentSegment
		svr.mux.RUnlock()

		trace := xray.NewSegment("MySql-InMemory-ScanStruct", parentSeg)
		defer trace.Close()
		defer func() {
			if err != nil {
				_ = trace.Seg.AddError(err)
			}
		}()

		trace.Capture("ScanStruct-Do", func() error {
			if rows.Next() {
				// now struct scan
				err = rows.StructScan(dest)

				// if err is sql.ErrNoRows then treat as no error
				if err != nil && err == sql.ErrNoRows {
					endOfRows = true
					err = nil
					_ = rows.Close()
					return nil
				}

				if err != nil {
					// has error
					endOfRows = false // although error but may not be at end of rows
					_ = rows.Close()
					return err
				}

				// struct scan successful, but may not be at end of rows
				endOfRows = false
				return nil
			} else {
				endOfRows = true
				err = nil
				_ = rows.Close()
				return nil
			}
		}, &xray.XTraceData{
			Meta: map[string]interface{}{
				"Rows-To-Scan":  rows,
				"Struct-Result": dest,
				"End-Of-Rows":   endOfRows,
			},
		})
	}

	// no more rows
	return endOfRows, err
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
func (svr *MySql) GetSingleRow(query string, args ...interface{}) (*sqlx.Row, error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return nil, err
	}

	svr.mux.RLock()
	db := svr.db
	parentSeg := svr._parentSegment
	svr.mux.RUnlock()

	// perform select action, and return sqlx row
	var row *sqlx.Row
	var err error

	// not in transaction mode
	// query using db object
	if !xray.XRayServiceOn() {
		row = db.QueryRowx(query, args...)
	} else {
		trace := xray.NewSegment("MySql-Select-GetSingleRow", parentSeg)
		defer trace.Close()
		defer func() {
			_ = trace.Seg.AddMetadata("SQL-Query", query)
			_ = trace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = trace.Seg.AddMetadata("Row-Result", row)
			if err != nil {
				_ = trace.Seg.AddError(err)
			}
		}()

		row = db.QueryRowxContext(trace.Ctx, query, args...)
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
func (t *MySqlTransaction) GetSingleRow(query string, args ...interface{}) (*sqlx.Row, error) {
	if err := t.ready(); err != nil {
		return nil, err
	}

	// verify if the database connection is good
	if err := t.parent.Ping(); err != nil {
		return nil, err
	}

	// perform select action, and return sqlx row
	var row *sqlx.Row
	var err error

	// in transaction mode
	// query using tx object
	if t._xrayTxSeg == nil {
		row = t.tx.QueryRowx(query, args...)
	} else {
		subTrace := t._xrayTxSeg.NewSubSegment("Transaction-Select-GetSingleRow")
		defer subTrace.Close()
		defer func() {
			_ = subTrace.Seg.AddMetadata("SQL-Query", query)
			_ = subTrace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = subTrace.Seg.AddMetadata("Row-Result", row)
			if err != nil {
				_ = subTrace.Seg.AddError(err)
			}
		}()

		row = t.tx.QueryRowxContext(subTrace.Ctx, query, args...)
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
//
//	row = *sqlx.Row
//	dest = pointer to slice, such as: variable to "*[]interface{}", this will receive the scanned data
//
// [ Return Values ]
//  1. notFound = true if no row is found in current scan
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is set as nil and notFound is true)
func (svr *MySql) ScanSliceByRow(row *sqlx.Row, dest *[]interface{}) (notFound bool, err error) {
	// if row is nil, treat as no row and not an error
	if row == nil {
		if dest != nil {
			*dest = nil
		}
		return true, nil
	}

	// perform slice scan on the given row
	if !xray.XRayServiceOn() {
		var scanned []interface{}
		scanned, err = row.SliceScan()

		// if err is sql.ErrNoRows then treat as no error
		if err != nil && err == sql.ErrNoRows {
			if dest != nil {
				*dest = nil
			}
			return true, nil
		}

		if err != nil {
			// has error
			if dest != nil {
				*dest = nil
			}
			return false, err // although error but may not be not found
		}

		// assign scanned data to caller's destination
		if dest != nil {
			*dest = scanned
		}
		notFound = false
		err = nil
	} else {
		svr.mux.RLock()
		parentSeg := svr._parentSegment
		svr.mux.RUnlock()

		trace := xray.NewSegment("MySql-InMemory-ScanSliceByRow", parentSeg)
		defer trace.Close()
		defer func() {
			if err != nil {
				_ = trace.Seg.AddError(err)
			}
		}()

		trace.Capture("ScanSliceByRow-Do", func() error {
			var scanned []interface{}
			scanned, err = row.SliceScan()

			// if err is sql.ErrNoRows then treat as no error
			if err != nil && err == sql.ErrNoRows {
				if dest != nil {
					*dest = nil
				}
				notFound = true
				err = nil
				return nil
			}

			if err != nil {
				// has error
				notFound = false
				if dest != nil {
					*dest = nil
				}
				return err // although error but may not be not found
			}

			// assign scanned data to caller's destination
			if dest != nil {
				*dest = scanned
			}
			notFound = false
			err = nil
			return nil
		}, &xray.XTraceData{
			Meta: map[string]interface{}{
				"Row-To-Scan":  row,
				"Slice-Result": dest,
			},
		})
	}

	// slice scan success
	return notFound, err
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
func (svr *MySql) ScanStructByRow(row *sqlx.Row, dest interface{}) (notFound bool, err error) {
	// if row is nil, treat as no row and not an error
	if row == nil {
		return true, nil
	}

	// now struct scan
	if !xray.XRayServiceOn() {
		err = row.StructScan(dest)

		// if err is sql.ErrNoRows then treat as no error
		if err != nil && err == sql.ErrNoRows {
			return true, nil
		}

		if err != nil {
			// has error
			return false, err // although error but may not be not found
		}

		notFound = false
		err = nil
	} else {
		svr.mux.RLock()
		parentSeg := svr._parentSegment
		svr.mux.RUnlock()

		trace := xray.NewSegment("MySql-InMemory-ScanStructByRow", parentSeg)
		defer trace.Close()
		defer func() {
			if err != nil {
				_ = trace.Seg.AddError(err)
			}
		}()

		trace.Capture("ScanStructByRow-Do", func() error {
			err = row.StructScan(dest)

			// if err is sql.ErrNoRows then treat as no error
			if err != nil && err == sql.ErrNoRows {
				notFound = true
				err = nil
				return nil
			}

			if err != nil {
				// has error
				notFound = false
				return err // although error but may not be not found
			}

			notFound = false
			err = nil
			return nil
		}, &xray.XTraceData{
			Meta: map[string]interface{}{
				"Row-To-Scan":   row,
				"Struct-Result": dest,
			},
		})
	}

	// struct scan successful
	return notFound, err
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
func (svr *MySql) ScanColumnsByRow(row *sqlx.Row, dest ...interface{}) (notFound bool, err error) {
	// if row is nil, treat as no row and not an error
	if row == nil {
		return true, nil
	}

	// now scan columns from row
	if !xray.XRayServiceOn() {
		err = row.Scan(dest...)

		// if err is sql.ErrNoRows then treat as no error
		if err != nil && err == sql.ErrNoRows {
			return true, nil
		}

		if err != nil {
			// has error
			return false, err // although error but may not be not found
		}

		notFound = false
		err = nil
	} else {
		svr.mux.RLock()
		parentSeg := svr._parentSegment
		svr.mux.RUnlock()

		trace := xray.NewSegment("MySql-InMemory-ScanColumnsByRow", parentSeg)
		defer trace.Close()
		defer func() {
			if err != nil {
				_ = trace.Seg.AddError(err)
			}
		}()

		trace.Capture("ScanColumnsByRow_Do", func() error {
			err = row.Scan(dest...)

			// if err is sql.ErrNoRows then treat as no error
			if err != nil && err == sql.ErrNoRows {
				notFound = true
				err = nil
				return nil
			}

			if err != nil {
				// has error
				notFound = false
				return err // although error but may not be not found
			}

			notFound = false
			err = nil
			return nil
		}, &xray.XTraceData{
			Meta: map[string]interface{}{
				"Row-To-Scan":      row,
				"Dest-Vars-Result": dest,
			},
		})
	}

	// scan columns successful
	return notFound, err
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
func (svr *MySql) GetScalarString(query string, args ...interface{}) (retVal string, retNotFound bool, retErr error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return "", false, err
	}

	svr.mux.RLock()
	db := svr.db
	parentSeg := svr._parentSegment
	svr.mux.RUnlock()

	// get row using query string and parameters
	var row *sqlx.Row

	// not in transaction
	// use db object
	if !xray.XRayServiceOn() {
		row = db.QueryRowx(query, args...)
	} else {
		trace := xray.NewSegment("MySql-Select-GetScalarString", parentSeg)
		defer trace.Close()
		defer func() {
			_ = trace.Seg.AddMetadata("SQL-Query", query)
			_ = trace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = trace.Seg.AddMetadata("Row-Result", row)
			if retErr != nil {
				_ = trace.Seg.AddError(retErr)
			}
		}()

		row = db.QueryRowxContext(trace.Ctx, query, args...)
	}

	if row == nil {
		return "", false, errors.New("Scalar Query Yielded Empty Row")
	} else {
		retErr = row.Err()

		if retErr != nil {
			if retErr == sql.ErrNoRows {
				// no rows
				retErr = nil
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
		retErr = nil
		return "", true, nil
	} else {
		// return value
		return retVal, false, retErr
	}
}

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
func (t *MySqlTransaction) GetScalarString(query string, args ...interface{}) (retVal string, retNotFound bool, retErr error) {
	if err := t.ready(); err != nil {
		return "", false, err
	}

	// verify if the database connection is good
	if err := t.parent.Ping(); err != nil {
		return "", false, err
	}

	// get row using query string and parameters
	var row *sqlx.Row

	// in transaction
	// use tx object
	if t._xrayTxSeg == nil {
		row = t.tx.QueryRowx(query, args...)
	} else {
		subTrace := t._xrayTxSeg.NewSubSegment("Transaction-Select-GetScalarString")
		defer subTrace.Close()
		defer func() {
			_ = subTrace.Seg.AddMetadata("SQL-Query", query)
			_ = subTrace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = subTrace.Seg.AddMetadata("Row-Result", row)
			if retErr != nil {
				_ = subTrace.Seg.AddError(retErr)
			}
		}()

		row = t.tx.QueryRowxContext(subTrace.Ctx, query, args...)
	}

	if row == nil {
		return "", false, errors.New("Scalar Query Yielded Empty Row")
	} else {
		retErr = row.Err()

		if retErr != nil {
			if retErr == sql.ErrNoRows {
				// no rows
				retErr = nil
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
		retErr = nil
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
func (svr *MySql) GetScalarNullString(query string, args ...interface{}) (retVal sql.NullString, retNotFound bool, retErr error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return sql.NullString{}, false, err
	}

	svr.mux.RLock()
	db := svr.db
	parentSeg := svr._parentSegment
	svr.mux.RUnlock()

	// get row using query string and parameters
	var row *sqlx.Row

	// not in transaction
	// use db object
	if !xray.XRayServiceOn() {
		row = db.QueryRowx(query, args...)
	} else {
		trace := xray.NewSegment("MySql-Select-GetScalarNullString", parentSeg)
		defer trace.Close()
		defer func() {
			_ = trace.Seg.AddMetadata("SQL-Query", query)
			_ = trace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = trace.Seg.AddMetadata("Row-Result", row)
			if retErr != nil {
				_ = trace.Seg.AddError(retErr)
			}
		}()

		row = db.QueryRowxContext(trace.Ctx, query, args...)
	}

	if row == nil {
		retErr = errors.New("Scalar Query Yielded Empty Row")
		return sql.NullString{}, false, retErr
	} else {
		retErr = row.Err()

		if retErr != nil {
			if retErr == sql.ErrNoRows {
				// no rows
				retErr = nil
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
		retErr = nil
		return sql.NullString{}, true, nil
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
func (t *MySqlTransaction) GetScalarNullString(query string, args ...interface{}) (retVal sql.NullString, retNotFound bool, retErr error) {
	if err := t.ready(); err != nil {
		return sql.NullString{}, false, err
	}

	// verify if the database connection is good
	if err := t.parent.Ping(); err != nil {
		return sql.NullString{}, false, err
	}

	// get row using query string and parameters
	var row *sqlx.Row

	// in transaction
	// use tx object
	if t._xrayTxSeg == nil {
		row = t.tx.QueryRowx(query, args...)
	} else {
		subTrace := t._xrayTxSeg.NewSubSegment("Transaction-Select-GetScalarNullString")
		defer subTrace.Close()
		defer func() {
			_ = subTrace.Seg.AddMetadata("SQL-Query", query)
			_ = subTrace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = subTrace.Seg.AddMetadata("Row-Result", row)
			if retErr != nil {
				_ = subTrace.Seg.AddError(retErr)
			}
		}()

		row = t.tx.QueryRowxContext(subTrace.Ctx, query, args...)
	}

	if row == nil {
		retErr = errors.New("Scalar Query Yielded Empty Row")
		return sql.NullString{}, false, retErr
	} else {
		retErr = row.Err()

		if retErr != nil {
			if retErr == sql.ErrNoRows {
				// no rows
				retErr = nil
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
		retErr = nil
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
//	actionQuery = sql action query, optionally having parameters marked as ?1, ?2 .. ?N, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. MySqlResult = represents the sql action result received (including error info if applicable)
func (svr *MySql) ExecByOrdinalParams(actionQuery string, args ...interface{}) MySqlResult {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	svr.mux.RLock()
	db := svr.db
	parentSeg := svr._parentSegment
	svr.mux.RUnlock()

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

	if !xray.XRayServiceOn() {
		// not in transaction mode,
		// action using db object
		result, err = db.Exec(actionQuery, args...)
	} else {
		// not in transaction mode,
		// action using db object
		trace := xray.NewSegment("MySql-Exec-ExecByOrdinalParams", parentSeg)
		defer trace.Close()
		defer func() {
			_ = trace.Seg.AddMetadata("SQL-Query", actionQuery)
			_ = trace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = trace.Seg.AddMetadata("Exec-Result", result)
			if err != nil {
				_ = trace.Seg.AddError(err)
			}
		}()

		result, err = db.ExecContext(trace.Ctx, actionQuery, args...)
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

// ExecByOrdinalParams executes action query string and parameters to return result, if error, returns error object within result
// [ Parameters ]
//
//	actionQuery = sql action query, optionally having parameters marked as ?1, ?2 .. ?N, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. MySqlResult = represents the sql action result received (including error info if applicable)
func (t *MySqlTransaction) ExecByOrdinalParams(actionQuery string, args ...interface{}) MySqlResult {
	if err := t.ready(); err != nil {
		return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// verify if the database connection is good
	if err := t.parent.Ping(); err != nil {
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

	if t._xrayTxSeg == nil {
		// in transaction mode,
		// action using tx object
		result, err = t.tx.Exec(actionQuery, args...)
	} else {
		// in transaction mode,
		// action using tx object
		subTrace := t._xrayTxSeg.NewSubSegment("Transaction-Exec-ExecByOrdinalParams")
		defer subTrace.Close()
		defer func() {
			_ = subTrace.Seg.AddMetadata("SQL-Query", actionQuery)
			_ = subTrace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = subTrace.Seg.AddMetadata("Exec-Result", result)
			if err != nil {
				_ = subTrace.Seg.AddError(err)
			}
		}()

		result, err = t.tx.ExecContext(subTrace.Ctx, actionQuery, args...)
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
//  1. MySqlResult = represents the sql action result received (including error info if applicable)
func (svr *MySql) ExecByNamedMapParam(actionQuery string, args map[string]interface{}) MySqlResult {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	svr.mux.RLock()
	db := svr.db
	parentSeg := svr._parentSegment
	svr.mux.RUnlock()

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

	if !xray.XRayServiceOn() {
		// not in transaction mode,
		// action using db object
		result, err = db.NamedExec(actionQuery, args)
	} else {
		// not in transaction mode,
		// action using db object
		trace := xray.NewSegment("MySql-Exec-ExecByNamedMapParam", parentSeg)
		defer trace.Close()
		defer func() {
			_ = trace.Seg.AddMetadata("SQL-Query", actionQuery)
			_ = trace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = trace.Seg.AddMetadata("Exec-Result", result)
			if err != nil {
				_ = trace.Seg.AddError(err)
			}
		}()

		result, err = db.NamedExecContext(trace.Ctx, actionQuery, args)
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
//  1. MySqlResult = represents the sql action result received (including error info if applicable)
func (t *MySqlTransaction) ExecByNamedMapParam(actionQuery string, args map[string]interface{}) MySqlResult {
	if err := t.ready(); err != nil {
		return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// verify if the database connection is good
	if err := t.parent.Ping(); err != nil {
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

	if t._xrayTxSeg == nil {
		// in transaction mode,
		// action using tx object
		result, err = t.tx.NamedExec(actionQuery, args)
	} else {
		// in transaction mode,
		// action using tx object
		subTrace := t._xrayTxSeg.NewSubSegment("Transaction-Exec-ExecByNamedMapParam")
		defer subTrace.Close()
		defer func() {
			_ = subTrace.Seg.AddMetadata("SQL-Query", actionQuery)
			_ = subTrace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = subTrace.Seg.AddMetadata("Exec-Result", result)
			if err != nil {
				_ = subTrace.Seg.AddError(err)
			}
		}()

		result, err = t.tx.NamedExecContext(subTrace.Ctx, actionQuery, args)

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
//  1. in sql = instead of defining ordinal parameters ?, each parameter in sql does not need to be ordinal, rather define with :xyz (must have : in front of param name), where xyz is name of parameter, such as :customerID
//  2. in go = using a struct to contain fields to match parameters, make sure struct tags match to the sql parameter names, such as struct tag `db:"customerID"` must match parameter name in sql as ":customerID" (the : is not part of the match)
//
// [ Parameters ]
//
//	actionQuery = sql action query, with named parameters using :xyz syntax
//	args = required, the struct variable, whose fields having struct tags matching sql parameter names
//
// [ Return Values ]
//  1. MySqlResult = represents the sql action result received (including error info if applicable)
func (svr *MySql) ExecByStructParam(actionQuery string, args interface{}) MySqlResult {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	svr.mux.RLock()
	db := svr.db
	parentSeg := svr._parentSegment
	svr.mux.RUnlock()

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

	if !xray.XRayServiceOn() {
		// not in transaction mode,
		// action using db object
		result, err = db.NamedExec(actionQuery, args)
	} else {
		// not in transaction mode,
		// action using db object
		trace := xray.NewSegment("MySql-Exec-ExecByStructParam", parentSeg)
		defer trace.Close()
		defer func() {
			_ = trace.Seg.AddMetadata("SQL-Query", actionQuery)
			_ = trace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = trace.Seg.AddMetadata("Exec-Result", result)
			if err != nil {
				_ = trace.Seg.AddError(err)
			}
		}()

		result, err = db.NamedExecContext(trace.Ctx, actionQuery, args)
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
//  1. MySqlResult = represents the sql action result received (including error info if applicable)
func (t *MySqlTransaction) ExecByStructParam(actionQuery string, args interface{}) MySqlResult {
	if err := t.ready(); err != nil {
		return MySqlResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// verify if the database connection is good
	if err := t.parent.Ping(); err != nil {
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

	if t._xrayTxSeg == nil {
		// in transaction mode,
		// action using tx object
		result, err = t.tx.NamedExec(actionQuery, args)
	} else {
		// in transaction mode,
		// action using tx object
		subTrace := t._xrayTxSeg.NewSubSegment("Transaction-Exec-ExecByStructParam")
		defer subTrace.Close()
		defer func() {
			_ = subTrace.Seg.AddMetadata("SQL-Query", actionQuery)
			_ = subTrace.Seg.AddMetadata("SQL-Param-Values", args)
			_ = subTrace.Seg.AddMetadata("Exec-Result", result)
			if err != nil {
				_ = subTrace.Seg.AddError(err)
			}
		}()

		result, err = t.tx.NamedExecContext(subTrace.Ctx, actionQuery, args)
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
