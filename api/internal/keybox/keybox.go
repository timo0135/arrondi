// Package keybox chiffre les clés privées Enable Banking (cahier des charges,
// section 10) : AES-256-GCM, nonce aléatoire de 12 octets par chiffrement,
// clé maître de 32 octets lue dans un secret Docker et jamais stockée en base.
package keybox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Box chiffre et déchiffre avec la clé maître.
type Box struct{ aead cipher.AEAD }

// New crée une boîte à partir d'une clé maître de 32 octets.
func New(masterKey []byte) (*Box, error) {
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("clé maître : %d octets, attendu 32", len(masterKey))
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// LoadFile lit la clé maître depuis un fichier : 32 octets bruts, ou leur
// encodage hexadécimal (64 caractères) ou base64 (`openssl rand -base64 32`).
func LoadFile(path string) (*Box, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("clé maître : %w", err)
	}
	if len(raw) == 32 {
		return New(raw)
	}
	s := strings.TrimSpace(string(raw))
	if k, err := hex.DecodeString(s); err == nil && len(k) == 32 {
		return New(k)
	}
	if k, err := base64.StdEncoding.DecodeString(s); err == nil && len(k) == 32 {
		return New(k)
	}
	return nil, errors.New("clé maître : contenu illisible (attendu 32 octets, en hexadécimal ou base64)")
}

// Seal chiffre plaintext ; aad lie le chiffré à son propriétaire (identifiant utilisateur).
func (b *Box) Seal(plaintext, aad []byte) (ciphertext, nonce []byte, err error) {
	nonce = make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return b.aead.Seal(nil, nonce, plaintext, aad), nonce, nil
}

// Open déchiffre ; échoue si le chiffré, le nonce ou aad ont été modifiés.
func (b *Box) Open(ciphertext, nonce, aad []byte) ([]byte, error) {
	if len(nonce) != b.aead.NonceSize() {
		return nil, errors.New("keybox : nonce invalide")
	}
	pt, err := b.aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, errors.New("keybox : déchiffrement impossible (clé maître différente ou donnée altérée)")
	}
	return pt, nil
}
