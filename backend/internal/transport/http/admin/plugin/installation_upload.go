package plugin

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	install "github.com/ArtisanCloud/PowerX/internal/service/plugin_install"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

func uploadInvalid() error {
	return dto.NewErrorWithCode(400, "PLUGIN_INSTALL_SOURCE_INVALID", "Invalid plugin package", nil)
}
func uploadPath(root, name string) (string, error) {
	if strings.Contains(name, "\\") || filepath.IsAbs(name) {
		return "", uploadInvalid()
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", uploadInvalid()
	}
	return filepath.Join(root, clean), nil
}
func prepareInstallationUpload(ctx context.Context, form *multipart.Form) (string, func(), error) {
	root, e := os.MkdirTemp("", "px-plugin-task-upload-*")
	if e != nil {
		return "", nil, e
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	var bytes int64
	var paths []string
	count := 0
	write := func(name string, size int64, reader io.Reader, executable bool) error {
		count++
		if count > 10000 || size < 0 || size > install.MaxSourceBytes-bytes {
			return uploadInvalid()
		}
		path, e := uploadPath(root, name)
		if e != nil {
			return e
		}
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return e
		}
		mode := os.FileMode(0600)
		if executable || isPluginExecutablePath(root, path) {
			mode = 0700
		}
		out, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if e != nil {
			return uploadInvalid()
		}
		n, err := io.Copy(budget.Writer(ctx, out), io.LimitReader(reader, size+1))
		closeErr := out.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if n != size {
			return uploadInvalid()
		}
		bytes += n
		paths = append(paths, name)
		return nil
	}
	if form == nil {
		return "", cleanup, uploadInvalid()
	}
	if files := form.File["file"]; len(files) == 1 && len(form.File["files"]) == 0 {
		if !isSupportedPackageFile(files[0].Filename) {
			return "", cleanup, uploadInvalid()
		}
		in, e := files[0].Open()
		if e != nil {
			return "", cleanup, e
		}
		defer in.Close()
		gz, e := gzip.NewReader(in)
		if e != nil {
			return "", cleanup, uploadInvalid()
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		entries := 0
		for {
			hdr, e := tr.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return "", cleanup, uploadInvalid()
			}
			entries++
			if entries > 10000 {
				return "", cleanup, uploadInvalid()
			}
			if hdr.Typeflag == tar.TypeDir && filepath.Clean(hdr.Name) == "." {
				continue
			}
			path, e := uploadPath(root, hdr.Name)
			if e != nil {
				return "", cleanup, e
			}
			switch hdr.Typeflag {
			case tar.TypeDir:
				if e = os.MkdirAll(path, 0700); e != nil {
					return "", cleanup, e
				}
			case tar.TypeReg, tar.TypeRegA:
				if e = write(hdr.Name, hdr.Size, tr, hdr.Mode&0111 != 0); e != nil {
					return "", cleanup, e
				}
			default:
				return "", cleanup, uploadInvalid()
			}
		}
	} else {
		files := form.File["files"]
		if len(files) == 0 || len(files) > 10000 {
			return "", cleanup, uploadInvalid()
		}
		hints := uploadedDirRelPaths(form)
		if len(hints) != 0 && len(hints) != len(files) {
			return "", cleanup, uploadInvalid()
		}
		for i, header := range files {
			name := header.Filename
			if len(hints) > 0 {
				name = hints[i]
			}
			in, e := header.Open()
			if e != nil {
				return "", cleanup, e
			}
			e = write(name, header.Size, in, false)
			_ = in.Close()
			if e != nil {
				return "", cleanup, e
			}
		}
	}
	source := root
	if !hasManifest(source) {
		source = detectUploadedRoot(root, paths)
	}
	if !hasManifest(source) {
		return "", cleanup, uploadInvalid()
	}
	if e = ensureBackendBinsExecutable(source); e != nil {
		return "", cleanup, e
	}
	return source, cleanup, nil
}
