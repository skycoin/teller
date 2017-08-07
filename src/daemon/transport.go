package daemon

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net"

	"time"

	"io"

	"github.com/skycoin/skycoin/src/cipher"
)

const (
	handshakeTimeout = 5 * time.Second
	readTimeout      = 30 * time.Second
	writeTimeout     = 5 * time.Second
)

var (
	ErrEmptyPubkey            = errors.New("pubkey is not set")
	ErrEmptySeckey            = errors.New("peckey is not set")
	ErrHandShakeInvalidAckMsg = errors.New("invalid handshake ack message")
	ErrHandShakeInvalidAckSeq = errors.New("invalid handkshake ack sequence")
	ErrHandShakeInvalidAuth   = errors.New("invalid handshake auth message")
	ErrAuth                   = errors.New("authenticate failed")
)

// Auth records the keys for authentication
type Auth struct {
	RPubkey cipher.PubKey // remote public key
	LSeckey cipher.SecKey // local secret key
	// Nonce   []byte
}

// Nonce returns the Nonce value
// func (a Auth) Nonce() [8]byte {
// 	nonce := [8]byte{}
// 	copy(nonce[:], a.nonce[:])

// 	return nonce
// }

type transport struct {
	*Auth
	fd net.Conn
}

func init() {
	rand.Seed(int64(time.Now().Nanosecond()))
}

func newTransport(fd net.Conn, auth *Auth, solicited bool) (*transport, error) {
	if auth == nil {
		return nil, errors.New("Auth must not be nil")
	}

	fd.SetDeadline(time.Now().Add(handshakeTimeout))
	t := &transport{
		Auth: auth,
		fd:   fd,
	}

	if solicited {
		if err := t.solicitedHandshake(); err != nil {
			return nil, err
		}
	} else {
		if err := t.unsolicitedHandshake(); err != nil {
			return nil, err
		}
	}

	return t, nil
}

// Write writes message with dead line, data will be encrypted
func (ts *transport) Write(data Messager) error {
	ts.fd.SetWriteDeadline(time.Now().Add(writeTimeout))
	return ts.writeMsg(data, true)
}

// Read reads message with dead line
func (ts *transport) Read() (Messager, error) {
	ts.fd.SetReadDeadline(time.Now().Add(readTimeout))
	return ts.readMsg()
}

func (ts *transport) writeMsg(data Messager, encrypt bool) error {
	pkt, err := newPacket(data, encrypt, ts)
	if err != nil {
		return err
	}

	d, err := pkt.bytes()
	if err != nil {
		return err
	}
	_, err = ts.fd.Write(d)
	if err != nil {
		return err
	}
	return nil
}

func (ts *transport) readMsg() (Messager, error) {
	var packet packet
	if err := binary.Read(ts.fd, binary.LittleEndian, &packet.head); err != nil {
		if err != io.EOF {
			return nil, fmt.Errorf("Read packet header failed, err:%v", err)
		}
		return nil, err
	}

	if packet.head.Version != msgVersion {
		return nil, errors.New("Invalid version in message")
	}

	// check if the Len is vaild
	if packet.head.Len > maxPayloadLen {
		return nil, fmt.Errorf("Payload len:%d is invalid, should not bigger than %d", packet.head.Len, maxPayloadLen)
	}

	// read the payload base on the Len
	data := make([]byte, packet.head.Len)
	n, err := ts.fd.Read(data)
	if err != nil {
		return nil, fmt.Errorf("read payload content failed, err:%v", err)
	}

	if n != int(packet.head.Len) {
		return nil, fmt.Errorf("got: %d, but want:%d bytes data", n, packet.head.Len)
	}

	if err := packet.decodePayload(data); err != nil {
		return nil, err
	}

	return packet.decodeMessage(ts, packet.head.Nonce)
}

