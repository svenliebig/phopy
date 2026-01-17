package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"phopy/internal/app"
	"phopy/internal/config"
	appErrors "phopy/internal/errors"
	"phopy/internal/infra/exif"
	"phopy/internal/infra/fs"
	"phopy/internal/presentation"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		exitWithError(appErrors.Wrap(appErrors.InvalidConfig, "config", "", err))
	}

	filesystem := fs.OSFS{}
	exifReader := exif.Reader{}

	if _, err := filesystem.Stat(cfg.SourceDir); err != nil {
		exitWithError(appErrors.Wrap(appErrors.NotFound, "stat", cfg.SourceDir, err))
	}

	planner := app.Planner{
		FS:   filesystem,
		Exif: exifReader,
	}

	plan, err := planner.Plan(ctx, cfg.SourceDir, cfg.TargetDir, cfg.StartDate, cfg.EndDate)
	if err != nil {
		exitWithError(appErrors.Wrap(appErrors.Internal, "plan", cfg.SourceDir, err))
	}

	printer := presentation.Printer{
		Writer:  os.Stdout,
		Verbose: cfg.Verbose,
	}

	if cfg.DryRun {
		printer.PrintDryRun(plan)
		return
	}

	includeOverrides := false
	overridesConfirmed := 0
	if len(plan.OverrideItems) > 0 {
		confirmed, confirmErr := confirmOverrides(len(plan.OverrideItems))
		if confirmErr != nil {
			exitWithError(appErrors.Wrap(appErrors.Internal, "prompt", "", confirmErr))
		}
		includeOverrides = confirmed
		if confirmed {
			overridesConfirmed = len(plan.OverrideItems)
		}
	}

	if err := filesystem.MkdirAll(cfg.TargetDir, 0o755); err != nil {
		exitWithError(appErrors.Wrap(appErrors.IOFailure, "mkdir", cfg.TargetDir, err))
	}

	executor := app.Executor{FS: filesystem}
	if err := executor.Execute(ctx, plan, includeOverrides); err != nil {
		exitWithError(appErrors.Wrap(appErrors.IOFailure, "copy", cfg.TargetDir, err))
	}

	printer.PrintExecution(plan, overridesConfirmed)
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
