package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// MediaFetcher 仅接收供应商输出，默认网络传输逐次解析并阻止内部地址与重定向绕过。
type MediaFetcher interface {
	Do(*http.Request) (*http.Response, error)
}

func publicMediaURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" || u.User != nil || u.Scheme != "https" && u.Scheme != "http" {
		return MediaError(502, "AI_MEDIA_URL_INVALID")
	}
	return nil
}
func newMediaFetcher() MediaFetcher {
	transport := &http.Transport{Proxy: nil, ResponseHeaderTimeout: 30 * time.Second, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		addresses, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil {
			return nil, e
		}
		if len(addresses) == 0 {
			return nil, fmt.Errorf("media address missing")
		}
		for _, a := range addresses {
			if !a.IP.IsGlobalUnicast() || a.IP.IsPrivate() || a.IP.IsLoopback() || a.IP.IsLinkLocalUnicast() {
				return nil, MediaError(502, "AI_MEDIA_URL_INVALID")
			}
		}
		return (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
	}}
	return &http.Client{Transport: transport, Timeout: 5 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return MediaError(502, "AI_MEDIA_REDIRECT_LIMIT")
		}
		return publicMediaURL(req.URL.String())
	}}
}

type mediaProviderOutput struct {
	TraceID string   `json:"trace_id,omitempty"`
	Status  int      `json:"upstream_status,omitempty"`
	Kind    string   `json:"kind"`
	URLs    []string `json:"urls,omitempty"`
	Data    [][]byte `json:"data,omitempty"`
}

func (s *MediaRuntime) materialize(ctx context.Context, tenant, task string, output mediaProviderOutput) ([]MediaAssetResult, error) {
	if s.media == nil {
		return nil, MediaError(503, "AI_MEDIA_UNAVAILABLE")
	}
	results := []MediaAssetResult{}
	for i := 0; i < len(output.Data)+len(output.URLs); i++ {
		var body io.ReadCloser
		if i < len(output.Data) {
			body = io.NopCloser(bytes.NewReader(output.Data[i]))
		} else {
			raw := output.URLs[i-len(output.Data)]
			if e := publicMediaURL(raw); e != nil {
				return nil, e
			}
			req, e := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
			if e != nil {
				return nil, e
			}
			resp, e := s.fetch.Do(req)
			if e != nil {
				var business *dto.AppError
				if errors.As(e, &business) {
					return nil, business
				}
				return nil, MediaError(502, "AI_MEDIA_DOWNLOAD_FAILED")
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return nil, MediaError(502, "AI_MEDIA_DOWNLOAD_FAILED")
			}
			body = resp.Body
		}
		result, err := s.storeOutput(ctx, tenant, task, i, output.Kind, body)
		body.Close()
		if err != nil {
			return nil, err
		}
		results = append(results, *result)
	}
	if len(results) == 0 {
		return nil, MediaError(502, "AI_OUTPUT_INVALID")
	}
	return results, nil
}
func (s *MediaRuntime) storeOutput(ctx context.Context, tenant, task string, index int, kind string, body io.Reader) (*MediaAssetResult, error) {
	directory := os.TempDir()
	// 先为实际临时磁盘预留流式写入窗口；同盘正式转存继续共用租约并按需扩容。
	ctx, release, err := budget.BeginPath(ctx, directory, "ai_media_download", 1<<20)
	if err != nil {
		return nil, err
	}
	defer release()
	file, err := os.CreateTemp(directory, "powerx-ai-media-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	const limit int64 = 512 << 20
	size, checksum, err := copyMediaOutput(ctx, file, body, limit)
	if err != nil {
		return nil, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	head := make([]byte, 512)
	n, _ := file.Read(head)
	mime := http.DetectContentType(head[:n])
	if !strings.HasPrefix(mime, kind+"/") {
		return nil, MediaError(502, "AI_OUTPUT_TYPE_MISMATCH")
	}
	asset, err := s.media.StoreGenerated(ctx, tenant, task, index, kind, mime, checksum, size, file)
	if err != nil {
		return nil, MediaError(502, "AI_MEDIA_TRANSFER_FAILED")
	}
	return &MediaAssetResult{asset.UUID, kind, asset.MimeType, size, checksum, "sha256"}, nil
}

// copyMediaOutput 同时限制单份成果大小和新增磁盘字节；摘要只在完整写入后返回。
func copyMediaOutput(ctx context.Context, destination io.Writer, body io.Reader, limit int64) (int64, string, error) {
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(budget.Writer(ctx, destination), hash), io.LimitReader(body, limit+1))
	if err != nil {
		return size, "", err
	}
	if size == 0 || size > limit {
		return size, "", MediaError(502, "AI_MEDIA_SIZE_INVALID")
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}
