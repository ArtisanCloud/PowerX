package plugin

import (
	"mime/multipart"
	"testing"
)

func TestUploadedDirRelPathsPrefersLegacyFilePaths(t *testing.T) {
	form := &multipart.Form{
		Value: map[string][]string{
			"file_paths":      {"legacy/plugin.yaml"},
			"file_paths_json": {`["json/plugin.yaml"]`},
		},
	}

	got := uploadedDirRelPaths(form)
	if len(got) != 1 || got[0] != "legacy/plugin.yaml" {
		t.Fatalf("uploadedDirRelPaths() = %#v, want legacy file_paths", got)
	}
}

func TestUploadedDirRelPathsReadsJSONList(t *testing.T) {
	form := &multipart.Form{
		Value: map[string][]string{
			"file_paths_json": {`["mac/plugin.yaml","mac/web-admin/app.vue"]`},
		},
	}

	got := uploadedDirRelPaths(form)
	if len(got) != 2 || got[0] != "mac/plugin.yaml" || got[1] != "mac/web-admin/app.vue" {
		t.Fatalf("uploadedDirRelPaths() = %#v, want JSON file paths", got)
	}
}

func TestUploadedDirRelPathsInvalidJSONReturnsNil(t *testing.T) {
	form := &multipart.Form{
		Value: map[string][]string{
			"file_paths_json": {"not-json"},
		},
	}

	if got := uploadedDirRelPaths(form); got != nil {
		t.Fatalf("uploadedDirRelPaths() = %#v, want nil", got)
	}
}
