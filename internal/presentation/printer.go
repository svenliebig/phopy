package presentation

import (
	"fmt"
	"io"
	"strings"
	"time"

	"phopy/internal/domain"
)

type Printer struct {
	Writer  io.Writer
	Verbose bool
}

func (p Printer) PrintDryRun(plan domain.CopyPlan) {
	fmt.Fprintln(p.Writer, "Copying:")
	fmt.Fprintln(p.Writer)

	for _, line := range formatCopyLines(plan.Items) {
		fmt.Fprintln(p.Writer, line)
	}

	fmt.Fprintln(p.Writer)
	fmt.Fprintln(p.Writer, "Override Required:")
	for _, item := range plan.OverrideItems {
		fmt.Fprintln(p.Writer, item.FileMeta.Name)
	}

	fmt.Fprintln(p.Writer)
	p.printSummary(plan, true, 0)

	if p.Verbose && len(plan.Warnings) > 0 {
		fmt.Fprintln(p.Writer)
		fmt.Fprintln(p.Writer, "Warnings:")
		for _, warning := range plan.Warnings {
			fmt.Fprintln(p.Writer, "- "+warning)
		}
	}
}

func (p Printer) PrintExecution(plan domain.CopyPlan, overridesConfirmed int) {
	fmt.Fprintln(p.Writer, "Copying:")
	fmt.Fprintln(p.Writer)

	for _, line := range formatCopyLines(plan.Items) {
		fmt.Fprintln(p.Writer, line)
	}

	if len(plan.OverrideItems) > 0 {
		fmt.Fprintln(p.Writer)
		fmt.Fprintln(p.Writer, "Override Required:")
		for _, item := range plan.OverrideItems {
			fmt.Fprintln(p.Writer, item.FileMeta.Name)
		}
	}

	fmt.Fprintln(p.Writer)
	p.printSummary(plan, false, overridesConfirmed)
}

func (p Printer) printSummary(plan domain.CopyPlan, dryRun bool, overridesConfirmed int) {
	rangeStart := formatDate(plan.RangeStart)
	rangeEnd := formatDate(plan.RangeEnd)

	if rangeStart == "" || rangeEnd == "" {
		fmt.Fprintf(p.Writer, "Copied %d RAW and %d JPEG files.\n", plan.RawCount, plan.JpegCount)
	} else {
		fmt.Fprintf(p.Writer, "Copied %d RAW and %d JPEG files from %s until %s.\n", plan.RawCount, plan.JpegCount, rangeStart, rangeEnd)
	}

	fmt.Fprintf(p.Writer, "Skipped %d JPEGs because their RAW files existed.\n", plan.SkippedJPEGs)

	overrideCount := plan.RawOverrides + plan.JpegOverrides
	if dryRun {
		if overrideCount > 0 {
			fmt.Fprintln(p.Writer, dryRunOverrideLine(plan))
		} else {
			fmt.Fprintln(p.Writer, "No override confirmation would be required.")
		}
		return
	}

	if overrideCount == 0 {
		fmt.Fprintln(p.Writer, "No override confirmation was required.")
		return
	}
	if overridesConfirmed == 0 {
		fmt.Fprintln(p.Writer, runtimeOverrideLine(plan, false))
	} else {
		fmt.Fprintln(p.Writer, runtimeOverrideLine(plan, true))
	}
}

func formatCopyLines(items []domain.CopyItem) []string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		date := item.FileMeta.TakenAt.Format("2006-01-02 15:04")
		lines = append(lines, fmt.Sprintf("Copy %s  %s", item.FileMeta.Name, date))
	}

	if len(lines) <= 4 {
		return lines
	}
	head := lines[:2]
	tail := lines[len(lines)-2:]
	return append(append(head, "..."), tail...)
}

func formatDate(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("2006-01-02")
}

func JoinLines(lines []string) string {
	return strings.Join(lines, "\n")
}

func dryRunOverrideLine(plan domain.CopyPlan) string {
	if plan.RawOverrides > 0 && plan.JpegOverrides == 0 {
		return fmt.Sprintf("Would ask for override confirmation for %d RAW files when not in dry run.", plan.RawOverrides)
	}
	if plan.RawOverrides == 0 && plan.JpegOverrides > 0 {
		return fmt.Sprintf("Would ask for override confirmation for %d JPEG files when not in dry run.", plan.JpegOverrides)
	}
	return fmt.Sprintf("Would ask for override confirmation for %d RAW and %d JPEG files when not in dry run.", plan.RawOverrides, plan.JpegOverrides)
}

func runtimeOverrideLine(plan domain.CopyPlan, confirmed bool) string {
	verb := "declined"
	if confirmed {
		verb = "granted"
	}
	if plan.RawOverrides > 0 && plan.JpegOverrides == 0 {
		return fmt.Sprintf("Override confirmation %s for %d RAW files.", verb, plan.RawOverrides)
	}
	if plan.RawOverrides == 0 && plan.JpegOverrides > 0 {
		return fmt.Sprintf("Override confirmation %s for %d JPEG files.", verb, plan.JpegOverrides)
	}
	return fmt.Sprintf("Override confirmation %s for %d RAW and %d JPEG files.", verb, plan.RawOverrides, plan.JpegOverrides)
}
