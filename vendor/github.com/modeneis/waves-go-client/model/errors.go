package model

import (
	"fmt"
)

// APIError represents a Digits API Error response
type APIError struct {
	Errors []ErrorDetail `json:"errors"`
}

// ErrorDetail represents an individual error in an APIError.
type ErrorDetail struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (e *APIError) Error() string {
	if len(e.Errors) > 0 {
		err := e.Errors[0]
		return fmt.Sprintf("waves-go-client: %d %v", err.Code, err.Message)
	}
	return ""
}

// Empty returns true if the Errors slice is empty, false otherwise.
func (e APIError) Empty() bool {
	ret := false
	if len(e.Errors) == 0 {
		ret = true
	}
	return ret
}

// FirstError returns the first error among err and apiError which is non-nil
// (or non-Empty in the case of apiError) or nil if neither represent errors.
//
// A common use case is an API which prefers to return any a network error,
// if any, and return an API error in the absence of network errors.
func FirstError(err error, apiError *APIError) error {
	if err != nil {
		return err
	}
	if apiError != nil && !apiError.Empty() {
		return apiError
	}
	return nil
}
