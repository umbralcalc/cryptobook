// Package conservation pins the accounting invariants that make an age-cohort model's
// series comparable to the market's, after two defects showed the models were losing
// track of volume in two different ways.
//
// # Defect one: departures that were never reported
//
// In the capped variants the oldest cohort's survivors are DISCARDED — `aged` gives cohort
// c whatever survived in cohort c-1, and nothing reads `left` at the oldest cohort. That
// volume left the book without being traded and appeared in no output column.
//
// The market counts exactly that volume as a cancellation. pkg/feed/bucket.go derives
// cancelled = removed - executed, where executed is capped by volume actually traded at
// that price, so a diff feed cannot see WHY an order left and attributes every untraded
// departure to cancellation. The two series were not like-for-like.
//
// It mattered most where the mechanism was weakest. Expiry is the maximally age-weighted
// cancellation channel — volume that survived every cohort — which is exactly what a
// time-in-queue mechanism exists to test, so the defect deleted the mechanism's own signal
// in proportion to how much the mechanism had been weakened to reach its regime. At
// haz0 0.18 expiry is 0.13% of cancellations; at haz0 0.016 it is 43%.
//
// # Defect two: depth the arrival damping could not see
//
// Level depth was summed with a slice width hardcoded to 8 while ncoh was 12, so each
// level's four oldest cohorts were invisible to the arrival damping — 22.4% of resting
// volume. That is not a reporting error: the damping denominator is dynamics, and it
// contradicted the config's own pre-registered header ("ONLY THE COHORT COUNT MOVES ...
// arrival side and driver are all inherited unchanged"). Correcting it moved mean depth
// from 232.1 to 214.7 and VOIDED the AX-BB result. See DECISIONS.md.
//
// # What is pinned, and why these two
//
// Both defects are the same failure: the model lost track of volume, silently, in a way
// no existing test could see. So the fix is not two patches but two laws.
//
//	depth(t+1) - depth(t) = arrivals(t) - cancellations(t) - trades(t)
//	sum over levels of the depth the damping reads = total resting volume
//
// The first is the market aggregator's own decomposition. Each config satisfies it its own
// way — cfg/lob_ages.yaml has an ABSORBING oldest cohort and so conserved volume already,
// which is why the correction is NOT uniform and adding an expiry term there would
// double-count. Both laws are checked against every cfg/lob_ages*.yaml DISCOVERED on disk,
// not a hand-written list, because a hand-written list is what let one defect sit in two
// files while a third was clean.
package conservation

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/umbralcalc/cryptobook/pkg/cfgrun"
	"github.com/umbralcalc/cryptobook/pkg/claims"
	"github.com/umbralcalc/cryptobook/pkg/diagnostics"
)

const (
	phase   = "2 — Residual diagnostics"
	dataset = "synthetic — every cfg/lob_ages*.yaml discovered on disk rather than listed. " +
		"Model-internal: the conservation law is the market aggregator's own decomposition, " +
		"but no market data is read here"

	// An exact identity needs COVERAGE, not precision — the residual is either zero or it
	// is not — so this runs fewer and shorter members than a statistical claim would.
	seeds  = 8
	steps  = 2000
	settle = 100

	// nGroups is the (side, level) count every age model shares: 2 sides x 8 levels.
	nGroups = 16
)

var (
	rePartition = regexp.MustCompile(`(?m)^\s+- name: (\w+)\s*$`)
	reAgeWidth  = regexp.MustCompile(`\{name: ages, width: (\d+)\}`)
	reNcoh      = regexp.MustCompile(`ncoh: \[([\d.]+)\]`)
	// The width the config actually uses to sum a level's cohorts. Parsed rather than
	// assumed, so that hardcoding it back to a literal shows up as lost coverage.
	reLevelWidth = regexp.MustCompile(`sum\(slice\(ages, j \* ncoh, ([a-z0-9_]+)\)\)`)
)

type model struct {
	file, partition string
	width           int // total `ages` slots
	ncoh            int // cohorts per level
	levelWidth      int // cohorts per level the damping actually sums
}

