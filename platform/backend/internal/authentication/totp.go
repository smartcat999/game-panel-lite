package authentication

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

type SecretCipher struct {
	aead cipher.AEAD
}

func NewSecretCipher(key []byte) (*SecretCipher, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretCipher{aead: aead}, nil
}

func (c *SecretCipher) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, plaintext, nil), nil
}

func (c *SecretCipher) Decrypt(ciphertext []byte) ([]byte, error) {
	nonceSize := c.aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, ErrTOTPInvalid
	}
	return c.aead.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], nil)
}

type TOTPService struct {
	store  Store
	cipher *SecretCipher
	now    func() time.Time
}

func NewTOTPService(store Store, secretCipher *SecretCipher) *TOTPService {
	return &TOTPService{store: store, cipher: secretCipher, now: time.Now}
}

func (s *TOTPService) Enroll(ctx context.Context, userID string) (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	encrypted, err := s.cipher.Encrypt([]byte(secret))
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	if err := s.store.PutTOTP(ctx, TOTPRecord{UserID: userID, EncryptedSecret: encrypted, LastAcceptedCounter: -1, CreatedAt: now}); err != nil {
		return "", err
	}
	return secret, nil
}

func (s *TOTPService) Verify(ctx context.Context, userID, code string) error {
	if len(code) != 6 {
		return ErrTOTPInvalid
	}
	record, err := s.store.TOTPByUser(ctx, userID)
	if err != nil {
		return ErrTOTPInvalid
	}
	plaintext, err := s.cipher.Decrypt(record.EncryptedSecret)
	if err != nil {
		return ErrTOTPInvalid
	}
	now := s.now().UTC()
	current := now.Unix() / 30
	for _, counter := range []int64{current, current - 1, current + 1} {
		if hmac.Equal([]byte(totpCode(string(plaintext), counter)), []byte(code)) {
			accepted, acceptErr := s.store.AcceptTOTPCounter(ctx, userID, counter, now)
			if acceptErr != nil || !accepted {
				return ErrTOTPInvalid
			}
			return nil
		}
	}
	return ErrTOTPInvalid
}

func totpCode(secret string, counter int64) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return ""
	}
	message := make([]byte, 8)
	binary.BigEndian.PutUint64(message, uint64(counter))
	digest := hmac.New(sha1.New, key)
	_, _ = digest.Write(message)
	sum := digest.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1_000_000)
}
