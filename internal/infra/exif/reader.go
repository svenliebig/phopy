package exif

import (
	"context"
	"errors"
	"os"
	"time"

	goexif "github.com/rwcarlsen/goexif/exif"
)

type Reader struct{}

func (Reader) DateTimeOriginal(ctx context.Context, path string) (time.Time, error) {
	select {
	case <-ctx.Done():
		return time.Time{}, ctx.Err()
	default:
	}

	file, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}
	defer file.Close()

	x, err := goexif.Decode(file)
	if err != nil {
		return time.Time{}, err
	}

	if tag, err := x.Get(goexif.DateTimeOriginal); err == nil {
		if str, err := tag.StringVal(); err == nil {
			parsed, err := time.Parse("2006:01:02 15:04:05", str)
			if err == nil {
				return parsed, nil
			}
		}
	}

	if parsed, err := x.DateTime(); err == nil {
		return parsed, nil
	}

	return time.Time{}, errors.New("exif datetime not found")
}
