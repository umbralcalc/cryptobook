package recovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
)

// TestSyntheticRecoveryExpectedBehaviour is the binding test named by every claim
// in behaviour.go — one subtest per claim, named by the claim's ID.
func TestSyntheticRecoveryExpectedBehaviour(t *testing.T) {
	for _, claim := range ObservedBehaviour() {
		t.Run(claim.ID, func(t *testing.T) {
			if err := claims.Verify(claim); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

// readConfig returns a config's text for the drift checks below.
func readConfig(t *testing.T, name string) string {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(cfgrun.ConfigDir(), name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(source)
}

// TestTruthMatchesTheConfig pins the recovery target in this package to the
// parameters the config actually generates with.
//
// Without it, editing the config's rates would leave every claim comparing the
// posterior against the wrong truth — and the claims would keep passing or failing
// for reasons unrelated to the sampler. That is a silent-wrongness failure mode,
// so it gets a mechanical check rather than a comment.
func TestTruthMatchesTheConfig(t *testing.T) {
	source := readConfig(t, recoveryConfig)
	for i, spelling := range []string{
		fmt.Sprintf("limit_rate: [%g]", trueRates[0]),
		fmt.Sprintf("cancel_rate: [%g]", trueRates[1]),
		fmt.Sprintf("market_rate: [%g]", trueRates[2]),
	} {
		if !strings.Contains(source, spelling) {
			t.Errorf(
				"%s does not contain %q — trueRates[%d] (%s) has drifted from the "+
					"config it is supposed to describe",
				recoveryConfig, spelling, i, rateNames[i])
		}
	}
	if !strings.Contains(source, fmt.Sprintf(
		"[%g, %g, %g]", priorRates[0], priorRates[1], priorRates[2])) {
		t.Errorf("%s does not contain the prior %v", recoveryConfig, priorRates)
	}
}

// TestProposalMatchesTheConfig pins proposalVariance to the config's
// default_covariance.
//
// The ESS measurement draws its own proposals in Go, so if the config's proposal
// width changed and proposalVariance did not, the reported ESS would describe a
// distribution the recovery run never samples from — a measurement of nothing,
// which would still produce a perfectly plausible number.
func TestProposalMatchesTheConfig(t *testing.T) {
	source := readConfig(t, recoveryConfig)
	for i, variance := range proposalVariance {
		if !strings.Contains(source, fmt.Sprintf("%g", variance)) {
			t.Errorf(
				"%s does not mention %g, proposalVariance[%d] (%s); the ESS "+
					"measurement would be sampling a different proposal than the config "+
					"does", recoveryConfig, variance, i, rateNames[i])
		}
	}
}

// TestSurfaceConfigHasNoSampler guards what makes the identification claims
// meaningful: the surface config must contain no posterior sampler at all. If a
// sampler leaked into it, the "advantage of the true value" numbers would be
// measuring the sampler's luck rather than the shape of the likelihood.
func TestSurfaceConfigHasNoSampler(t *testing.T) {
	source := readConfig(t, surfaceConfig)
	for _, forbidden := range []string{
		"posterior_estimation", "sampler:", "lob_post_sampler",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf(
				"%s contains %q — it must hold the likelihood surface ONLY, with the "+
					"parameters fixed, or the identification claims measure the sampler "+
					"instead of the surface",
				surfaceConfig, forbidden)
		}
	}
}

// TestWindowHistoryDepthMatchesWindowDepth guards the trap that broke this spike's
// first result.
//
// window_data_history_depth must EQUAL the window depth. It sets the outer replay
// partition's state-history depth, and FromHistoryIteration walks that buffer from
// row depth-2 down to depth-Window.Depth-1 — so anything larger anchors the window
// in rows that are still zero-filled. The likelihood is then scored against zeros,
// which does not weaken it but inverts it: a near-zero parameter outscores the truth
// and the posterior converges onto its prior and freezes.
//
// stochadex v0.13.1 now warns on the oversized side, in response to this repo
// reporting it. This test stays for two reasons: a warning goes to the log, where
// CI will not fail on it and a human may not read it; and the equality is a property
// of these configs worth asserting locally rather than inheriting.
func TestWindowHistoryDepthMatchesWindowDepth(t *testing.T) {
	for _, name := range []string{recoveryConfig, surfaceConfig} {
		t.Run(name, func(t *testing.T) {
			source := readConfig(t, name)
			depth, err := scalarAfter(source, "depth: ")
			if err != nil {
				t.Fatalf("finding the window depth: %v", err)
			}
			if !strings.Contains(source, fmt.Sprintf("{lob_flow: %s}", depth)) {
				t.Errorf(
					"window depth is %s but window_data_history_depth is not "+
						"{lob_flow: %s}; a larger value makes the window replay zeros for "+
						"most of the run and the likelihood silently meaningless",
					depth, depth)
			}
		})
	}
}

// scalarAfter returns the value following the first occurrence of a key, up to the
// end of that line.
func scalarAfter(source, key string) (string, error) {
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if after, found := strings.CutPrefix(trimmed, key); found {
			return strings.TrimSpace(after), nil
		}
	}
	return "", fmt.Errorf("no line starting with %q", key)
}
