package aws

import (
	"errors"

	"github.com/aws/aws-sdk-go/aws/awserr"
)

func ToAwsError(err error) awserr.Error {
	if err == nil {
		return nil
	}

	var e awserr.Error
	if errors.As(err, &e) {
		return e
	} else {
		return awserr.New("UnknownError", err.Error(), err)
	}
}
