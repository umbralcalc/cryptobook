// Command gen-claims regenerates CLAIMS.md from the repo's claim set.
//
// Run it from the repo root after adding or changing a claim:
//
//	go run ./cmd/gen-claims
//
// It refuses to write an invalid set, so a claim missing its binding or its stated
// limitations cannot reach the page even by regenerating.
package main

import (
	"fmt"
	"os"

	"github.com/umbralcalc/cryptobook/internal/claimset"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

const claimsFile = "CLAIMS.md"

func main() {
	set := claimset.All()
	if err := claims.Check(set); err != nil {
		fmt.Fprintf(os.Stderr, "gen-claims: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(claimsFile, []byte(claims.Markdown(set)), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gen-claims: writing %s: %v\n", claimsFile, err)
		os.Exit(1)
	}
	fmt.Printf("gen-claims: wrote %s (%d claims)\n", claimsFile, len(set))
}
