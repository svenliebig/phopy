package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"phopy/internal/app"
	"phopy/internal/config"
	"phopy/internal/domain"
	appErrors "phopy/internal/errors"
	"phopy/internal/infra/exif"
	"phopy/internal/infra/fs"
	"phopy/internal/logging"
	"phopy/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	// Create infrastructure
	filesystem := fs.OSFS{}
	exifReader := exif.Reader{}
	logger := logging.New(os.Stdout, cfg.Verbose)

	// Verify source directory exists
	if _, err := filesystem.Stat(cfg.SourceDir); err != nil {
		return appErrors.Wrap(appErrors.NotFound, "stat", cfg.SourceDir, err)
	}

	// Create TUI config
	tuiConfig := tui.Config{
		SourceDir: cfg.SourceDir,
		TargetDir: cfg.TargetDir,
		DryRun:    cfg.DryRun,
		Verbose:   cfg.Verbose,
	}

	// Create the TUI model and program early so we can send progress updates
	m := tui.NewModel(tuiConfig)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))

	// Create planner with progress callback
	planner := app.Planner{
		FS:     filesystem,
		Exif:   exifReader,
		Logger: logger,
		OnProgress: func(current, total int) {
			p.Send(tui.ScanProgressMsg{Current: current, Total: total})
		},
	}

	// Channels for communication
	planDone := make(chan struct{})
	var plan domain.CopyPlan
	var planErr error

	// Run planning in background
	go func() {
		defer close(planDone)
		plan, planErr = planner.Plan(ctx, cfg.SourceDir, cfg.TargetDir, cfg.StartDate, cfg.EndDate)
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

	// If quitting early, exit gracefully
	if final.Quitting {
		return nil
	}

	// Wait for planning to complete
	<-planDone
	if planErr != nil {
		return appErrors.Wrap(appErrors.Internal, "plan", cfg.SourceDir, planErr)
	}

	// If dry run, we're done (TUI already showed the preview)
	if cfg.DryRun {
		return nil
	}

	// Check if we got the plan and handle execution
	if final.Phase == tui.PhaseError {
		return final.Err
	}

	// Determine if overrides were confirmed
	includeOverrides := final.OverridesConfirmed > 0

	// Ensure target directory exists
	if err := filesystem.MkdirAll(cfg.TargetDir, 0o755); err != nil {
		return appErrors.Wrap(appErrors.IOFailure, "mkdir", cfg.TargetDir, err)
	}

	// Execute the copy
	executor := app.Executor{FS: filesystem, Logger: logger}
	if err := executor.Execute(ctx, final.Plan, includeOverrides); err != nil {
		return appErrors.Wrap(appErrors.IOFailure, "copy", cfg.TargetDir, err)
	}

	// Print completion summary
	printCompletionSummary(final.Plan, includeOverrides)

	return nil
}

func printCompletionSummary(plan domain.CopyPlan, overridesIncluded bool) {
	successColor := lipgloss.Color("#85DCB0")
	primaryColor := lipgloss.Color("#E8A87C")
	dimColor := lipgloss.Color("#9CA3AF")

	successStyle := lipgloss.NewStyle().
		Foreground(successColor).
		Bold(true)

	statStyle := lipgloss.NewStyle().
		Foreground(primaryColor)

	dimStyle := lipgloss.NewStyle().
		Foreground(dimColor)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(successColor).
		Padding(1, 2).
		MarginTop(1)

	// Build summary content
	totalCopied := plan.RawCount + plan.JpegCount
	content := fmt.Sprintf("%s Copy completed successfully!\n\n", successStyle.Render("✓"))
	content += fmt.Sprintf("  %s %s\n", dimStyle.Render("RAW files:"), statStyle.Render(fmt.Sprintf("◆ %d", plan.RawCount)))
	content += fmt.Sprintf("  %s %s\n", dimStyle.Render("JPEG files:"), statStyle.Render(fmt.Sprintf("◇ %d", plan.JpegCount)))
	content += fmt.Sprintf("  %s %s\n", dimStyle.Render("Total:"), statStyle.Render(fmt.Sprintf("%d files", totalCopied)))

	if plan.SkippedJPEGs > 0 {
		content += fmt.Sprintf("  %s %s\n", dimStyle.Render("Skipped:"), dimStyle.Render(fmt.Sprintf("○ %d JPEGs (RAW exists)", plan.SkippedJPEGs)))
	}

	if overridesIncluded && (plan.RawOverrides+plan.JpegOverrides) > 0 {
		content += fmt.Sprintf("  %s %s\n", dimStyle.Render("Overwritten:"), statStyle.Render(fmt.Sprintf("⚠ %d files", plan.RawOverrides+plan.JpegOverrides)))
	}

	fmt.Println(boxStyle.Render(content))
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
