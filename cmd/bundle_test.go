package cmd

import (
	"bytes"
	"testing"
)

func TestBundleRejectsUnknownSubcommand(t *testing.T) {
	command := newBundleCmd().cmd
	command.SetArgs([]string{"buidl"})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})

	if err := command.Execute(); err == nil {
		t.Fatal("unknown bundle subcommand exited successfully")
	}
}

func TestBundleWithoutSubcommandShowsHelp(t *testing.T) {
	command := newBundleCmd().cmd
	command.SetArgs(nil)

	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&bytes.Buffer{})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte("Available Commands:")) {
		t.Fatalf("bundle help not shown:\n%s", output.String())
	}
}
