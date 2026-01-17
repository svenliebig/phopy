package domain

import (
	"path/filepath"
	"strings"
	"time"
)

type FileMeta struct {
	SourcePath   string
	RelativePath string
	Name         string
	BaseName     string
	Ext          string
	TakenAt      time.Time
	IsRAW        bool
	IsJPEG       bool
}

func NewFileMeta(sourcePath, relativePath string, takenAt time.Time) FileMeta {
	name := filepath.Base(sourcePath)
	ext := strings.ToLower(filepath.Ext(name))
	base := strings.TrimSuffix(name, filepath.Ext(name))
	isRaw := IsRawExtension(ext)
	isJpeg := IsJpegExtension(ext)

	return FileMeta{
		SourcePath:   sourcePath,
		RelativePath: relativePath,
		Name:         name,
		BaseName:     strings.ToLower(base),
		Ext:          ext,
		TakenAt:      takenAt,
		IsRAW:        isRaw,
		IsJPEG:       isJpeg,
	}
}

func IsRawExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".arw", ".cr2", ".cr3", ".nef", ".raf", ".rw2", ".orf", ".dng":
		return true
	default:
		return false
	}
}

func IsJpegExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return true
	default:
		return false
	}
}
