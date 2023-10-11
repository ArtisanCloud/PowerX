package pluginx

import (
	"archive/zip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DownloadFile 下载url到指定目录的指定文件名
func DownloadFile(url string, dir string, filename string) error {
	// 创建HTTP请求
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 确保目录存在
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	// 创建文件
	filePath := filepath.Join(dir, filename)
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// 将响应内容写入文件
	_, err = io.Copy(file, resp.Body)
	return err
}

// UnzipFile 解压zip文件到指定目录
func UnzipFile(zipPath string, dest string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		// 构建目标文件路径
		destPath := filepath.Join(dest, file.Name)

		// 如果是目录，则创建目录
		if file.FileInfo().IsDir() {
			err = os.MkdirAll(destPath, file.Mode())
			if err != nil {
				return err
			}
		} else {
			err = os.MkdirAll(filepath.Dir(destPath), 0755)
			if err != nil {
				return err
			}

			srcFile, err := file.Open()
			if err != nil {
				return err
			}
			defer srcFile.Close()

			destFile, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer destFile.Close()

			_, err = io.Copy(destFile, srcFile)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// CopyAndRenameFile 复制并重命名文件
func CopyAndRenameFile(src string, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	return err
}

// CopyAndRenameDir 复制并重命名目录
func CopyAndRenameDir(src string, dest string) error {
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dest, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		return CopyAndRenameFile(path, destPath)
	})

	return err
}

func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	err = os.Chmod(dst, srcInfo.Mode())
	if err != nil {
		return err
	}

	return nil
}

func CopyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !srcInfo.IsDir() {
		return fmt.Errorf("源路径不是一个目录: %s", src)
	}

	err = os.MkdirAll(dst, srcInfo.Mode())
	if err != nil {
		return err
	}

	entries, err := ioutil.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			err = CopyDir(srcPath, dstPath)
			if err != nil {
				return err
			}
		} else {
			err = CopyFile(srcPath, dstPath)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// 复制目录下所有内容到目标目录

// ReplaceFileString 替换文件中的字符串为指定字符串, 次数为-1时替换所有
func ReplaceFileString(filePath string, old string, new string, n int) error {
	// 读取文件内容
	srcFile, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	content, err := io.ReadAll(srcFile)
	if err != nil {
		return err
	}

	// 替换字符串
	replacedContent := strings.Replace(string(content), old, new, n)

	// 将替换后的内容写回文件
	err = os.WriteFile(filePath, []byte(replacedContent), 0644)
	return err
}

// StringToHyphenCase 将字符串转换为连缀符格式
func StringToHyphenCase(s string) string {
	var result string
	for i, r := range s {
		if i == 0 {
			result += strings.ToLower(string(r))
		} else {
			if r >= 'A' && r <= 'Z' {
				result += "-" + strings.ToLower(string(r))
			} else {
				result += string(r)
			}
		}
	}
	return result
}

func StringToHyphen(s string) string {
	var result string
	for i, r := range s {
		if i == 0 {
			result += strings.ToLower(string(r))
		} else {
			if r >= 'A' && r <= 'Z' {
				result += "-" + strings.ToLower(string(r))
			} else {
				result += string(r)
			}
		}
	}
	return result
}
