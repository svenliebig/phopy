package presentation

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"phopy/internal/domain"
)

func TestFormatCopyLinesTruncates(t *testing.T) {
	items := make([]domain.CopyItem, 0, 6)
	for i := 0; i < 6; i++ {
		items = append(items, domain.CopyItem{
			FileMeta: domain.FileMeta{
				Name:    fmt.Sprintf("DSC000%d.ARW", i),
				TakenAt: time.Date(2024, 10, 2, 10+i, 0, 0, 0, time.Local),
			},
		})
	}

	lines := formatCopyLines(items)
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}
	if lines[2] != "..." {
		t.Fatalf("expected ellipsis, got %q", lines[2])
	}
}

func TestPrintDryRunOutputIncludesSections(t *testing.T) {
	var buf bytes.Buffer
	printer := Printer{Writer: &buf}

	now := time.Date(2024, 10, 2, 15, 1, 0, 0, time.Local)
	plan := domain.CopyPlan{
		Items: []domain.CopyItem{
			{FileMeta: domain.FileMeta{Name: "DSC0001.ARW", TakenAt: now}},
		},
		OverrideItems: []domain.CopyItem{
			{FileMeta: domain.FileMeta{Name: "DSC0002.ARW", TakenAt: now}},
		},
		RawCount:     1,
		JpegCount:    0,
		SkippedJPEGs: 0,
		RangeStart:   &now,
		RangeEnd:     &now,
	}

	printer.PrintDryRun(plan)
	output := buf.String()
	if !strings.Contains(output, "Copying:") {
		t.Fatalf("expected Copying section")
	}
	if !strings.Contains(output, "Override Required:") {
		t.Fatalf("expected Override Required section")
	}
	if !strings.Contains(output, "Copy DSC0001.ARW") {
		t.Fatalf("expected copy line")
	}
}