func discover() ([]model, error) {
	paths, err := filepath.Glob(filepath.Join(cfgrun.ConfigDir(), "lob_ages*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("conservation: no cfg/lob_ages*.yaml found — this check " +
			"discovers its own inputs, so an empty glob means it is silently testing nothing")
	}
	out := make([]model, 0, len(paths))
	for _, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(path)
		part, width := rePartition.FindSubmatch(source), reAgeWidth.FindSubmatch(source)
		ncoh, lvl := reNcoh.FindSubmatch(source), reLevelWidth.FindSubmatch(source)
		if part == nil || width == nil || ncoh == nil || lvl == nil {
			return nil, fmt.Errorf("conservation: %s does not expose the layout this check "+
				"reads (partition name, `ages` width, ncoh, level-depth slice). If it is an "+
				"age model, teach this check to read it; if it is not, do not name it "+
				"lob_ages*", name)
		}
		w, err := strconv.Atoi(string(width[1]))
		if err != nil {
			return nil, err
		}
		n, err := strconv.ParseFloat(string(ncoh[1]), 64)
		if err != nil {
			return nil, err
		}
		m := model{file: name, partition: string(part[1]), width: w, ncoh: int(n)}
		if token := string(lvl[1]); token == "ncoh" {
			m.levelWidth = m.ncoh
		} else {
			lw, err := strconv.Atoi(token)
			if err != nil {
				return nil, fmt.Errorf("conservation: %s sums level depth with width %q, "+
					"which is neither ncoh nor a literal", name, token)
			}
			m.levelWidth = lw
		}
		out = append(out, m)
	}
	return out, nil
}

type measured struct {
	worstResidual float64 // largest |volume that vanished unreported| at any step
	dampingSees   float64 // fraction of resting volume the damping's slice covers
	expiryShare   float64 // fraction of reported cancellations that left by the age cap
}

func measure(m model) (measured, error) {
	stores, err := cfgrun.RunEnsemble(m.file, cfgrun.Subs{
		"max_steps: 400": fmt.Sprintf("max_steps: %d", steps),
	}, cfgrun.DefaultSeeds[:seeds])
	if err != nil {
		return measured{}, err
	}
	var worst, sees, share []float64
	for _, storage := range stores {
		rows := storage.GetValues(m.partition)
		if len(rows) <= settle {
			return measured{}, fmt.Errorf("conservation: %s produced too few rows", m.file)
		}
		rows = rows[settle:]
		var maxResidual, total, visible, expired, cancelled float64
		for _, row := range rows {
			after := 0.0
			for i := 0; i < m.width; i++ {
				after += row[i]
			}
			// depth(t+1) = depth(t) + arrivals - cancellations - trades. Anything left
			// over is volume that vanished without being reported anywhere.
			residual := row[m.width+3] + row[m.width] - row[m.width+1] - row[m.width+2] - after
			if residual < 0 {
				residual = -residual
			}
			if residual > maxResidual {
				maxResidual = residual
			}
			// What the configured level-depth slice actually covers, against the truth.
			for j := 0; j < nGroups; j++ {
				for k := 0; k < m.levelWidth && k < m.ncoh; k++ {
					visible += row[j*m.ncoh+k]
				}
			}
			total += after
			expired += row[m.width+7]
			cancelled += row[m.width+1]
		}
		worst = append(worst, maxResidual)
		if total > 0 {
			sees = append(sees, visible/total)
		}
		if cancelled > 0 {
			share = append(share, expired/cancelled)
		}
	}
	return measured{
		worstResidual: diagnostics.Mean(worst),
		dampingSees:   diagnostics.Mean(sees),
		expiryShare:   diagnostics.Mean(share),
	}, nil
}

func measureAll() ([]model, map[string]measured, error) {
	models, err := discover()
	if err != nil {
		return nil, nil, err
	}
	out := make(map[string]measured, len(models))
	for _, m := range models {
		r, err := measure(m)
		if err != nil {
			return nil, nil, err
		}
		out[m.file] = r
	}
	return models, out, nil
}

func label(file string) string { return strings.TrimSuffix(file, ".yaml") }

// byName selects a config explicitly. The ordered claims below name the three configs they
// contrast, and glob order is not a contract worth resting that on.
func byName(m map[string]measured, file string) measured {
	got, ok := m[file]
	if !ok {
		panic("conservation: " + file + " was not discovered; this claim contrasts three " +
			"named configs and cannot be stated without it")
	}
	return got
}

