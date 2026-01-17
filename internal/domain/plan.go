package domain

import "time"

type CopyItem struct {
	FileMeta   FileMeta
	TargetPath string
}

type CopyPlan struct {
	Items         []CopyItem
	OverrideItems []CopyItem
	SkippedJPEGs  int
	RangeStart    *time.Time
	RangeEnd      *time.Time
	RawCount      int
	JpegCount     int
	RawOverrides  int
	JpegOverrides int
	Warnings      []string
}
