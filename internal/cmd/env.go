package cmd

import (
	"fmt"
	"os"

	"github.com/AdguardTeam/golibs/log"
	"github.com/caarlos0/env/v7"
)

// environments stores the values of the parsed environment variables.
type environments struct {
	ListenAddr        string     `env:"LISTEN_ADDR,required"`
	ListenPort        uint16     `env:"LISTEN_PORT,required"`
	SNIMappingCSVPath string     `env:"SNI_MAPPING_CSV_PATH"`
	LogVerbose        strictBool `env:"VERBOSE" envDefault:"0"`
	LogFile           string     `env:"LOGFILE"`
}

// readEnvs reads the configuration defined by the environment variables.  See
// environments.
func readEnvs() (envs *environments, err error) {
	envs = &environments{}
	err = env.Parse(envs)
	if err != nil {
		return nil, fmt.Errorf("parsing environment variables: %w", err)
	}

	if envs.LogVerbose {
		log.SetLevel(log.DEBUG)
	}

	if envs.LogFile != "" {
		f, fErr := os.OpenFile(envs.LogFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if fErr != nil {
			return nil, fErr
		}

		log.SetOutput(f)
	}

	return envs, nil
}

// strictBool is a type for booleans that are parsed from the environment more
// strictly than the usual bool.  It only accepts "0" and "1" as valid values.
//
// TODO(e.burkov, a.garipov):  Move to golibs?
type strictBool bool

// UnmarshalText implements the encoding.TextUnmarshaler interface for
// *strictBool.
func (sb *strictBool) UnmarshalText(b []byte) (err error) {
	const (
		strictBoolFalse = '0'
		strictBoolTrue  = '1'
	)

	if len(b) == 1 {
		switch b[0] {
		case strictBoolFalse:
			*sb = false

			return nil
		case strictBoolTrue:
			*sb = true

			return nil
		default:
			// Go on and return an error.
		}
	}

	return fmt.Errorf("invalid value %q, supported: %q, %q", b, strictBoolFalse, strictBoolTrue)
}
