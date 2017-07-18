package daemon

// Decryptor interface for decryption
type Decryptor interface {
	Decrypt(d []byte) ([]byte, error)
}

// Encryptor interface for encryption
type Encryptor interface {
	Encrypt(d []byte) ([]byte, error)
}
