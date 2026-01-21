package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"phopy/internal/domain"
	"phopy/internal/logging"
)

// ProgressFunc is called during scanning to report progress
type ProgressFunc func(current, total int)

type Planner struct {
	FS            FileSystem
	Exif          ExifReader
	ExifWorkers   int
	Logger        logging.Logger
	OnProgress    ProgressFunc
	AllowOverride bool
}

// shouldIncludeSource checks if a source file should be included in the plan.
// Returns false if the target file already exists and AllowOverride is false.
func (p *Planner) shouldIncludeSource(sourcePath, sourceDir, targetDir string) bool {
	if p.AllowOverride {
		return true
	}
	rel, err := filepath.Rel(sourceDir, sourcePath)
	if err != nil {
		return true // fallback to include
	}
	targetPath := filepath.Join(targetDir, rel)
	exists, _ := p.FS.Exists(targetPath)
	return !exists
}

func (p *Planner) Plan(ctx context.Context, sourceDir, targetDir string, startDate, endDate *time.Time) (domain.CopyPlan, error) {
	if p.FS == nil || p.Exif == nil {
		return domain.CopyPlan{}, errors.New("planner requires FS and Exif")
	}

	stop := p.Logger.Measure("Planning copy")
	defer stop()

	metas, warnings, skippedJPEGs, skippedRAWsDate, skippedRAWsDupl, err := p.scan(ctx, sourceDir, targetDir, startDate, endDate)
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

	var items []domain.CopyItem
	rawCount := 0
	jpegCount := 0

	for _, meta := range metas {
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

	// Only detect overrides when AllowOverride is true
	var overrides []domain.CopyItem
	rawOverrides := 0
	jpegOverrides := 0
	if p.AllowOverride {
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
	}

	rangeStart, rangeEnd := deriveRange(items, startDate, endDate)
	p.Logger.Verbosef("Planned %d items (%d RAW, %d JPEG), %d JPEGs skipped, %d RAWs skipped (date), %d RAWs skipped (dupl), %d overrides", len(items), rawCount, jpegCount, skippedJPEGs, skippedRAWsDate, skippedRAWsDupl, rawOverrides+jpegOverrides)

	return domain.CopyPlan{
		Items:           items,
		OverrideItems:   overrides,
		SkippedJPEGs:    skippedJPEGs,
		SkippedRAWsDate: skippedRAWsDate,
		SkippedRAWsDupl: skippedRAWsDupl,
		RangeStart:      rangeStart,
		RangeEnd:        rangeEnd,
		RawCount:        rawCount,
		JpegCount:       jpegCount,
		RawOverrides:    rawOverrides,
		JpegOverrides:   jpegOverrides,
		Warnings:        warnings,
	}, nil
}

func (p *Planner) scan(ctx context.Context, sourceDir, targetDir string, startDate, endDate *time.Time) ([]domain.FileMeta, []string, int, int, int, error) {
	stop := p.Logger.Measure("Scanning source directory")
	defer stop()

	// Phase 1: Walk directory and separate RAW and JPEG paths, build RAW base names set
	var rawPaths []string
	var jpegPaths []string
	rawBaseNames := make(map[string]bool)

	err := p.FS.WalkDir(sourceDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(d.Name())
		name := d.Name()
		baseName := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))

		if domain.IsRawExtension(ext) {
			rawPaths = append(rawPaths, path)
			rawBaseNames[baseName] = true
		} else if domain.IsJpegExtension(ext) {
			jpegPaths = append(jpegPaths, path)
		}
		return nil
	})
	if err != nil {
		return nil, nil, 0, 0, 0, err
	}

	// Phase 2: Filter paths based on target existence and RAW counterparts
	var pathsToProcess []string
	skippedJPEGs := 0
	skippedRAWsDupl := 0

	// Add RAW files that should be included
	for _, path := range rawPaths {
		if p.shouldIncludeSource(path, sourceDir, targetDir) {
			pathsToProcess = append(pathsToProcess, path)
		} else {
			skippedRAWsDupl++
		}
	}

	// Add JPEG files that should be included (no RAW counterpart and target doesn't exist)
	for _, path := range jpegPaths {
		name := filepath.Base(path)
		baseName := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))

		if rawBaseNames[baseName] {
			// Skip JPEG because RAW exists
			skippedJPEGs++
			continue
		}

		if p.shouldIncludeSource(path, sourceDir, targetDir) {
			pathsToProcess = append(pathsToProcess, path)
		}
	}

	totalFound := len(rawPaths) + len(jpegPaths)
	p.Logger.Verbosef("Found %d candidate files in %s (%d RAW, %d JPEG)", totalFound, sourceDir, len(rawPaths), len(jpegPaths))
	p.Logger.Verbosef("Processing %d files after filtering (%d JPEGs skipped for RAW, %d RAWs skipped for duplicate)", len(pathsToProcess), skippedJPEGs, skippedRAWsDupl)

	// Phase 3: Process remaining files with EXIF workers
	workerCount := p.ExifWorkers
	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}
	if workerCount < 1 {
		workerCount = 1
	}
	p.Logger.Verbosef("Using %d EXIF workers", workerCount)

	type result struct {
		meta        domain.FileMeta
		warning     string
		skip        bool
		skipRAWDate bool
		err         error
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

				ext := filepath.Ext(path)
				isRAW := domain.IsRawExtension(ext)

				// Early exit: if ModTime is before startDate, EXIF date will also be before
				// (EXIF date is typically <= ModTime in real photo workflows)
				if startDate != nil && info.ModTime().Before(*startDate) {
					results <- result{skip: true, skipRAWDate: isRAW}
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
					results <- result{skip: true, skipRAWDate: isRAW}
					continue
				}
				if endDate != nil && takenAt.After(*endDate) {
					results <- result{skip: true, skipRAWDate: isRAW}
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
		for _, path := range pathsToProcess {
			select {
			case <-ctx.Done():
				return
			case jobs <- path:
			}
		}
	}()

	var metas []domain.FileMeta
	var warnings []string
	skippedRAWsDate := 0
	total := len(pathsToProcess)
	for i := range pathsToProcess {
		res := <-results
		if res.err != nil {
			return nil, nil, 0, 0, 0, res.err
		}
		if res.warning != "" {
			warnings = append(warnings, res.warning)
		}
		if res.skip {
			if res.skipRAWDate {
				skippedRAWsDate++
			}
			// Still report progress for skipped files
			if p.OnProgress != nil {
				p.OnProgress(i+1, total)
			}
			continue
		}
		metas = append(metas, res.meta)

		// Report progress
		if p.OnProgress != nil {
			p.OnProgress(i+1, total)
		}
	}

	return metas, warnings, skippedJPEGs, skippedRAWsDate, skippedRAWsDupl, nil
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
