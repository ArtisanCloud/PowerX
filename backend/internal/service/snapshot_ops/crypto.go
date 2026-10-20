package snapshot_ops

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"

	"golang.org/x/crypto/scrypt"
)

const envelopeMagic = "PXSNAP01"
const chunkSize = 1024 * 1024

// encryptStream 每块使用独立序号和 nonce，末尾认证空块防止截断成功。
func encryptStream(dst io.Writer, src io.Reader, password string) error {
	if len(password) < 12 || len(password) > 1024 {
		return failure(400, "SNAPSHOT_PASSPHRASE_INVALID", "恢复包口令需为 12–1024 字节")
	}
	salt := make([]byte, 16)
	prefix := make([]byte, 4)
	if _, e := rand.Read(salt); e != nil {
		return e
	}
	if _, e := rand.Read(prefix); e != nil {
		return e
	}
	key, e := scrypt.Key([]byte(password), salt, 32768, 8, 1, 32)
	if e != nil {
		return e
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return e
	}
	g, e := cipher.NewGCM(block)
	if e != nil {
		return e
	}
	header := append(append([]byte(envelopeMagic), salt...), prefix...)
	if _, e = dst.Write(header); e != nil {
		return e
	}
	buf := make([]byte, chunkSize)
	var seq uint64
	for {
		n, re := io.ReadFull(src, buf)
		if re != nil && re != io.EOF && re != io.ErrUnexpectedEOF {
			return re
		}
		nonce := make([]byte, 12)
		copy(nonce, prefix)
		binary.BigEndian.PutUint64(nonce[4:], seq)
		seq++
		ct := g.Seal(nil, nonce, buf[:n], header)
		if e = binary.Write(dst, binary.BigEndian, uint32(len(ct))); e != nil {
			return e
		}
		if _, e = dst.Write(ct); e != nil {
			return e
		}
		if n == 0 {
			return nil
		}
	}
}
func decryptStream(dst io.Writer, src io.Reader, password string) error {
	if len(password) < 12 || len(password) > 1024 {
		return failure(400, "SNAPSHOT_PASSPHRASE_INVALID", "恢复包口令无效")
	}
	header := make([]byte, len(envelopeMagic)+20)
	if _, e := io.ReadFull(src, header); e != nil {
		return failure(422, "SNAPSHOT_PACKAGE_INVALID", "恢复包不完整")
	}
	if string(header[:len(envelopeMagic)]) != envelopeMagic {
		return failure(422, "SNAPSHOT_PACKAGE_INVALID", "不是受支持的系统恢复包")
	}
	key, e := scrypt.Key([]byte(password), header[8:24], 32768, 8, 1, 32)
	if e != nil {
		return e
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return e
	}
	g, e := cipher.NewGCM(block)
	if e != nil {
		return e
	}
	var seq uint64
	for {
		var size uint32
		if e = binary.Read(src, binary.BigEndian, &size); e != nil {
			return failure(422, "SNAPSHOT_PACKAGE_TRUNCATED", "恢复包被截断")
		}
		if size < 16 || size > chunkSize+16 {
			return failure(422, "SNAPSHOT_PACKAGE_INVALID", "恢复包分块无效")
		}
		ct := make([]byte, size)
		if _, e = io.ReadFull(src, ct); e != nil {
			return failure(422, "SNAPSHOT_PACKAGE_TRUNCATED", "恢复包被截断")
		}
		nonce := make([]byte, 12)
		copy(nonce, header[24:])
		binary.BigEndian.PutUint64(nonce[4:], seq)
		seq++
		plain, e := g.Open(nil, nonce, ct, header)
		if e != nil {
			return failure(422, "SNAPSHOT_DECRYPT_FAILED", "口令错误或恢复包内容损坏")
		}
		if len(plain) == 0 {
			var tail [1]byte
			n, re := src.Read(tail[:])
			if n != 0 || !errors.Is(re, io.EOF) {
				return failure(422, "SNAPSHOT_PACKAGE_INVALID", "恢复包含有额外内容")
			}
			return nil
		}
		if _, e = dst.Write(plain); e != nil {
			return e
		}
	}
}
func encryptConfig(raw, key []byte) ([]byte, error) {
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	g, e := cipher.NewGCM(block)
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, g.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	return g.Seal(nonce, nonce, raw, []byte(Contract)), nil
}
func decryptConfig(raw, key []byte) ([]byte, error) {
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	g, e := cipher.NewGCM(block)
	if e != nil {
		return nil, e
	}
	if len(raw) < g.NonceSize() {
		return nil, errors.New("配置密文不完整")
	}
	return g.Open(nil, raw[:g.NonceSize()], raw[g.NonceSize():], []byte(Contract))
}
