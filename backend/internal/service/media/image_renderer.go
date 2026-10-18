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
)

const maxImageSourceBytes = 32 << 20
const maxImagePixels int64 = 24_000_000

type imageRenderError struct{ code string }

func (e *imageRenderError) Error() string { return e.code }

func renderSource(ctx context.Context, body io.Reader, checksum string) (image.Image, string, error) {
	raw, err := io.ReadAll(io.LimitReader(body, maxImageSourceBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(raw) > maxImageSourceBytes {
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
	if cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width) > maxImagePixels/int64(cfg.Height) {
		return nil, "", &imageRenderError{"MEDIA_IMAGE_PIXEL_LIMIT"}
	}
	if format != "jpeg" && format != "png" && format != "gif" {
		return nil, "", &imageRenderError{"MEDIA_IMAGE_FORMAT_UNSUPPORTED"}
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", &imageRenderError{"MEDIA_IMAGE_DECODE_FAILED"}
	}
	if format == "jpeg" {
		img = orientImage(img, jpegOrientation(raw))
	}
	return img, format, nil
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
