package app

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	"phopy/internal/domain"
)

type mockFS struct {
	entries []mockEntry
	exists  map[string]bool
}

type mockEntry struct {
	path    string
	isDir   bool
	modTime time.Time
}

func (m mockFS) WalkDir(root string, fn fs.WalkDirFunc) error {
	for _, entry := range m.entries {
		dirEntry := mockDirEntry{name: filepath.Base(entry.path), isDir: entry.isDir}
		if err := fn(entry.path, dirEntry, nil); err != nil {
			return err
		}
	}
	return nil
}

func (m mockFS) Stat(path string) (fs.FileInfo, error) {
	for _, entry := range m.entries {
		if entry.path == path {
			return mockFileInfo{name: filepath.Base(path), modTime: entry.modTime}, nil
		}
	}
	return nil, fs.ErrNotExist
}

func (m mockFS) Exists(path string) (bool, error) {
	return m.exists[path], nil
}

func (m mockFS) MkdirAll(path string, perm fs.FileMode) error {
	return nil
}

func (m mockFS) CopyFile(src, dst string) error {
	return nil
}

type mockExif struct {
	timestamps map[string]time.Time
	err        error
}

func (m mockExif) DateTimeOriginal(ctx context.Context, path string) (time.Time, error) {
	if m.err != nil {
		return time.Time{}, m.err
	}
	if ts, ok := m.timestamps[path]; ok {
		return ts, nil
	}
	return time.Time{}, errors.New("missing exif")
}

// trackingExif records which files had EXIF read
type trackingExif struct {
	timestamps map[string]time.Time
	called     map[string]bool
}

func newTrackingExif(timestamps map[string]time.Time) *trackingExif {
	return &trackingExif{
		timestamps: timestamps,
		called:     make(map[string]bool),
	}
}

func (m *trackingExif) DateTimeOriginal(ctx context.Context, path string) (time.Time, error) {
	m.called[path] = true
	if ts, ok := m.timestamps[path]; ok {
		return ts, nil
	}
	return time.Time{}, errors.New("missing exif")
}

type mockDirEntry struct {
	name  string
	isDir bool
}

func (m mockDirEntry) Name() string               { return m.name }
func (m mockDirEntry) IsDir() bool                { return m.isDir }
func (m mockDirEntry) Type() fs.FileMode          { return 0 }
func (m mockDirEntry) Info() (fs.FileInfo, error) { return nil, nil }

type mockFileInfo struct {
	name    string
	modTime time.Time
}

func (m mockFileInfo) Name() string       { return m.name }
func (m mockFileInfo) Size() int64        { return 0 }
func (m mockFileInfo) Mode() fs.FileMode  { return 0 }
func (m mockFileInfo) ModTime() time.Time { return m.modTime }
func (m mockFileInfo) IsDir() bool        { return false }
func (m mockFileInfo) Sys() interface{}   { return nil }

