package daemon

import (
	"testing"

	"encoding/json"

	"github.com/stretchr/testify/assert"
)

type DecryptMock struct {
	Value []byte
}

func (dm DecryptMock) Decrypt(d []byte) ([]byte, error) {
	return dm.Value, nil
}

type EncryptMock struct {
	Value []byte
}

func (em EncryptMock) Encrypt(d []byte) ([]byte, error) {
	return em.Value, nil
}

type UnknowMessage struct{}

func (um UnknowMessage) Version() uint16 {
	return msgVersion
}

func (um UnknowMessage) Type() MsgType {
	return stringToMsgType("unknow")
}

func (um UnknowMessage) Process(wk *Session) error {
	return nil
}

func (um UnknowMessage) Serialize() ([]byte, error) {
	return []byte{}, nil
}

func TestNewPacket(t *testing.T) {
	auth := AuthMessage{
		Pubkey: "pubkey",
		Nonce:  "nonce",
		EncSeq: []byte("enc_data"),
	}

	um := UnknowMessage{}

	authBytes, _ := json.Marshal(auth)
	umBytes, _ := json.Marshal(um)

	testData := []struct {
		Msg         Messager
		Encrypt     bool
		Ept         Encryptor
		Dpt         Decryptor
		DeocdeError error
	}{
		{
			auth,
			false,
			nil,
			nil,
			nil,
		},
		{
			auth,
			true,
			EncryptMock{authBytes},
			DecryptMock{authBytes},
			nil,
		},
		{
			um,
			false,
			EncryptMock{umBytes},
			DecryptMock{umBytes},
			MsgNotRegisterError{um.Type().String()},
		},
	}

	for _, d := range testData {
		p, err := newPacket(d.Msg, d.Encrypt, d.Ept)
		assert.Nil(t, err)
		assert.NotNil(t, p)

		msg, err := p.decodeMessage(d.Dpt)
		if err != nil {
			assert.Equal(t, d.DeocdeError, err)
			continue
		}
		m := msg.(*AuthMessage)
		assert.Equal(t, auth, *m)
	}
}
