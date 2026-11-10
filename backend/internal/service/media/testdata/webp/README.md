# WebP codec fixtures

These original synthetic fixtures contain a 160×80 solid RGB (200,50,100) image.
`alpha.webp` has alpha 96 on its left half and 255 on its right half.

Generated locally with libwebp `cwebp -exact -q 90`; `lossless.webp` additionally
uses `-lossless`. The containers exercise VP8, VP8L and alpha-bearing VP8X.
`animated.webp` combines the lossy and alpha images using `img2webp -d 50`.
The decoder currently rejects animation; tests require a visible decode failure.

No external artwork or user files are included. Tests decode these real bytes;
changing a PNG filename or MIME to WebP is not format-support evidence.
