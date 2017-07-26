package daemon

import (
	"encoding/json"
	"fmt"
	"reflect"
)

var regMessageMap = map[MsgType]reflect.Type{}

var (
	PingMsgType         = PingMessage{}.Type()
	PongMsgType         = PongMessage{}.Type()
	BindRequestMsgType  = BindRequest{}.Type()
	BindResponseMsgType = BindResponse{}.Type()
)

func init() {
	// register message into regMessageMap
	registerMessage(&AuthMessage{})
	registerMessage(&AuthAckMessage{})
	registerMessage(&PingMessage{})
	registerMessage(&PongMessage{})
	registerMessage(&BindRequest{})
	registerMessage(&BindResponse{})
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

// AuthMessage messgae send from client to server, provides client's pubkey and nonce
// and an encrypted data. Server will use ECDH to caculate the key and use chacha20
// to decrypt data, and then reply AuthAckMessage with the data, the whole AuthAckMessage
// will be encrypted.
type AuthMessage struct {
	Base
	Pubkey string `json:"pubkey"`
	Nonce  string `json:"nonce"`
	EncSeq []byte `json:"enc_seq"`
}

// Type returns the message type
func (am AuthMessage) Type() MsgType {
	return stringToMsgType("AUTH")
}

// Serialize use json marshal into bytes
func (am AuthMessage) Serialize() ([]byte, error) {
	return json.Marshal(am)
}

// AuthAckMessage ack message for replying the AuthMessage,
type AuthAckMessage struct {
	Base
	EncSeq []byte `json:"enc_seq"`
}

// Type returns the ack message type
func (aam AuthAckMessage) Type() MsgType {
	return stringToMsgType("AACK")
}

// Serialize json marshal into bytes
func (aam AuthAckMessage) Serialize() ([]byte, error) {
	return json.Marshal(aam)
}

// Version returns the protocol version
func (aam AuthAckMessage) Version() uint16 {
	return msgVersion
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
	BtcAddress string `json:"btc_address, omitempty"`
	Err        string `json:"error,omitempty"`
}

// Type returns message type
func (br BindResponse) Type() MsgType {
	return stringToMsgType("BRSP")
}

// Serialize returns the json marshal value
func (br BindResponse) Serialize() ([]byte, error) {
	return json.Marshal(br)
}
