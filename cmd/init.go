package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mberwanger/quartermaster/internal/manifest"
	"github.com/mberwanger/quartermaster/internal/provider"
	"github.com/mberwanger/quartermaster/internal/repo"
	"github.com/mberwanger/quartermaster/internal/target"
)

type initCmd struct {
	cmd         *cobra.Command
	dir         string
	sources     []string
	rulesets    []string
	targets     []string
	noTelemetry bool
	noHooks     bool
	noSync      bool
	force       bool
}

func newInitCmd() *initCmd {
	v := &initCmd{}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "First-time setup for a repository",
		Long: `Set a repository up to consume knowledge: resolve the given sources, write
the manifest, ready the .gitignore, install the git hooks that keep materialized
knowledge current, and run the first sync.

Sources are given in precedence order; when two bundles produce a rule with the
same id, the later source wins. Targets are detected from the repository when not
given explicitly.`,
		Example: "  qm init --source file://../platform-knowledge --ruleset voice",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return v.run(cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&v.dir, "dir", ".", "Repository directory to initialize")
	cmd.Flags().StringArrayVar(&v.sources, "source", nil, "Bundle source URL (repeatable, precedence by order)")
	cmd.Flags().StringArrayVar(&v.rulesets, "ruleset", nil, "Rulesets to apply (repeatable; default all a source offers)")
	cmd.Flags().StringArrayVar(&v.targets, "target", nil, "Harness targets (repeatable; default detected from the repository)")
	cmd.Flags().BoolVar(&v.noTelemetry, "no-telemetry", false, "Opt out of usage telemetry, and of the session hook that feeds it")
	cmd.Flags().BoolVar(&v.noHooks, "no-hooks", false, "Do not install git or harness hooks")
	cmd.Flags().BoolVar(&v.noSync, "no-sync", false, "Do not run the first sync")
	cmd.Flags().BoolVar(&v.force, "force", false, "Overwrite an existing manifest")

	v.cmd = cmd
	return v
}

func (v *initCmd) run(out io.Writer) error {
	if len(v.sources) == 0 {
		return fmt.Errorf("at least one --source is required")
	}

	manifestPath := filepath.Join(v.dir, manifest.FileName)
	if _, err := os.Stat(manifestPath); err == nil && !v.force {
		return fmt.Errorf("%s already exists; re-run with --force to overwrite", manifest.FileName)
	}

	bundles, err := v.resolveSources()
	if err != nil {
		return err
	}

	targets, err := v.resolveTargets()
	if err != nil {
		return err
	}

	if err := writeManifest(manifestPath, bundles, targets, !v.noTelemetry); err != nil {
		return err
	}

	var report strings.Builder
	fmt.Fprintf(&report, "wrote %s\n", manifest.FileName)
	fmt.Fprintf(&report, "  targets   %s\n", strings.Join(targets, ", "))

	if added, err := repo.EnsureGitignore(v.dir, ignorePatterns(targets)); err != nil {
		return err
	} else if len(added) > 0 {
		fmt.Fprintf(&report, "  gitignore +%d entr%s\n", len(added), plural(len(added), "y", "ies"))
	}

	if !v.noHooks {
		installed, skipped, err := repo.InstallHooks(v.dir)
		if err != nil {
			return err
		}
		if len(installed) > 0 {
			fmt.Fprintf(&report, "  hooks     %s\n", strings.Join(installed, ", "))
		}
		for _, s := range skipped {
			fmt.Fprintf(&report, "  hooks     skipped %s (already exists, not ours)\n", s)
		}

		// The session hook records what the repository looked like when a
		// session ended, so it only makes sense where telemetry is on. It goes
		// in the harness's own settings, and only for a harness that has the
		// concept.
		if !v.noTelemetry && slices.Contains(targets, "claude") {
			wrote, err := repo.InstallSessionHook(v.dir)
			if err != nil {
				return err
			}
			if wrote {
				fmt.Fprintf(&report, "  hooks     session (%s, commit it)\n", filepath.Join(".claude", "settings.json"))
			}
			// The file is only worth writing there because git carries it. A
			// repository that ignores it gets a hook in this checkout and
			// nowhere else, which is worse than not having one, because it
			// looks installed.
			if repo.SessionHookIgnored(v.dir) {
				fmt.Fprintf(&report, "  warning   .gitignore covers that file, so the hook stays in this\n")
				fmt.Fprintf(&report, "            checkout only. Un-ignore it to share it.\n")
			}
		}
	}

	_, _ = io.WriteString(out, report.String())

	if v.noSync {
		fmt.Fprintf(out, "initialized; run qm sync to materialize\n")
		return nil
	}

	fmt.Fprintln(out, "---")
	return syncRepo(v.dir, out, false)
}

