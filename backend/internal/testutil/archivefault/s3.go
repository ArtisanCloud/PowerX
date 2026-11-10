// Package archivefault 提供自有内存版本化 S3 协议夹具，不代表任何云厂商验收。
package archivefault

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Object 保留测试接收的精确版本与内容。
type Object struct {
	Key, Version string
	Body         []byte
	At           time.Time
}

// Peer 只服务当前测试自己的对象和故障点。
type Peer struct {
	URL       string
	mu        sync.Mutex
	objects   []Object
	mode      string
	fired     bool
	hook      func() error
	hookError error
	deletes   int
	counter   int
}

// New 启动并登记自有 HTTP 服务，测试结束后自动关闭。
func New(t *testing.T) *Peer {
	t.Helper()
	p := &Peer{}
	s := httptest.NewServer(http.HandlerFunc(p.serve))
	p.URL = s.URL
	cwd, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("owned archive fault protocol pid=%d cwd=%s command=%q listener=%s", os.Getpid(), cwd, os.Args, s.URL)
	t.Cleanup(s.Close)
	return p
}

// Arm 在对象写入后、确认返回前触发测试故障，不增加生产故障注入路由。
func (p *Peer) Arm(mode string, hook func() error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mode, p.fired, p.hook, p.hookError = mode, false, hook, nil
}

// Snapshot 返回只读诊断，用于证明故障真正触发及未删除远端版本。
func (p *Peer) Snapshot() ([]Object, int, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Object(nil), p.objects...), p.deletes, p.fired, p.hookError
}

func (p *Peer) serve(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	errOut := func(code string, status int) {
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, "<Error><Code>%s</Code><Message>fixture-private-upstream-detail</Message></Error>", code)
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") || p.mode == "credential" && p.fired {
		errOut("AccessDenied", 403)
		return
	}
	q, key := r.URL.Query(), strings.TrimPrefix(r.URL.Path, "/archives/")
	if r.Method == "GET" {
		switch {
		case q.Has("versioning"):
			_, _ = io.WriteString(w, "<VersioningConfiguration><Status>Enabled</Status></VersioningConfiguration>")
			return
		case q.Has("acl"):
			_, _ = io.WriteString(w, "<AccessControlPolicy><AccessControlList/></AccessControlPolicy>")
			return
		case q.Has("policy"):
			errOut("NoSuchBucketPolicy", 404)
			return
		case q.Has("lifecycle"):
			errOut("NoSuchLifecycleConfiguration", 404)
			return
		case q.Has("versions"):
			type row struct {
				Key     string `xml:"Key"`
				Version string `xml:"VersionId"`
				Latest  bool   `xml:"IsLatest"`
				Size    int    `xml:"Size"`
				At      string `xml:"LastModified"`
			}
			out := struct {
				XMLName   xml.Name `xml:"ListVersionsResult"`
				Rows      []row    `xml:"Version"`
				Truncated bool     `xml:"IsTruncated"`
			}{}
			latest := map[string]string{}
			for _, o := range p.objects {
				latest[o.Key] = o.Version
			}
			for _, o := range p.objects {
				if strings.HasPrefix(o.Key, q.Get("prefix")) {
					out.Rows = append(out.Rows, row{o.Key, o.Version, latest[o.Key] == o.Version, len(o.Body), o.At.Format(time.RFC3339)})
				}
			}
			_ = xml.NewEncoder(w).Encode(out)
			return
		}
		for i := len(p.objects) - 1; i >= 0; i-- {
			o := p.objects[i]
			if o.Key == key && (q.Get("versionId") == "" || o.Version == q.Get("versionId")) {
				w.Header().Set("x-amz-version-id", o.Version)
				w.Header().Set("Content-Length", strconv.Itoa(len(o.Body)))
				_, _ = w.Write(o.Body)
				return
			}
		}
		errOut("NoSuchVersion", 404)
		return
	}
	if r.Method == "PUT" {
		if r.Header.Get("If-None-Match") == "*" {
			for _, o := range p.objects {
				if o.Key == key {
					errOut("PreconditionFailed", 412)
					return
				}
			}
		}
		body, e := io.ReadAll(r.Body)
		if e != nil {
			errOut("IncompleteBody", 400)
			return
		}
		p.counter++
		version := strconv.Itoa(p.counter)
		p.objects = append(p.objects, Object{key, version, body, time.Now().UTC()})
		object, manifest := strings.Contains(key, "/objects/"), strings.Contains(key, "/manifests/")
		if !p.fired && (object && p.mode != "" && p.mode != "manifest-ack" || manifest && p.mode == "manifest-ack") {
			p.fired = true
			if p.hook != nil {
				p.hookError = p.hook()
			}
			if p.mode == "object-ack" || p.mode == "manifest-ack" {
				// The remote write happened. Dropping the real socket loses its ACK.
				conn, _, e := w.(http.Hijacker).Hijack()
				if e == nil {
					_ = conn.Close()
				}
				return
			}
		}
		w.Header().Set("x-amz-version-id", version)
		return
	}
	if r.Method == "DELETE" {
		p.deletes++
		for i, o := range p.objects {
			if o.Key == key && o.Version == q.Get("versionId") {
				p.objects = append(p.objects[:i], p.objects[i+1:]...)
				w.WriteHeader(204)
				return
			}
		}
		errOut("NoSuchVersion", 404)
		return
	}
	errOut("InvalidRequest", 400)
}
