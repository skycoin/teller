package daemon

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

var regMessageMap = map[MsgType]reflect.Type{}

var (
	PingMsgType            = PingMessage{}.Type()
	PongMsgType            = PongMessage{}.Type()
	MonitorMsgType         = MonitorMessage{}.Type()
	GetExchgLogsMsgType    = GetExchangeLogsMessage{}.Type()
	GetExchgLogsAckMsgType = GetExchangeLogsAckMessage{}.Type()
)

func init() {
	// register message into regMessageMap
	registerMessage(&AuthMessage{})
	registerMessage(&AuthAckMessage{})
	registerMessage(&PingMessage{})
	registerMessage(&PongMessage{})
	registerMessage(&MonitorMessage{})
	registerMessage(&MonitorAckMessage{})
	registerMessage(&GetExchangeLogsMessage{})
	registerMessage(&GetExchangeLogsAckMessage{})
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

// // Version returns the protocol message version
// func (pm PingMessage) Version() uint16 {
// 	return msgVersion
// }

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

// // Version returns the protocol message version
// func (pm PongMessage) Version() uint16 {
// 	return msgVersion
// }

// MonitorMessage command message
type MonitorMessage struct {
	Base
	DepositCoin struct {
		CoinName string `json:"coin"`
		Address  string `json:"address"`
	} `json:"deposit_coin"`
	ICOCoin struct {
		CoinName string `json:"coin"`
		Address  string `json:"address"`
	} `json:"ico_coin"`
}

// Type returns the message type
func (mm MonitorMessage) Type() MsgType {
	return stringToMsgType("MNIT")
}

// Serialize encode the message into json bytes
func (mm MonitorMessage) Serialize() ([]byte, error) {
	return json.Marshal(mm)
}

// // Version returns the protocol version
// func (mm MonitorMessage) Version() uint16 {
// 	return msgVersion
// }

// Result represents the request message result, ususally will be embeded
// in other Ack message.
type Result struct {
	Success bool   `json:"success"`
	Err     string `json:"err"`
}

// MonitorAckMessage represents the ack message of monitor message
type MonitorAckMessage struct {
	Base
	Result `json:"result"`
}

// Type returns the message type
func (mam MonitorAckMessage) Type() MsgType {
	return stringToMsgType("MNAK")
}

// Serialize serialize the monitor ack message
func (mam MonitorAckMessage) Serialize() ([]byte, error) {
	return json.Marshal(mam)
}

// GetExchangeLogsMessage rerpesents the message to require exchange logs
type GetExchangeLogsMessage struct {
	Base
	StartID int
	EndID   int
}

// Type returns the message type
func (gelm GetExchangeLogsMessage) Type() MsgType {
	return stringToMsgType("GLOG")
}

// Serialize serialize the message
func (gelm GetExchangeLogsMessage) Serialize() ([]byte, error) {
	return json.Marshal(gelm)
}

// ExchangeLog represents the log of exchange
type ExchangeLog struct {
	ID      int       `json:"logid"`
	Time    time.Time `json:"time"`
	Deposit struct {
		Address string
		Coin    uint64
	} `json:"deposit"`
	ICO struct {
		Address string
		Coin    uint64
	} `json:"ico"`
	Tx struct {
		Hash      string `json:"hash"`
		Confirmed bool   `json:"coinfirmed"`
	} `json:"tx"`
}

// GetExchangeLogsAckMessage represents the ack message of GetExchangeLogsMessage
type GetExchangeLogsAckMessage struct {
	Base
	Result   `json:"result"`
	MaxLogID int           `json:"max_log_id"`
	Logs     []ExchangeLog `json:"exchange_logs"`
}

// Type returns the exchange logs ack message type
func (gelam GetExchangeLogsAckMessage) Type() MsgType {
	return stringToMsgType("ALOG")
}

// Serialize get exchange logs ack message
func (gelam GetExchangeLogsAckMessage) Serialize() ([]byte, error) {
	return json.Marshal(gelam)
}
