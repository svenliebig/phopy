package app

import (
	"context"
	"io/fs"
	"time"
)

type FileSystem interface {
	WalkDir(root string, fn fs.WalkDirFunc) error
	Stat(path string) (fs.FileInfo, error)
	Exists(path string) (bool, error)
	MkdirAll(path string, perm fs.FileMode) error
	CopyFile(src, dst string) error
}

type ExifReader interface {
	DateTimeOriginal(ctx context.Context, path string) (time.Time, error)
}