func TestPlannerSkipsJPEGWhenRAWExists(t *testing.T) {
	sourceDir := "/source"
	targetDir := "/target"
	rawPath := filepath.Join(sourceDir, "DSC0001.ARW")
	jpegPath := filepath.Join(sourceDir, "DSC0001.JPG")

	now := time.Date(2024, 10, 2, 15, 1, 0, 0, time.Local)
	mock := mockFS{
		entries: []mockEntry{
			{path: rawPath, modTime: now},
			{path: jpegPath, modTime: now},
		},
		exists: map[string]bool{},
	}

	planner := Planner{
		FS:   mock,
		Exif: mockExif{timestamps: map[string]time.Time{rawPath: now, jpegPath: now}},
	}

	plan, err := planner.Plan(context.Background(), sourceDir, targetDir, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.SkippedJPEGs != 1 {
		t.Fatalf("expected 1 skipped JPEG, got %d", plan.SkippedJPEGs)
	}
	if plan.RawCount != 1 || plan.JpegCount != 0 {
		t.Fatalf("unexpected counts: raw=%d jpeg=%d", plan.RawCount, plan.JpegCount)
	}
}

func TestPlannerDetectsOverrides(t *testing.T) {
	sourceDir := "/source"
	targetDir := "/target"
	rawPath := filepath.Join(sourceDir, "DSC0002.ARW")
	targetPath := filepath.Join(targetDir, "DSC0002.ARW")

	now := time.Date(2024, 10, 2, 15, 2, 0, 0, time.Local)
	mock := mockFS{
		entries: []mockEntry{
			{path: rawPath, modTime: now},
		},
		exists: map[string]bool{
			targetPath: true,
		},
	}

	planner := Planner{
		FS:            mock,
		Exif:          mockExif{timestamps: map[string]time.Time{rawPath: now}},
		AllowOverride: true, // Enable override mode to detect existing files
	}

	plan, err := planner.Plan(context.Background(), sourceDir, targetDir, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.OverrideItems) != 1 {
		t.Fatalf("expected 1 override, got %d", len(plan.OverrideItems))
	}
	if plan.RawOverrides != 1 {
		t.Fatalf("expected 1 raw override, got %d", plan.RawOverrides)
	}
}

func TestPlannerSkipsExistingWhenOverrideFalse(t *testing.T) {
	sourceDir := "/source"
	targetDir := "/target"
	rawPath1 := filepath.Join(sourceDir, "DSC0001.ARW")
	rawPath2 := filepath.Join(sourceDir, "DSC0002.ARW")
	targetPath1 := filepath.Join(targetDir, "DSC0001.ARW")

	now := time.Date(2024, 10, 2, 15, 2, 0, 0, time.Local)
	mock := mockFS{
		entries: []mockEntry{
			{path: rawPath1, modTime: now},
			{path: rawPath2, modTime: now},
		},
		exists: map[string]bool{
			targetPath1: true, // DSC0001.ARW already exists in target
		},
	}

	planner := Planner{
		FS:            mock,
		Exif:          mockExif{timestamps: map[string]time.Time{rawPath1: now, rawPath2: now}},
		AllowOverride: false, // Default: skip existing files
	}

	plan, err := planner.Plan(context.Background(), sourceDir, targetDir, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only DSC0002.ARW should be included (DSC0001.ARW skipped because target exists)
	if len(plan.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(plan.Items))
	}
	if plan.Items[0].FileMeta.Name != "DSC0002.ARW" {
		t.Fatalf("expected DSC0002.ARW, got %s", plan.Items[0].FileMeta.Name)
	}
	// No overrides should be detected when AllowOverride is false
	if len(plan.OverrideItems) != 0 {
		t.Fatalf("expected 0 overrides, got %d", len(plan.OverrideItems))
	}
	// Should track skipped RAW as duplicate
	if plan.SkippedRAWsDupl != 1 {
		t.Fatalf("expected 1 skipped RAW duplicate, got %d", plan.SkippedRAWsDupl)
	}
}

func TestPlannerFiltersByDateRange(t *testing.T) {
	sourceDir := "/source"
	targetDir := "/target"
	rawPath := filepath.Join(sourceDir, "DSC0003.ARW")

	now := time.Date(2024, 10, 2, 15, 2, 0, 0, time.Local)
	mock := mockFS{
		entries: []mockEntry{
			{path: rawPath, modTime: now},
		},
		exists: map[string]bool{},
	}

	start := now.Add(24 * time.Hour)
	end := now.Add(48 * time.Hour)
	planner := Planner{
		FS:   mock,
		Exif: mockExif{timestamps: map[string]time.Time{rawPath: now}},
	}

	plan, err := planner.Plan(context.Background(), sourceDir, targetDir, &start, &end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(plan.Items))
	}
	// Should track skipped RAW as date-filtered
	if plan.SkippedRAWsDate != 1 {
		t.Fatalf("expected 1 skipped RAW date, got %d", plan.SkippedRAWsDate)
	}
}

func TestDeriveRangeFallsBackToItems(t *testing.T) {
	now := time.Now()
	later := now.Add(2 * time.Hour)

	items := []domain.CopyItem{
		{FileMeta: domain.FileMeta{TakenAt: later}},
		{FileMeta: domain.FileMeta{TakenAt: now}},
	}
	start, end := deriveRange(items, nil, nil)
	if start == nil || end == nil {
		t.Fatalf("expected start and end to be set")
	}
	if !start.Equal(now) || !end.Equal(later) {
		t.Fatalf("unexpected range: %v - %v", start, end)
	}
}

func TestPlannerSkipsExifReadWhenModTimeBeforeStartDate(t *testing.T) {
	sourceDir := "/source"
	targetDir := "/target"
	oldPath := filepath.Join(sourceDir, "DSC0001.ARW")  // ModTime before startDate
	newPath := filepath.Join(sourceDir, "DSC0002.ARW")  // ModTime after startDate

	oldTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.Local)
	newTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.Local)
	startDate := time.Date(2024, 6, 1, 0, 0, 0, 0, time.Local)

	mock := mockFS{
		entries: []mockEntry{
			{path: oldPath, modTime: oldTime}, // ModTime is 2024-01-01, before startDate
			{path: newPath, modTime: newTime}, // ModTime is 2024-06-15, after startDate
		},
		exists: map[string]bool{},
	}

	// Use tracking mock to verify which files have EXIF read
	exifMock := newTrackingExif(map[string]time.Time{
		oldPath: oldTime,
		newPath: newTime,
	})

	planner := Planner{
		FS:   mock,
		Exif: exifMock,
	}

	plan, err := planner.Plan(context.Background(), sourceDir, targetDir, &startDate, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only DSC0002.ARW should be included
	if len(plan.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(plan.Items))
	}
	if plan.Items[0].FileMeta.Name != "DSC0002.ARW" {
		t.Fatalf("expected DSC0002.ARW, got %s", plan.Items[0].FileMeta.Name)
	}

	// Verify EXIF was NOT read for the old file (optimization)
	if exifMock.called[oldPath] {
		t.Fatalf("EXIF should NOT have been read for %s (ModTime before startDate)", oldPath)
	}

	// Verify EXIF WAS read for the new file
	if !exifMock.called[newPath] {
		t.Fatalf("EXIF should have been read for %s", newPath)
	}

	// Should track skipped RAW as date-filtered
	if plan.SkippedRAWsDate != 1 {
		t.Fatalf("expected 1 skipped RAW date, got %d", plan.SkippedRAWsDate)
	}
}
