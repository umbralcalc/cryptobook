// Package cfgrun runs this repo's YAML configs from Go and hands back the
// resulting storage, so tests can assert on what a config produced.
//
// The models in cfg/ are pure config — partitions, params, expressions, wiring and
// the analysis tier are all data, resolved by the engine with no Go and no
// toolchain. This package deliberately adds no modelling of its own. It does
// exactly three things: locate a config, substitute the parameter a test is
// varying, and capture output into a StateTimeStorage instead of stdout.
//
// # Why substitution rather than building configs in Go
//
// A behavioural claim is a statement about how the model responds to a changed
// parameter, so a test has to run the same model at several parameter values. The
// alternatives were worse: a config file per sweep point multiplies files that must
// stay in sync, and constructing the run in Go would mean the thing under test is
// no longer the config that ships.
//
// So a test names the exact YAML text it is changing:
//
//	storage, err := cfgrun.Run("lob_generator.yaml", cfgrun.Subs{
//	    "cancel_rate: [0.15]": "cancel_rate: [0.225]",
//	})
//
// Substitution is plain text, and a substitution whose target is ABSENT is an
// error, never a silent no-op. That property is the whole point: if someone
// reformats a config, the tests fail loudly rather than quietly all measuring the
// unmodified model and agreeing with each other.
package cfgrun

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/api"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Subs maps exact existing YAML text to its replacement. Keys are matched
// literally, and every key must occur at least once in the config.
type Subs map[string]string

// ConfigDir returns the repo's cfg/ directory, located relative to this source
// file so tests resolve it from any working directory.
func ConfigDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("cfgrun: cannot locate this package's source path")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "cfg")
}

// Run loads the named config from cfg/, applies subs, and runs it — the macros:
// tier when the config has one, otherwise the main: simulation — returning the
// storage it produced.
func Run(name string, subs Subs) (*simulator.StateTimeStorage, error) {
	source, err := os.ReadFile(filepath.Join(ConfigDir(), name))
	if err != nil {
		return nil, fmt.Errorf("cfgrun: reading %s: %w", name, err)
	}
	substituted, err := apply(string(source), subs)
	if err != nil {
		return nil, fmt.Errorf("cfgrun: %s: %w", name, err)
	}
	// The engine loads configs by path (and re-reads the path itself for some
	// checks), so the substituted document has to exist as a file.
	temp, err := os.CreateTemp("", "*-"+name)
	if err != nil {
		return nil, fmt.Errorf("cfgrun: temp file: %w", err)
	}
	defer os.Remove(temp.Name())
	if _, err := temp.WriteString(substituted); err != nil {
		temp.Close()
		return nil, fmt.Errorf("cfgrun: writing temp config: %w", err)
	}
	if err := temp.Close(); err != nil {
		return nil, fmt.Errorf("cfgrun: closing temp config: %w", err)
	}
	config := api.LoadApiRunConfigFromYaml(temp.Name())
	if len(config.Macros) > 0 {
		return api.RunMacros(config)
	}
	return runMain(config)
}

// apply performs the substitutions, erroring on any whose target is absent.
func apply(source string, subs Subs) (string, error) {
	missing := make([]string, 0)
	// Sorted so an error message is stable and a run is reproducible regardless of
	// map iteration order.
	keys := make([]string, 0, len(subs))
	for key := range subs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !strings.Contains(source, key) {
			missing = append(missing, key)
			continue
		}
		source = strings.ReplaceAll(source, key, subs[key])
	}
	if len(missing) > 0 {
		return "", fmt.Errorf(
			"substitution target(s) not found, so the test would have silently "+
				"measured the unmodified config: %q", missing)
	}
	return source, nil
}

// runMain runs a main:-tier config, capturing output into storage rather than
// printing it. Only the output function is replaced — every partition, param and
// wiring decision still comes from the config.
func runMain(config *api.ApiRunConfig) (*simulator.StateTimeStorage, error) {
	generator := config.GetConfigGenerator()
	if err := api.CheckForDeadlock(generator); err != nil {
		return nil, err
	}
	storage := simulator.NewStateTimeStorage()
	simulation := generator.GetSimulation()
	simulation.OutputFunction = &simulator.StateTimeStorageOutputFunction{
		Store: storage,
	}
	generator.SetSimulation(simulation)
	simulator.NewPartitionCoordinator(generator.GenerateConfigs()).Run()
	return storage, nil
}

// MeanColumn returns the mean of one column of a partition's recorded rows,
// skipping the first burnIn rows. Averaging a column of model output is analysis,
// not modelling — it states no dynamics the config does not already contain.
func MeanColumn(
	storage *simulator.StateTimeStorage,
	partition string,
	column int,
	burnIn int,
) (float64, error) {
	rows := storage.GetValues(partition)
	if len(rows) <= burnIn {
		return 0, fmt.Errorf(
			"cfgrun: partition %q has %d rows, need more than the %d-row burn-in",
			partition, len(rows), burnIn)
	}
	total, count := 0.0, 0
	for _, row := range rows[burnIn:] {
		if column < 0 || column >= len(row) {
			return 0, fmt.Errorf(
				"cfgrun: column %d is outside partition %q's width %d",
				column, partition, len(row))
		}
		total += row[column]
		count++
	}
	return total / float64(count), nil
}

// LastRow returns a partition's final recorded row.
func LastRow(
	storage *simulator.StateTimeStorage,
	partition string,
) ([]float64, error) {
	rows := storage.GetValues(partition)
	if len(rows) == 0 {
		return nil, fmt.Errorf("cfgrun: partition %q recorded no rows", partition)
	}
	return rows[len(rows)-1], nil
}

