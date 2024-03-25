package cmd

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	goFlags "github.com/jessevdk/go-flags"
)

// Options represents command-line arguments.
type Options struct {
	// ListenAddr is the address the tool will be listening to.
	ListenAddr string `yaml:"listen" short:"l" long:"listen" description:"Address the tool will be listening to (required)." value-name:"<IP>" required:"true"`

	// Ports is the ports for both plain HTTP and TLS traffic. Passed as
	// plainPort:tlsPort
	Ports string `yaml:"ports" short:"p" long:"ports" description:"Port for accepting plain HTTP (required)." value-name:"<PLAIN_PORT:TLS_PORT>" required:"true"`

	// SNIMappingsPath is a path to the file with SNI mappings.
	SNIMappingsPath string `yaml:"sni-mappings-path" long:"sni-mappings-path" description:"Path to the file with SNI mappings (optional)."`

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
