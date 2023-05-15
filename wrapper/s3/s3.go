package s3

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

// =================================================================================================================
// AWS CREDENTIAL:
//		use $> aws configure (to set aws access key and secret to target machine)
//		Store AWS Access ID and Secret Key into Default Profile Using '$ aws configure' cli
//
// To Install & Setup AWS CLI on Host:
//		1) https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2-linux.html
//				On Ubuntu, if host does not have zip and unzip:
//					$> sudo apt install zip
//					$> sudo apt install unzip
//				On Ubuntu, to install AWS CLI v2:
//					$> curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
//					$> unzip awscliv2.zip
//					$> sudo ./aws/install
//		2) $> aws configure set region awsRegionName --profile default
// 		3) $> aws configure
//				follow prompts to enter Access ID and Secret Key
//
// AWS Region Name Reference:
//		us-west-2, us-east-1, ap-northeast-1, etc
//		See: https://docs.aws.amazon.com/general/latest/gr/rande.html
// =================================================================================================================

import (
	"bytes"
	"context"
	"errors"
	util "github.com/aldelo/common"
	awshttp2 "github.com/aldelo/common/wrapper/aws"
	"github.com/aldelo/common/wrapper/aws/awsregion"
	"github.com/aldelo/common/wrapper/xray"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	awsxray "github.com/aws/aws-xray-sdk-go/xray"
	"net/http"
	"os"
	"time"
)

// ================================================================================================================
// STRUCTS
// ================================================================================================================

// S3 struct encapsulates the AWS S3 access functionality
type S3 struct {
	// define the AWS region that s3 is located at
	AwsRegion awsregion.AWSRegion

	// custom http2 client options
	HttpOptions *awshttp2.HttpClientSettings

	// bucket name
	BucketName string

	// store aws session object
	sess *session.Session

	// store s3 object to share across session
	s3Obj *s3.S3

	// store uploader object to share across session
	uploader *s3manager.Uploader

	// store downloader object to share across session
	downloader *s3manager.Downloader

	_parentSegment *xray.XRayParentSegment
}

// ================================================================================================================
// STRUCTS FUNCTIONS
// ================================================================================================================

// ----------------------------------------------------------------------------------------------------------------
// utility functions
// ----------------------------------------------------------------------------------------------------------------

// Connect will establish a connection to the s3 service
func (s *S3) Connect(parentSegment ...*xray.XRayParentSegment) (err error) {
	if xray.XRayServiceOn() {
		if len(parentSegment) > 0 {
			s._parentSegment = parentSegment[0]
		}

		seg := xray.NewSegment("S3-Connect", s._parentSegment)
		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("S3-AWS-Region", s.AwsRegion)
			_ = seg.Seg.AddMetadata("S3-Bucket-Name", s.BucketName)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()

		err = s.connectInternal()

		if err == nil {
			awsxray.AWS(s.s3Obj.Client)
		}

		return err
	} else {
		return s.connectInternal()
	}
}

// Connect will establish a connection to the s3 service
func (s *S3) connectInternal() error {
	// clean up prior session reference
	s.sess = nil

	if !s.AwsRegion.Valid() || s.AwsRegion == awsregion.UNKNOWN {
		return errors.New("Connect To S3 Failed: (AWS Session Error) " + "Region is Required")
	}

	// create custom http2 client if needed
	var httpCli *http.Client
	var httpErr error

	if s.HttpOptions == nil {
		s.HttpOptions = new(awshttp2.HttpClientSettings)
	}

	// use custom http2 client
	h2 := &awshttp2.AwsHttp2Client{
		Options: s.HttpOptions,
	}

	if httpCli, httpErr = h2.NewHttp2Client(); httpErr != nil {
		return errors.New("Connect to S3 Failed: (AWS Session Error) " + "Create Custom Http2 Client Errored = " + httpErr.Error())
	}

	// establish aws session connection and keep session object in struct
	if sess, err := session.NewSession(
		&aws.Config{
			Region:     aws.String(s.AwsRegion.Key()),
			HTTPClient: httpCli,
		}); err != nil {
		// aws session error
		return errors.New("Connect To S3 Failed: (AWS Session Error) " + err.Error())
	} else {
		// aws session obtained
		s.sess = sess

		// create cached objects for shared use
		s.s3Obj = s3.New(s.sess)

		if s.s3Obj == nil {
			return errors.New("Connect To S3 Object Failed: (New S3 Connection) " + "Connection Object Nil")
		}

		s.uploader = s3manager.NewUploader(s.sess)

		if s.uploader == nil {
			return errors.New("Connect To S3Manager Uploader Failed: (New S3Manager Uploader Connection) " + "Connection Object Nil")
		}

		s.downloader = s3manager.NewDownloader(s.sess)

		if s.downloader == nil {
			return errors.New("Connect To S3Manager Downloader Failed: (New S3Manager Downloader Connection) " + "Connection Object Nil")
		}

		// session stored to struct
		return nil
	}
}

