package knowledge_space

import "path/filepath"

// OCRArtifacts captures local (filesystem) artifacts produced by an OCR processor.
// It is meant to be consumed by ArtifactStore to write into MinIO/S3 (or local artifact staging)
// and should not be persisted directly into DB snapshots.
type OCRArtifacts struct {
	RawFormat string // tsv|hocr
	Pages     []OCRArtifactPage
}

type OCRArtifactPage struct {
	PageNumber int

	// Local paths on the worker filesystem.
	ImagePath string
	RawPath   string

	// Image metadata (used for bbox normalization validation / UI scaling).
	Width  int
	Height int
}

func (p OCRArtifactPage) ImageExt() string { return filepath.Ext(p.ImagePath) }
func (p OCRArtifactPage) RawExt() string   { return filepath.Ext(p.RawPath) }

