package sqlserver

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
	"net/url"
	"strings"
	"sync"

	util "github.com/aldelo/common"
	"github.com/jmoiron/sqlx"

	// this package is used by database/sql as we are wrapping the sql access functionality in this utility package
	_ "github.com/denisenkom/go-mssqldb"
)

// ++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++
// SQLServer struct Usage Guide
// ++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++

/*
	***************************************************************************************************************
	First, Create "../model/global.go"
	***************************************************************************************************************

	package model

	import (
			"errors"
			"time"
			data "github.com/aldelo/common/wrapper/sqlserver"
	)

	// package level accessible to the sqlserver database object
	var db *data.SQLServer

	// SetDB allows code outside of package to set the sqlserver database reference
	func SetDB(dbx *data.SQLServer) {
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
			data "github.com/aldelo/common/wrapper/sqlserver"
			"???/model" // ??? is path to the model package
			...
	)

	...

	func main() {
		...

		// ========================================
		// setup database connection
		// ========================================

		//
		// declare sqlserver object
		//
		s := new(data.SQLServer)

		//
		// set sqlserver dsn fields
		//
		s.AppName = ""		// application name from the calling agent
		s.Host = "" 		// from aws aurora endpoint
		s.Port = 0 			// custom port number if applicable (0 will ignore this field)
		s.Database = ""		// database name
		s.UserName = ""		// database server user name
		s.Password = ""		// database server user password
		s.Encrypted = false	// set to false to not use encryption
		s.Instance = ""		// optional
		s.Timeout = 15		// seconds

		//
		// open sqlserver database connection
		//
		if err := s.Open(); err != nil {
			s.Close()
		} else {
			// add sqlserver object to model global
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
	Third, Using SqlServer Struct
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

// SQLServer struct encapsulates the SQLServer database access functionality (using sqlx package)
type SQLServer struct {
	// SQLServer connection properties
	Host      string
	Port      int
	Instance  string
	Database  string
	UserName  string
	Password  string
	Timeout   int
	Encrypted bool
	AppName   string

	// SQLSvr state object
	db *sqlx.DB
	tx *sqlx.Tx

	mu   sync.Mutex
	txMu sync.RWMutex
}

// SQLResult defines sql action query result info
type SQLResult struct {
	RowsAffected    int64
	NewlyInsertedID int64
	Err             error
}

// ================================================================================================================
// STRUCT FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// GetDsnADO serialize SQLServer dsn to ado style connection string, for use in database connectivity (dsn.Port is ignored)
func (svr *SQLServer) GetDsnADO() (string, error) {
	//
	// first validate input
	//
	if len(svr.Host) == 0 {
		return "", errors.New("SQL Server Host is Required")
	}

	if len(svr.Database) == 0 {
		return "", errors.New("SQL Database is Required")
	}

	if len(svr.UserName) == 0 {
		return "", errors.New("User ID is Required")
	}

	//
	// now create ado style connection string
	//
	str := "server=" + svr.Host

	if len(svr.Instance) > 0 {
		str += "\\" + svr.Instance + ";"
	} else {
		str += ";"
	}

	str += "database=" + svr.Database + ";"

	if len(svr.AppName) > 0 {
		str += "app name=" + svr.AppName + ";"
	}

	str += "user id=" + svr.UserName + ";"

	if len(svr.Password) > 0 {
		str += "password=" + svr.Password + ";"
	}

	if svr.Timeout > 0 {
		str += "connection timeout=" + util.Itoa(svr.Timeout) + ";"
	} else {
		str += "connection timeout=0;"
	}

	if !svr.Encrypted {
		str += "encrypt=disable;"
	} else {
		str += "encrypt=true;"
	}

	// remove last semi-colon from str
	str = str[:len(str)-1]

	// return to caller
	return str, nil
}

// GetDsnURL serialize sql server dsn to url style connection string, for use in database connectivity
func (svr *SQLServer) GetDsnURL() (string, error) {
	//
	// first validate input
	//
	if len(svr.Host) == 0 {
		return "", errors.New("SQL Server Host is Required")
	}

	if len(svr.Database) == 0 {
		return "", errors.New("SQL Database is Required")
	}

	if len(svr.UserName) == 0 {
		return "", errors.New("User ID is Required")
	}

	//
	// now create url style connection string
	//
	query := url.Values{}
	query.Add("app name", svr.AppName)
	query.Add("database", svr.Database)

	if svr.Timeout >= 0 && svr.Timeout <= 60 {
		query.Add("connection timeout", util.Itoa(svr.Timeout))
	} else {
		query.Add("connection timeout", "0")
	}

	if !svr.Encrypted {
		query.Add("encrypt", "disable")
	} else {
		query.Add("encrypt", "true")
	}

	var h string

	if svr.Port > 0 {
		h = fmt.Sprintf("%s:%d", svr.Host, svr.Port)
	} else if len(svr.Instance) > 0 {
		h = fmt.Sprintf("%s\\%s", svr.Host, svr.Instance)
	} else {
		h = svr.Host
	}

	u := url.URL{
		Scheme:   "sqlserver",
		User:     url.UserPassword(svr.UserName, svr.Password),
		Host:     h,
		RawQuery: query.Encode(),
	}

	// return to caller
	return u.String(), nil
}

// Open a database by connecting to it, using the dsn properties defined in the struct fields
//
//	useADOConnectString = if ignored, default is true, to use URL connect string format, set parameter value to false explicitly
func (svr *SQLServer) Open(useADOConnectString ...bool) error {
	//
	// get parameter value,
	// default is expected
	//
	ado := true

	if len(useADOConnectString) > 0 {
		ado = useADOConnectString[0]
	}

	// declare
	var str string
	var err error

	// get connect string
	if ado {
		str, err = svr.GetDsnADO()
	} else {
		str, err = svr.GetDsnURL()
	}

	if err != nil {
		svr.mu.Lock()
		svr.tx = nil
		svr.db = nil
		svr.mu.Unlock()
		return err
	}

	// validate connection string
	if len(str) == 0 {
		svr.mu.Lock()
		svr.tx = nil
		svr.db = nil
		svr.mu.Unlock()
		return errors.New("SQL Server Connect String Generated Cannot Be Empty")
	}

	// now ready to open sql server database
	db, err := sqlx.Open("sqlserver", str)

	if err != nil {
		svr.mu.Lock()
		svr.tx = nil
		svr.db = nil
		svr.mu.Unlock()
		return err
	}

	// test sql server state object
	if err = db.Ping(); err != nil {
		_ = db.Close()

		svr.mu.Lock()
		svr.tx = nil
		svr.db = nil
		svr.mu.Unlock()

		return err
	}

	// upon open, transaction object already nil
	svr.mu.Lock()
	svr.db = db
	svr.tx = nil
	svr.mu.Unlock()

	// sql server state object successfully opened
	return nil
}

// Close will close the database connection and set db to nil
func (svr *SQLServer) Close() error {
	svr.txMu.Lock()
	defer svr.txMu.Unlock()

	svr.mu.Lock()
	db := svr.db
	svr.db = nil
	svr.tx = nil
	svr.mu.Unlock()

	if db != nil {
		if err := db.Close(); err != nil {
			return err
		}
	}

	return nil
}

// Ping tests if current database connection is still active and ready
func (svr *SQLServer) Ping() error {
	svr.mu.Lock()
	db := svr.db
	svr.mu.Unlock()

	if db == nil {
		return errors.New("SQL Server Not Connected")
	}

	if err := db.Ping(); err != nil {
		return err
	}

	// database ok
	return nil
}

// Begin starts a database transaction, and stores the transaction object until commit or rollback
func (svr *SQLServer) Begin() error {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return err
	}

	svr.txMu.Lock()
	defer svr.txMu.Unlock()

	svr.mu.Lock()
	if svr.tx != nil {
		svr.mu.Unlock()
		return errors.New("Transaction Already Started")
	}
	db := svr.db
	svr.mu.Unlock()

	if db == nil {
		return errors.New("SQL Server Not Connected")
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	svr.mu.Lock()
	svr.tx = tx
	svr.mu.Unlock()

	// return nil as success
	return nil
}

// Commit finalizes a database transaction, and commits changes to database
func (svr *SQLServer) Commit() error {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return err
	}

	svr.txMu.Lock()
	defer svr.txMu.Unlock()

	svr.mu.Lock()
	tx := svr.tx
	svr.mu.Unlock()

	// does transaction already exist
	if tx == nil {
		return errors.New("Transaction Does Not Exist")
	}

	// perform tx commit
	if err := tx.Commit(); err != nil {
		return err
	}

	// commit successful
	svr.mu.Lock()
	svr.tx = nil
	svr.mu.Unlock()
	return nil
}

// Rollback cancels pending database changes for the current transaction and clears out transaction object
func (svr *SQLServer) Rollback() error {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return err
	}

	svr.txMu.Lock()
	defer svr.txMu.Unlock()

	svr.mu.Lock()
	tx := svr.tx
	svr.mu.Unlock()

	if tx == nil {
		return errors.New("Transaction Does Not Exist")
	}

	if err := tx.Rollback(); err != nil {
		svr.mu.Lock()
		svr.tx = nil
		svr.mu.Unlock()
		return err
	}

	svr.mu.Lock()
	svr.tx = nil
	svr.mu.Unlock()
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
//	query = sql query, optionally having parameters marked as @p1, @p2, ... @pN, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. notFound = indicates no rows found in query (aka sql.ErrNoRows), if error is detected, notFound is always false
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is nil)
//
// [ Notes ]
//  1. if error == nil, and len(dest struct slice) == 0 then zero struct slice result
func (svr *SQLServer) GetStructSlice(dest interface{}, query string, args ...interface{}) (notFound bool, retErr error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return false, err
	}

	svr.mu.Lock()
	db := svr.db
	tx := svr.tx
	svr.mu.Unlock()

	if db == nil {
		return false, errors.New("SQL Server Not Connected")
	}

	// perform select action, and unmarshal result rows into target struct slice
	var err error

	if tx == nil {
		// not in transaction mode
		// query using db object
		err = db.Select(dest, query, args...)
	} else {
		// in transaction mode
		// query using tx object
		svr.txMu.RLock()
		err = tx.Select(dest, query, args...)
		svr.txMu.RUnlock()
	}

	// if err is sql.ErrNoRows then treat as no error
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true, nil
		}
		return false, err
	}

	return false, nil
}

// GetStruct performs query with optional variadic parameters, and unmarshal single result row into single target struct,
// such as: Customer struct where one row of data represent a customer
// [ Parameters ]
//
//	dest = pointer to struct or address of struct, this is the result of row to be marshaled into this struct
//	query = sql query, optionally having parameters marked as @p1, @p2, ... @pN, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. notFound = indicates no rows found in query (aka sql.ErrNoRows), if error is detected, notFound is always false
//  2. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and dest is nil)
func (svr *SQLServer) GetStruct(dest interface{}, query string, args ...interface{}) (notFound bool, retErr error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return false, err
	}

	svr.mu.Lock()
	db := svr.db
	tx := svr.tx
	svr.mu.Unlock()

	if db == nil {
		return false, errors.New("SQL Server Not Connected")
	}

	// perform select action, and unmarshal result row (single row) into target struct (single object)
	var err error

	if tx == nil {
		// not in transaction mode
		// query using db object
		err = db.Get(dest, query, args...)
	} else {
		// in transaction mode
		// query using tx object
		svr.txMu.RLock()
		err = tx.Get(dest, query, args...)
		svr.txMu.RUnlock()
	}

	// if err is sql.ErrNoRows then treat as no error
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return true, nil
		}
		return false, err
	}

	return false, nil
}

// ----------------------------------------------------------------------------------------------------------------
// query and get rows helpers
// ----------------------------------------------------------------------------------------------------------------

// GetRowsByOrdinalParams performs query with optional variadic parameters to get ROWS of result, and returns *sqlx.Rows
// [ Parameters ]
//
//	query = sql query, optionally having parameters marked as @p1, @p2, ... @pN, where each represents a parameter position
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
func (svr *SQLServer) GetRowsByOrdinalParams(query string, args ...interface{}) (*sqlx.Rows, error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return nil, err
	}

	svr.mu.Lock()
	db := svr.db
	tx := svr.tx
	svr.mu.Unlock()

	if db == nil {
		return nil, errors.New("SQL Server Not Connected")
	}

	if tx != nil {
		svr.txMu.RLock()
		rows, err := tx.Queryx(query, args...)
		svr.txMu.RUnlock()
		return rows, err
	}

	return db.Queryx(query, args...)
}

// GetRowsByNamedMapParam performs query with named map containing parameters to get ROWS of result, and returns *sqlx.Rows
// [ Syntax ]
//  1. in sql = instead of defining ordinal parameters @p1..@pN, each parameter in sql does not need to be ordinal, rather define with :xyz (must have : in front of param name), where xyz is name of parameter, such as :customerID
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
func (svr *SQLServer) GetRowsByNamedMapParam(query string, args map[string]interface{}) (*sqlx.Rows, error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return nil, err
	}

	svr.mu.Lock()
	db := svr.db
	tx := svr.tx
	svr.mu.Unlock()

	if db == nil {
		return nil, errors.New("SQL Server Not Connected")
	}

	// perform select action, and return sqlx rows
	var rows *sqlx.Rows
	var err error

	if tx == nil {
		// not in transaction mode
		// query using db object
		rows, err = db.NamedQuery(query, args)
	} else {
		// in transaction mode
		// query using tx object
		svr.txMu.RLock()
		rows, err = tx.NamedQuery(query, args)
		svr.txMu.RUnlock()
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
//  1. in sql = instead of defining ordinal parameters @p1..@pN, each parameter in sql does not need to be ordinal, rather define with :xyz (must have : in front of param name), where xyz is name of parameter, such as :customerID
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
func (svr *SQLServer) GetRowsByStructParam(query string, args interface{}) (*sqlx.Rows, error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return nil, err
	}

	svr.mu.Lock()
	db := svr.db
	tx := svr.tx
	svr.mu.Unlock()

	if db == nil {
		return nil, errors.New("SQL Server Not Connected")
	}

	// perform select action, and return sqlx rows
	var rows *sqlx.Rows
	var err error

	if tx == nil {
		// not in transaction mode
		// query using db object
		rows, err = db.NamedQuery(query, args)
	} else {
		// in transaction mode
		// query using tx object
		svr.txMu.RLock()
		rows, err = tx.NamedQuery(query, args)
		svr.txMu.RUnlock()
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
func (svr *SQLServer) ScanSlice(rows *sqlx.Rows, dest *[]interface{}) (endOfRows bool, err error) {
	// ensure rows pointer is set
	if rows == nil {
		return true, nil
	}

	// call rows.Next() first to position the row
	if rows.Next() {
		scanned, scanErr := rows.SliceScan()

		// if err is sql.ErrNoRows then treat as no error
		if scanErr != nil && errors.Is(scanErr, sql.ErrNoRows) {
			return true, nil
		}

		if scanErr != nil {
			// has error
			return false, scanErr
		}

		// slice scan successful, may not be at end of rows
		*dest = scanned
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
func (svr *SQLServer) ScanStruct(rows *sqlx.Rows, dest interface{}) (endOfRows bool, err error) {
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
//	query = sql query, optionally having parameters marked as @p1, @p2, ... @pN, where each represents a parameter position
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
func (svr *SQLServer) GetSingleRow(query string, args ...interface{}) (*sqlx.Row, error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return nil, err
	}

	svr.mu.Lock()
	db := svr.db
	tx := svr.tx
	svr.mu.Unlock()

	if db == nil {
		return nil, errors.New("SQL Server Not Connected")
	}

	// perform select action, and return sqlx row
	var row *sqlx.Row
	var err error

	if tx == nil {
		// not in transaction mode
		// query using db object
		row = db.QueryRowx(query, args...)
	} else {
		// in transaction mode
		// query using tx object
		svr.txMu.RLock()
		row = tx.QueryRowx(query, args...)
		svr.txMu.RUnlock()
	}

	if row == nil {
		return nil, errors.New("No Row Data Found From Query")
	}

	if err = row.Err(); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return row, nil
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
func (svr *SQLServer) ScanSliceByRow(row *sqlx.Row, dest *[]interface{}) (notFound bool, err error) {
	// if row is nil, treat as no row and not an error
	if row == nil {
		if dest != nil {
			*dest = nil
		}
		return true, nil
	}

	// perform slice scan on the given row
	scanned, scanErr := row.SliceScan()

	// if err is sql.ErrNoRows then treat as no error
	if scanErr != nil && errors.Is(scanErr, sql.ErrNoRows) {
		if dest != nil {
			*dest = nil
		}
		return true, nil
	}

	if scanErr != nil {
		// has error
		if dest != nil {
			*dest = nil
		}
		return false, scanErr // although error but may not be not found
	}

	// slice scan success
	if dest != nil {
		*dest = scanned
	}

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
func (svr *SQLServer) ScanStructByRow(row *sqlx.Row, dest interface{}) (notFound bool, err error) {
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
func (svr *SQLServer) ScanColumnsByRow(row *sqlx.Row, dest ...interface{}) (notFound bool, err error) {
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
//	query = sql query, optionally having parameters marked as @p1, @p2, ... @pN, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. retVal = string value of scalar result, if no value, blank is returned
//  2. retNotFound = now row found
//  3. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and retVal is returned as blank)
func (svr *SQLServer) GetScalarString(query string, args ...interface{}) (retVal string, retNotFound bool, retErr error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return "", false, err
	}

	svr.mu.Lock()
	db := svr.db
	tx := svr.tx
	svr.mu.Unlock()

	if db == nil {
		return "", false, errors.New("SQL Server Not Connected")
	}

	// get row using query string and parameters
	var row *sqlx.Row

	if tx == nil {
		// not in transaction
		// use db object
		row = db.QueryRowx(query, args...)
	} else {
		// in transaction
		// use tx object
		svr.txMu.RLock()
		row = tx.QueryRowx(query, args...)
		svr.txMu.RUnlock()
	}

	if row == nil {
		return "", false, errors.New("Scalar Query Yielded Empty Row")
	}

	if err := row.Err(); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// no rows
			return "", true, nil
		}
		return "", false, err
	}

	if err := row.Scan(&retVal); errors.Is(err, sql.ErrNoRows) {
		return "", true, nil
	} else if err != nil {
		return "", false, err
	}

	return retVal, false, nil
}

// GetScalarNullString performs query with optional variadic parameters, and returns the first row and first column value in sql.NullString{} data type
// [ Parameters ]
//
//	query = sql query, optionally having parameters marked as @p1, @p2, ... @pN, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. retVal = string value of scalar result, if no value, sql.NullString{} is returned
//  2. retNotFound = now row found
//  3. if error != nil, then error is encountered (if error == sql.ErrNoRows, then error is treated as nil, and retVal is returned as sql.NullString{})
func (svr *SQLServer) GetScalarNullString(query string, args ...interface{}) (retVal sql.NullString, retNotFound bool, retErr error) {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return sql.NullString{}, false, err
	}

	svr.mu.Lock()
	db := svr.db
	tx := svr.tx
	svr.mu.Unlock()

	if db == nil {
		return sql.NullString{}, false, errors.New("SQL Server Not Connected")
	}

	// get row using query string and parameters
	var row *sqlx.Row

	if tx == nil {
		// not in transaction
		// use db object
		row = db.QueryRowx(query, args...)
	} else {
		// in transaction
		// use tx object
		svr.txMu.RLock()
		row = tx.QueryRowx(query, args...)
		svr.txMu.RUnlock()
	}

	if row == nil {
		return sql.NullString{}, false, errors.New("Scalar Query Yielded Empty Row")
	}

	if err := row.Err(); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.NullString{}, true, nil
		}
		return sql.NullString{}, false, err
	}

	if err := row.Scan(&retVal); errors.Is(err, sql.ErrNoRows) {
		return sql.NullString{}, true, nil
	} else if err != nil {
		return sql.NullString{}, false, err
	}

	return retVal, false, nil
}

// ----------------------------------------------------------------------------------------------------------------
// execute helpers
// ----------------------------------------------------------------------------------------------------------------

// ExecByOrdinalParams executes action query string and parameters to return result, if error, returns error object within result
// [ Parameters ]
//
//	actionQuery = sql action query, optionally having parameters marked as @p1, @p2, ... @pN, where each represents a parameter position
//	args = conditionally required if positioned parameters are specified, must appear in the same order as the positional parameters
//
// [ Return Values ]
//  1. SQLResult = represents the sql action result received (including error info if applicable)
func (svr *SQLServer) ExecByOrdinalParams(query string, args ...interface{}) SQLResult {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// keep query trimmed
	query = util.Trim(query)

	if len(query) == 0 {
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("ExecByOrdinalParams() Error: Empty Query String")}
	}

	normalized := strings.ToUpper(strings.TrimSpace(query))
	isInsert := strings.HasPrefix(normalized, "INSERT")

	if !strings.HasSuffix(query, ";") {
		query += ";"
	}

	if isInsert {
		query += "SELECT PKID=SCOPE_IDENTITY(), RowsAffected=@@ROWCOUNT;"
	} else {
		query += "SELECT RowsAffected=@@ROWCOUNT;"
	}

	svr.mu.Lock()
	db := svr.db
	tx := svr.tx
	svr.mu.Unlock()

	if db == nil {
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("ExecByOrdinalParams() Error: Database Connection is Nil")}
	}

	// perform exec action, and return to caller
	var rows *sqlx.Rows
	var err error

	if tx == nil {
		// not in transaction mode,
		// action using db object
		rows, err = db.Queryx(query, args...)
	} else {
		// in transaction mode,
		// action using tx object
		svr.txMu.RLock()
		rows, err = tx.Queryx(query, args...)
		svr.txMu.RUnlock()
	}

	if err != nil {
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	if rows == nil {
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("ExecByOrdinalParams() Error: Queryx Returned Nil Rows")}
	}

	defer rows.Close()

	if rows.Next() == false {
		if rows.Err() != nil {
			return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: rows.Err()}
		}
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("ExecByOrdinalParams() Error: Rows.Next() Yielded No Data")}
	}

	// evaluate rows affected
	var affected int64
	var newID int64

	if isInsert {
		if err = rows.Scan(&newID, &affected); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
		}
	} else {
		if err = rows.Scan(&affected); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
		}
	}

	// return result
	return SQLResult{RowsAffected: affected, NewlyInsertedID: newID, Err: nil}
}

// ExecByNamedMapParam executes action query string with named map containing parameters to return result, if error, returns error object within result
// [ Syntax ]
//  1. in sql = instead of defining ordinal parameters @p1..@pN, each parameter in sql does not need to be ordinal, rather define with :xyz (must have : in front of param name), where xyz is name of parameter, such as :customerID
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
//  1. SQLResult = represents the sql action result received (including error info if applicable)
func (svr *SQLServer) ExecByNamedMapParam(query string, args map[string]interface{}) SQLResult {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// keep query trimmed
	query = util.Trim(query)

	if len(query) == 0 {
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("ExecByNamedMapParam() Error: Query is Empty")}
	}

	normalized := strings.ToUpper(strings.TrimSpace(query))
	isInsert := strings.HasPrefix(normalized, "INSERT")

	if !strings.HasSuffix(query, ";") {
		query += ";"
	}

	if isInsert {
		query += "SELECT PKID=SCOPE_IDENTITY(), RowsAffected=@@ROWCOUNT;"
	} else {
		query += "SELECT RowsAffected=@@ROWCOUNT;"
	}

	svr.mu.Lock()
	db := svr.db
	tx := svr.tx
	svr.mu.Unlock()

	if db == nil {
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("ExecByNamedMapParam() Error: Database Connection is Nil")}
	}

	// perform exec action, and return to caller
	var rows *sqlx.Rows
	var err error

	if tx == nil {
		// not in transaction mode,
		// action using db object
		rows, err = db.NamedQuery(query, args)
	} else {
		// in transaction mode,
		// action using tx object
		svr.txMu.RLock()
		rows, err = tx.NamedQuery(query, args)
		svr.txMu.RUnlock()
	}

	if err != nil {
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	if rows == nil {
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("ExecByNamedMapParam() Error: NamedQuery Returned Nil Rows")}
	}

	defer rows.Close()

	if rows.Next() == false {
		if rows.Err() != nil {
			return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: rows.Err()}
		}
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("ExecByNamedMapParam() Error: Rows.Next() Yielded No Data")}
	}

	// evaluate rows affected
	var affected int64
	var newID int64

	if isInsert {
		if err = rows.Scan(&newID, &affected); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
		}
	} else {
		if err = rows.Scan(&affected); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
		}
	}

	// return result
	return SQLResult{RowsAffected: affected, NewlyInsertedID: newID, Err: nil}
}

// ExecByStructParam executes action query string with struct containing parameters to return result, if error, returns error object within result,
// the struct fields' struct tags must match the parameter names, such as: struct tag `db:"customerID"` must match parameter name in sql as ":customerID"
// [ Syntax ]
//  1. in sql = instead of defining ordinal parameters @p1..@pN, each parameter in sql does not need to be ordinal, rather define with :xyz (must have : in front of param name), where xyz is name of parameter, such as :customerID
//  2. in go = using a struct to contain fields to match parameters, make sure struct tags match to the sql parameter names, such as struct tag `db:"customerID"` must match parameter name in sql as ":customerID" (the : is not part of the match)
//
// [ Parameters ]
//
//	actionQuery = sql action query, with named parameters using :xyz syntax
//	args = required, the struct variable, whose fields having struct tags matching sql parameter names
//
// [ Return Values ]
//  1. SQLResult = represents the sql action result received (including error info if applicable)
func (svr *SQLServer) ExecByStructParam(query string, args interface{}) SQLResult {
	// verify if the database connection is good
	if err := svr.Ping(); err != nil {
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	// keep query trimmed
	query = util.Trim(query)

	if len(query) == 0 {
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("ExecByStructParam() Error: Query is Empty")}
	}

	normalized := strings.ToUpper(strings.TrimSpace(query))
	isInsert := strings.HasPrefix(normalized, "INSERT")

	if !strings.HasSuffix(query, ";") {
		query += ";"
	}

	if isInsert {
		query += "SELECT PKID=SCOPE_IDENTITY(), RowsAffected=@@ROWCOUNT;"
	} else {
		query += "SELECT RowsAffected=@@ROWCOUNT;"
	}

	svr.mu.Lock()
	db := svr.db
	tx := svr.tx
	svr.mu.Unlock()

	if db == nil {
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("ExecByStructParam() Error: Database Connection is Nil")}
	}

	// perform exec action, and return to caller
	var rows *sqlx.Rows
	var err error

	if tx == nil {
		// not in transaction mode,
		// action using db object
		rows, err = db.NamedQuery(query, args)
	} else {
		// in transaction mode,
		// action using tx object
		svr.txMu.RLock()
		rows, err = tx.NamedQuery(query, args)
		svr.txMu.RUnlock()
	}

	if err != nil {
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
	}

	if rows == nil {
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("ExecByStructParam() Error: NamedQuery Returned Nil Rows")}
	}

	defer rows.Close()

	if rows.Next() == false {
		if rows.Err() != nil {
			return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: rows.Err()}
		}
		return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: errors.New("ExecByStructParam() Error: Rows.Next() Yielded No Data")}
	}

	// evaluate rows affected
	var affected int64
	var newID int64

	if isInsert {
		if err = rows.Scan(&newID, &affected); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
		}
	} else {
		if err = rows.Scan(&affected); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return SQLResult{RowsAffected: 0, NewlyInsertedID: 0, Err: err}
		}
	}

	// return result
	return SQLResult{RowsAffected: affected, NewlyInsertedID: newID, Err: nil}
}
