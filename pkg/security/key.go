package security

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
)

func parsePublicKey(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("unexpected block %s, wanted PUBLIC KEY", block.Type)
	}

	return x509.ParsePKCS1PublicKey(block.Bytes)
}

// ParsePublicKeyFromString loads a public key from a string.
func ParsePublicKeyFromString(s string) (*rsa.PublicKey, error) {
	return parsePublicKey([]byte(s))
}

// ParsePublicKeyFromFile loads a public key from a file.
func ParsePublicKeyFromFile(filename string) (*rsa.PublicKey, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return parsePublicKey(data)
}
