package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"

	"golang.org/x/image/riff"
	"golang.org/x/image/vp8"
	"golang.org/x/image/vp8l"
	_ "golang.org/x/image/webp"
)

const maxImageSourceBytes = 64 << 20
const maxImagePixels int64 = 80_000_000

type imageRenderError struct{ code string }

func (e *imageRenderError) Error() string { return e.code }

type imageContextReader struct {
	context.Context
	io.Reader
}

func (r imageContextReader) Read(p []byte) (int, error) {
	if err := r.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}

func renderSource(ctx context.Context, body io.Reader, checksum string) (image.Image, string, error) {
	return renderSourceWithOptions(ctx, body, checksum, defaultImageDerivativeOptions())
}

func renderSourceWithOptions(ctx context.Context, body io.Reader, checksum string, options ImageDerivativeOptions) (image.Image, string, error) {
	if err := options.validate(); err != nil {
		return nil, "", err
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	raw, err := io.ReadAll(io.LimitReader(imageContextReader{ctx, body}, options.MaxSourceBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(raw)) > options.MaxSourceBytes {
		return nil, "", &imageRenderError{"MEDIA_IMAGE_BYTE_LIMIT"}
	}
	sum := sha256.Sum256(raw)
	if checksum != "" && hex.EncodeToString(sum[:]) != checksum {
		return nil, "", &imageRenderError{"MEDIA_IMAGE_SOURCE_CHECKSUM_MISMATCH"}
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, "", &imageRenderError{"MEDIA_IMAGE_DECODE_FAILED"}
	}
	if err := validateImageDimensions(cfg, int64(len(raw)), options); err != nil {
		return nil, "", err
	}
	if format != "jpeg" && format != "png" && format != "gif" && format != "webp" {
		return nil, "", &imageRenderError{"MEDIA_IMAGE_FORMAT_UNSUPPORTED"}
	}
	if format == "webp" {
		if err := validateWebPFrame(raw, cfg, options); err != nil {
			return nil, "", err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	img, _, err := image.Decode(imageContextReader{ctx, bytes.NewReader(raw)})
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		return nil, "", &imageRenderError{"MEDIA_IMAGE_DECODE_FAILED"}
	}
	if format == "jpeg" {
		img = orientImage(img, jpegOrientation(raw))
	}
	return img, format, nil
}

// VP8X canvas dimensions can differ from the embedded VP8/VP8L frame header.
// Check both before allocating pixels; DecodeConfig alone reads only the canvas.
func validateWebPFrame(raw []byte, canvas image.Config, options ImageDerivativeOptions) error {
	fail := func() error { return &imageRenderError{"MEDIA_IMAGE_DECODE_FAILED"} }
	_, chunks, err := riff.NewReader(bytes.NewReader(raw))
	if err != nil {
		return fail()
	}
	for {
		kind, n, body, err := chunks.Next()
		if err != nil {
			return fail()
		}
		var width, height int
		switch string(kind[:]) {
		case "VP8X":
			var header [10]byte
			if n != 10 {
				return fail()
			}
			if _, err := io.ReadFull(body, header[:]); err != nil || header[0]&2 != 0 {
				return fail()
			}
			continue
		case "ANIM", "ANMF":
			return fail()
		case "VP8 ":
			if int64(n) > options.MaxSourceBytes {
				return fail()
			}
			decoder := vp8.NewDecoder()
			decoder.Init(body, int(n))
			frame, err := decoder.DecodeFrameHeader()
			if err != nil {
				return fail()
			}
			width, height = frame.Width, frame.Height
		case "VP8L":
			frame, err := vp8l.DecodeConfig(body)
			if err != nil {
				return fail()
			}
			width, height = frame.Width, frame.Height
		default:
			continue
		}
		if err := validateImageDimensions(image.Config{Width: width, Height: height}, int64(len(raw)), options); err != nil {
			return err
		}
		if width != canvas.Width || height != canvas.Height {
			return fail()
		}
		return nil
	}
}

// 仅提取 JPEG EXIF 的 Orientation；原始 EXIF 和原图字节始终保留。
func jpegOrientation(raw []byte) int {
	if len(raw) < 4 || raw[0] != 0xff || raw[1] != 0xd8 {
		return 1
	}
	for pos := 2; pos+4 <= len(raw); {
		if raw[pos] != 0xff {
			return 1
		}
		marker := raw[pos+1]
		if marker == 0xda || marker == 0xd9 {
			return 1
		}
		if marker == 0xff {
			pos++
			continue
		}
		if marker == 0x01 || (marker >= 0xd0 && marker <= 0xd7) {
			pos += 2
			continue
		}
		n := int(binary.BigEndian.Uint16(raw[pos+2 : pos+4]))
		if n < 2 || n > len(raw)-pos-2 {
			return 1
		}
		segment := raw[pos+4 : pos+2+n]
		pos += n + 2
		if marker != 0xe1 || !bytes.HasPrefix(segment, []byte("Exif\x00\x00")) {
			continue
		}
		tiff := segment[6:]
		if len(tiff) < 8 {
			return 1
		}
		var order binary.ByteOrder
		switch string(tiff[:2]) {
		case "II":
			order = binary.LittleEndian
		case "MM":
			order = binary.BigEndian
		default:
			return 1
		}
		if order.Uint16(tiff[2:4]) != 42 {
			return 1
		}
		off := uint64(order.Uint32(tiff[4:8]))
		if off+2 > uint64(len(tiff)) {
			return 1
		}
		count := uint64(order.Uint16(tiff[off : off+2]))
		off += 2
		for idx := uint64(0); idx < count && off+12 <= uint64(len(tiff)); idx++ {
			entry := tiff[off : off+12]
			off += 12
			if order.Uint16(entry[:2]) == 0x0112 && order.Uint16(entry[2:4]) == 3 && order.Uint32(entry[4:8]) == 1 {
				v := int(order.Uint16(entry[8:10]))
				if v >= 1 && v <= 8 {
					return v
				}
				return 1
			}
		}
	}
	return 1
}

type orientedImage struct {
	image.Image
	orientation int
}

func orientImage(src image.Image, orientation int) image.Image {
	if orientation < 2 || orientation > 8 {
		return src
	}
	return orientedImage{src, orientation}
}
func (i orientedImage) Bounds() image.Rectangle {
	b := i.Image.Bounds()
	if i.orientation >= 5 {
		return image.Rect(0, 0, b.Dy(), b.Dx())
	}
	return image.Rect(0, 0, b.Dx(), b.Dy())
}
func (i orientedImage) At(x, y int) color.Color {
	b := i.Image.Bounds()
	w, h := b.Dx(), b.Dy()
	sx, sy := x, y
	switch i.orientation {
	case 2:
		sx = w - 1 - x
	case 3:
		sx, sy = w-1-x, h-1-y
	case 4:
		sy = h - 1 - y
	case 5:
		sx, sy = y, x
	case 6:
		sx, sy = y, h-1-x
	case 7:
		sx, sy = w-1-y, h-1-x
	case 8:
		sx, sy = w-1-y, x
	}
	return i.Image.At(b.Min.X+sx, b.Min.Y+sy)
}

// resizeImage 使用面积平均进行缩小；按预乘 alpha 累积，透明边缘不会产生黑色杂边。
func resizeImage(ctx context.Context, src image.Image, edge int) (*image.RGBA, error) {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if w > edge || h > edge {
		if w >= h {
			dw = edge
			dh = max(1, h*edge/w)
		} else {
			dh = edge
			dw = max(1, w*edge/h)
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		y0, y1 := y*h/dh, max(y*h/dh+1, (y+1)*h/dh)
		for x := 0; x < dw; x++ {
			x0, x1 := x*w/dw, max(x*w/dw+1, (x+1)*w/dw)
			var r, g, bb, a, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					rr, gg, bv, aa := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
					r += uint64(rr)
					g += uint64(gg)
					bb += uint64(bv)
					a += uint64(aa)
					n++
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(bb / n >> 8), uint8(a / n >> 8)})
		}
	}
	return dst, nil
}

func encodeDerivative(ctx context.Context, src image.Image, format string, edge int) ([]byte, string, int, int, error) {
	img, err := resizeImage(ctx, src, edge)
	if err != nil {
		return nil, "", 0, 0, err
	}
	var buf bytes.Buffer
	mime := "image/png"
	if format == "jpeg" {
		mime = "image/jpeg"
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&buf, img)
	}
	if err == nil {
		err = ctx.Err()
	}
	return buf.Bytes(), mime, img.Bounds().Dx(), img.Bounds().Dy(), err
}

func derivativeErrorCode(err error) string {
	var render *imageRenderError
	if errors.As(err, &render) {
		return render.code
	}
	return "MEDIA_DERIVATIVE_GENERATION_FAILED"
}
