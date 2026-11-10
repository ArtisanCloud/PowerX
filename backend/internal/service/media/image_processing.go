package media

import (
	"context"
	"image"
)

const defaultImageWorkingBytes int64 = 512 << 20

func defaultImageDerivativeOptions() ImageDerivativeOptions {
	return ImageDerivativeOptions{MaxSourceBytes: maxImageSourceBytes, MaxPixels: maxImagePixels, MaxWorkingBytes: defaultImageWorkingBytes, MaxParallelJobs: 1}
}

func normalizeImageDerivativeOptions(o ImageDerivativeOptions) ImageDerivativeOptions {
	d := defaultImageDerivativeOptions()
	if o.MaxSourceBytes == 0 {
		o.MaxSourceBytes = d.MaxSourceBytes
	}
	if o.MaxPixels == 0 {
		o.MaxPixels = d.MaxPixels
	}
	if o.MaxWorkingBytes == 0 {
		o.MaxWorkingBytes = d.MaxWorkingBytes
	}
	if o.MaxParallelJobs == 0 {
		o.MaxParallelJobs = d.MaxParallelJobs
	}
	return o
}

func (o ImageDerivativeOptions) validate() error {
	if o.MaxSourceBytes < 1 || o.MaxSourceBytes > 512<<20 || o.MaxPixels < 1 || o.MaxPixels > 200_000_000 || o.MaxWorkingBytes < 1 || o.MaxWorkingBytes > 4<<30 || o.MaxParallelJobs < 1 || o.MaxParallelJobs > 8 {
		return &imageRenderError{"MEDIA_IMAGE_CONFIGURATION_INVALID"}
	}
	return nil
}

type imageProcessingPolicy struct {
	options ImageDerivativeOptions
	slots   chan struct{}
}

func newImageProcessingPolicy(o ImageDerivativeOptions) *imageProcessingPolicy {
	o = normalizeImageDerivativeOptions(o)
	n := o.MaxParallelJobs
	if n < 1 || n > 8 {
		n = 1
	}
	return &imageProcessingPolicy{options: o, slots: make(chan struct{}, n)}
}

// ConfigureImageDerivatives 在启动 worker 前注入配置，不用于运行中热切换。
func (s *MediaService) ConfigureImageDerivatives(o ImageDerivativeOptions) {
	s.imageProcessing = newImageProcessingPolicy(o)
}

var fallbackImageProcessing = newImageProcessingPolicy(ImageDerivativeOptions{})

func (s *MediaService) imageProcessingPolicy() *imageProcessingPolicy {
	if s.imageProcessing != nil {
		return s.imageProcessing
	}
	return fallbackImageProcessing
}

func (p *imageProcessingPolicy) acquire(ctx context.Context) (func(), error) {
	if err := p.options.validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case p.slots <- struct{}{}:
		return func() { <-p.slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func validateImageDimensions(cfg image.Config, sourceBytes int64, o ImageDerivativeOptions) error {
	if cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width) > o.MaxPixels/int64(cfg.Height) {
		return &imageRenderError{"MEDIA_IMAGE_PIXEL_LIMIT"}
	}
	// 覆盖 16-bit PNG 的 8 bytes/pixel、扫描行缓冲、压缩输入缓冲扩容、两份最大预览
	// 与解码器余量。在完整解码之前拒绝；这是估算预算，不是操作系统 RSS 硬上限。
	working := int64(cfg.Width)*int64(cfg.Height)*8 + int64(cfg.Width)*24 + sourceBytes*2 + 2*1600*1600*4 + (16 << 20)
	if working > o.MaxWorkingBytes {
		return &imageRenderError{"MEDIA_IMAGE_MEMORY_LIMIT"}
	}
	return nil
}
