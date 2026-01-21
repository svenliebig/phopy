package app

import (
	"context"
	"errors"

	"phopy/internal/domain"
	"phopy/internal/logging"
)

// CopyProgressFunc is called during copy with progress updates
type CopyProgressFunc func(current, total int, currentFile string)

type Executor struct {
	FS         FileSystem
	Logger     logging.Logger
	OnProgress CopyProgressFunc
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

	// Build list of items to copy
	var itemsToCopy []domain.CopyItem
	for _, item := range plan.Items {
		if !overrideTargets[item.TargetPath] {
			itemsToCopy = append(itemsToCopy, item)
		}
	}

	totalItems := len(itemsToCopy)
	e.Logger.Verbosef("Copying %d of %d items", totalItems, len(plan.Items))

	for i, item := range itemsToCopy {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Report progress before copying
		if e.OnProgress != nil {
			e.OnProgress(i, totalItems, item.FileMeta.Name)
		}

		if err := e.FS.CopyFile(item.FileMeta.SourcePath, item.TargetPath); err != nil {
			return err
		}
	}

	// Report completion
	if e.OnProgress != nil {
		e.OnProgress(totalItems, totalItems, "")
	}

	return nil
}
