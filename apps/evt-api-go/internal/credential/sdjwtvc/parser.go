package sdjwtvc

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ParsedSDJWT is the compact SD-JWT representation split into its logical
// components.
//
// IssuerJWT is always present.
//
// Disclosures contains the encoded disclosure strings exactly as received.
//
// KBJWT is populated if an SD-JWT+KB presentation was supplied. The first
// implementation slice detects but does not validate the KB-JWT.
type ParsedSDJWT struct {
	IssuerJWT  string
	Disclosures []string
	KBJWT      string
}

// ParseCompact parses the RFC 9901 compact SD-JWT serialization.
//
// Examples:
//
//	<issuer-jwt>~
//	<issuer-jwt>~<disclosure>~
//	<issuer-jwt>~<disclosure>~<disclosure>~
//	<issuer-jwt>~<disclosure>~<kb-jwt>
//
// JWS JSON Serialization is intentionally outside this first slice.
func ParseCompact(encoded string) (ParsedSDJWT, error) {
	var out ParsedSDJWT

	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return out, ErrCredentialEmpty
	}

	parts := strings.Split(encoded, "~")

	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return out, errors.New("SD-JWT issuer-signed JWT is missing")
	}

	out.IssuerJWT = parts[0]

	if strings.Count(out.IssuerJWT, ".") != 2 {
		return out, errors.New(
			"SD-JWT issuer component is not compact JWS serialization",
		)
	}

	if len(parts) == 1 {
		return out, nil
	}

	rest := append([]string(nil), parts[1:]...)

	// A normal SD-JWT without KB-JWT commonly terminates with "~".
	if len(rest) > 0 && rest[len(rest)-1] == "" {
		rest = rest[:len(rest)-1]
	}

	if len(rest) == 0 {
		return out, nil
	}

	// A final compact JWT after the disclosures is the KB-JWT.
	last := rest[len(rest)-1]
	if strings.Count(last, ".") == 2 {
		out.KBJWT = last
		rest = rest[:len(rest)-1]
	}

	for i, disclosure := range rest {
		disclosure = strings.TrimSpace(disclosure)

		if disclosure == "" {
			return out, fmt.Errorf(
				"empty disclosure at position %d",
				i,
			)
		}

		if strings.Contains(disclosure, ".") {
			return out, fmt.Errorf(
				"invalid disclosure at position %d",
				i,
			)
		}

		out.Disclosures = append(out.Disclosures, disclosure)
	}

	return out, nil
}

// DecodeIssuerJWTUnverified extracts the protected header and claims without
// validating the signature.
//
// This function exists solely so that Cvera can obtain iss/kid/alg needed for
// key discovery. No value returned here may be considered trusted until
// cryptographic verification succeeds.
func DecodeIssuerJWTUnverified(
	compact string,
) (header map[string]any, payload map[string]any, err error) {
	parts := strings.Split(compact, ".")
	if len(parts) != 3 {
		return nil, nil, errors.New(
			"issuer JWT must contain exactly three compact JWS segments",
		)
	}

	headerBytes, err := decodeBase64URL(parts[0])
	if err != nil {
		return nil, nil, fmt.Errorf(
			"decode issuer JWT header: %w",
			err,
		)
	}

	payloadBytes, err := decodeBase64URL(parts[1])
	if err != nil {
		return nil, nil, fmt.Errorf(
			"decode issuer JWT payload: %w",
			err,
		)
	}

	var h map[string]any
	if err := json.Unmarshal(headerBytes, &h); err != nil {
		return nil, nil, fmt.Errorf(
			"parse issuer JWT header: %w",
			err,
		)
	}

	var p map[string]any
	if err := json.Unmarshal(payloadBytes, &p); err != nil {
		return nil, nil, fmt.Errorf(
			"parse issuer JWT payload: %w",
			err,
		)
	}

	return h, p, nil
}

func decodeBase64URL(value string) ([]byte, error) {
	value = strings.TrimSpace(value)

	if value == "" {
		return nil, errors.New("base64url value is empty")
	}

	return base64.RawURLEncoding.DecodeString(value)
}