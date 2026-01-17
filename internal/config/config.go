package config

import (
	"errors"
	"flag"
	"os"
	"strings"
	"time"
)

type Config struct {
	SourceDir string
	TargetDir string
	DryRun    bool
	Verbose   bool
	StartDate *time.Time
	EndDate   *time.Time
}

func Parse(args []string) (Config, error) {
	var cfg Config

	fs := flag.NewFlagSet("phopy", flag.ContinueOnError)
	fs.StringVar(&cfg.SourceDir, "source", "", "Source directory to copy from")
	fs.StringVar(&cfg.SourceDir, "s", "", "Source directory to copy from (shorthand)")
	fs.StringVar(&cfg.TargetDir, "target", "", "Target directory to copy to")
	fs.StringVar(&cfg.TargetDir, "t", "", "Target directory to copy to (shorthand)")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "Dry run (no copy)")
	fs.BoolVar(&cfg.DryRun, "d", false, "Dry run (shorthand)")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "Verbose output")
	fs.BoolVar(&cfg.Verbose, "v", false, "Verbose output (shorthand)")
	fromDate := fs.String("from", "", "From date (YYYY-MM-DD)")
	fs.StringVar(fromDate, "f", "", "From date (YYYY-MM-DD) (shorthand)")
	untilDate := fs.String("until", "", "Until date (YYYY-MM-DD)")
	fs.StringVar(untilDate, "u", "", "Until date (YYYY-MM-DD) (shorthand)")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	if cfg.SourceDir == "" {
		cfg.SourceDir = envOrEmpty("PHOPY_SOURCE_DIR")
	}
	if cfg.TargetDir == "" {
		cfg.TargetDir = envOrEmpty("PHOPY_TARGET_DIR")
	}
	if !cfg.Verbose {
		cfg.Verbose = envTruthy("PHOPY_VERBOSE")
	}
	if *fromDate == "" {
		*fromDate = envOrEmpty("PHOPY_FROM")
		if *fromDate == "" {
			*fromDate = envOrEmpty("PHOPY_START_DATE")
		}
	}
	if *untilDate == "" {
		*untilDate = envOrEmpty("PHOPY_UNTIL")
		if *untilDate == "" {
			*untilDate = envOrEmpty("PHOPY_END_DATE")
		}
	}

	if cfg.SourceDir == "" || cfg.TargetDir == "" {
		return Config{}, errors.New("source and target are required")
	}

	if *fromDate != "" {
		parsed, err := time.ParseInLocation("2006-01-02", *fromDate, time.Local)
		if err != nil {
			return Config{}, errors.New("invalid from date, use YYYY-MM-DD")
		}
		cfg.StartDate = &parsed
	}
	if *untilDate != "" {
		parsed, err := time.ParseInLocation("2006-01-02", *untilDate, time.Local)
		if err != nil {
			return Config{}, errors.New("invalid until date, use YYYY-MM-DD")
		}
		parsed = parsed.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
		cfg.EndDate = &parsed
	}

	return cfg, nil
}

func envOrEmpty(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func envTruthy(key string) bool {
	val := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return val == "1" || val == "true" || val == "yes" || val == "y"
}
