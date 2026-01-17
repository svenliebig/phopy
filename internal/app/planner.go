package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"phopy/internal/domain"
	"phopy/internal/logging"
)

type Planner struct {
	FS          FileSystem
	Exif        ExifReader
	ExifWorkers int
	Logger      logging.Logger
}

func (p *Planner) Plan(ctx context.Context, sourceDir, targetDir string, startDate, endDate *time.Time) (domain.CopyPlan, error) {
	if p.FS == nil || p.Exif == nil {
		return domain.CopyPlan{}, errors.New("planner requires FS and Exif")
	}

	stop := p.Logger.Measure("Planning copy")
	defer stop()

	metas, warnings, err := p.scan(ctx, sourceDir, startDate, endDate)
	if err != nil {
		return domain.CopyPlan{}, err
	}
	p.Logger.Verbosef("Collected %d candidate files (%d warnings)", len(metas), len(warnings))

	sort.Slice(metas, func(i, j int) bool {
		if metas[i].TakenAt.Equal(metas[j].TakenAt) {
			return metas[i].Name < metas[j].Name
		}
		return metas[i].TakenAt.Before(metas[j].TakenAt)
	})

	rawByBase := make(map[string]bool, len(metas))
	for _, meta := range metas {
		if meta.IsRAW {
			rawByBase[meta.BaseName] = true
		}
	}

	var items []domain.CopyItem
	skippedJPEGs := 0
	rawCount := 0
	jpegCount := 0

	for _, meta := range metas {
		if meta.IsJPEG && rawByBase[meta.BaseName] {
			skippedJPEGs++
			continue
		}

		targetPath := filepath.Join(targetDir, meta.RelativePath)
		items = append(items, domain.CopyItem{
			FileMeta:   meta,
			TargetPath: targetPath,
		})

		if meta.IsRAW {
			rawCount++
		} else if meta.IsJPEG {
			jpegCount++
		}
	}

	var overrides []domain.CopyItem
	rawOverrides := 0
	jpegOverrides := 0
	for _, item := range items {
		exists, err := p.FS.Exists(item.TargetPath)
		if err != nil {
			return domain.CopyPlan{}, err
		}
		if exists {
			overrides = append(overrides, item)
			if item.FileMeta.IsRAW {
				rawOverrides++
			} else if item.FileMeta.IsJPEG {
				jpegOverrides++
			}
		}
	}

	rangeStart, rangeEnd := deriveRange(items, startDate, endDate)
	p.Logger.Verbosef("Planned %d items (%d RAW, %d JPEG), %d JPEGs skipped, %d overrides", len(items), rawCount, jpegCount, skippedJPEGs, rawOverrides+jpegOverrides)

	return domain.CopyPlan{
		Items:         items,
		OverrideItems: overrides,
		SkippedJPEGs:  skippedJPEGs,
		RangeStart:    rangeStart,
		RangeEnd:      rangeEnd,
		RawCount:      rawCount,
		JpegCount:     jpegCount,
		RawOverrides:  rawOverrides,
		JpegOverrides: jpegOverrides,
		Warnings:      warnings,
	}, nil
}

func (p *Planner) scan(ctx context.Context, sourceDir string, startDate, endDate *time.Time) ([]domain.FileMeta, []string, error) {
	stop := p.Logger.Measure("Scanning source directory")
	defer stop()

	var paths []string
	err := p.FS.WalkDir(sourceDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(d.Name())
		if !domain.IsRawExtension(ext) && !domain.IsJpegExtension(ext) {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	p.Logger.Verbosef("Found %d candidate files in %s", len(paths), sourceDir)

	workerCount := p.ExifWorkers
	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}
	if workerCount < 1 {
		workerCount = 1
	}
	p.Logger.Verbosef("Using %d EXIF workers", workerCount)

	type result struct {
		meta    domain.FileMeta
		warning string
		skip    bool
		err     error
	}

	jobs := make(chan string)
	results := make(chan result)

	for i := 0; i < workerCount; i++ {
		go func() {
			for path := range jobs {
				info, statErr := p.FS.Stat(path)
				if statErr != nil {
					results <- result{err: statErr}
					continue
				}

				takenAt, exifErr := p.Exif.DateTimeOriginal(ctx, path)
				warning := ""
				if exifErr != nil {
					if errors.Is(exifErr, context.Canceled) || errors.Is(exifErr, context.DeadlineExceeded) {
						results <- result{err: exifErr}
						continue
					}
					takenAt = info.ModTime()
					warning = fmt.Sprintf("EXIF not found for %s, using filesystem time", filepath.Base(path))
				}

				if startDate != nil && takenAt.Before(*startDate) {
					results <- result{skip: true}
					continue
				}
				if endDate != nil && takenAt.After(*endDate) {
					results <- result{skip: true}
					continue
				}

				rel, relErr := filepath.Rel(sourceDir, path)
				if relErr != nil {
					rel = filepath.Base(path)
				}

				results <- result{
					meta:    domain.NewFileMeta(path, rel, takenAt),
					warning: warning,
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, path := range paths {
			select {
			case <-ctx.Done():
				return
			case jobs <- path:
			}
		}
	}()

	var metas []domain.FileMeta
	var warnings []string
	for range paths {
		res := <-results
		if res.err != nil {
			return nil, nil, res.err
		}
		if res.warning != "" {
			warnings = append(warnings, res.warning)
		}
		if res.skip {
			continue
		}
		metas = append(metas, res.meta)
	}

	return metas, warnings, nil
}

func deriveRange(items []domain.CopyItem, startDate, endDate *time.Time) (*time.Time, *time.Time) {
	if startDate != nil || endDate != nil {
		return startDate, endDate
	}
	if len(items) == 0 {
		return nil, nil
	}
	min := items[0].FileMeta.TakenAt
	max := items[0].FileMeta.TakenAt
	for _, item := range items[1:] {
		if item.FileMeta.TakenAt.Before(min) {
			min = item.FileMeta.TakenAt
		}
		if item.FileMeta.TakenAt.After(max) {
			max = item.FileMeta.TakenAt
		}
	}
	return &min, &max
}
