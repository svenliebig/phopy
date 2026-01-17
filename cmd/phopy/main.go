package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"phopy/internal/app"
	"phopy/internal/config"
	appErrors "phopy/internal/errors"
	"phopy/internal/infra/exif"
	"phopy/internal/infra/fs"
	"phopy/internal/logging"
	"phopy/internal/presentation"

	"github.com/spf13/cobra"
)

func main() {
	cmd := newRootCmd()
	if err := cmd.Execute(); err != nil {
		if isInvalidConfig(err) {
			_ = cmd.Usage()
		}
		exitWithError(err)
	}
}

type cliOptions struct {
	sourceDir string
	targetDir string
	dryRun    bool
	verbose   bool
	fromDate  string
	untilDate string
}

func newRootCmd() *cobra.Command {
	opts := cliOptions{}
	cmd := &cobra.Command{
		Use:           "phopy",
		Short:         "Copy photos into dated folders",
		Long:          "phopy copies photos from a source directory into a target directory, grouped by date.\n\nEnvironment variables:\n  PHOPY_SOURCE_DIR     Source directory to copy from\n  PHOPY_TARGET_DIR     Target directory to copy to\n  PHOPY_VERBOSE        Verbose output (true/1/yes)\n  PHOPY_FROM           Start date (YYYY-MM-DD)\n  PHOPY_START_DATE     Start date (YYYY-MM-DD)\n  PHOPY_UNTIL          End date (YYYY-MM-DD)\n  PHOPY_END_DATE       End date (YYYY-MM-DD)",
		Example:       "  phopy --source ~/Photos --target ~/Archive\n  phopy -s ./in -t ./out --from 2024-01-01 --until 2024-12-31 --dry-run",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&opts.sourceDir, "source", "s", "", "Source directory to copy from (env: PHOPY_SOURCE_DIR)")
	cmd.Flags().StringVarP(&opts.targetDir, "target", "t", "", "Target directory to copy to (env: PHOPY_TARGET_DIR)")
	cmd.Flags().BoolVarP(&opts.dryRun, "dry-run", "d", false, "Dry run (no copy)")
	cmd.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "Verbose output (env: PHOPY_VERBOSE)")
	cmd.Flags().StringVarP(&opts.fromDate, "from", "f", "", "Start date (YYYY-MM-DD) (env: PHOPY_FROM, PHOPY_START_DATE)")
	cmd.Flags().StringVarP(&opts.untilDate, "until", "u", "", "End date (YYYY-MM-DD) (env: PHOPY_UNTIL, PHOPY_END_DATE)")

	cmd.AddCommand(newCompletionCmd())

	return cmd
}

func run(ctx context.Context, opts cliOptions) error {
	cfg, err := config.FromOptions(config.Options{
		SourceDir: opts.sourceDir,
		TargetDir: opts.targetDir,
		DryRun:    opts.dryRun,
		Verbose:   opts.verbose,
		FromDate:  opts.fromDate,
		UntilDate: opts.untilDate,
	})
	if err != nil {
		return appErrors.Wrap(appErrors.InvalidConfig, "config", "", err)
	}

	logger := logging.New(os.Stdout, cfg.Verbose)
	logger.Verbosef("Config: source=%s target=%s dry-run=%t date-range=%s", cfg.SourceDir, cfg.TargetDir, cfg.DryRun, formatDateRange(cfg.StartDate, cfg.EndDate))

	filesystem := fs.OSFS{}
	exifReader := exif.Reader{}

	if _, err := filesystem.Stat(cfg.SourceDir); err != nil {
		return appErrors.Wrap(appErrors.NotFound, "stat", cfg.SourceDir, err)
	}

	planner := app.Planner{
		FS:     filesystem,
		Exif:   exifReader,
		Logger: logger,
	}

	plan, err := planner.Plan(ctx, cfg.SourceDir, cfg.TargetDir, cfg.StartDate, cfg.EndDate)
	if err != nil {
		return appErrors.Wrap(appErrors.Internal, "plan", cfg.SourceDir, err)
	}

	printer := presentation.Printer{
		Writer:  os.Stdout,
		Verbose: cfg.Verbose,
	}

	if cfg.DryRun {
		logger.Verbosef("Dry run: no files will be copied")
		printer.PrintDryRun(plan)
		return nil
	}

	includeOverrides := false
	overridesConfirmed := 0
	if len(plan.OverrideItems) > 0 {
		logger.Verbosef("Override confirmation required for %d files", len(plan.OverrideItems))
		confirmed, confirmErr := confirmOverrides(len(plan.OverrideItems))
		if confirmErr != nil {
			return appErrors.Wrap(appErrors.Internal, "prompt", "", confirmErr)
		}
		includeOverrides = confirmed
		if confirmed {
			overridesConfirmed = len(plan.OverrideItems)
		}
	}

	logger.Verbosef("Ensuring target directory exists")
	if err := filesystem.MkdirAll(cfg.TargetDir, 0o755); err != nil {
		return appErrors.Wrap(appErrors.IOFailure, "mkdir", cfg.TargetDir, err)
	}

	executor := app.Executor{FS: filesystem, Logger: logger}
	if err := executor.Execute(ctx, plan, includeOverrides); err != nil {
		return appErrors.Wrap(appErrors.IOFailure, "copy", cfg.TargetDir, err)
	}
	logger.Verbosef("Copy complete")

	printer.PrintExecution(plan, overridesConfirmed)
	return nil
}

func isInvalidConfig(err error) bool {
	var appErr *appErrors.AppError
	if errors.As(err, &appErr) {
		return appErr.Kind == appErrors.InvalidConfig
	}
	return false
}

func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:       "completion [bash|zsh|fish|powershell]",
		Short:     "Generate shell completion scripts",
		Long:      "Generate shell completion scripts for phopy. The output should be sourced in your shell.",
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(os.Stdout, true)
			case "zsh":
				return root.GenZshCompletion(os.Stdout)
			case "fish":
				return root.GenFishCompletion(os.Stdout, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return fmt.Errorf("unsupported shell: %s", args[0])
			}
		},
	}
	return cmd
}

func confirmOverrides(count int) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Override %d existing files? [y/N]: ", count)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, appErrors.UserMessage(err))
	os.Exit(1)
}

func formatDateRange(startDate, endDate *time.Time) string {
	if startDate == nil && endDate == nil {
		return "all dates"
	}
	start := "any"
	if startDate != nil {
		start = startDate.Format("2006-01-02")
	}
	end := "any"
	if endDate != nil {
		end = endDate.Format("2006-01-02")
	}
	return fmt.Sprintf("%s to %s", start, end)
}
