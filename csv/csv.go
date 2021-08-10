package csv

/*
 * Copyright 2020-2021 Aldelo, LP
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
	"bufio"
	"encoding/csv"
	"errors"
	"io"
	"os"
)

// Csv defines a struct for csv parsing and handling
type Csv struct {
	f           *os.File
	r           *bufio.Reader
	cr          *csv.Reader
	ParsedCount int // data lines parsed count (data lines refers to lines below title columns)
	TriedCount  int // data lines tried count (data lines refers to lines below title columns)
}

// Open will open a csv file path for access
func (c *Csv) Open(path string) error {
	if c == nil {
		return errors.New("Open File Failed: " + "Csv Nil")
	}

	c.ParsedCount = -1
	c.TriedCount = -1

	var err error

	// open file
	c.f, err = os.Open(path)

	if err != nil {
		return errors.New("Open File Failed: " + err.Error())
	}

	// open bufio reader
	c.r = bufio.NewReader(c.f)

	if c.r == nil {
		return errors.New("Open Reader Failed: " + "Reader Nil")
	}

	return nil
}

// SkipHeaderRow will skip one header row,
// before calling csv parser loop, call this skip row to advance forward
func (c *Csv) SkipHeaderRow() error {
	if c == nil {
		return errors.New("Skip Header Row Failed: " + "Csv Nil")
	}

	if c.f == nil {
		return errors.New("Skip Header Row Failed: " + "File Nil")
	}

	if c.r == nil {
		return errors.New("Skip Header Row Failed: " + "Reader Nil")
	}

	_, _, err := c.r.ReadLine()

	if err != nil {
		return errors.New("Skip Header Row Failed: " + err.Error())
	}

	return nil
}

// BeginCsvReader will start the csv parsing,
// this is called AFTER SkipHeaderRow is called,
// this sets the csv reader object and allows csv parsing access
func (c *Csv) BeginCsvReader() error {
	if c == nil {
		return errors.New("Begin Csv Reader Failed: " + "Csv Nil")
	}

	if c.f == nil {
		return errors.New("Begin Csv Reader Failed: " + "File Nil")
	}

	if c.r == nil {
		return errors.New("Begin Csv Reader Failed: " + "Reader Nil")
	}

	c.cr = csv.NewReader(c.r)

	if c.cr == nil {
		return errors.New("Begin Csv Reader Failed: " + "Csv Reader Nil")
	}

	return nil
}

// ReadCsv will read the current line of csv row, and return parsed csv elements,
// each time ReadCsv is called, the next row of csv is read
func (c *Csv) ReadCsv() (eof bool, record []string, err error) {
	if c == nil {
		return false, []string{}, errors.New("Read Csv Row Failed: " + "Csv Nil")
	}

	if c.cr == nil {
		return false, []string{}, errors.New("Read Csv Row Failed: " + "Csv Reader Nil")
	}

	// read record of csv
	record, err = c.cr.Read()

	if err == io.EOF {
		return true, []string{}, nil
	}

	// always increment tried count
	c.TriedCount++

	if err != nil {
		return false, []string{}, errors.New("Read Csv Row Failed: " + err.Error())
	}

	if len(record) <= 0 {
		return false, []string{}, nil
	}

	c.ParsedCount++

	return false, record, nil
}

// Close will close a csv file
func (c *Csv) Close() error {
	if c == nil {
		return nil
	}

	if c.f == nil {
		return nil
	}

	c.r = nil
	c.cr = nil

	return c.f.Close()
}
