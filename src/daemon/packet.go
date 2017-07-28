package daemon

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"reflect"

	"github.com/skycoin/skycoin/src/cipher"
)

// raw message struct
// | 2 bytes | 2 bytes  |  8 bytes | ......  |
// | version | body len |  nonce   | payload |

const (
	maxPayloadLen = 8 * 1024 // bytes
	msgVersion    = 0x01     // packet version
)

// Header represents the packet's header
type header struct {
	Version uint16
	Len     uint16
	Nonce   nonce
}

// Packet the first layer of data protocol
type packet struct {
	head    header
	payload payload
}

// MsgType message type
type MsgType [4]byte

func (mt MsgType) String() string {
	return string(mt[:])
}

func stringToMsgType(s string) MsgType {
	var mt MsgType
	copy(mt[:], s[:4])
	return mt
}

// payload struct
type payload struct {
	Encrypt bool    `json:"encrypt"` // whether the data is encrypted
	Type    MsgType `json:"type"`    // message type
	Data    []byte  `json:"data"`    // raw message
}

// decodePayload decodes payload
func (p *packet) decodePayload(data []byte) error {
	return json.NewDecoder(bytes.NewReader(data)).Decode(&p.payload)
}

// decodeMessage decodes the packet into registered message
func (p packet) decodeMessage(dpt Decryptor, nonce nonce) (Messager, error) {
	var data []byte
	var err error
	if p.payload.Encrypt {
		data, err = dpt.Decrypt(p.payload.Data, nonce)
		if err != nil {
			return nil, err
		}
	} else {
		// only AuthMessage can be raw data without encryption
		switch p.payload.Type {
		case HandshakeMsgType:
			data = p.payload.Data
		default:
			return nil, ErrAuth
		}
	}

	msgType, ok := regMessageMap[p.payload.Type]
	if !ok {
		return nil, MsgNotRegisterError{p.payload.Type.String()}
	}
	msgV := reflect.New(msgType.Elem()).Interface()
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(msgV); err != nil {
		return nil, err
	}
	return msgV.(Messager), nil
}

// newPacket creates a packet instance
func newPacket(msg Messager, encrypt bool, ept Encryptor) (*packet, error) {
	p := packet{}

	// fill the payload parts
	p.payload.Encrypt = encrypt
	p.payload.Type = msg.Type()
	d, err := msg.Serialize()
	if err != nil {
		return nil, err
	}

	p.head.Nonce = makeNonce(cipher.RandByte(8))
	if encrypt {
		encData, err := ept.Encrypt(d, p.head.Nonce)
		if err != nil {
			return nil, err
		}
		p.payload.Data = encData
	} else {
		p.payload.Data = d
	}

	// marshal the payload into json bytes
	pd, err := json.Marshal(p.payload)
	if err != nil {
		return nil, err
	}

	// fill the packet header
	p.head.Version = msg.Version()
	p.head.Len = uint16(len(pd))

	return &p, nil
}

func (p *packet) bytes() ([]byte, error) {
	buf := bytes.Buffer{}
	// write version
	if err := binary.Write(&buf, binary.LittleEndian, p.head.Version); err != nil {
		return nil, err
	}

	if err := binary.Write(&buf, binary.LittleEndian, p.head.Len); err != nil {
		return nil, err
	}

	if err := binary.Write(&buf, binary.LittleEndian, p.head.Nonce.Bytes()); err != nil {
		return nil, err
	}

	payloadBytes, err := json.Marshal(p.payload)
	if err != nil {
		return nil, err
	}

	return append(buf.Bytes(), payloadBytes...), nil
}
