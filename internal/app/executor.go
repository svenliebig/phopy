package app

import (
	"context"
	"errors"

	"phopy/internal/domain"
	"phopy/internal/logging"
)

type Executor struct {
	FS     FileSystem
	Logger logging.Logger
}

func (e *Executor) Execute(ctx context.Context, plan domain.CopyPlan, includeOverrides bool) error {
	if e.FS == nil {
		return errors.New("executor requires FS")
	}

	stop := e.Logger.Measure("Copying files")
	defer stop()

	overrideTargets := map[string]bool{}
	if !includeOverrides {
		for _, item := range plan.OverrideItems {
			overrideTargets[item.TargetPath] = true
		}
	}

	totalItems := len(plan.Items)
	skippedOverrides := 0
	if !includeOverrides {
		skippedOverrides = len(plan.OverrideItems)
	}
	toCopy := totalItems - skippedOverrides
	e.Logger.Verbosef("Copying %d of %d items", toCopy, totalItems)

	for _, item := range plan.Items {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if overrideTargets[item.TargetPath] {
			continue
		}
		if err := e.FS.CopyFile(item.FileMeta.SourcePath, item.TargetPath); err != nil {
			return err
		}
	}
	return nil
}
