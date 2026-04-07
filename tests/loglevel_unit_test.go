package main

import (
	"testing"

	"github.com/this-is-alpha-iota/clyde/cli/loglevel"
)

func TestLevelString(t *testing.T) {
	tests := []struct {
		level    loglevel.Level
		expected string
	}{
		{loglevel.Silent, "silent"},
		{loglevel.Quiet, "quiet"},
		{loglevel.Normal, "normal"},
		{loglevel.Verbose, "verbose"},
		{loglevel.Debug, "debug"},
		{loglevel.Level(99), "Level(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("Level(%d).String() = %q, want %q", int(tt.level), got, tt.expected)
			}
		})
	}
}

func TestLevelOrdering(t *testing.T) {
	// Verify levels are ordered: loglevel.Silent < loglevel.Quiet < loglevel.Normal < loglevel.Verbose < loglevel.Debug
	if !(loglevel.Silent < loglevel.Quiet && loglevel.Quiet < loglevel.Normal && loglevel.Normal < loglevel.Verbose && loglevel.Verbose < loglevel.Debug) {
		t.Error("Log levels are not in expected order: loglevel.Silent < loglevel.Quiet < loglevel.Normal < loglevel.Verbose < loglevel.Debug")
	}
}

func TestShouldShow(t *testing.T) {
	tests := []struct {
		name      string
		current   loglevel.Level
		threshold loglevel.Level
		want      bool
	}{
		// loglevel.Silent shows nothing
		{"silent shows nothing at quiet", loglevel.Silent, loglevel.Quiet, false},
		{"silent shows nothing at normal", loglevel.Silent, loglevel.Normal, false},
		{"silent shows nothing at verbose", loglevel.Silent, loglevel.Verbose, false},
		{"silent shows nothing at debug", loglevel.Silent, loglevel.Debug, false},
		// But silent >= silent is true (edge case, matches convention)
		{"silent >= silent", loglevel.Silent, loglevel.Silent, true},

		// loglevel.Quiet shows quiet-level and below
		{"quiet shows quiet", loglevel.Quiet, loglevel.Quiet, true},
		{"quiet hides normal", loglevel.Quiet, loglevel.Normal, false},
		{"quiet hides verbose", loglevel.Quiet, loglevel.Verbose, false},
		{"quiet hides debug", loglevel.Quiet, loglevel.Debug, false},

		// loglevel.Normal shows quiet, normal
		{"normal shows quiet", loglevel.Normal, loglevel.Quiet, true},
		{"normal shows normal", loglevel.Normal, loglevel.Normal, true},
		{"normal hides verbose", loglevel.Normal, loglevel.Verbose, false},
		{"normal hides debug", loglevel.Normal, loglevel.Debug, false},

		// loglevel.Verbose shows quiet, normal, verbose
		{"verbose shows quiet", loglevel.Verbose, loglevel.Quiet, true},
		{"verbose shows normal", loglevel.Verbose, loglevel.Normal, true},
		{"verbose shows verbose", loglevel.Verbose, loglevel.Verbose, true},
		{"verbose hides debug", loglevel.Verbose, loglevel.Debug, false},

		// loglevel.Debug shows everything
		{"debug shows quiet", loglevel.Debug, loglevel.Quiet, true},
		{"debug shows normal", loglevel.Debug, loglevel.Normal, true},
		{"debug shows verbose", loglevel.Debug, loglevel.Verbose, true},
		{"debug shows debug", loglevel.Debug, loglevel.Debug, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.current.ShouldShow(tt.threshold); got != tt.want {
				t.Errorf("Level(%s).ShouldShow(%s) = %v, want %v",
					tt.current, tt.threshold, got, tt.want)
			}
		})
	}
}

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedLevel loglevel.Level
		expectedArgs  []string
	}{
		{
			name:          "no flags = loglevel.Normal",
			args:          []string{},
			expectedLevel: loglevel.Normal,
			expectedArgs:  []string{},
		},
		{
			name:          "--silent flag",
			args:          []string{"--silent"},
			expectedLevel: loglevel.Silent,
			expectedArgs:  []string{},
		},
		{
			name:          "-q flag",
			args:          []string{"-q"},
			expectedLevel: loglevel.Quiet,
			expectedArgs:  []string{},
		},
		{
			name:          "--quiet flag",
			args:          []string{"--quiet"},
			expectedLevel: loglevel.Quiet,
			expectedArgs:  []string{},
		},
		{
			name:          "-v flag",
			args:          []string{"-v"},
			expectedLevel: loglevel.Verbose,
			expectedArgs:  []string{},
		},
		{
			name:          "--verbose flag",
			args:          []string{"--verbose"},
			expectedLevel: loglevel.Verbose,
			expectedArgs:  []string{},
		},
		{
			name:          "--debug flag",
			args:          []string{"--debug"},
			expectedLevel: loglevel.Debug,
			expectedArgs:  []string{},
		},
		{
			name:          "flag with other args",
			args:          []string{"-v", "Hello", "World"},
			expectedLevel: loglevel.Verbose,
			expectedArgs:  []string{"Hello", "World"},
		},
		{
			name:          "flag at end of args",
			args:          []string{"Hello", "World", "--debug"},
			expectedLevel: loglevel.Debug,
			expectedArgs:  []string{"Hello", "World"},
		},
		{
			name:          "flag in middle of args",
			args:          []string{"Hello", "-q", "World"},
			expectedLevel: loglevel.Quiet,
			expectedArgs:  []string{"Hello", "World"},
		},
		{
			name:          "multiple flags (last wins)",
			args:          []string{"-q", "--verbose", "--debug"},
			expectedLevel: loglevel.Debug,
			expectedArgs:  []string{},
		},
		{
			name:          "flag preserved with -f",
			args:          []string{"-v", "-f", "prompt.txt"},
			expectedLevel: loglevel.Verbose,
			expectedArgs:  []string{"-f", "prompt.txt"},
		},
		{
			name:          "no verbosity flags means normal",
			args:          []string{"What", "is", "2+2?"},
			expectedLevel: loglevel.Normal,
			expectedArgs:  []string{"What", "is", "2+2?"},
		},
		{
			name:          "only non-verbosity flags",
			args:          []string{"-f", "prompt.txt"},
			expectedLevel: loglevel.Normal,
			expectedArgs:  []string{"-f", "prompt.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, remaining := loglevel.ParseFlags(tt.args)

			if level != tt.expectedLevel {
				t.Errorf("loglevel.ParseFlags(%v) level = %v, want %v", tt.args, level, tt.expectedLevel)
			}

			if len(remaining) != len(tt.expectedArgs) {
				t.Errorf("loglevel.ParseFlags(%v) remaining len = %d, want %d\n  got:  %v\n  want: %v",
					tt.args, len(remaining), len(tt.expectedArgs), remaining, tt.expectedArgs)
				return
			}

			for i, arg := range remaining {
				if arg != tt.expectedArgs[i] {
					t.Errorf("loglevel.ParseFlags(%v) remaining[%d] = %q, want %q",
						tt.args, i, arg, tt.expectedArgs[i])
				}
			}
		})
	}
}

func TestParseFlagsNilArgs(t *testing.T) {
	level, remaining := loglevel.ParseFlags(nil)
	if level != loglevel.Normal {
		t.Errorf("loglevel.ParseFlags(nil) level = %v, want loglevel.Normal", level)
	}
	if len(remaining) != 0 {
		t.Errorf("loglevel.ParseFlags(nil) remaining = %v, want []", remaining)
	}
}
