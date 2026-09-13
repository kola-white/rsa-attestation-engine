package sdjwtvc

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type disclosureKind int

const (
	disclosureObjectProperty disclosureKind = iota + 1
	disclosureArrayElement
)

type decodedDisclosure struct {
	Encoded string
	Digest  string

	Kind disclosureKind

	Name  string
	Value any
}

// ApplyDisclosures applies RFC 9901 disclosures to the verified issuer JWT
// payload.
//
// The input issuer payload MUST already have passed JWS verification.
//
// It returns:
//
//   - the processed payload;
//   - the selectively disclosed paths actually applied;
//   - an error if disclosure integrity/structure is invalid.
//
// This implementation supports sha-256, which RFC 9901 requires
// implementations to support.
func ApplyDisclosures(
	payload map[string]any,
	encodedDisclosures []string,
) (map[string]any, []string, error) {
	if payload == nil {
		return nil, nil, errors.New(
			"SD-JWT payload is nil",
		)
	}

	hashAlg := "sha-256"

	if rawAlg, ok := payload["_sd_alg"]; ok {
		value, ok := rawAlg.(string)
		if !ok {
			return nil, nil, errors.New(
				"_sd_alg must be a string",
			)
		}

		hashAlg = value
	}

	if hashAlg != "sha-256" {
		return nil, nil, fmt.Errorf(
			"unsupported SD-JWT disclosure hash algorithm %q",
			hashAlg,
		)
	}

	disclosures := make(
		map[string]decodedDisclosure,
		len(encodedDisclosures),
	)

	for i, encoded := range encodedDisclosures {
		disclosure, err := decodeDisclosure(
			encoded,
			hashAlg,
		)
		if err != nil {
			return nil, nil, fmt.Errorf(
				"disclosure %d: %w",
				i,
				err,
			)
		}

		if _, exists := disclosures[disclosure.Digest]; exists {
			return nil, nil, fmt.Errorf(
				"duplicate disclosure digest %q",
				disclosure.Digest,
			)
		}

		disclosures[disclosure.Digest] = disclosure
	}

	copied, err := deepCopyJSON(payload)
	if err != nil {
		return nil, nil, err
	}

	used := map[string]bool{}
	referenced := map[string]bool{}
	var disclosedPaths []string

	processed, err := processDisclosureValue(
		copied,
		"",
		disclosures,
		used,
		referenced,
		&disclosedPaths,
	)
	if err != nil {
		return nil, nil, err
	}

	result, ok := processed.(map[string]any)
	if !ok {
		return nil, nil, errors.New(
			"processed SD-JWT payload is not a JSON object",
		)
	}

	// _sd_alg controls disclosure hashing and is not part of the processed
	// user-claim payload.
	delete(result, "_sd_alg")

	for digest := range disclosures {
		if !used[digest] {
			return nil, nil, fmt.Errorf(
				"disclosure digest %q is not referenced by the SD-JWT payload",
				digest,
			)
		}
	}

	sort.Strings(disclosedPaths)

	return result, disclosedPaths, nil
}

func decodeDisclosure(
	encoded string,
	hashAlg string,
) (decodedDisclosure, error) {
	var out decodedDisclosure

	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return out, errors.New("disclosure is empty")
	}

	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return out, fmt.Errorf(
			"base64url decode: %w",
			err,
		)
	}

	var values []any
	if err := json.Unmarshal(raw, &values); err != nil {
		return out, fmt.Errorf(
			"decode disclosure JSON: %w",
			err,
		)
	}

	if len(values) != 2 && len(values) != 3 {
		return out, fmt.Errorf(
			"disclosure array length = %d, want 2 or 3",
			len(values),
		)
	}

	salt, ok := values[0].(string)
	if !ok || strings.TrimSpace(salt) == "" {
		return out, errors.New(
			"disclosure salt must be a non-empty string",
		)
	}

	digest, err := disclosureDigest(encoded, hashAlg)
	if err != nil {
		return out, err
	}

	out.Encoded = encoded
	out.Digest = digest

	if len(values) == 3 {
		name, ok := values[1].(string)
		if !ok || strings.TrimSpace(name) == "" {
			return out, errors.New(
				"object-property disclosure name must be a non-empty string",
			)
		}

		out.Kind = disclosureObjectProperty
		out.Name = name
		out.Value = values[2]

		return out, nil
	}

	out.Kind = disclosureArrayElement
	out.Value = values[1]

	return out, nil
}

