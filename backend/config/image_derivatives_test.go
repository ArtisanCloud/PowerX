package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestImageDerivativeConfigurationDefaultsOverridesAndValidation(t *testing.T) {
	cfg := GetDefaults()
	if cfg.Storage.ImageDerivatives.MaxPixels != 80_000_000 || cfg.Storage.ImageDerivatives.MaxSourceBytes != 64<<20 || cfg.Storage.ImageDerivatives.MaxWorkingBytes != 512<<20 || cfg.Storage.ImageDerivatives.MaxParallelJobs != 1 {
		t.Fatalf("unexpected image derivative defaults: %+v", cfg.Storage.ImageDerivatives)
	}
	if err := yaml.Unmarshal([]byte("storage:\n  image_derivatives:\n    max_pixels: 60000000\n    max_parallel_jobs: 2\n"), cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.ImageDerivatives.MaxPixels != 60_000_000 || cfg.Storage.ImageDerivatives.MaxParallelJobs != 2 || cfg.Storage.ImageDerivatives.MaxWorkingBytes != 512<<20 {
		t.Fatal("partial YAML overrides did not preserve defaults")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*ImageDerivativeConfig)
	}{
		{"max_pixels", func(o *ImageDerivativeConfig) { o.MaxPixels = -1 }},
		{"max_source_bytes", func(o *ImageDerivativeConfig) { o.MaxSourceBytes = 513 << 20 }},
		{"max_working_bytes", func(o *ImageDerivativeConfig) { o.MaxWorkingBytes = 0 }},
		{"max_parallel_jobs", func(o *ImageDerivativeConfig) { o.MaxParallelJobs = 9 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := GetDefaults()
			tc.mutate(&c.Storage.ImageDerivatives)
			err := c.Validate()
			if err == nil || !strings.Contains(err.Error(), "storage.image_derivatives."+tc.name) {
				t.Fatalf("invalid resource setting was accepted: %v", err)
			}
		})
	}
}
