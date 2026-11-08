package keybox

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func key() []byte { return bytes.Repeat([]byte{7}, 32) }

func TestSealOpen(t *testing.T) {
	b, err := New(key())
	if err != nil {
		t.Fatal(err)
	}
	ct, nonce, err := b.Seal([]byte("-----BEGIN PRIVATE KEY-----"), []byte("user-1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(nonce) != 12 || bytes.Contains(ct, []byte("PRIVATE")) {
		t.Fatalf("nonce %d octets, chiffré en clair ?", len(nonce))
	}
	pt, err := b.Open(ct, nonce, []byte("user-1"))
	if err != nil || string(pt) != "-----BEGIN PRIVATE KEY-----" {
		t.Fatalf("Open = %q, %v", pt, err)
	}
	if _, err := b.Open(ct, nonce, []byte("user-2")); err == nil {
		t.Error("le chiffré d'un utilisateur ne doit pas s'ouvrir pour un autre")
	}
	ct[0] ^= 1
	if _, err := b.Open(ct, nonce, []byte("user-1")); err == nil {
		t.Error("un chiffré altéré doit être refusé")
	}
	_, n2, _ := b.Seal([]byte("x"), nil)
	if bytes.Equal(nonce, n2) {
		t.Error("nonce réutilisé")
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string][]byte{
		"raw": key(), "hex": []byte(hex.EncodeToString(key()) + "\n"),
		"b64": []byte(base64.StdEncoding.EncodeToString(key()) + "\n"),
	} {
		p := filepath.Join(dir, name)
		os.WriteFile(p, content, 0o600)
		if _, err := LoadFile(p); err != nil {
			t.Errorf("%s : %v", name, err)
		}
	}
	p := filepath.Join(dir, "court")
	os.WriteFile(p, []byte("trop court"), 0o600)
	if _, err := LoadFile(p); err == nil {
		t.Error("clé trop courte acceptée")
	}
	if _, err := New(make([]byte, 16)); err == nil {
		t.Error("clé de 16 octets acceptée")
	}
}
