// Generates public SYNTHETIC fixtures using one existing issuance call.
// No private keys are written and no production service is reachable here.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/authority"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/credential"
	ec "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/evidencecredential"
	pv "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/portableverify"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var signCalls int

type signer struct{ key *rsa.PrivateKey }

func (s signer) KeyID() string             { return "local-only-fixture" }
func (s signer) PublicKey() *rsa.PublicKey { return &s.key.PublicKey }
func (s signer) SignEvidenceClaims(ctx context.Context, c ec.Claims) (string, error) {
	signCalls++
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	t.Header["kid"] = s.KeyID()
	t.Header["typ"] = "dc+sd-jwt"
	raw, e := t.SignedString(s.key)
	return raw + "~", e
}

type facts struct {
	value ec.Facts
	err   error
}

func (f facts) WithFacts(ctx context.Context, digest string, fn func(ec.Facts) error) error {
	if f.err != nil {
		return f.err
	}
	return fn(f.value)
}
func snapshot(now time.Time, negative bool) authority.AuthoritySnapshot {
	at := now.Add(-time.Hour).UTC().Truncate(time.Microsecond)
	id := uuid.NewString()
	r := authority.AuthorizationRecord{AuthorizationID: id, IssuerID: ec.Issuer, AuthoritySubjectID: "employer", ClaimType: credential.ClaimTypeEmploymentRole, Adjudication: &authority.AuthorizationAdjudication{DecisionID: uuid.NewString(), AuthorizationID: id, IssuerID: ec.Issuer, AuthoritySubjectID: "employer", ClaimType: credential.ClaimTypeEmploymentRole, Outcome: authority.AdjudicationApproved, DecisionMakerID: "local-adjudicator", AuthorityBasis: "local-only-test", EvidenceRef: "local-only-proof", DecidedAt: at.Add(time.Minute), RecordedAt: at.Add(time.Minute)}}
	var journal authority.AuthorizationJournal
	events := []authority.AuthorizationEvent{authority.EventSubmit, authority.EventApprove, authority.EventActivate}
	if negative {
		events = append(events, authority.EventRevoke)
	}
	for i, event := range events {
		stamp := at.Add(time.Duration(i) * time.Minute)
		if err := journal.Append(authority.AuthorizationJournalEvent{EventID: uuid.NewString(), AuthorizationID: id, EventType: event, EffectiveAt: stamp, RecordedAt: stamp, ActorID: "local-adjudicator", EvidenceRef: "local-only-proof", ConditionsSatisfied: true}); err != nil {
			panic(err)
		}
	}
	return authority.AuthoritySnapshot{Complete: true, RecordingPosition: int64(2 + len(events)), RecordedThrough: now, Entries: []authority.AuthoritySnapshotEntry{{Record: r, Journal: journal}}}
}
func must(e error) {
	if e != nil {
		panic(e)
	}
}
func clone[T any](v T) T {
	b, e := json.Marshal(v)
	must(e)
	var x T
	must(json.Unmarshal(b, &x))
	return x
}
func public(k *rsa.PrivateKey) string {
	b, e := x509.MarshalPKIXPublicKey(&k.PublicKey)
	must(e)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: b}))
}
func signed(k *rsa.PrivateKey, kid, typ string, m map[string]any) string {
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims(m))
	t.Header["kid"] = kid
	t.Header["typ"] = typ
	v, e := t.SignedString(k)
	must(e)
	return v
}

type vector struct {
	Name     string    `json:"name"`
	Input    pv.Input  `json:"input"`
	Expected pv.Result `json:"expected"`
}