// DefaultSeeds is the ensemble this project scores claims on: thirty-two members.
//
// Sized from measurement rather than convention, on 2026-08-02. With 64 members at 8000
// steps a depth correlation's standard deviation across seeds is ~0.024, so the standard
// error of an N-member mean is 0.024/sqrt(N):
//
//	N = 8   -> 0.0086
//	N = 32  -> 0.0043
//	N = 64  -> 0.0030
//
// Thirty-two puts the standard error at ~0.004 — five times finer than the 0.021 the
// damping calibration tried and failed to resolve on one seed, which is the scale this
// project's comparisons have actually been decided at.
//
// It is affordable: 64 members at 8000 steps take ~2.6s, so 32 take ~1.3s. The cost
// argument against ensembling was wrong, and it was wrong by two orders of magnitude.
//
// A NOTE ON THE STATISTIC. An earlier pass sized this from the RANGE over 8 seeds. That
// was a mistake: a range over 8 samples is so noisy that two independent estimates of the
// same quantity came out 0.056 and 0.115. It also produced a spurious finding that noise
// stops falling above 8000 steps. Measured properly by standard deviation over 64
// members, it falls as 1/sqrt(n) with no saturation — 0.0526 to 0.0244 for a 4x length
// increase. Report standard deviations, not ranges.
var DefaultSeeds = func() []uint64 {
	seeds := make([]uint64, 32)
	for i := range seeds {
		seeds[i] = uint64(20260802 + i)
	}
	return seeds
}()

// DefaultSteps is the run length claims are scored at: 8000.
//
// Noise falls as 1/sqrt(steps) with no saturation, so this is a straight trade of compute
// for precision. 8000 quadruples the 2000 this project used to use and halves the spread;
// going further keeps working but the ensemble is the cheaper lever, since it reduces the
// error of the MEAN while run length reduces the spread of individual members.
const DefaultSteps = 8000

// EnsembleStat is one quantity summarised over an ensemble: the mean claims report, and
// the spread that says how far a single member could have been from it.
type EnsembleStat struct {
	Mean, SD float64
}

// Summarise returns the mean and sample standard deviation of an ensemble's values.
func Summarise(values []float64) EnsembleStat {
	if len(values) == 0 {
		return EnsembleStat{}
	}
	mean := 0.0
	for _, v := range values {
		mean += v / float64(len(values))
	}
	if len(values) == 1 {
		return EnsembleStat{Mean: mean}
	}
	sum := 0.0
	for _, v := range values {
		sum += (v - mean) * (v - mean)
	}
	return EnsembleStat{Mean: mean, SD: math.Sqrt(sum / float64(len(values)-1))}
}

// StdError is the standard error of the ensemble mean — SD / sqrt(N).
func (e EnsembleStat) StdError(members int) float64 {
	if members <= 0 {
		return 0
	}
	return e.SD / math.Sqrt(float64(members))
}

// RunEnsemble runs one member per seed and returns their storages, index-aligned to
// seeds.
//
// # Why the engine's ensembler rather than calling Run in a loop
//
// simulator.RunSeededEnsemble varies the GLOBAL seed through the ConfigGenerator, which
// reseeds every partition coherently. Substituting a partition's `seed:` line in the YAML
// — which is what an inline loop would do — varies one partition and leaves the others
// pinned, so members would share randomness they should not. The engine also rebuilds a
// fresh ConfigGenerator per member, which is load-bearing: GenerateConfigs hands back the
// same Iteration pointers it was given, so reusing one generator across concurrent
// members would share mutable iteration state between them.
//
// Concurrency is left at the engine's default (GOMAXPROCS). Members are independent, so
// this is the one place in this repo where parallelism is free of ordering concerns.
func RunEnsemble(
	name string,
	subs Subs,
	seeds []uint64,
) ([]*simulator.StateTimeStorage, error) {
	source, err := os.ReadFile(filepath.Join(ConfigDir(), name))
	if err != nil {
		return nil, fmt.Errorf("cfgrun: reading %s: %w", name, err)
	}
	substituted, err := apply(string(source), subs)
	if err != nil {
		return nil, fmt.Errorf("cfgrun: %s: %w", name, err)
	}
	// Members are rebuilt by RE-LOADING this file, so it has to outlive every member.
	temp, err := os.CreateTemp("", "*-"+name)
	if err != nil {
		return nil, fmt.Errorf("cfgrun: temp file: %w", err)
	}
	defer os.Remove(temp.Name())
	if _, err := temp.WriteString(substituted); err != nil {
		temp.Close()
		return nil, fmt.Errorf("cfgrun: writing temp config: %w", err)
	}
	if err := temp.Close(); err != nil {
		return nil, fmt.Errorf("cfgrun: closing temp config: %w", err)
	}

	config := api.LoadApiRunConfigFromYaml(temp.Name())
	if len(config.Macros) > 0 {
		return nil, fmt.Errorf(
			"cfgrun: %s has a macros: tier; the engine's ensembler runs main: "+
				"partitions only", name)
	}
	if err := api.CheckForDeadlock(config.GetConfigGenerator()); err != nil {
		return nil, err
	}
	build := func() *simulator.ConfigGenerator {
		generator := api.LoadApiRunConfigFromYaml(temp.Name()).GetConfigGenerator()
		simulation := generator.GetSimulation()
		simulation.OutputFunction = &simulator.NilOutputFunction{}
		generator.SetSimulation(simulation)
		return generator
	}
	runs := simulator.RunSeededEnsemble(build, seeds, 0)
	out := make([]*simulator.StateTimeStorage, len(runs))
	for i, run := range runs {
		if run.Storage == nil {
			return nil, fmt.Errorf("cfgrun: ensemble member %d (seed %d) produced no storage",
				i, run.Seed)
		}
		out[i] = run.Storage
	}
	return out, nil
}
