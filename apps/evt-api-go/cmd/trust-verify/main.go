// Local public-artifact runner: trusted policy/anchor are supplied by operator.
package main

import (
	"encoding/json"
	"io"
	"os"

	hp "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/holderpresentation"
	t "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/portabletrust"
)

type session struct {
	Inputs        []t.Input  `json:"inputs"`
	Presentations []hp.Input `json:"presentations"`
	Mode          string     `json:"mode"`
}

func main() {
	raw, e := io.ReadAll(io.LimitReader(os.Stdin, (32<<20)+1))
	if e != nil || len(raw) > 32<<20 {
		os.Exit(2)
	}
	var s session
	if json.Unmarshal(raw, &s) != nil {
		os.Exit(2)
	}
	if s.Mode != "" && s.Mode != "holder" && s.Mode != "presentation" {
		os.Exit(2)
	}
	state := t.NewState()
	replay := hp.NewStore()
	out := []any{}
	for n, in := range s.Inputs {
		if s.Mode != "" {
			if n >= len(s.Presentations) {
				os.Exit(2)
			}
			c := s.Presentations[n]
			var r hp.Result
			if s.Mode == "holder" {
				r = hp.EvaluateHolderWithEvaluator(c, t.BindEvaluator(in, state))
			} else {
				r = hp.VerifyWithEvaluator(c, replay, t.BindEvaluator(in, state))
			}
			out = append(out, r)
		} else {
			r, e := t.Verify(in, state)
			if e != nil {
				out = append(out, map[string]string{"error": "INVALID_POLICY"})
			} else {
				out = append(out, r)
			}
		}
	}
	if json.NewEncoder(os.Stdout).Encode(out) != nil {
		os.Exit(2)
	}
}
