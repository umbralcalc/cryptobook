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
