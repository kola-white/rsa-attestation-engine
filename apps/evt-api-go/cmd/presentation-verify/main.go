// Local-only public-fixture runner. Transactions are trusted operator inputs,
// never initialized from an incoming presentation. No production wiring.
package main

import (
	"encoding/json"
	p "github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/holderpresentation"
	"io"
	"os"
	"sync"
)

type session struct {
	Inputs     []p.Input `json:"inputs"`
	Concurrent bool      `json:"concurrent"`
	Holder     bool      `json:"holder"`
}

func main() {
	data, e := io.ReadAll(io.LimitReader(os.Stdin, (16<<20)+1))
	if e != nil || len(data) > 16<<20 {
		os.Exit(2)
	}
	var s session
	if json.Unmarshal(data, &s) != nil {
		os.Exit(2)
	}
	store := p.NewStore()
	out := make([]p.Result, len(s.Inputs))
	var wg sync.WaitGroup
	for n, in := range s.Inputs {
		eval := func(n int, in p.Input) {
			defer wg.Done()
			if s.Holder {
				out[n] = p.EvaluateHolder(in)
			} else {
				out[n] = p.Verify(in, store)
			}
		}
		wg.Add(1)
		if s.Concurrent {
			go eval(n, in)
		} else {
			eval(n, in)
		}
	}
	wg.Wait()
	if json.NewEncoder(os.Stdout).Encode(out) != nil {
		os.Exit(2)
	}
}
