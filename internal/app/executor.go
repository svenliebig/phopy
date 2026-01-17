package app

import (
	"context"
	"errors"

	"phopy/internal/domain"
)

type Executor struct {
	FS FileSystem
}

func (e *Executor) Execute(ctx context.Context, plan domain.CopyPlan, includeOverrides bool) error {
	if e.FS == nil {
		return errors.New("executor requires FS")
	}

	overrideTargets := map[string]bool{}
	if !includeOverrides {
		for _, item := range plan.OverrideItems {
			overrideTargets[item.TargetPath] = true
		}
	}

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
