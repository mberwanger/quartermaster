package main

import (
	_ "embed"
	"os"

	"github.com/mberwanger/quartermaster/cmd"
	vers "github.com/mberwanger/quartermaster/internal/version"
)

// nolint: gochecknoglobals
var (
	version = "0.0.0"
	commit  = ""
	date    = ""
	builtBy = ""
)

//go:embed art.txt
var asciiArt string

func main() {
	cmd.Execute(
		buildVersion(version, commit, date, builtBy),
		os.Exit,
		os.Args[1:],
	)
}

func buildVersion(version, commit, date, builtBy string) vers.Version {
	return vers.GetVersion(
		vers.WithAsciiArt(asciiArt),
		func(v *vers.Version) {
			if commit != "" {
				v.GitCommit = commit
			}
			if date != "" {
				v.BuildDate = date
			}
			if version != "" {
				v.GitVersion = version
			}
			if builtBy != "" {
				v.BuiltBy = builtBy
			}
		},
	)
}
