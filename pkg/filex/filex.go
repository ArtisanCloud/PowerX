package filex

import (
	"io"
	"mime/multipart"
	"os"
	"strings"
)

func GetMediaType(contentType string) string {

	switch {
	case strings.HasPrefix(contentType, "image/"):
		return "image"
	case strings.HasPrefix(contentType, "video/"):
		return "video"
	case strings.HasPrefix(contentType, "audio/"):
		return "audio"
	default:
		return "other"
	}
}

func SaveFileToLocal(fileHeader *multipart.FileHeader, uploadPath string) error {
	file, err := fileHeader.Open()
	if err != nil {
		return err
	}
	defer file.Close()

	dst, err := os.Create(uploadPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, file)
	if err != nil {
		return err
	}

	return nil
}
