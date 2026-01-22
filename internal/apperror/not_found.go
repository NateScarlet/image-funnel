package apperror

import (
	"errors"
	"fmt"
	"io/fs"
	"syscall"

	"main/internal/scalar"
)

func NewErrDocumentNotFound(id scalar.ID) error {
	return &AppError{
		Code:    "NOT_FOUND",
		Message: fmt.Sprintf("document %q not found", id),
		Locales: Locales{
			Zh: fmt.Sprintf("未找到文档 %q", id),
		},
	}
}

func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if ErrCode(err) == "NOT_FOUND" {
		return true
	}
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	if errors.Is(err, syscall.ENOTDIR) {
		return true
	}

	return false
}

func IgnoreNotFound[T any](v T, err error) (T, error) {
	if IsNotFound(err) {
		return v, nil
	}
	return v, err
}
