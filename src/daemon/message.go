package daemon

import (
	"encoding/json"
	"fmt"
	"reflect"
)

var regMessageMap = map[MsgType]reflect.Type{}

var (
	HandshakeMsgType      = HandshakeMessage{}.Type()
	HandshakeAckMsgType   = HandshakeAckMessage{}.Type()
	PingMsgType           = PingMessage{}.Type()
	PongMsgType           = PongMessage{}.Type()
	BindRequestMsgType    = BindRequest{}.Type()
	BindResponseMsgType   = BindResponse{}.Type()
	StatusRequestMsgType  = StatusRequest{}.Type()
	StatusResponseMsgType = StatusResponse{}.Type()
)

func init() {
	// register message into regMessageMap
	registerMessage(&HandshakeMessage{})
	registerMessage(&HandshakeAckMessage{})
	registerMessage(&PingMessage{})
	registerMessage(&PongMessage{})
	registerMessage(&BindRequest{})
	registerMessage(&BindResponse{})
	registerMessage(&StatusRequest{})
	registerMessage(&StatusResponse{})
}

// Messager interface describes what should be implemented as a message
type Messager interface {
	Type() MsgType
	ID() int
	Serialize() ([]byte, error)
	Version() uint16
	SetID(id int)
}

func registerMessage(m Messager) error {
	if _, exist := regMessageMap[m.Type()]; exist {
		return fmt.Errorf("Message of %s has already be registered", m.Type())
	}

	regMessageMap[m.Type()] = reflect.TypeOf(m)
	return nil
}

// Base represents the base of messages
type Base struct {
	Id int `json:"id"`
}

// ID returns the id
func (b Base) ID() int {
	return b.Id
}

// Version returns the protocol version
func (b Base) Version() uint16 {
	return msgVersion
}

// SetID sets the id
func (b *Base) SetID(id int) {
	b.Id = id
}

// HandshakeMessage will send from teller to proxy with Pubkey and
// seq number without encryption, teller will waitting for an
// encrypted HandshakeAckMessage with seq + 1
type HandshakeMessage struct {
	Base
	Pubkey string `json:"pubkey"`
	Seq    int32  `json:"seq"`
}

// Type returns the message type
func (hs HandshakeMessage) Type() MsgType {
	return stringToMsgType("HSHK")
}

// Serialize marshal struct into bytes
func (hs HandshakeMessage) Serialize() ([]byte, error) {
	return json.Marshal(hs)
}

// HandshakeAckMessage the ack message of HandshakeMessage
type HandshakeAckMessage struct {
	Base
	Seq int32 `json:"seq"`
}

// Type returns the message type
func (hsa HandshakeAckMessage) Type() MsgType {
	return stringToMsgType("HACK")
}

// Serialize returns json marshal value
func (hsa HandshakeAckMessage) Serialize() ([]byte, error) {
	return json.Marshal(hsa)
}

// PingMessage represents a ping message, as heart beat message
type PingMessage struct {
	Base
	Value string `json:"value"`
}

// Type returns the message type
func (pm PingMessage) Type() MsgType {
	return stringToMsgType("PING")
}

// Serialize returns the json marshal value
func (pm PingMessage) Serialize() ([]byte, error) {
	return json.Marshal(pm)
}

// PongMessage represents the ack message of ping.
type PongMessage struct {
	Base
	Value string `json:"value"`
}

// Type returns the message type
func (pm PongMessage) Type() MsgType {
	return stringToMsgType("PONG")
}

// Serialize returns the json marshal value
func (pm PongMessage) Serialize() ([]byte, error) {
	return json.Marshal(pm)
}

// BindRequest request for binding skycoin address with a deposit address
type BindRequest struct {
	Base
	SkyAddress string `json:"skycoin_address"`
}

// Type returns message type
func (br BindRequest) Type() MsgType {
	return stringToMsgType("BREQ")
}

// Serialize returns the json marshal value
func (br BindRequest) Serialize() ([]byte, error) {
	return json.Marshal(br)
}

// BindResponse response message for bind request
type BindResponse struct {
	Base
	BtcAddress string `json:"btc_address,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Type returns message type
func (br BindResponse) Type() MsgType {
	return stringToMsgType("BRSP")
}

// Serialize returns the json marshal value
func (br BindResponse) Serialize() ([]byte, error) {
	return json.Marshal(br)
}

// StatusRequest request to get skycoin status
type StatusRequest struct {
	Base
	SkyAddress string `json:"sky_address"`
}

// Type retusn message type
func (sr StatusRequest) Type() MsgType {
	return stringToMsgType("SREQ")
}

// Serialize returns the json marshal value
func (sr StatusRequest) Serialize() ([]byte, error) {
	return json.Marshal(sr)
}

// DepositStatus json struct for deposit status
type DepositStatus struct {
	Seq      uint64 `json:"seq"`
	UpdateAt int64  `json:"update_at"`
	Status   string `json:"status"`
}

// StatusResponse response to status request
type StatusResponse struct {
	Base
	Statuses []DepositStatus `json:"statuses,omitempty"`
	Error    string          `json:"error,omitempty"`
}

// Type returns message type
func (sr StatusResponse) Type() MsgType {
	return stringToMsgType("SRSP")
}

// Serialize returns the json marshal value
func (sr StatusResponse) Serialize() ([]byte, error) {
	return json.Marshal(sr)
}
