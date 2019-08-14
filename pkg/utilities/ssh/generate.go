package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"

	"golang.org/x/crypto/ssh"
)

type KeyPair struct {
	PrivatePEM, PublicRSA []byte
}

func GenerateKeyPair(nBits int) (*KeyPair, error) {
	privKeyRSA, err := rsa.GenerateKey(rand.Reader, nBits)
	if err != nil {
		return nil, err
	}

	if err := privKeyRSA.Validate(); err != nil {
		return nil, err
	}

	publicRsaKey, err := ssh.NewPublicKey(&privKeyRSA.PublicKey)
	if err != nil {
		return nil, err
	}

	return &KeyPair{
		PrivatePEM: pem.EncodeToMemory(&pem.Block{
			Type:    "RSA PRIVATE KEY",
			Headers: nil,
			Bytes:   x509.MarshalPKCS1PrivateKey(privKeyRSA),
		}),
		PublicRSA: ssh.MarshalAuthorizedKey(publicRsaKey),
	}, nil
}
