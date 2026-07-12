package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

var (
	ErrSignupVerificationInvalidContact = errors.New("signup verification invalid contact")
	ErrSignupVerificationCodeInvalid    = errors.New("signup verification code invalid")
	ErrSignupVerificationCodeExpired    = errors.New("signup verification code expired")
)

type SignupVerificationDriver interface {
	Send(ctx context.Context, msg SignupVerificationMessage) error
}

type SignupVerificationMessage struct {
	Contact string
	Channel string
	Code    string
	TTL     time.Duration
}

type SignupVerificationService struct {
	driver SignupVerificationDriver
	ttl    time.Duration
	now    func() time.Time
	mu     sync.Mutex
	codes  map[string]signupVerificationCode
}

type signupVerificationCode struct {
	Code      string
	ExpiresAt time.Time
}

func NewSignupVerificationService(driver SignupVerificationDriver, ttl time.Duration) *SignupVerificationService {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &SignupVerificationService{
		driver: driver,
		ttl:    ttl,
		now:    time.Now,
		codes:  map[string]signupVerificationCode{},
	}
}

func (s *SignupVerificationService) Send(ctx context.Context, contact string) error {
	if s == nil || s.driver == nil {
		return errors.New("signup verification service not configured")
	}
	normalized, channel, err := normalizeSignupContact(contact)
	if err != nil {
		return err
	}
	code, err := secureDigits(6)
	if err != nil {
		return err
	}
	msg := SignupVerificationMessage{
		Contact: normalized,
		Channel: channel,
		Code:    code,
		TTL:     s.ttl,
	}
	if err := s.driver.Send(ctx, msg); err != nil {
		return err
	}
	s.mu.Lock()
	s.codes[normalized] = signupVerificationCode{Code: code, ExpiresAt: s.now().Add(s.ttl)}
	s.mu.Unlock()
	return nil
}

func (s *SignupVerificationService) IssueForTest(contact string, code string, ttl time.Duration) error {
	normalized, _, err := normalizeSignupContact(contact)
	if err != nil {
		return err
	}
	if strings.TrimSpace(code) == "" {
		return ErrSignupVerificationCodeInvalid
	}
	if ttl <= 0 {
		ttl = s.ttl
	}
	s.mu.Lock()
	s.codes[normalized] = signupVerificationCode{Code: strings.TrimSpace(code), ExpiresAt: s.now().Add(ttl)}
	s.mu.Unlock()
	return nil
}

func (s *SignupVerificationService) Verify(ctx context.Context, contact string, code string) error {
	if s == nil {
		return errors.New("signup verification service not configured")
	}
	normalized, _, err := normalizeSignupContact(contact)
	if err != nil {
		return err
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return ErrSignupVerificationCodeInvalid
	}
	s.mu.Lock()
	record, ok := s.codes[normalized]
	if ok && record.Code == code && !s.now().After(record.ExpiresAt) {
		delete(s.codes, normalized)
	}
	s.mu.Unlock()
	if !ok {
		return ErrSignupVerificationCodeInvalid
	}
	if s.now().After(record.ExpiresAt) {
		return ErrSignupVerificationCodeExpired
	}
	if record.Code != code {
		return ErrSignupVerificationCodeInvalid
	}
	_ = ctx
	return nil
}

type LocalSignupVerificationDriver struct{}

func (LocalSignupVerificationDriver) Send(ctx context.Context, msg SignupVerificationMessage) error {
	logger.InfoF(ctx, "[saas_signup.verification] channel=%s contact=%s code=%s ttl=%s", msg.Channel, msg.Contact, msg.Code, msg.TTL)
	return nil
}

func normalizeSignupContact(contact string) (normalized string, channel string, err error) {
	contact = strings.TrimSpace(contact)
	if contact == "" {
		return "", "", ErrSignupVerificationInvalidContact
	}
	if strings.Contains(contact, "@") {
		email := strings.ToLower(contact)
		if !strings.Contains(email, ".") {
			return "", "", ErrSignupVerificationInvalidContact
		}
		return email, "email", nil
	}
	phone := strings.ReplaceAll(contact, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	if len(phone) < 6 {
		return "", "", ErrSignupVerificationInvalidContact
	}
	return phone, "phone", nil
}

func secureDigits(length int) (string, error) {
	if length <= 0 {
		return "", nil
	}
	out := make([]byte, length)
	max := big.NewInt(10)
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate verification code: %w", err)
		}
		out[i] = byte('0' + n.Int64())
	}
	return string(out), nil
}
