package sdjwtvc

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultHTTPTimeout = 5 * time.Second
	defaultMaxHTTPBody = int64(1024 * 1024)
	maxRedirects       = 3
)

// URLValidator allows deployments to add stricter SSRF/network policy around
// issuer-controlled URLs.
//
// The built-in validation always requires HTTPS. Production deployments can
// additionally restrict private networks, DNS ranges or issuer allowlists.
type URLValidator func(*url.URL) error

// IssuerMetadataResolver implements Cvera's default frozen SD-JWT VC
// verification-key profile:
//
//	iss
//	  ↓
//	/.well-known/jwt-vc-issuer
//	  ↓
//	jwks OR jwks_uri
//	  ↓
//	kid
//	  ↓
//	public verification key
//
// Successful resolution does NOT establish issuer authority.
type IssuerMetadataResolver struct {
	Client       *http.Client
	MaxBodyBytes int64
	ValidateURL  URLValidator
}

type issuerMetadata struct {
	Issuer  string          `json:"issuer"`
	JWKSURI string          `json:"jwks_uri,omitempty"`
	JWKS    json.RawMessage `json:"jwks,omitempty"`
}

type jwkSet struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`

	KeyOps []string `json:"key_ops,omitempty"`

	// RSA
	N string `json:"n,omitempty"`
	E string `json:"e,omitempty"`

	// EC / OKP
	Crv string `json:"crv,omitempty"`
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`
}

func (r *IssuerMetadataResolver) ResolveIssuerKey(
	ctx context.Context,
	issuer string,
	kid string,
	alg string,
) (any, string, error) {
	issuerURL, err := validateIssuerURL(issuer)
	if err != nil {
		return nil, "", err
	}

	if err := r.validateURL(issuerURL); err != nil {
		return nil, "", fmt.Errorf(
			"issuer URL rejected: %w",
			err,
		)
	}

	metadataURL := buildIssuerMetadataURL(issuerURL)

	if err := r.validateURL(metadataURL); err != nil {
		return nil, "", fmt.Errorf(
			"issuer metadata URL rejected: %w",
			err,
		)
	}

	var metadata issuerMetadata
	if err := r.fetchJSON(
		ctx,
		metadataURL.String(),
		&metadata,
	); err != nil {
		return nil, "", fmt.Errorf(
			"retrieve JWT VC Issuer Metadata: %w",
			err,
		)
	}

	if metadata.Issuer != issuer {
		return nil, "", fmt.Errorf(
			"JWT VC Issuer Metadata issuer %q does not match credential iss %q",
			metadata.Issuer,
			issuer,
		)
	}

	hasInlineJWKS := len(metadata.JWKS) > 0 &&
		string(metadata.JWKS) != "null"

	hasJWKSURI := strings.TrimSpace(metadata.JWKSURI) != ""

	if hasInlineJWKS == hasJWKSURI {
		return nil, "", errors.New(
			"JWT VC Issuer Metadata must contain exactly one of jwks or jwks_uri",
		)
	}

	var set jwkSet

	if hasInlineJWKS {
		if err := json.Unmarshal(metadata.JWKS, &set); err != nil {
			return nil, "", fmt.Errorf(
				"parse inline issuer JWKS: %w",
				err,
			)
		}
	} else {
		jwksURL, err := url.Parse(metadata.JWKSURI)
		if err != nil {
			return nil, "", fmt.Errorf(
				"parse jwks_uri: %w",
				err,
			)
		}

		if err := r.validateURL(jwksURL); err != nil {
			return nil, "", fmt.Errorf(
				"jwks_uri rejected: %w",
				err,
			)
		}

		if err := r.fetchJSON(
			ctx,
			jwksURL.String(),
			&set,
		); err != nil {
			return nil, "", fmt.Errorf(
				"retrieve issuer JWKS: %w",
				err,
			)
		}
	}

	key, err := selectVerificationKey(set, kid, alg)
	if err != nil {
		return nil, "", err
	}

	return key, "jwt-vc-issuer-metadata", nil
}

