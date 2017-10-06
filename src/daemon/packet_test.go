package daemon

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
	return stringToMsgType("unknown")
}

func (um UnknowMessage) Process(wk *Session) error {
	return nil
}

func (um UnknowMessage) Serialize() ([]byte, error) {
	return []byte{}, nil
}
