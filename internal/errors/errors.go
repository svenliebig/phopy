package errors

import "fmt"

type Kind string

const (
	InvalidConfig Kind = "invalid_config"
	NotFound      Kind = "not_found"
	ExifFailure   Kind = "exif_failure"
	IOFailure     Kind = "io_failure"
	Internal      Kind = "internal"
)

type AppError struct {
	Kind Kind
	Op   string
	Path string
	Err  error
}

func (e *AppError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s: %s: %v", e.Op, e.Path, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func Wrap(kind Kind, op, path string, err error) error {
	if err == nil {
		return nil
	}
	return &AppError{
		Kind: kind,
		Op:   op,
		Path: path,
		Err:  err,
	}
}

func UserMessage(err error) string {
	appErr, ok := err.(*AppError)
	if !ok {
		return err.Error()
	}
	switch appErr.Kind {
	case InvalidConfig:
		return fmt.Sprintf("Invalid configuration: %v", appErr.Err)
	case NotFound:
		return fmt.Sprintf("Path not found: %s", appErr.Path)
	case ExifFailure:
		return fmt.Sprintf("EXIF read failed: %s", appErr.Path)
	case IOFailure:
		return fmt.Sprintf("I/O error: %s", appErr.Path)
	default:
		return fmt.Sprintf("Unexpected error: %v", appErr.Err)
	}
}