// ObservedBehaviour pins both conservation laws across every discovered age model.
func ObservedBehaviour() []claims.Claim {
	models, m, err := measureAll()
	if err != nil {
		panic("conservation: measuring observed behaviour: " + err.Error())
	}
	binding := claims.Binding{
		TestName: "TestConservation",
		TestFile: "pkg/conservation/behaviour_test.go",
	}

	var residuals, coverage []claims.Observation
	var residualThresholds, coverageThresholds []claims.Threshold
	for i, mod := range models {
		residuals = append(residuals, claims.Observation{
			Label: label(mod.file), Value: m[mod.file].worstResidual,
		})
		coverage = append(coverage, claims.Observation{
			Label: label(mod.file), Value: m[mod.file].dampingSees,
		})
		residualThresholds = append(residualThresholds, claims.Threshold{
			ObsIndex: i, GreaterThan: false, Ref: 1e-9,
			RefLabel: "1e-9 (floating-point noise, not a tolerance)",
		})
		coverageThresholds = append(coverageThresholds, claims.Threshold{
			ObsIndex: i, GreaterThan: true, Ref: 0.999999,
			RefLabel: "0.999999 (the damping must see all of it)",
		})
	}

	return []claims.Claim{
		{
			ID: "every_age_model_conserves_volume_between_its_reported_flows",
			Statement: "At every step of every discovered age model, the change in resting " +
				"volume equals arrivals minus reported cancellations minus trades. This is " +
				"the decomposition pkg/feed/bucket.go applies to the market — a diff feed " +
				"attributes every untraded departure to cancellation because it cannot see " +
				"why an order left — so a model that lets volume leave by some third route " +
				"is not measuring the same quantity the market is. Two configs did exactly " +
				"that: volume leaving by the age cap was in no output column.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "largest absolute per-step discrepancy between the depth change and the " +
				"reported flows, over 8 members of 2000 steps",
			Limitations: "NOT PRE-REGISTERED — written after the defect was found, so it is " +
				"a regression guard rather than a test of a prediction. It checks that " +
				"departures are ACCOUNTED FOR, not that they are correctly CLASSIFIED: a " +
				"model that reported trades as cancellations would satisfy it. It also " +
				"cannot see a defect that is symmetric across the identity. The residual " +
				"is at the 1e-13 level, so the 1e-9 bound is floating-point headroom and " +
				"not a tolerance for real leakage.",
			Thresholds:   residualThresholds,
			Observations: residuals,
			Binding:      binding,
		},
		{
			ID: "the_arrival_damping_sees_every_resting_lot",
			Statement: "The slice width each config uses to sum a level's cohorts is read " +
				"out of the config and measured against the actual resting volume, and it " +
				"covers all of it. This is a dynamics check, not a reporting one: level " +
				"depth is the arrival damping's denominator, so volume it cannot see is " +
				"volume that fails to suppress arrivals. The width was hardcoded to 8 " +
				"while ncoh was 12, which was invisible at eight cohorts and hid 22.4% of " +
				"the book at twelve.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit: "fraction of resting volume covered by the configured level-depth slice, " +
				"over 8 members of 2000 steps",
			Limitations: "NOT PRE-REGISTERED, and narrow by design. It confirms the damping " +
				"sums the right NUMBER of slots, not that it sums the right ones or weights " +
				"them correctly — a config slicing the wrong offset at full width would pass. " +
				"It assumes the 2x8 (side, level) layout every age model currently shares, " +
				"and a model departing from that would need this taught to it rather than " +
				"quietly passing.",
			Thresholds:   coverageThresholds,
			Observations: coverage,
			Binding:      binding,
		},
		{
			ID: "expiry_dominates_cancellation_once_the_hazard_is_lowered_to_reach_the_regime",
			Statement: "The share of cancellations that leave by the age cap rather than by " +
				"the hazard is negligible in the eight-cohort variant and close to half in " +
				"the twelve-cohort one. That is the cost of the twelve-cohort model's one " +
				"permitted adjustment: haz0 was lowered from 0.18 to 0.016 to reach the " +
				"depth band, which turned a rising-hazard mechanism into a mostly-fixed " +
				"twelve-step conveyor belt. The model that most needed the time-in-queue " +
				"mechanism to be doing the work is the one where it was doing least, and " +
				"the departures it substituted were the ones the measurement omitted.",
			Gate:  "2.2",
			Phase: phase,
			Data:  dataset,
			Unit:  "expired volume as a fraction of all reported cancellations",
			Limitations: "NOT PRE-REGISTERED; bounds set after the measurement. It is a " +
				"statement about the CONFIGS AS SHIPPED, not about the mechanism — the " +
				"share is a function of haz0 and cohort count, so it moves if either is " +
				"re-set, and it should be read as recording why the twelve-cohort result " +
				"was contaminated rather than as a property of time-in-queue cancellation.",
			Thresholds: []claims.Threshold{
				{ObsIndex: 0, GreaterThan: false, Ref: 1e-12,
					RefLabel: "1e-12 (absorbing: nothing can leave by the cap)"},
				{ObsIndex: 1, GreaterThan: false, Ref: 0.01, RefLabel: "0.01"},
				{ObsIndex: 2, GreaterThan: true, Ref: 0.30, RefLabel: "0.30"},
			},
			Observations: []claims.Observation{
				{Label: "lob_ages (absorbing)", Value: byName(m, "lob_ages.yaml").expiryShare},
				{Label: "lob_ages_finite (haz0 0.18)", Value: byName(m, "lob_ages_finite.yaml").expiryShare},
				{Label: "lob_ages12 (haz0 0.016)", Value: byName(m, "lob_ages12.yaml").expiryShare},
			},
			Binding: binding,
		},
	}
}
