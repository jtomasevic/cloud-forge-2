package service

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"
)

// JWT verification (RS256 + JWKS): parse JOSE header/payload, verify PKCS1v1.5 signature over
// SHA256(header.payload), validate exp/iss, then read cf_account_id or sub as account id string.

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type jwtPayload struct {
	Exp         float64 `json:"exp"`
	Iss         string  `json:"iss"`
	Sub         string  `json:"sub"`
	CfAccountID string  `json:"cf_account_id"`
}

// parseJWTParts splits a compact JWS into its three Base64URL segments (no padding).
func parseJWTParts(raw string) (headerB64, payloadB64, sigB64 string, err error) {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) != 3 {
		return "", "", "", errors.New("jwt: expected 3 segments")
	}
	return parts[0], parts[1], parts[2], nil
}

// decodeJWTHeader unmarshals the JWT protected header JSON (alg must be RS256 for us).
func decodeJWTHeader(seg string) (jwtHeader, error) {
	b, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return jwtHeader{}, err
	}
	var h jwtHeader
	if err := json.Unmarshal(b, &h); err != nil {
		return jwtHeader{}, err
	}
	return h, nil
}

// decodeJWTPayload unmarshals registered claims we care about (exp, iss, sub, cf_account_id).
func decodeJWTPayload(seg string) (jwtPayload, error) {
	b, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return jwtPayload{}, err
	}
	var p jwtPayload
	if err := json.Unmarshal(b, &p); err != nil {
		return jwtPayload{}, err
	}
	return p, nil
}

// rsaPublicKeyFromJWK builds an *rsa.PublicKey from JWKS RSA modulus (n) and exponent (e), both Base64URL.
func rsaPublicKeyFromJWK(nB64, eB64 string) (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	eb, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}
	if len(eb) == 0 {
		return nil, errors.New("jwt: empty exponent")
	}
	e := new(big.Int).SetBytes(eb).Int64()
	if e <= 0 || e > 1<<31-1 {
		return nil, fmt.Errorf("jwt: invalid exponent %d", e)
	}
	N := new(big.Int).SetBytes(nb)
	return &rsa.PublicKey{N: N, E: int(e)}, nil
}

// jwksBody is a minimal JWKS document: we only consume RSA keys with n/e/kid.
type jwksBody struct {
	Keys []struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		Use string `json:"use"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

// fetchJWKS downloads and parses JWKS; returns kid→publicKey for signature verification.
func (s *cfRouterService) fetchJWKS(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	client := s.httpClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(s.deps.JWTPublicKeyURL), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("jwks: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var doc jwksBody
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := make(map[string]*rsa.PublicKey)
	for _, k := range doc.Keys {
		if !strings.EqualFold(k.Kty, "RSA") || k.N == "" || k.E == "" {
			continue
		}
		if k.Use != "" && !strings.EqualFold(k.Use, "sig") {
			continue
		}
		pub, err := rsaPublicKeyFromJWK(k.N, k.E)
		if err != nil {
			continue
		}
		if k.Kid == "" {
			continue
		}
		out[k.Kid] = pub
	}
	if len(out) == 0 {
		return nil, errors.New("jwks: no usable RSA keys")
	}
	return out, nil
}

// rsaKeyForKid returns the RSA public key for a JWT "kid". If refresh is true, the in-memory JWKS
// cache is cleared first (used after a signature failure to handle key rotation).
func (s *cfRouterService) rsaKeyForKid(ctx context.Context, kid string, refresh bool) (*rsa.PublicKey, error) {
	if refresh {
		s.jwksMu.Lock()
		s.jwksByKid = nil
		s.jwksMu.Unlock()
	}
	s.jwksMu.Lock()
	if s.jwksByKid != nil {
		if pub := s.jwksByKid[kid]; pub != nil {
			s.jwksMu.Unlock()
			return pub, nil
		}
	}
	keys, err := s.fetchJWKS(ctx)
	if err != nil {
		s.jwksMu.Unlock()
		return nil, err
	}
	s.jwksByKid = keys
	pub := s.jwksByKid[kid]
	s.jwksMu.Unlock()
	if pub == nil {
		return nil, fmt.Errorf("jwks: unknown kid %q", kid)
	}
	return pub, nil
}

// verifyRS256JWT validates a compact JWT and returns the CloudForge account id string from claims.
func verifyRS256JWT(ctx context.Context, s *cfRouterService, token string) (accountID string, err error) {
	headerB64, payloadB64, sigB64, err := parseJWTParts(token)
	if err != nil {
		return "", err
	}
	hdr, err := decodeJWTHeader(headerB64)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(hdr.Alg, "RS256") {
		return "", fmt.Errorf("jwt: unsupported alg %q", hdr.Alg)
	}
	if hdr.Kid == "" {
		return "", errors.New("jwt: missing kid")
	}
	payload, err := decodeJWTPayload(payloadB64)
	if err != nil {
		return "", err
	}
	if payload.Exp > 0 && time.Now().Unix() >= int64(payload.Exp) {
		return "", errors.New("jwt: expired")
	}
	if want := strings.TrimSpace(s.deps.JWTExpectedIssuer); want != "" {
		if strings.TrimSpace(payload.Iss) != want {
			return "", errors.New("jwt: issuer mismatch")
		}
	}

	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return "", err
	}
	signingInput := headerB64 + "." + payloadB64
	sum := sha256.Sum256([]byte(signingInput))

	pub, err := s.rsaKeyForKid(ctx, hdr.Kid, false)
	if err != nil {
		return "", err
	}
	verify := func(key *rsa.PublicKey) error {
		return rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig)
	}
	if err := verify(pub); err != nil {
		pub2, err2 := s.rsaKeyForKid(ctx, hdr.Kid, true)
		if err2 != nil {
			return "", err
		}
		if err := verify(pub2); err != nil {
			return "", err
		}
	}

	acct := strings.TrimSpace(payload.CfAccountID)
	if acct == "" {
		acct = strings.TrimSpace(payload.Sub)
	}
	if acct == "" {
		return "", errors.New("jwt: missing account id claim")
	}
	return acct, nil
}
