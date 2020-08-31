package aws

import "github.com/aws/aws-sdk-go/aws/awserr"

func ToAwsError(err error) awserr.Error {
	if err == nil {
		return nil
	}

	if e, ok := err.(awserr.Error); ok {
		return e
	} else {
		return awserr.New(err.Error(), err.Error(), nil)
	}
}