// solicitedHandshake will send AuthMessage with Pubkey, Nonce and an encrypted random seq,
// then wait for AuthAckMessage, and decrypt the it's content to see if the value is seq + 1.
func (ts *transport) solicitedHandshake() error {
	// prepare AuthMessage
	// nonce size must be 64bits, 8 bytes.
	// ts.Nonce = cipher.RandByte(8)

	// v, err := json.Marshal(handshakeSeq{rand.Int31n(1024)})
	// if err != nil {
	// 	return fmt.Errorf("marshal handshake seq failed: %v", err)
	// }

	// encSeq, err := ts.Encrypt(v, makeNonce(cipher.RandByte(8)))
	// if err != nil {
	// 	return err
	// }
	seq := rand.Int31n(1024)
	authMsg := HandshakeMessage{
		Pubkey: cipher.PubKeyFromSecKey(ts.LSeckey).Hex(),
		Seq:    seq,
	}

	// send msg
	if err := ts.writeMsg(&authMsg, false); err != nil {
		return err
	}

	// waiting for the ack, the ack message
	msg, err := ts.readMsg()
	if err != nil {
		return err
	}

	ackMsg, ok := msg.(*HandshakeAckMessage)
	if !ok {
		return ErrHandShakeInvalidAckMsg
	}

	if seq+1 != ackMsg.Seq {
		return ErrHandShakeInvalidAckSeq
	}

	// decrypt the AuthAckMessage's EncData
	// seqBytes, err := ts.Decrypt(ackMsg.EncSeq)
	// if err != nil {
	// 	return err
	// }
	// seq, err := strconv.Atoi(string(seqBytes))
	// if err != nil {
	// 	return err
	// }

	// if int32(seq) != (randSeq + 1) {
	// 	return ErrHandShakeInvalidAckSeq
	// }
	return nil
}

// unsolicitedHandshake will start to wait the AuthMessage, and
// use the Pubkey and Nonce to decrypt the encrypted seq, if all
// above are success, we will send back the seq++ value with the AuthAckMessage.

func (ts *transport) unsolicitedHandshake() error {
	// wait for AuthMessage data
	msg, err := ts.readMsg()
	if err != nil {
		return err
	}

	hs, ok := msg.(*HandshakeMessage)
	if !ok {
		return ErrHandShakeInvalidAuth
	}

	// validate the pubkey
	pubKey, err := cipher.PubKeyFromHex(hs.Pubkey)
	if err != nil {
		return err
	}

	ts.RPubkey = pubKey
	// ts.Nonce, err = base64.StdEncoding.DecodeString(authMsg.Nonce)
	// if err != nil {
	// 	return err
	// }

	// decrypt the enc_data in auth message
	// seqBytes, err := ts.Decrypt(authMsg.EncSeq)
	// if err != nil {
	// 	return err
	// }

	// seq, err := strconv.Atoi(string(seqBytes))
	// if err != nil {
	// 	return ErrAuth
	// }

	// write ack message back
	// seq++
	// seqStr := fmt.Sprintf("%d", seq)
	// encSeq, err := ts.Encrypt([]byte(seqStr))
	// if err != nil {
	// 	return err
	// }

	hsAck := HandshakeAckMessage{}
	hsAck.Id = hs.ID()
	hsAck.Seq = hs.Seq + 1

	return ts.writeMsg(&hsAck, true)
}

func (ts *transport) Encrypt(d []byte, nonce nonce) (v []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = ErrAuth
			return
		}
	}()

	empPk, empSk := cipher.PubKey{}, cipher.SecKey{}
	if ts.RPubkey == empPk {
		return []byte{}, ErrEmptyPubkey
	}

	if ts.LSeckey == empSk {
		return []byte{}, ErrEmptySeckey
	}

	key := cipher.ECDH(ts.RPubkey, ts.LSeckey)
	return cipher.Chacha20Decrypt(d, key, nonce.Bytes())
}

func (ts *transport) Decrypt(d []byte, nonce nonce) (v []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = ErrAuth
			return
		}
	}()
	empPk, empSk := cipher.PubKey{}, cipher.SecKey{}
	if ts.RPubkey == empPk {
		return []byte{}, ErrEmptyPubkey
	}

	if ts.LSeckey == empSk {
		return []byte{}, ErrEmptySeckey
	}

	key := cipher.ECDH(ts.RPubkey, ts.LSeckey)
	return cipher.Chacha20Encrypt(d, key, nonce.Bytes())
}

func (ts *transport) Close() error {
	return ts.fd.Close()
}