func validateIssuerURL(value string) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil {
		return nil, fmt.Errorf(
			"parse issuer identifier: %w",
			err,
		)
	}

	if u.Scheme != "https" {
		return nil, errors.New(
			"SD-JWT VC issuer identifier must use HTTPS",
		)
	}

	if u.Host == "" {
		return nil, errors.New(
			"SD-JWT VC issuer identifier must contain a host",
		)
	}

	if u.User != nil {
		return nil, errors.New(
			"SD-JWT VC issuer identifier must not contain userinfo",
		)
	}

	if u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New(
			"SD-JWT VC issuer identifier must not contain query or fragment components",
		)
	}

	return u, nil
}

func buildIssuerMetadataURL(issuer *url.URL) *url.URL {
	path := strings.TrimSuffix(issuer.EscapedPath(), "/")

	if path == "/" {
		path = ""
	}

	metadataPath := "/.well-known/jwt-vc-issuer"

	if path != "" {
		metadataPath += path
	}

	return &url.URL{
		Scheme: issuer.Scheme,
		Host:   issuer.Host,
		Path:   metadataPath,
	}
}

func (r *IssuerMetadataResolver) validateURL(u *url.URL) error {
	if u == nil {
		return errors.New("URL is nil")
	}

	if u.Scheme != "https" {
		return errors.New("URL must use HTTPS")
	}

	if u.Host == "" {
		return errors.New("URL must contain a host")
	}

	if u.User != nil {
		return errors.New("URL must not contain userinfo")
	}

	if r != nil && r.ValidateURL != nil {
		return r.ValidateURL(u)
	}

	return nil
}

func (r *IssuerMetadataResolver) fetchJSON(
	ctx context.Context,
	target string,
	dst any,
) error {
	client := r.httpClient()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		target,
		nil,
	)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf(
			"unexpected HTTP status %d",
			resp.StatusCode,
		)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return fmt.Errorf(
				"invalid Content-Type: %w",
				err,
			)
		}

		if mediaType != "application/json" &&
			mediaType != "application/jwk-set+json" {
			return fmt.Errorf(
				"unexpected Content-Type %q",
				mediaType,
			)
		}
	}

	maxBytes := defaultMaxHTTPBody
	if r != nil && r.MaxBodyBytes > 0 {
		maxBytes = r.MaxBodyBytes
	}

	reader := io.LimitReader(resp.Body, maxBytes+1)

	body, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	if int64(len(body)) > maxBytes {
		return fmt.Errorf(
			"HTTP response exceeds maximum size of %d bytes",
			maxBytes,
		)
	}

	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf(
			"decode JSON response: %w",
			err,
		)
	}

	return nil
}

func (r *IssuerMetadataResolver) httpClient() *http.Client {
	var client http.Client

	if r != nil && r.Client != nil {
		client = *r.Client
	}

	if client.Timeout == 0 {
		client.Timeout = defaultHTTPTimeout
	}

	originalRedirect := client.CheckRedirect

	client.CheckRedirect = func(
		req *http.Request,
		via []*http.Request,
	) error {
		if len(via) >= maxRedirects {
			return errors.New("too many HTTP redirects")
		}

		if req.URL == nil || req.URL.Scheme != "https" {
			return errors.New(
				"refusing redirect to non-HTTPS URL",
			)
		}

		if err := r.validateURL(req.URL); err != nil {
			return err
		}

		if originalRedirect != nil {
			return originalRedirect(req, via)
		}

		return nil
	}

	return &client
}