func disclosureDigest(
	encodedDisclosure string,
	hashAlg string,
) (string, error) {
	switch hashAlg {
	case "sha-256":
		sum := sha256.Sum256(
			[]byte(encodedDisclosure),
		)

		return base64.RawURLEncoding.EncodeToString(
			sum[:],
		), nil

	default:
		return "", fmt.Errorf(
			"unsupported disclosure hash algorithm %q",
			hashAlg,
		)
	}
}

func processDisclosureValue(
	value any,
	path string,
	disclosures map[string]decodedDisclosure,
	used map[string]bool,
	referenced map[string]bool,
	disclosedPaths *[]string,
) (any, error) {
	switch current := value.(type) {
	case map[string]any:
		return processDisclosureObject(
			current,
			path,
			disclosures,
			used,
			referenced,
			disclosedPaths,
		)

	case []any:
		return processDisclosureArray(
			current,
			path,
			disclosures,
			used,
			referenced,
			disclosedPaths,
		)

	default:
		return value, nil
	}
}

func processDisclosureObject(
	object map[string]any,
	path string,
	disclosures map[string]decodedDisclosure,
	used map[string]bool,
	referenced map[string]bool,
	disclosedPaths *[]string,
) (map[string]any, error) {
	result := make(map[string]any)

	// First process ordinary visible properties.
	for key, value := range object {
		if key == "_sd" || key == "_sd_alg" {
			continue
		}

		processed, err := processDisclosureValue(
			value,
			joinPath(path, key),
			disclosures,
			used,
			referenced,
			disclosedPaths,
		)
		if err != nil {
			return nil, err
		}

		result[key] = processed
	}

	rawDigests, hasSD := object["_sd"]
	if !hasSD {
		return result, nil
	}

	digestList, ok := rawDigests.([]any)
	if !ok {
		return nil, errors.New(
			"_sd must be an array",
		)
	}

	for _, rawDigest := range digestList {
		digest, ok := rawDigest.(string)
		if !ok || strings.TrimSpace(digest) == "" {
			return nil, errors.New(
				"_sd entries must be non-empty strings",
			)
		}

		if referenced[digest] {
			return nil, fmt.Errorf(
				"SD-JWT digest %q is referenced more than once",
				digest,
			)
		}

		referenced[digest] = true

		disclosure, supplied := disclosures[digest]
		if !supplied {
			// The holder did not disclose this claim.
			continue
		}

		if disclosure.Kind != disclosureObjectProperty {
			return nil, fmt.Errorf(
				"digest %q references an array disclosure from an object",
				digest,
			)
		}

		if _, exists := result[disclosure.Name]; exists {
			return nil, fmt.Errorf(
				"disclosure would overwrite existing claim %q",
				disclosure.Name,
			)
		}

		if used[digest] {
			return nil, fmt.Errorf(
				"disclosure digest %q used more than once",
				digest,
			)
		}

		used[digest] = true

		disclosedPath := joinPath(
			path,
			disclosure.Name,
		)

		processed, err := processDisclosureValue(
			disclosure.Value,
			disclosedPath,
			disclosures,
			used,
			referenced,
			disclosedPaths,
		)
		if err != nil {
			return nil, err
		}

		result[disclosure.Name] = processed

		*disclosedPaths = append(
			*disclosedPaths,
			disclosedPath,
		)
	}

	return result, nil
}