// bundleSpec is one resolved source ready to write into the manifest.
type bundleSpec struct {
	source   string
	digest   string
	pin      bool
	rulesets []string
}

// resolveSources resolves each source, validates it, and selects its rulesets.
func (v *initCmd) resolveSources() ([]bundleSpec, error) {
	want := map[string]bool{}
	for _, r := range v.rulesets {
		want[r] = true
	}
	matched := map[string]bool{}

	var out []bundleSpec
	for _, src := range v.sources {
		b, err := provider.Resolve(src, v.dir, provider.Auth{})
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", src, err)
		}

		offered := make(map[string]bool, len(b.Rulesets))
		for _, rs := range b.Rulesets {
			offered[rs.Name] = true
		}

		// Ruleset order is precedence, so preserve the order the author asked
		// for. Without a filter, fall back to the bundle's own order.
		var names []string
		if len(want) == 0 {
			for _, rs := range b.Rulesets {
				names = append(names, rs.Name)
			}
		} else {
			for _, r := range v.rulesets {
				if offered[r] {
					names = append(names, r)
					matched[r] = true
				}
			}
		}

		out = append(out, bundleSpec{
			source:   src,
			digest:   b.Meta.Digest,
			pin:      !strings.HasPrefix(src, "file://"),
			rulesets: names,
		})
	}

	// A requested ruleset no source provides is an error the author can fix now.
	for r := range want {
		if !matched[r] {
			return nil, fmt.Errorf("no source provides ruleset %q", r)
		}
	}
	return out, nil
}

// resolveTargets validates explicit targets or detects them from the repository.
func (v *initCmd) resolveTargets() ([]string, error) {
	if len(v.targets) > 0 {
		for _, name := range v.targets {
			if _, ok := target.Get(name); !ok {
				return nil, fmt.Errorf("unknown target %q; known targets are %s", name, strings.Join(target.Names(), ", "))
			}
		}
		return v.targets, nil
	}

	detected := detectTargets(v.dir)
	if len(detected) == 0 {
		// Nothing to go on: default to claude, the most common harness, and let
		// the author add targets to the manifest.
		return []string{"claude"}, nil
	}
	return detected, nil
}

// detectTargets infers which harnesses a repository uses from what it already
// contains.
func detectTargets(dir string) []string {
	var out []string
	if exists(filepath.Join(dir, ".claude")) || exists(filepath.Join(dir, "CLAUDE.md")) {
		out = append(out, "claude")
	}
	if exists(filepath.Join(dir, ".cursor")) {
		out = append(out, "cursor")
	}
	if exists(filepath.Join(dir, target.AgentsMDFile)) {
		out = append(out, "agents-md")
	}
	return out
}

// ignorePatterns gathers the gitignore entries for the selected targets, plus
// the Quartermaster state and knowledge directory that every repository has.
func ignorePatterns(targets []string) []string {
	patterns := []string{"/.quartermaster/"}
	for _, name := range targets {
		t, ok := target.Get(name)
		if !ok {
			continue
		}
		for _, p := range t.IgnorePaths() {
			patterns = append(patterns, "/"+p)
		}
	}
	return patterns
}

func writeManifest(path string, bundles []bundleSpec, targets []string, telemetry bool) error {
	var b strings.Builder
	b.WriteString("# Quartermaster manifest. Declares the knowledge this repository consumes.\n")
	b.WriteString("# Edit this file, then run `qm sync`. Generated files are gitignored, except\n")
	b.WriteString("# the AGENTS.md managed block.\n\n")

	b.WriteString("bundles:\n")
	for _, bs := range bundles {
		fmt.Fprintf(&b, "  - source: %s\n", bs.source)
		if bs.pin && bs.digest != "" {
			fmt.Fprintf(&b, "    digest: %s\n", bs.digest)
		}
		fmt.Fprintf(&b, "    rulesets: [%s]\n", strings.Join(bs.rulesets, ", "))
	}

	b.WriteString("\ntargets:\n")
	for _, t := range targets {
		fmt.Fprintf(&b, "  - %s\n", t)
	}

	fmt.Fprintf(&b, "\ntelemetry: %t\n", telemetry)

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