func selectVerificationKey(
	set jwkSet,
	kid string,
	alg string,
) (any, error) {
	if strings.TrimSpace(kid) == "" {
		return nil, errors.New(
			"verification key kid is empty",
		)
	}

	var match *jwk

	for i := range set.Keys {
		candidate := &set.Keys[i]

		if candidate.Kid != kid {
			continue
		}

		if match != nil {
			return nil, fmt.Errorf(
				"issuer JWKS contains duplicate kid %q",
				kid,
			)
		}

		match = candidate
	}

	if match == nil {
		return nil, fmt.Errorf(
			"issuer JWKS contains no key with kid %q",
			kid,
		)
	}

	if match.Use != "" && match.Use != "sig" {
		return nil, fmt.Errorf(
			"JWK %q use is %q, expected sig",
			kid,
			match.Use,
		)
	}

	if match.Alg != "" && match.Alg != alg {
		return nil, fmt.Errorf(
			"JWK %q alg %q does not match JWT alg %q",
			kid,
			match.Alg,
			alg,
		)
	}

	if len(match.KeyOps) > 0 && !containsString(
		match.KeyOps,
		"verify",
	) {
		return nil, fmt.Errorf(
			"JWK %q key_ops does not permit verify",
			kid,
		)
	}

	switch alg {
	case "RS256":
		return rsaPublicKeyFromJWK(*match)

	case "ES256":
		return ecdsaPublicKeyFromJWK(*match)

	case "EdDSA":
		return ed25519PublicKeyFromJWK(*match)

	default:
		return nil, fmt.Errorf(
			"unsupported JWS algorithm %q",
			alg,
		)
	}
}

func rsaPublicKeyFromJWK(k jwk) (*rsa.PublicKey, error) {
	if k.Kty != "RSA" {
		return nil, fmt.Errorf(
			"expected RSA JWK, got kty %q",
			k.Kty,
		)
	}

	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf(
			"decode RSA modulus: %w",
			err,
		)
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf(
			"decode RSA exponent: %w",
			err,
		)
	}

	if len(nBytes) == 0 || len(eBytes) == 0 {
		return nil, errors.New(
			"RSA JWK modulus or exponent is missing",
		)
	}

	exponent := 0
	for _, b := range eBytes {
		exponent = exponent<<8 + int(b)
	}

	if exponent <= 0 {
		return nil, errors.New(
			"RSA JWK exponent is invalid",
		)
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nBytes),
		E: exponent,
	}, nil
}

func ecdsaPublicKeyFromJWK(k jwk) (*ecdsa.PublicKey, error) {
	if k.Kty != "EC" {
		return nil, fmt.Errorf(
			"expected EC JWK, got kty %q",
			k.Kty,
		)
	}

	if k.Crv != "P-256" {
		return nil, fmt.Errorf(
			"ES256 requires P-256, got %q",
			k.Crv,
		)
	}

	xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, fmt.Errorf(
			"decode EC x coordinate: %w",
			err,
		)
	}

	yBytes, err := base64.RawURLEncoding.DecodeString(k.Y)
	if err != nil {
		return nil, fmt.Errorf(
			"decode EC y coordinate: %w",
			err,
		)
	}

	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)

	curve := elliptic.P256()

	if !curve.IsOnCurve(x, y) {
		return nil, errors.New(
			"EC JWK point is not on P-256 curve",
		)
	}

	return &ecdsa.PublicKey{
		Curve: curve,
		X:     x,
		Y:     y,
	}, nil
}

func ed25519PublicKeyFromJWK(k jwk) (ed25519.PublicKey, error) {
	if k.Kty != "OKP" {
		return nil, fmt.Errorf(
			"expected OKP JWK, got kty %q",
			k.Kty,
		)
	}

	if k.Crv != "Ed25519" {
		return nil, fmt.Errorf(
			"EdDSA requires Ed25519, got %q",
			k.Crv,
		)
	}

	x, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, fmt.Errorf(
			"decode Ed25519 public key: %w",
			err,
		)
	}

	if len(x) != ed25519.PublicKeySize {
		return nil, fmt.Errorf(
			"Ed25519 public key length = %d, want %d",
			len(x),
			ed25519.PublicKeySize,
		)
	}

	return ed25519.PublicKey(x), nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}