package filex

import (
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
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

func GetTempFilePath(fileName string) (string, error) {

	filePath := filepath.Join(os.TempDir(), fileName)
	return filePath, nil
}

func GetFileExtension(filename string) string {
	ext := filepath.Ext(filename)
	// 去掉后缀中的点号
	return ext[1:]
}

// CreateTempWithoutRandom creates a new temporary file in the directory dir
// with the specified filename.
// If dir is the empty string, CreateTempWithoutRandom uses the default directory for temporary files, as returned by TempDir.
// The caller can use the file's Name method to find the pathname of the file.
// It is the caller's responsibility to remove the file when it is no longer needed.
func CreateTempWithoutRandom(dir, filename string) (*os.File, error) {
	if dir == "" {
		dir = os.TempDir()
	}

	path := filepath.Join(dir, filename)

	// 先检查文件是否存在，存在则删除
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, &os.PathError{Op: "createtemp", Path: path, Err: err}
	}

	return f, nil
}
