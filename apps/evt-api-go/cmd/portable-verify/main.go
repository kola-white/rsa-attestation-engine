// Local-only runner: stdin contains public synthetic fixtures; no service wiring.
package main

import (
	"encoding/json"
	"fmt"
	p "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/portableverify"
	"io"
	"os"
)

func main() {
	b, e := io.ReadAll(io.LimitReader(os.Stdin, (4<<20)+1))
	if e != nil || len(b) > 4<<20 {
		fmt.Fprintln(os.Stderr, "invalid input size")
		os.Exit(2)
	}
	var in p.Input
	if json.Unmarshal(b, &in) != nil {
		fmt.Fprintln(os.Stderr, "invalid JSON")
		os.Exit(2)
	}
	r, e := p.Verify(in)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	if json.NewEncoder(os.Stdout).Encode(r) != nil {
		os.Exit(2)
	}
}
