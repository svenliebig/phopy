package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"phopy/internal/app"
	"phopy/internal/config"
	"phopy/internal/domain"
	appErrors "phopy/internal/errors"
	"phopy/internal/infra/exif"
	"phopy/internal/infra/fs"
	"phopy/internal/logging"
	"phopy/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
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
	override  bool
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
	cmd.Flags().BoolVarP(&opts.override, "override", "o", false, "Allow overwriting existing files in target directory")
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
		Override:  opts.override,
		FromDate:  opts.fromDate,
		UntilDate: opts.untilDate,
	})
	if err != nil {
		return appErrors.Wrap(appErrors.InvalidConfig, "config", "", err)
	}

	// Create infrastructure
	filesystem := fs.OSFS{}
	exifReader := exif.Reader{}
	logger := logging.New(os.Stdout, cfg.Verbose)

	// Verify source directory exists
	if _, err := filesystem.Stat(cfg.SourceDir); err != nil {
		return appErrors.Wrap(appErrors.NotFound, "stat", cfg.SourceDir, err)
	}

	// We need to declare p early so we can reference it in the ExecuteCopy callback
	var p *tea.Program
	var pMu sync.Mutex

	// Create the ExecuteCopy function that will be called by the TUI
	executeCopy := func(plan domain.CopyPlan, includeOverrides bool) tea.Cmd {
		return func() tea.Msg {
			// Ensure target directory exists
			if err := filesystem.MkdirAll(cfg.TargetDir, 0o755); err != nil {
				return tui.ErrorMsg{Err: appErrors.Wrap(appErrors.IOFailure, "mkdir", cfg.TargetDir, err)}
			}

			// Execute the copy with progress callback
			executor := app.Executor{
				FS:     filesystem,
				Logger: logger,
				OnProgress: func(current, total int, currentFile string) {
					pMu.Lock()
					prog := p
					pMu.Unlock()
					if prog != nil {
						prog.Send(tui.CopyProgressMsg{
							Current: current,
							Total:   total,
							File:    currentFile,
						})
					}
				},
			}

			if err := executor.Execute(ctx, plan, includeOverrides); err != nil {
				return tui.ErrorMsg{Err: appErrors.Wrap(appErrors.IOFailure, "copy", cfg.TargetDir, err)}
			}

			// Signal copy is done
			overrides := 0
			if includeOverrides {
				overrides = len(plan.OverrideItems)
			}
			return tui.CopyDoneMsg{OverridesConfirmed: overrides}
		}
	}

	// Create TUI config with the ExecuteCopy callback
	tuiConfig := tui.Config{
		SourceDir:   cfg.SourceDir,
		TargetDir:   cfg.TargetDir,
		DryRun:      cfg.DryRun,
		Verbose:     cfg.Verbose,
		ExecuteCopy: executeCopy,
	}

	// Create the TUI model and program
	m := tui.NewModel(tuiConfig)
	pMu.Lock()
	p = tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	pMu.Unlock()

	// Create planner with progress callback
	planner := app.Planner{
		FS:            filesystem,
		Exif:          exifReader,
		Logger:        logger,
		AllowOverride: cfg.Override,
		OnProgress: func(current, total int) {
			p.Send(tui.ScanProgressMsg{Current: current, Total: total})
		},
	}

	// Run planning in background
	go func() {
		plan, planErr := planner.Plan(ctx, cfg.SourceDir, cfg.TargetDir, cfg.StartDate, cfg.EndDate)
		if planErr != nil {
			p.Send(tui.ErrorMsg{Err: appErrors.Wrap(appErrors.Internal, "plan", cfg.SourceDir, planErr)})
			return
		}
		p.Send(tui.PlanReadyMsg{Plan: plan})
	}()

	// Run the TUI
	finalModel, err := p.Run()
	if err != nil {
		return appErrors.Wrap(appErrors.Internal, "tui", "", err)
	}

	final := finalModel.(tui.Model)

	// If there was an error in the TUI, return it
	if final.Phase == tui.PhaseError && final.Err != nil {
		return final.Err
	}

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

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, appErrors.UserMessage(err))
	os.Exit(1)
}
