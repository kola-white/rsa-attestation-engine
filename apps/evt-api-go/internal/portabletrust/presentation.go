package portabletrust

import (
	"errors"

	hp "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/holderpresentation"
	b "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/portableverify"
)

// BindEvaluator refuses cross-credential or policy substitution. No field in a
// credential/request can install this callback. Stage4C transport remains local.
func BindEvaluator(in Input, state *State) hp.CredentialEvaluator {
	return func(base b.Input) (b.Result, error) {
		if base.Credential != in.Credential || base.Policy.Now != in.Policy.Now || base.Policy.ExpectedSubject != in.Policy.ExpectedSubject || !in.Anchor.Synthetic {
			return blank(in).Credential, errors.New("presentation context mismatch")
		}
		r, e := Verify(in, state)
		return r.Credential, e
	}
}
