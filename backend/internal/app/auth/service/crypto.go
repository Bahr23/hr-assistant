package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/argon2"
)

type passwordCodec struct {
	time    uint32
	memory  uint32
	threads uint8
	keyLen  uint32
}

func newPasswordCodec() passwordCodec {
	return passwordCodec{time: 1, memory: 64 * 1024, threads: 4, keyLen: 32}
}

func (c passwordCodec) hash(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, c.time, c.memory, c.threads, c.keyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", c.memory, c.time, c.threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func (c passwordCodec) compare(encoded, password string) error {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return errors.New("invalid password hash")
	}
	var version int
	var memory, iterations uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return errors.New("invalid password hash version")
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return errors.New("invalid password hash parameters")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return err
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return err
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(expected)))
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return errors.New("password mismatch")
	}
	return nil
}

type tokenCodec struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

type tokenPair struct {
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

type tokenClaims struct {
	HRID      uuid.UUID
	SessionID uuid.UUID
}

type jwtClaims struct {
	SessionID string `json:"sid"`
	TokenType string `json:"typ"`
	jwt.RegisteredClaims
}

func newTokenCodec(secret string, accessTTL, refreshTTL time.Duration) tokenCodec {
	return tokenCodec{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func (c tokenCodec) issuePair(hrID, sessionID uuid.UUID, now time.Time) (tokenPair, error) {
	accessExpiry, refreshExpiry := now.Add(c.accessTTL), now.Add(c.refreshTTL)
	access, err := c.issue(hrID, sessionID, "access", now, accessExpiry)
	if err != nil {
		return tokenPair{}, err
	}
	refresh, err := c.issue(hrID, sessionID, "refresh", now, refreshExpiry)
	if err != nil {
		return tokenPair{}, err
	}
	return tokenPair{AccessToken: access, RefreshToken: refresh, AccessExpiresAt: accessExpiry, RefreshExpiresAt: refreshExpiry}, nil
}

func (c tokenCodec) issue(hrID, sessionID uuid.UUID, tokenType string, now, expiry time.Time) (string, error) {
	claims := jwtClaims{
		SessionID: sessionID.String(), TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "hr-assistant", Subject: hrID.String(), ID: uuid.NewString(),
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expiry),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(c.secret)
}

func (c tokenCodec) parseAccess(raw string) (tokenClaims, error)  { return c.parse(raw, "access") }
func (c tokenCodec) parseRefresh(raw string) (tokenClaims, error) { return c.parse(raw, "refresh") }

func (c tokenCodec) parse(raw, expectedType string) (tokenClaims, error) {
	token, err := jwt.ParseWithClaims(raw, &jwtClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return c.secret, nil
	}, jwt.WithIssuer("hr-assistant"), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return tokenClaims{}, errors.New("invalid token")
	}
	claims, ok := token.Claims.(*jwtClaims)
	if !ok || claims.TokenType != expectedType {
		return tokenClaims{}, errors.New("invalid token type")
	}
	hrID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return tokenClaims{}, err
	}
	sessionID, err := uuid.Parse(claims.SessionID)
	if err != nil {
		return tokenClaims{}, err
	}
	return tokenClaims{HRID: hrID, SessionID: sessionID}, nil
}

func (c tokenCodec) hash(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}
