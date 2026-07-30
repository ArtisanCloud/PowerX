package media

import "testing"

func TestCreateAssetMetadataAcceptsTopLevelContentSHA256(t *testing.T) {
	hash := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	metadata, err := createAssetMetadata(CreateAssetRequest{
		ContentSHA256: hash,
		Metadata:      map[string]any{"source": "crm"},
	})
	if err != nil {
		t.Fatalf("createAssetMetadata returned error: %v", err)
	}
	if got := metadata["content_sha256"]; got != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("content_sha256 = %v", got)
	}
	if got := metadata["source"]; got != "crm" {
		t.Fatalf("source metadata = %v", got)
	}
}

func TestCreateAssetMetadataRejectsConflictingContentSHA256(t *testing.T) {
	_, err := createAssetMetadata(CreateAssetRequest{
		ContentSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Metadata:      map[string]any{"content_sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
	})
	if err == nil {
		t.Fatal("expected conflict error")
	}
}
