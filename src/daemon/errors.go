package daemon

import (
	"fmt"
)

// MsgNotRegisterError represents the error that specific messgae type is not registered
type MsgNotRegisterError struct {
	Value string
}

func (mre MsgNotRegisterError) Error() string {
	return fmt.Sprintf("%v is not registered", mre.Value)
}

// MsgAlreadRegisterError represents the error that the message type is already registered.
type MsgAlreadRegisterError struct {
	Value string
}

func (mre MsgAlreadRegisterError) Error() string {
	return fmt.Sprintf("%v already registered", mre.Value)
}