func main() {
	if len(os.Args) != 2 {
		panic("public fixture output directory required")
	}
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		panic(err)
	}
	now := time.Unix(1800000000, 0).UTC()
	s := signer{key}
	p := ec.Prepared{Context: ec.Commitment{RequestID: uuid.NewString(), BindingID: uuid.NewString(), CaseID: "local-case", EmployerID: "employer", SubjectID: uuid.NewString(), RequestVersion: 1, SnapshotDigest: ec.Digest([]byte("snapshot")), ManifestID: uuid.NewString(), ManifestDigest: ec.Digest([]byte(`{"evidence":[{"scanStatus":"PENDING"}]}`)), ManualPolicy: ec.ManualPolicy}, Employment: credential.EmploymentRelationship{EmployerID: "employer", Title: "Engineer"}, Snapshot: "snapshot", Manifest: `{"evidence":[{"scanStatus":"PENDING"}]}`, Authority: snapshot(now, false), Key: ec.KeyBinding{Issuer: ec.Issuer, KID: s.KeyID(), Fingerprint: ec.Fingerprint(s.PublicKey()), PublicKey: s.PublicKey(), ValidFrom: now.Add(-time.Hour), ValidUntil: now.Add(time.Hour)}, Now: now, ExpiresAt: now.Add(30 * time.Minute)}
	manifest, _ := json.Marshal(map[string]any{"bindingId": p.Context.BindingID, "requestId": p.Context.RequestID, "caseId": p.Context.CaseID, "subjectId": p.Context.SubjectID, "employerId": p.Context.EmployerID, "issuerId": ec.Issuer, "claimType": credential.ClaimTypeEmploymentRole, "claimDigest": p.Context.SnapshotDigest, "policy": ec.ManualPolicy, "requiredChecks": []string{"role"}, "evidence": []map[string]string{{"uploadId": uuid.NewString(), "checkId": "role", "evidenceHash": ec.Digest([]byte("synthetic only")), "reviewId": uuid.NewString(), "reviewerId": uuid.NewString(), "reviewGrantId": uuid.NewString(), "scanStatus": "PENDING"}}})
	p.Manifest = string(manifest)
	p.Context.ManifestDigest = ec.Digest(manifest)
	claims, raw, err := ec.IssueLocked(context.Background(), p, s)
	if err != nil {
		panic(err)
	}
	if signCalls != 1 {
		panic("expected exactly one synthetic issuance")
	}
	ak, err := rsa.GenerateKey(rand.Reader, 3072)
	must(err)
	sk, err := rsa.GenerateKey(rand.Reader, 3072)
	must(err)
	n := now.Unix()
	base := pv.Input{Credential: raw, Trust: pv.Trust{Issuer: ec.Issuer, KID: s.KeyID(), PublicKey: public(key), AuthorityIssuer: "urn:cvera:synthetic-authority", AuthorityKID: "local-authority", AuthorityKey: public(ak), StatusIssuer: "urn:cvera:synthetic-status", StatusKID: "local-status", StatusKey: public(sk), Synthetic: true}, Policy: pv.Policy{Now: n, ExpectedSubject: claims.Subject}}
	auth := func(v *pv.Input, decision string) {
		v.Authority = signed(ak, base.Trust.AuthorityKID, "cvera-stage4b-fixture-authority+jwt", map[string]any{"iss": base.Trust.AuthorityIssuer, "synthetic": true, "iat": n - 3600, "exp": n + 7200, "credential_sha256": ec.Digest([]byte(v.Credential)), "issuer": ec.Issuer, "employer_id": p.Context.EmployerID, "claim_type": credential.ClaimTypeEmploymentRole, "decision": decision})
	}
	status := func(v *pv.Input, age int64, state string, empty bool) {
		entries := []map[string]string{}
		if !empty {
			entries = append(entries, map[string]string{"credential_id": claims.ID, "state": state})
		}
		v.Status = &pv.Status{Availability: "AVAILABLE", JWS: signed(sk, base.Trust.StatusKID, "cvera-stage4b-fixture-status+jwt", map[string]any{"iss": base.Trust.StatusIssuer, "synthetic": true, "iat": n - age, "credential_sha256": ec.Digest([]byte(v.Credential)), "entries": entries})}
	}
	auth(&base, "AUTHORIZED")
	status(&base, 0, "GOOD", false)
	expected := pv.Result{Contract: pv.Contract, Synthetic: true, Crypto: "VALID", Binding: "VALID", Temporal: "VALID", Authority: "AUTHORIZED", Status: "GOOD", Holder: "UNKNOWN", Eligible: true, Overall: "INDETERMINATE"}
	var vectors []vector
	add := func(name string, mut func(*pv.Input), change func(*pv.Result)) {
		v := clone(base)
		want := expected
		mut(&v)
		change(&want)
		want.Eligible = want.Crypto == "VALID" && want.Binding == "VALID" && want.Temporal == "VALID" && want.Authority == "AUTHORIZED" && want.Status == "GOOD"
		if want.Crypto == "INVALID" || want.Crypto == "UNSUPPORTED" || want.Binding == "INVALID" || want.Temporal == "INVALID" || want.Authority == "INVALID" || want.Authority == "UNAUTHORIZED" || want.Status == "INVALID" || want.Status == "REVOKED" {
			want.Overall = "REJECTED"
		}
		vectors = append(vectors, vector{name, v, want})
	}
	unchanged := func(*pv.Result) {}
	add("fresh", func(*pv.Input) {}, unchanged)
	for _, age := range []int64{899, 900, 901, 86400} {
		age := age
		add(fmt.Sprint("status-age-", age), func(v *pv.Input) { status(v, age, "GOOD", false) }, func(r *pv.Result) {
			if age > 900 {
				r.Status = "STALE"
			}
		})
	}
	for _, state := range []string{"missing", "unavailable", "missing-entry", "revoked", "future", "invalid-state", "invalid-signature", "untrusted-key", "wrong-binding"} {
		state := state
		add("status-"+state, func(v *pv.Input) {
			switch state {
			case "missing":
				v.Status = nil
			case "unavailable":
				v.Status = &pv.Status{Availability: "UNAVAILABLE"}
			case "missing-entry":
				status(v, 0, "GOOD", true)
			case "revoked":
				status(v, 0, "REVOKED", false)
			case "future":
				status(v, -1, "GOOD", false)
			case "invalid-state":
				status(v, 0, "VERIFIED", false)
			case "invalid-signature":
				v.Status.JWS = strings.Split(v.Status.JWS, ".")[0] + ".e30.invalid"
			case "untrusted-key":
				v.Trust.StatusKey = public(ak)
			case "wrong-binding":
				v.Status.JWS = signed(sk, base.Trust.StatusKID, "cvera-stage4b-fixture-status+jwt", map[string]any{"iss": base.Trust.StatusIssuer, "synthetic": true, "iat": n, "credential_sha256": ec.Digest([]byte("different")), "entries": []map[string]string{{"credential_id": claims.ID, "state": "GOOD"}}})
			}
		}, func(r *pv.Result) {
			switch state {
			case "missing", "unavailable", "missing-entry":
				r.Status = "UNKNOWN"
			case "revoked":
				r.Status = "REVOKED"
			default:
				r.Status = "INVALID"
			}
		})
	}

	for _, field := range []string{"crit", "b64"} {
		field := field
		add("status-null-"+field, func(v *pv.Input) {
			payload, err := base64.RawURLEncoding.DecodeString(strings.Split(v.Status.JWS, ".")[1])
			must(err)
			var claims jwt.MapClaims
			must(json.Unmarshal(payload, &claims))
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			token.Header["kid"] = base.Trust.StatusKID
			token.Header["typ"] = "cvera-stage4b-fixture-status+jwt"
			token.Header[field] = nil
			v.Status.JWS, err = token.SignedString(sk)
			must(err)
		}, func(r *pv.Result) { r.Status = "INVALID" })
	}
	for _, age := range []int64{60, 61} {
		age := age
		add(fmt.Sprint("configured-60-age-", age), func(v *pv.Input) { x := int64(60); v.Policy.MaxStatusAge = &x; status(v, age, "GOOD", false) }, func(r *pv.Result) {
			if age > 60 {
				r.Status = "STALE"
			}
		})
	}
	for _, h := range []string{"UNSUPPORTED", "UNRESOLVED", "VALID"} {
		h := h
		add("holder-"+h, func(v *pv.Input) { v.Holder = h }, func(r *pv.Result) {
			if h == "UNSUPPORTED" {
				r.Holder = "UNSUPPORTED"
			}
		})
	}
	for _, a := range []string{"missing", "UNKNOWN", "UNAUTHORIZED", "INVALID", "untrusted-key"} {
		a := a
		add("authority-"+a, func(v *pv.Input) {
			switch a {
			case "missing":
				v.Authority = ""
			case "untrusted-key":
				v.Trust.AuthorityKey = public(sk)
			default:
				auth(v, a)
			}
		}, func(r *pv.Result) {
			switch a {
			case "missing", "UNKNOWN":
				r.Authority = "UNKNOWN"
			case "untrusted-key":
				r.Authority = "INVALID"
			default:
				r.Authority = a
			}
		})
	}
	early := func(state string) func(*pv.Result) {
		return func(r *pv.Result) {
			r.Crypto = state
			r.Binding = "UNKNOWN"
			r.Temporal = "UNKNOWN"
			r.Authority = "UNKNOWN"
			r.Status = "UNKNOWN"
		}
	}
	for _, c := range []string{"tampered-payload", "wrong-key", "wrong-issuer", "invalid-signature", "unsupported-serialization"} {
		c := c
		add("credential-"+c, func(v *pv.Input) {
			switch c {
			case "tampered-payload":
				p := strings.Split(raw, ".")
				p[1] = base64.RawURLEncoding.EncodeToString([]byte(`{"tampered":true}`))
				v.Credential = strings.Join(p, ".")
			case "wrong-key":
				v.Trust.PublicKey = public(sk)
			case "wrong-issuer":
				v.Trust.Issuer = "https://untrusted.invalid"
			case "invalid-signature":
				v.Credential = strings.Join(strings.Split(raw, ".")[:2], ".") + ".invalid~"
			default:
				v.Credential = raw + "fake~"
			}
		}, early(map[bool]string{true: "UNSUPPORTED", false: "INVALID"}[c == "unsupported-serialization"]))
	}
	payload := map[string]any{}
	b, err := json.Marshal(claims)
	must(err)
	must(json.Unmarshal(b, &payload))
	mutated := func(v *pv.Input, change func(map[string]any)) {
		m := clone(payload)
		change(m)
		v.Credential = signed(key, s.KeyID(), "dc+sd-jwt", m) + "~"
		auth(v, "AUTHORIZED")
		status(v, 0, "GOOD", false)
	}
	add("malformed-binding", func(v *pv.Input) {
		mutated(v, func(m map[string]any) { m["evidence_binding"].(map[string]any)["snapshot_sha256"] = "not-a-digest" })
	}, func(r *pv.Result) { r.Binding = "INVALID" })
	add("wrong-subject-binding", func(v *pv.Input) { v.Policy.ExpectedSubject = "00000000-0000-4000-8000-000000000099" }, func(r *pv.Result) { r.Binding = "INVALID" })
	add("expired", func(v *pv.Input) {
		mutated(v, func(m map[string]any) { m["iat"] = n - 7200; m["nbf"] = n - 7200; m["exp"] = n })
	}, func(r *pv.Result) { r.Temporal = "INVALID" })
	add("not-yet-valid", func(v *pv.Input) {
		mutated(v, func(m map[string]any) { m["iat"] = n + 1; m["nbf"] = n + 1; m["exp"] = n + 3600 })
	}, func(r *pv.Result) { r.Temporal = "INVALID" })
	add("unsupported-vct", func(v *pv.Input) { mutated(v, func(m map[string]any) { m["vct"] = "urn:unsupported" }) }, early("UNSUPPORTED"))
	add("unsupported-algorithm", func(v *pv.Input) {
		t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims(payload))
		t.Header["typ"] = "dc+sd-jwt"
		t.Header["kid"] = s.KeyID()
		x, e := t.SignedString([]byte("LOCAL-NEGATIVE-ONLY"))
		must(e)
		v.Credential = x + "~"
	}, early("UNSUPPORTED"))
	add("untrusted-credential-freshness-override", func(v *pv.Input) {
		mutated(v, func(m map[string]any) { m["maxStatusAgeSeconds"] = 999999 })
		status(v, 901, "GOOD", false)
	}, func(r *pv.Result) { r.Status = "STALE" })
	add("untrusted-status-freshness-override", func(v *pv.Input) {
		v.Status.JWS = signed(sk, base.Trust.StatusKID, "cvera-stage4b-fixture-status+jwt", map[string]any{"iss": base.Trust.StatusIssuer, "synthetic": true, "iat": n - 901, "maxStatusAgeSeconds": 999999, "credential_sha256": ec.Digest([]byte(raw)), "entries": []map[string]string{{"credential_id": claims.ID, "state": "GOOD"}}})
	}, func(r *pv.Result) { r.Status = "STALE" })
	for _, v := range vectors {
		got, e := pv.Verify(v.Input)
		must(e)
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(v.Expected)
		if string(a) != string(b) {
			panic(fmt.Sprintf("vector %s: got %s want %s", v.Name, a, b))
		}
	}
	out := os.Args[1]
	must(os.MkdirAll(out, 0755))
	data, e := json.MarshalIndent(vectors, "", "  ")
	must(e)
	must(os.WriteFile(filepath.Join(out, "vectors.json"), append(data, '\n'), 0644))
	data, e = json.MarshalIndent(map[string]any{"synthetic": true, "issuanceCalls": signCalls, "credentialSha256": ec.Digest([]byte(raw)), "deterministicNow": n, "vectors": len(vectors), "privateKeysWritten": false, "negativeSignatures": "mutated fixture JWTs only; no additional issuance workflow calls"}, "", "  ")
	must(e)
	must(os.WriteFile(filepath.Join(out, "manifest.json"), append(data, '\n'), 0644))
	fmt.Println(string(data))
}
