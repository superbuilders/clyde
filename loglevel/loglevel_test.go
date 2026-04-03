package loglevel

import (
	"testing"
)

func TestLevelString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{Silent, "silent"},
		{Quiet, "quiet"},
		{Normal, "normal"},
		{Verbose, "verbose"},
		{Debug, "debug"},
		{Level(99), "Level(99)"},
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
	// Verify levels are ordered: Silent < Quiet < Normal < Verbose < Debug
	if !(Silent < Quiet && Quiet < Normal && Normal < Verbose && Verbose < Debug) {
		t.Error("Log levels are not in expected order: Silent < Quiet < Normal < Verbose < Debug")
	}
}

func TestShouldShow(t *testing.T) {
	tests := []struct {
		name      string
		current   Level
		threshold Level
		want      bool
	}{
		// Silent shows nothing
		{"silent shows nothing at quiet", Silent, Quiet, false},
		{"silent shows nothing at normal", Silent, Normal, false},
		{"silent shows nothing at verbose", Silent, Verbose, false},
		{"silent shows nothing at debug", Silent, Debug, false},
		// But silent >= silent is true (edge case, matches convention)
		{"silent >= silent", Silent, Silent, true},

		// Quiet shows quiet-level and below
		{"quiet shows quiet", Quiet, Quiet, true},
		{"quiet hides normal", Quiet, Normal, false},
		{"quiet hides verbose", Quiet, Verbose, false},
		{"quiet hides debug", Quiet, Debug, false},

		// Normal shows quiet, normal
		{"normal shows quiet", Normal, Quiet, true},
		{"normal shows normal", Normal, Normal, true},
		{"normal hides verbose", Normal, Verbose, false},
		{"normal hides debug", Normal, Debug, false},

		// Verbose shows quiet, normal, verbose
		{"verbose shows quiet", Verbose, Quiet, true},
		{"verbose shows normal", Verbose, Normal, true},
		{"verbose shows verbose", Verbose, Verbose, true},
		{"verbose hides debug", Verbose, Debug, false},

		// Debug shows everything
		{"debug shows quiet", Debug, Quiet, true},
		{"debug shows normal", Debug, Normal, true},
		{"debug shows verbose", Debug, Verbose, true},
		{"debug shows debug", Debug, Debug, true},
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
		expectedLevel Level
		expectedArgs  []string
	}{
		{
			name:          "no flags = Normal",
			args:          []string{},
			expectedLevel: Normal,
			expectedArgs:  []string{},
		},
		{
			name:          "--silent flag",
			args:          []string{"--silent"},
			expectedLevel: Silent,
			expectedArgs:  []string{},
		},
		{
			name:          "-q flag",
			args:          []string{"-q"},
			expectedLevel: Quiet,
			expectedArgs:  []string{},
		},
		{
			name:          "--quiet flag",
			args:          []string{"--quiet"},
			expectedLevel: Quiet,
			expectedArgs:  []string{},
		},
		{
			name:          "-v flag",
			args:          []string{"-v"},
			expectedLevel: Verbose,
			expectedArgs:  []string{},
		},
		{
			name:          "--verbose flag",
			args:          []string{"--verbose"},
			expectedLevel: Verbose,
			expectedArgs:  []string{},
		},
		{
			name:          "--debug flag",
			args:          []string{"--debug"},
			expectedLevel: Debug,
			expectedArgs:  []string{},
		},
		{
			name:          "flag with other args",
			args:          []string{"-v", "Hello", "World"},
			expectedLevel: Verbose,
			expectedArgs:  []string{"Hello", "World"},
		},
		{
			name:          "flag at end of args",
			args:          []string{"Hello", "World", "--debug"},
			expectedLevel: Debug,
			expectedArgs:  []string{"Hello", "World"},
		},
		{
			name:          "flag in middle of args",
			args:          []string{"Hello", "-q", "World"},
			expectedLevel: Quiet,
			expectedArgs:  []string{"Hello", "World"},
		},
		{
			name:          "multiple flags (last wins)",
			args:          []string{"-q", "--verbose", "--debug"},
			expectedLevel: Debug,
			expectedArgs:  []string{},
		},
		{
			name:          "flag preserved with -f",
			args:          []string{"-v", "-f", "prompt.txt"},
			expectedLevel: Verbose,
			expectedArgs:  []string{"-f", "prompt.txt"},
		},
		{
			name:          "no verbosity flags means normal",
			args:          []string{"What", "is", "2+2?"},
			expectedLevel: Normal,
			expectedArgs:  []string{"What", "is", "2+2?"},
		},
		{
			name:          "only non-verbosity flags",
			args:          []string{"-f", "prompt.txt"},
			expectedLevel: Normal,
			expectedArgs:  []string{"-f", "prompt.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, remaining := ParseFlags(tt.args)

			if level != tt.expectedLevel {
				t.Errorf("ParseFlags(%v) level = %v, want %v", tt.args, level, tt.expectedLevel)
			}

			if len(remaining) != len(tt.expectedArgs) {
				t.Errorf("ParseFlags(%v) remaining len = %d, want %d\n  got:  %v\n  want: %v",
					tt.args, len(remaining), len(tt.expectedArgs), remaining, tt.expectedArgs)
				return
			}

			for i, arg := range remaining {
				if arg != tt.expectedArgs[i] {
					t.Errorf("ParseFlags(%v) remaining[%d] = %q, want %q",
						tt.args, i, arg, tt.expectedArgs[i])
				}
			}
		})
	}
}

func TestParseFlagsNilArgs(t *testing.T) {
	level, remaining := ParseFlags(nil)
	if level != Normal {
		t.Errorf("ParseFlags(nil) level = %v, want Normal", level)
	}
	if len(remaining) != 0 {
		t.Errorf("ParseFlags(nil) remaining = %v, want []", remaining)
	}
}