// Disconnect will disjoin from aws session by clearing it
func (s *S3) Disconnect() {
	s.s3Obj = nil
	s.uploader = nil
	s.downloader = nil
	s.sess = nil
}

// UpdateParentSegment updates this struct's xray parent segment, if no parent segment, set nil
func (s *S3) UpdateParentSegment(parentSegment *xray.XRayParentSegment) {
	s._parentSegment = parentSegment
}

// UploadFile will upload the specified file to S3 in the bucket name defined within S3 struct
//
// Parameters:
//
//	timeOutDuration = nil if no timeout pre-set via context; otherwise timeout duration typically in seconds via context
//	sourceFilePath = fully qualified source file path and name to upload
//	targetKey = the actual key name without any parts with / indicating folder
//	targetFolder = if the upload position is under one or more 'folder' sub-hierarchy, then specify the target folder names from left to right
//
// Return Values:
//
//	location = value indicating the location where upload was persisted to on s3 bucket
//	err = error encountered while attempting to upload
func (s *S3) UploadFile(timeOutDuration *time.Duration, sourceFilePath string, targetKey string, targetFolder ...string) (location string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("S3-UploadFile", s._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("S3-UploadFile-SourceFilePath", sourceFilePath)
			_ = seg.Seg.AddMetadata("S3-UploadFile-TargetKey", targetKey)
			_ = seg.Seg.AddMetadata("S3-UploadFile-TargetFolder", targetFolder)
			_ = seg.Seg.AddMetadata("S3-UploadFile-Result-Location", location)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if s.uploader == nil {
		err = errors.New("S3 UploadFile Failed: " + "Uploader is Required")
		return "", err
	}

	if util.LenTrim(s.BucketName) <= 0 {
		err = errors.New("S3 UploadFile Failed: " + "Bucket Name is Required")
		return "", err
	}

	if util.LenTrim(sourceFilePath) <= 0 {
		err = errors.New("S3 UploadFile Failed: " + "Source File Path is Required")
		return "", err
	}

	if util.LenTrim(targetKey) <= 0 {
		err = errors.New("S3 UploadFile Failed: " + "Target Key is Required")
		return "", err
	}

	if !util.FileExists(sourceFilePath) {
		err = errors.New("S3 UploadFile Failed: " + "Source File Does Not Exist at Path")
		return "", err
	}

	// define key
	key := targetKey

	if len(targetFolder) > 0 {
		preKey := ""

		for _, v := range targetFolder {
			preKey += v + "/"
		}

		key = preKey + key
	}

	// open file to prepare for upload
	if f, err := os.Open(sourceFilePath); err != nil {
		// open file failed
		err = errors.New("S3 UploadFile Failed: (Open Source File) " + err.Error())
		return "", err
	} else {
		// open file successful
		defer f.Close()

		// upload content to s3 bucket as an object with the key being custom
		var output *s3manager.UploadOutput

		if timeOutDuration != nil {
			ctx, cancel := context.WithTimeout(segCtx, *timeOutDuration)
			defer cancel()

			output, err = s.uploader.UploadWithContext(ctx, &s3manager.UploadInput{
				Bucket: aws.String(s.BucketName),
				Key:    aws.String(key),
				Body:   f,
			})
		} else {
			if segCtxSet {
				output, err = s.uploader.UploadWithContext(segCtx,
					&s3manager.UploadInput{
						Bucket: aws.String(s.BucketName),
						Key:    aws.String(key),
						Body:   f,
					})
			} else {
				output, err = s.uploader.Upload(&s3manager.UploadInput{
					Bucket: aws.String(s.BucketName),
					Key:    aws.String(key),
					Body:   f,
				})
			}
		}

		// evaluate result
		if err != nil {
			// upload error
			err = errors.New("S3 UploadFile Failed: (Upload Source File) " + err.Error())
			return "", err
		} else {
			// upload success
			location = output.Location
			return location, nil
		}
	}
}

// Upload will upload the specified bytes to S3 in the bucket name defined within S3 struct
//
// Parameters:
//
//	timeOutDuration = nil if no timeout pre-set via context; otherwise timeout duration typically in seconds via context
//	data = slice of bytes to upload to s3
//	targetKey = the actual key name without any parts with / indicating folder
//	targetFolder = if the upload position is under one or more 'folder' sub-hierarchy, then specify the target folder names from left to right
//
// Return Values:
//
//	location = value indicating the location where upload was persisted to on s3 bucket
//	err = error encountered while attempting to upload
func (s *S3) Upload(timeOutDuration *time.Duration, data []byte, targetKey string, targetFolder ...string) (location string, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("S3-Upload", s._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("S3-Upload-TargetKey", targetKey)
			_ = seg.Seg.AddMetadata("S3-Upload-TargetFolder", targetFolder)
			_ = seg.Seg.AddMetadata("S3-Upload-Result-Location", location)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if s.uploader == nil {
		err = errors.New("S3 Upload Failed: " + "Uploader is Required")
		return "", err
	}

	if util.LenTrim(s.BucketName) <= 0 {
		err = errors.New("S3 Upload Failed: " + "Bucket Name is Required")
		return "", err
	}

	if data == nil {
		err = errors.New("S3 Upload Failed: " + "Data To Upload is Required (Slice=Nil)")
		return "", err
	}

	if len(data) <= 0 {
		err = errors.New("S3 Upload Failed: " + "Data To Upload is Required (Len=0)")
		return "", err
	}

	if util.LenTrim(targetKey) <= 0 {
		err = errors.New("S3 Upload Failed: " + "Target Key is Required")
		return "", err
	}

	// generate io.Reader from byte slice
	r := bytes.NewReader(data)

	// define key
	key := targetKey

	if len(targetFolder) > 0 {
		preKey := ""

		for _, v := range targetFolder {
			preKey += v + "/"
		}

		key = preKey + key
	}

	// upload content to s3 bucket as an object with the key being custom
	var output *s3manager.UploadOutput

	if timeOutDuration != nil {
		ctx, cancel := context.WithTimeout(segCtx, *timeOutDuration)
		defer cancel()

		output, err = s.uploader.UploadWithContext(ctx, &s3manager.UploadInput{
			Bucket: aws.String(s.BucketName),
			Key:    aws.String(key),
			Body:   r,
		})
	} else {
		if segCtxSet {
			output, err = s.uploader.UploadWithContext(segCtx,
				&s3manager.UploadInput{
					Bucket: aws.String(s.BucketName),
					Key:    aws.String(key),
					Body:   r,
				})
		} else {
			output, err = s.uploader.Upload(&s3manager.UploadInput{
				Bucket: aws.String(s.BucketName),
				Key:    aws.String(key),
				Body:   r,
			})
		}
	}

	// evaluate result
	if err != nil {
		// upload error
		err = errors.New("S3 Upload Failed: (Upload Bytes) " + err.Error())
		return "", err
	} else {
		// upload success
		location = output.Location
		return location, nil
	}
}

// DownloadFile will download an object from S3 bucket by key and persist into file on disk
//
// Parameters:
//
//	timeOutDuration = nil if no timeout pre-set via context; otherwise timeout duration typically in seconds via context
//	writeToFilePath = file path that will save the file containing s3 object content
//	targetKey = the actual key name without any parts with / indicating folder
//	targetFolder = if the download position is under one or more 'folder' sub-hierarchy, then specify the target folder names from left to right
//
// Return Values:
//
//	location = local disk file path where downloaded content is stored into
//	notFound = key was not found in s3 bucket
//	err = error encountered while attempting to download
func (s *S3) DownloadFile(timeOutDuration *time.Duration, writeToFilePath string, targetKey string, targetFolder ...string) (location string, notFound bool, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("S3-DownloadFile", s._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("S3-DownloadFile-TargetKey", targetKey)
			_ = seg.Seg.AddMetadata("S3-DownloadFile-TargetFolder", targetFolder)
			_ = seg.Seg.AddMetadata("S3-DownloadFile-WriteToFilePath", writeToFilePath)
			_ = seg.Seg.AddMetadata("S3-DownloadFile-Result-NotFound", notFound)
			_ = seg.Seg.AddMetadata("S3-DownloadFile-Result-Location", location)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if s.downloader == nil {
		err = errors.New("S3 DownloadFile Failed: " + "Downloader is Required")
		return "", false, err
	}

	if util.LenTrim(s.BucketName) <= 0 {
		err = errors.New("S3 DownloadFile Failed: " + "Bucket Name is Required")
		return "", false, err
	}

	if util.LenTrim(writeToFilePath) <= 0 {
		err = errors.New("S3 DownloadFile Failed: " + "Write To File Path is Required")
		return "", false, err
	}

	if util.LenTrim(targetKey) <= 0 {
		err = errors.New("S3 DownloadFile Failed: " + "Target Key is Required")
		return "", false, err
	}

	// create write buffer to store s3 download content
	f, e := os.Create(writeToFilePath)

	if e != nil {
		err = errors.New("S3 DownloadFile Failed: " + "Create File for Write To File Path Failed")
		return "", false, err
	}

	defer f.Close()

	// define key
	key := targetKey

	if len(targetFolder) > 0 {
		preKey := ""

		for _, v := range targetFolder {
			preKey += v + "/"
		}

		key = preKey + key
	}

	// download content from s3 bucket as an object with the key being custom
	var bytesCount int64

	if timeOutDuration != nil {
		ctx, cancel := context.WithTimeout(segCtx, *timeOutDuration)
		defer cancel()

		bytesCount, err = s.downloader.DownloadWithContext(ctx, f, &s3.GetObjectInput{
			Bucket: aws.String(s.BucketName),
			Key:    aws.String(key),
		})
	} else {
		if segCtxSet {
			bytesCount, err = s.downloader.DownloadWithContext(segCtx, f, &s3.GetObjectInput{
				Bucket: aws.String(s.BucketName),
				Key:    aws.String(key),
			})
		} else {
			bytesCount, err = s.downloader.Download(f, &s3.GetObjectInput{
				Bucket: aws.String(s.BucketName),
				Key:    aws.String(key),
			})
		}
	}

	// evaluate result
	if err != nil {
		// download error
		err = errors.New("S3 DownloadFile Failed: (Download File) " + err.Error())
		return "", false, err
	} else {
		// download successful
		if bytesCount <= 0 {
			// not found
			notFound = true
			return "", notFound, nil
		} else {
			// found
			location = writeToFilePath
			return location, false, nil
		}
	}
}

// Download will download an object from S3 bucket by key and return via byte slice
//
// Parameters:
//
//	timeOutDuration = nil if no timeout pre-set via context; otherwise timeout duration typically in seconds via context
//	targetKey = the actual key name without any parts with / indicating folder
//	targetFolder = if the download position is under one or more 'folder' sub-hierarchy, then specify the target folder names from left to right
//
// Return Values:
//
//	data = byte slice of object downloaded from s3 bucket by key
//	notFound = key was not found in s3 bucket
//	err = error encountered while attempting to download
func (s *S3) Download(timeOutDuration *time.Duration, targetKey string, targetFolder ...string) (data []byte, notFound bool, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("S3-Download", s._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("S3-Download-TargetKey", targetKey)
			_ = seg.Seg.AddMetadata("S3-Download-TargetFolder", targetFolder)
			_ = seg.Seg.AddMetadata("S3-Download-Result-NotFound", notFound)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if s.downloader == nil {
		err = errors.New("S3 Download Failed: " + "Downloader is Required")
		return nil, false, err
	}

	if util.LenTrim(s.BucketName) <= 0 {
		err = errors.New("S3 Download Failed: " + "Bucket Name is Required")
		return nil, false, err
	}

	if util.LenTrim(targetKey) <= 0 {
		err = errors.New("S3 Download Failed: " + "Target Key is Required")
		return nil, false, err
	}

	// create write buffer to store s3 download content
	buf := aws.NewWriteAtBuffer([]byte{})

	// define key
	key := targetKey

	if len(targetFolder) > 0 {
		preKey := ""

		for _, v := range targetFolder {
			preKey += v + "/"
		}

		key = preKey + key
	}

	// download content from s3 bucket as an object with the key being custom
	var bytesCount int64

	if timeOutDuration != nil {
		ctx, cancel := context.WithTimeout(segCtx, *timeOutDuration)
		defer cancel()

		bytesCount, err = s.downloader.DownloadWithContext(ctx, buf, &s3.GetObjectInput{
			Bucket: aws.String(s.BucketName),
			Key:    aws.String(key),
		})
	} else {
		if segCtxSet {
			bytesCount, err = s.downloader.DownloadWithContext(segCtx, buf, &s3.GetObjectInput{
				Bucket: aws.String(s.BucketName),
				Key:    aws.String(key),
			})
		} else {
			bytesCount, err = s.downloader.Download(buf, &s3.GetObjectInput{
				Bucket: aws.String(s.BucketName),
				Key:    aws.String(key),
			})
		}
	}

	// evaluate result
	if err != nil {
		// download error
		err = errors.New("S3 Download Failed: (Download Bytes) " + err.Error())
		return nil, false, err
	} else {
		// download successful
		if bytesCount <= 0 {
			// not found
			notFound = true
			return nil, notFound, nil
		} else {
			// found
			data = buf.Bytes()
			return data, false, nil
		}
	}
}

// Delete will delete an object from S3 bucket by key
//
// Parameters:
//
//	timeOutDuration = nil if no timeout pre-set via context; otherwise timeout duration typically in seconds via context
//	targetKey = the actual key name without any parts with / indicating folder
//	targetFolder = if the delete position is under one or more 'folder' sub-hierarchy, then specify the target folder names from left to right
//
// Return Values:
//
//	deleteSuccess = true if delete was successfully completed; false if delete failed to perform, check error if any
//	err = error encountered while attempting to download
func (s *S3) Delete(timeOutDuration *time.Duration, targetKey string, targetFolder ...string) (deleteSuccess bool, err error) {
	segCtx := context.Background()
	segCtxSet := false

	seg := xray.NewSegmentNullable("S3-Delete", s._parentSegment)

	if seg != nil {
		segCtx = seg.Ctx
		segCtxSet = true

		defer seg.Close()
		defer func() {
			_ = seg.Seg.AddMetadata("S3-Delete-TargetKey", targetKey)
			_ = seg.Seg.AddMetadata("S3-Delete-TargetFolder", targetFolder)
			_ = seg.Seg.AddMetadata("S3-Delete-Result-Success", deleteSuccess)

			if err != nil {
				_ = seg.Seg.AddError(err)
			}
		}()
	}

	// validate
	if s.s3Obj == nil {
		err = errors.New("S3 Delete Failed: " + "S3 Object is Required")
		return false, err
	}

	if util.LenTrim(s.BucketName) <= 0 {
		err = errors.New("S3 Delete Failed: " + "Bucket Name is Required")
		return false, err
	}

	if util.LenTrim(targetKey) <= 0 {
		err = errors.New("S3 Delete Failed: " + "Target Key is Required")
		return false, err
	}

	// define key
	key := targetKey

	if len(targetFolder) > 0 {
		preKey := ""

		for _, v := range targetFolder {
			preKey += v + "/"
		}

		key = preKey + key
	}

	// delete object from s3 bucket
	if timeOutDuration != nil {
		ctx, cancel := context.WithTimeout(segCtx, *timeOutDuration)
		defer cancel()

		_, err = s.s3Obj.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(s.BucketName),
			Key:    aws.String(key),
		})
	} else {
		if segCtxSet {
			_, err = s.s3Obj.DeleteObjectWithContext(segCtx, &s3.DeleteObjectInput{
				Bucket: aws.String(s.BucketName),
				Key:    aws.String(key),
			})
		} else {
			_, err = s.s3Obj.DeleteObject(&s3.DeleteObjectInput{
				Bucket: aws.String(s.BucketName),
				Key:    aws.String(key),
			})
		}
	}

	// evaluate result
	if err != nil {
		// delete error
		err = errors.New("S3 Delete Failed: (Delete Object) " + err.Error())
		return false, err
	} else {
		// delete successful
		deleteSuccess = true
		return deleteSuccess, nil
	}
}
