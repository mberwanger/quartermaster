// Package preflight verifies and compiles a store before it is emitted or
// consumed directly from source.
package preflight

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mberwanger/quartermaster/internal/bundle"
	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/pack"
	"github.com/mberwanger/quartermaster/internal/validate"
)

// Options describes the source metadata recorded in the compiled bundle.
type Options struct {
	Root   string
	Repo   string
	Commit string
}

// Result contains the verified bundle and validation summary.
type Result struct {
	Bundle     *bundle.Bundle
	Validation validate.Result
}

// ValidationError reports document findings that prevent compilation.
type ValidationError struct {
	Result validate.Result
}

func (e *ValidationError) Error() string {
	var report strings.Builder
	for _, finding := range e.Result.Findings {
		fmt.Fprintf(&report, "%s: %s\n", finding.Path, finding.Message)
	}
	fmt.Fprintf(&report, "\n%d finding(s) across %d checked doc(s)",
		len(e.Result.Findings), e.Result.Checked)
	return report.String()
}

// Run applies every non-emitting store check and compiles the verified result
// in memory. Callers may write, publish, or consume the returned bundle.
func Run(options Options) (*Result, error) {
	storeConfig, err := config.Load(options.Root)
	if err != nil {
		return nil, err
	}

	validationResult, err := validate.Run(options.Root, storeConfig)
	if err != nil {
		return nil, err
	}
	if !validationResult.OK() {
		return nil, &ValidationError{Result: validationResult}
	}

	packages, err := loadPackages(options.Root, storeConfig)
	if err != nil {
		return nil, err
	}

	compiledBundle, err := bundle.Build(bundle.Options{
		Root:     options.Root,
		Config:   storeConfig,
		Packages: packages,
		Repo:     options.Repo,
		Commit:   options.Commit,
	})
	if err != nil {
		return nil, err
	}

	return &Result{
		Bundle:     compiledBundle,
		Validation: validationResult,
	}, nil
}

func loadPackages(root string, storeConfig *config.Config) (pack.File, error) {
	if storeConfig.Packages == "" {
		return pack.File{}, nil
	}
	return pack.Load(filepath.Join(root, storeConfig.Packages))
}
