package cmd

import (
	"bytes"
	"testing"

	"github.com/mberwanger/quartermaster/internal/version"
)

func TestRootRejectsUnknownCommand(t *testing.T) {
	root := newRootCmd(version.Version{}, func(int) {})
	root.cmd.SetArgs([]string{"nosuch"})
	root.cmd.SetOut(&bytes.Buffer{})
	root.cmd.SetErr(&bytes.Buffer{})

	if err := root.cmd.Execute(); err == nil {
		t.Fatal("unknown root command exited successfully")
	}
}

func TestRootWithoutSubcommandShowsHelp(t *testing.T) {
	root := newRootCmd(version.Version{}, func(int) {})
	root.cmd.SetArgs(nil)

	var output bytes.Buffer
	root.cmd.SetOut(&output)
	root.cmd.SetErr(&bytes.Buffer{})

	if err := root.cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte("Available Commands:")) {
		t.Fatalf("root help not shown:\n%s", output.String())
	}
}
