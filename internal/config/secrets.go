package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const secretSalt = "ux-agent"

func machineKey() ([]byte, error) {
	host, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(host + home + secretSalt))
	return sum[:], nil
}

func EncryptSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	key, err := machineKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	iv := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, iv, []byte(plain), nil)
	tagSize := gcm.Overhead()
	data, tag := sealed[:len(sealed)-tagSize], sealed[len(sealed)-tagSize:]
	return fmt.Sprintf("%s:%s:%s", hex.EncodeToString(iv), hex.EncodeToString(tag), hex.EncodeToString(data)), nil
}

func DecryptSecret(enc string) (string, error) {
	parts := strings.Split(enc, ":")
	if len(parts) != 3 {
		return "", errors.New("invalid encrypted secret format")
	}
	iv, err := hex.DecodeString(parts[0])
	if err != nil {
		return "", err
	}
	tag, err := hex.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	data, err := hex.DecodeString(parts[2])
	if err != nil {
		return "", err
	}
	key, err := machineKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(iv) != gcm.NonceSize() || len(tag) != gcm.Overhead() {
		return "", errors.New("invalid encrypted secret sizes")
	}
	sealed := append(append([]byte(nil), data...), tag...)
	plain, err := gcm.Open(nil, iv, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func RedactSecret(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:3] + "-****" + s[len(s)-4:]
}
