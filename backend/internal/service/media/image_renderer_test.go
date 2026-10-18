package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStandardImageLimitsOrientationAndNoUpscale(t *testing.T) {
	ctx := context.Background()
	img := image.NewRGBA(image.Rect(0, 0, 10, 20))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var jpegBytes bytes.Buffer
	require.NoError(t, jpeg.Encode(&jpegBytes, img, nil))
	// EXIF little-endian IFD0: Orientation=6，手机竖图显示应旋转 90 度。
	exif := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	app := []byte{0xff, 0xe1, 0, byte(len(exif) + 2)}
	app = append(app, exif...)
	raw := append([]byte{}, jpegBytes.Bytes()[:2]...)
	raw = append(raw, app...)
	raw = append(raw, jpegBytes.Bytes()[2:]...)
	decoded, format, err := renderSource(ctx, bytes.NewReader(raw), payloadHash(raw))
	require.NoError(t, err)
	require.Equal(t, "jpeg", format)
	require.Equal(t, image.Rect(0, 0, 20, 10), decoded.Bounds())
	encoded, mime, w, h, err := encodeDerivative(ctx, decoded, format, 320)
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", mime)
	require.Equal(t, 20, w)
	require.Equal(t, 10, h)
	require.NotEmpty(t, encoded)
	// 只用 PNG IHDR 即可拒绝超像素图片，不进入完整解码/大块内存分配。
	header := make([]byte, 13)
	binary.BigEndian.PutUint32(header[:4], 100_000)
	binary.BigEndian.PutUint32(header[4:8], 100_000)
	header[8] = 8
	header[9] = 6
	chunk := append([]byte("IHDR"), header...)
	blob := []byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13}
	blob = append(blob, chunk...)
	crc := make([]byte, 4)
	binary.BigEndian.PutUint32(crc, crc32.ChecksumIEEE(chunk))
	blob = append(blob, crc...)
	_, _, err = renderSource(ctx, bytes.NewReader(blob), "")
	require.Equal(t, "MEDIA_IMAGE_PIXEL_LIMIT", derivativeErrorCode(err))
	for n := 0; n < len(raw); n++ {
		_ = jpegOrientation(raw[:n])
	} // 截断 EXIF 不得 panic。
}
