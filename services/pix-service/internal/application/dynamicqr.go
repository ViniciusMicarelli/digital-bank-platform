package application

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-common/money"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/domain"
	"github.com/ViniciusMicarelli/digital-bank-platform/services/pix-service/internal/ports"
)

// QRSigner signs the payloads of this bank's dynamic QR codes.
type QRSigner struct {
	Key ed25519.PrivateKey
	Kid string
	// Location is where payloads are served, without scheme, e.g.
	// "pix.digitalbank.example/pix/qr/v2"; the txid is appended.
	Location string
}

// NewQRSigner derives the Ed25519 key from a 32-byte seed (a KMS data key).
func NewQRSigner(seed []byte, location string) QRSigner {
	key := ed25519.NewKeyFromSeed(seed)
	sum := sha256.Sum256(key.Public().(ed25519.PublicKey))
	return QRSigner{Key: key, Kid: base64.RawURLEncoding.EncodeToString(sum[:12]), Location: location}
}

// JWK is the public key that verifies dynamic QR payloads.
func (q QRSigner) JWK() ports.JWK {
	return ports.JWK{Kid: q.Kid, Kty: "OKP", Crv: "Ed25519", Alg: "EdDSA",
		X: base64.RawURLEncoding.EncodeToString(q.Key.Public().(ed25519.PublicKey))}
}

// SetQRSigner enables dynamic QR codes.
func (s *Service) SetQRSigner(q QRSigner) { s.qr = &q }

// SetQRFetcher sets how payloads of dynamic codes (from any bank) are fetched.
func (s *Service) SetQRFetcher(f ports.QRFetcher) { s.fetcher = f }

var ErrDynamicQRDisabled = errors.New("dynamic QR codes are not configured")

// QRJWKS returns the keys that verify this bank's dynamic QR payloads.
func (s *Service) QRJWKS() []ports.JWK {
	if s.qr == nil {
		return nil
	}
	return []ports.JWK{s.qr.JWK()}
}

// DynamicPayload is the signed (JWS) payload of a dynamic QR code, as served
// at its URL. Field names follow the Central Bank's Pix API.
func (s *Service) DynamicPayload(ctx context.Context, txid string) (string, error) {
	if s.qr == nil {
		return "", ErrDynamicQRDisabled
	}
	q, err := s.store.QRCode(ctx, txid)
	if err != nil {
		return "", err
	}
	holder, err := s.accounts.Holder(ctx, q.AccountID)
	if err != nil {
		return "", err
	}
	status := "ATIVA"
	switch {
	case q.ExpiresAt != nil && !s.now().Before(*q.ExpiresAt):
		status = "EXPIRADA"
	case q.OneTime && q.PaidTransferID.String() != "00000000-0000-0000-0000-000000000000":
		status = "CONCLUIDA"
	}
	valor := map[string]string{}
	if q.Amount != nil {
		valor["original"] = q.Amount.Decimal()
	}
	calendario := map[string]any{"criacao": q.CreatedAt.UTC().Format(time.RFC3339), "apresentacao": s.now().UTC().Format(time.RFC3339)}
	if q.ExpiresAt != nil {
		calendario["expiracao"] = int(q.ExpiresAt.Sub(q.CreatedAt).Seconds())
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{
		"txid": q.TxID, "revisao": 0, "status": status, "chave": q.Key, "valor": valor,
		"solicitacaoPagador": q.Description, "calendario": calendario,
		"recebedor": map[string]string{"nome": holder.Name},
		"iat":       s.now().Unix(),
	})
	tok.Header["kid"] = s.qr.Kid
	return tok.SignedString(s.qr.Key)
}

// resolveDynamic fetches and verifies a dynamic code's payload, and fills the
// code with what the receiver's bank signed.
func (s *Service) resolveDynamic(ctx context.Context, code *domain.BRCode) error {
	if s.fetcher == nil {
		return ErrDynamicQRDisabled
	}
	jws, keys, err := s.fetcher.Fetch(ctx, code.URL)
	if err != nil {
		return fmt.Errorf("%w: fetch QR payload: %v", domain.ErrInvalidArgument, err)
	}
	claims := jwt.MapClaims{}
	_, err = jwt.ParseWithClaims(jws, claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		for _, k := range keys {
			if k.Kid == kid && k.Kty == "OKP" && k.Crv == "Ed25519" {
				x, err := base64.RawURLEncoding.DecodeString(k.X)
				if err != nil || len(x) != ed25519.PublicKeySize {
					break
				}
				return ed25519.PublicKey(x), nil
			}
		}
		return nil, errors.New("unknown signing key")
	}, jwt.WithValidMethods([]string{"EdDSA"}))
	if err != nil {
		return fmt.Errorf("%w: QR payload signature: %v", domain.ErrInvalidArgument, err)
	}
	if st, _ := claims["status"].(string); st != "ATIVA" {
		return fmt.Errorf("%w: charge is %s", domain.ErrQRUnavailable, st)
	}
	code.Key, _ = claims["chave"].(string)
	code.TxID, _ = claims["txid"].(string)
	code.Description, _ = claims["solicitacaoPagador"].(string)
	if v, ok := claims["valor"].(map[string]any); ok {
		if orig, _ := v["original"].(string); orig != "" {
			m, err := money.Parse(orig, money.BRL)
			if err != nil {
				return fmt.Errorf("%w: bad amount in QR payload", domain.ErrInvalidArgument)
			}
			code.Amount = &m
		}
	}
	if code.Key == "" || code.TxID == "" {
		return fmt.Errorf("%w: incomplete QR payload", domain.ErrInvalidArgument)
	}
	return nil
}