func processDisclosureArray(
	array []any,
	path string,
	disclosures map[string]decodedDisclosure,
	used map[string]bool,
	referenced map[string]bool,
	disclosedPaths *[]string,
) ([]any, error) {
	result := make([]any, 0, len(array))

	for index, item := range array {
		placeholder, isPlaceholder := item.(map[string]any)

		if isPlaceholder &&
			len(placeholder) == 1 {
			rawDigest, hasDigest := placeholder["..."]

			if hasDigest {
				digest, ok := rawDigest.(string)
				if !ok || strings.TrimSpace(digest) == "" {
					return nil, errors.New(
						"array disclosure digest must be a non-empty string",
					)
				}

				if referenced[digest] {
					return nil, fmt.Errorf(
						"SD-JWT digest %q is referenced more than once",
						digest,
					)
				}

				referenced[digest] = true

				disclosure, supplied := disclosures[digest]
				if !supplied {
					// Undisclosed array element disappears from the
					// processed payload.
					continue
				}

				if disclosure.Kind != disclosureArrayElement {
					return nil, fmt.Errorf(
						"digest %q references an object-property disclosure from an array",
						digest,
					)
				}

				if used[digest] {
					return nil, fmt.Errorf(
						"disclosure digest %q used more than once",
						digest,
					)
				}

				used[digest] = true

				elementPath := fmt.Sprintf(
					"%s[%d]",
					path,
					index,
				)

				processed, err := processDisclosureValue(
					disclosure.Value,
					elementPath,
					disclosures,
					used,
					referenced,
					disclosedPaths,
				)
				if err != nil {
					return nil, err
				}

				result = append(result, processed)

				*disclosedPaths = append(
					*disclosedPaths,
					elementPath,
				)

				continue
			}
		}

		processed, err := processDisclosureValue(
			item,
			fmt.Sprintf("%s[%d]", path, index),
			disclosures,
			used,
			referenced,
			disclosedPaths,
		)
		if err != nil {
			return nil, err
		}

		result = append(result, processed)
	}

	return result, nil
}

func deepCopyJSON(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf(
			"copy SD-JWT payload: %w",
			err,
		)
	}

	var copied any
	if err := json.Unmarshal(encoded, &copied); err != nil {
		return nil, fmt.Errorf(
			"copy SD-JWT payload: %w",
			err,
		)
	}

	return copied, nil
}

func joinPath(parent string, child string) string {
	if parent == "" {
		return child
	}

	return parent + "." + child
}

// VisibleEmploymentClaimPaths reports visible employment-domain claim paths
// after SD-JWT disclosure processing.
//
// These represent what was actually exposed in this credential/presentation,
// rather than attempting to infer undisclosed claim names.
func VisibleEmploymentClaimPaths(
	payload map[string]any,
) []string {
	var paths []string

	for _, root := range []string{
		"subject",
		"employment",
	} {
		value, ok := payload[root]
		if !ok {
			continue
		}

		collectVisiblePaths(
			value,
			root,
			&paths,
		)
	}

	// sub is a standard JWT claim and may serve as the subject identifier.
	if _, ok := payload["sub"]; ok {
		paths = append(paths, "sub")
	}

	sort.Strings(paths)

	return paths
}

func collectVisiblePaths(
	value any,
	path string,
	paths *[]string,
) {
	switch current := value.(type) {
	case map[string]any:
		if len(current) == 0 {
			*paths = append(*paths, path)
			return
		}

		for key, child := range current {
			collectVisiblePaths(
				child,
				joinPath(path, key),
				paths,
			)
		}

	case []any:
		if len(current) == 0 {
			*paths = append(*paths, path)
			return
		}

		for index, child := range current {
			collectVisiblePaths(
				child,
				fmt.Sprintf(
					"%s[%d]",
					path,
					index,
				),
				paths,
			)
		}

	default:
		*paths = append(*paths, path)
	}
}