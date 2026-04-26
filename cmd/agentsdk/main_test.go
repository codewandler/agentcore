package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRootCommandRegistersRun(t *testing.T) {
	cmd := rootCmd()
	run, _, err := cmd.Find([]string{"run"})
	require.NoError(t, err)
	require.NotNil(t, run)
	require.Equal(t, "run", run.Name())
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	err := run([]string{"run", "./agent", "--bad"})
	require.Error(t, err)
}
