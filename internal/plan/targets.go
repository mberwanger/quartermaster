package plan

import (
	"fmt"
	"strings"

	"github.com/mberwanger/quartermaster/internal/bundle"
	"github.com/mberwanger/quartermaster/internal/ruleset"
	"github.com/mberwanger/quartermaster/internal/state"
	"github.com/mberwanger/quartermaster/internal/target"
)

// ResolveTargets maps a manifest's target names to renderers, failing on an
// unknown name with the list of known targets.
func ResolveTargets(names []string) ([]target.Target, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("manifest declares no targets; known targets are %s", strings.Join(target.Names(), ", "))
	}
	var targets []target.Target
	for _, name := range names {
		t, ok := target.Get(name)
		if !ok {
			return nil, fmt.Errorf("unknown target %q; known targets are %s", name, strings.Join(target.Names(), ", "))
		}
		targets = append(targets, t)
	}
	return targets, nil
}

// bundlesForTarget converts the resolved bundle records into the summary a
// target renders a pointer from.
func bundlesForTarget(bundles []state.Bundle) []target.Bundle {
	out := make([]target.Bundle, 0, len(bundles))
	for _, b := range bundles {
		out = append(out, target.Bundle{Source: b.Source, Digest: b.Digest, Rulesets: b.Rulesets})
	}
	return out
}

func findRuleset(b *bundle.Bundle, name string) (ruleset.Compiled, bool) {
	for _, c := range b.Rulesets {
		if c.Name == name {
			return c, true
		}
	}
	return ruleset.Compiled{}, false
}
