package utils

import (
	"bytes"
	"fmt"
	"runtime"
)

const (
	MAX_SIZE = 1024
)

type ErrplaneError struct {
	err        string
	cause      error
	stacktrace []byte
}

func (self *ErrplaneError) Error() string {
	buffer := bytes.NewBufferString("")
	if self.err != "" {
		fmt.Fprintf(buffer, "Error: %s. ")
	}
	fmt.Fprintf(buffer, "Stacktrace: \n%s\n", string(self.stacktrace))
	if self.cause != nil {
		fmt.Fprintf(buffer, "\nCaused by: %s\n", self.cause.Error())
	}
	return buffer.String()
}

func NewErrplaneError(err string) *ErrplaneError {
	return NewErrplaneErrorWithCause(err, nil)
}

func WrapInErrplaneError(err error) *ErrplaneError {
	return NewErrplaneErrorWithCause("", err)
}

func NewErrplaneErrorWithCause(err string, cause error) *ErrplaneError {
	buffer := make([]byte, MAX_SIZE)
	size := runtime.Stack(buffer, false)
	return &ErrplaneError{err, cause, buffer[:size]}
}
