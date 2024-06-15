package cmd

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	goFlags "github.com/jessevdk/go-flags"
)

// Options represents command-line arguments.
type Options struct {
	// ConfigPath specifies path to the configuration file.
	ConfigPath string `short:"c" long:"config-path" description:"Path to the config file." required:"true"`

	// Verbose defines whether we should write the DEBUG-level log or not.
	Verbose bool `yaml:"verbose" short:"v" long:"verbose" description:"Verbose output (optional)." optional:"yes" optional-value:"true"`
}

// type check
var _ fmt.Stringer = (*Options)(nil)

// String implements the fmt.Stringer interface for *Options.
func (o *Options) String() (str string) {
	b, err := yaml.Marshal(o)
	if err != nil {
		return fmt.Sprintf("Failed to stringify options due to %s", err)
	}

	return string(b)
}

// parseOptions parses os.Args and creates the Options struct.
func parseOptions() (o *Options, err error) {
	opts := &Options{}
	parser := goFlags.NewParser(opts, goFlags.Default|goFlags.IgnoreUnknown)
	remainingArgs, err := parser.ParseArgs(os.Args[1:])
	if err != nil {
		return nil, err
	}

	if len(remainingArgs) > 0 {
		return nil, fmt.Errorf("unknown arguments: %v", remainingArgs)
	}

	return opts, nil
}
