package daemon

type nonce [8]byte

func (n nonce) Bytes() []byte {
	non := make([]byte, len(n))
	copy(non[:], n[:])
	return non
}

func makeNonce(v []byte) nonce {
	var n nonce
	copy(n[:], v[:])
	return n
}

// Decryptor interface for decryption
type Decryptor interface {
	Decrypt(d []byte, nonce nonce) ([]byte, error)
}

// Encryptor interface for encryption
type Encryptor interface {
	Encrypt(d []byte, nonce nonce) ([]byte, error)
}
