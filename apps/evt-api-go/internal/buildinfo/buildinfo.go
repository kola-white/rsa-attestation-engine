package buildinfo

// GitSHA identifies the source revision from which the running binary
// was built.
//
// Development builds default to "development". Production/deployment
// builds must override this value at compile time using:
//
//	go build -ldflags "-X github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/buildinfo.GitSHA=<git-sha>"
//
// Runtime code must not infer revision identity from a .git checkout.
var GitSHA = "development"
