package bundle

import (
	"github.com/mberwanger/quartermaster/internal/config"
	"github.com/mberwanger/quartermaster/internal/pack"
	"github.com/mberwanger/quartermaster/internal/validate"
)

// Compile builds the store at root from disk, refusing one that does not
// validate. Build assembles a bundle from parts already in hand; Compile is the
// same artifact starting from a place on disk, which is what every command that
// reads a store actually wants.
//
// Validation is a precondition rather than a separate phase. A store that fails
// its own rules has no bundle, so there is nothing to hand back and no way for a
// caller to skip the check by accident. That is why `qm bundle validate` and
// `qm bundle build` run the same code: validate compiles the artifact and throws
// it away, and the only difference between them is whether anything is written.
func Compile(root string, src Source) (*Bundle, error) {
	storeConfig, err := config.Load(root)
	if err != nil {
		return nil, err
	}

	validation, err := validate.Run(root, storeConfig)
	if err != nil {
		return nil, err
	}
	if !validation.OK() {
		return nil, &validate.Error{Result: validation}
	}

	packages, err := pack.Load(root, storeConfig.Packages)
	if err != nil {
		return nil, err
	}

	b, err := Build(Options{
		Root:     root,
		Config:   storeConfig,
		Packages: packages,
		Repo:     src.Repo,
		Commit:   src.Commit,
	})
	if err != nil {
		return nil, err
	}
	b.Meta.Checked = validation.Checked

	return b, nil
}
