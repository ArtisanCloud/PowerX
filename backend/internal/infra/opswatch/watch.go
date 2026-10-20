// Package opswatch 将私有运维账本的文件变更转换为通知，不定时读取任务状态。
package opswatch

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fsnotify/fsnotify"
)

type Subscription struct {
	Changes <-chan []string
	Errors  <-chan error
	cancel  context.CancelFunc
	done    <-chan struct{}
}

func (s *Subscription) Close() { s.cancel(); <-s.done }

// Subscribe 仅监听宿主给定的目录，通知中不包含文件路径或账本正文。
// accept 可进一步限制单个任务；慢消费者合并资源名，不丢失最后状态。
func Subscribe(ctx context.Context, resources map[string][]string, accept func(string) bool) (*Subscription, error) {
	w, e := fsnotify.NewWatcher()
	if e != nil {
		return nil, e
	}
	directories := map[string]string{}
	for resource, paths := range resources {
		for _, dir := range paths {
			if !filepath.IsAbs(dir) {
				w.Close()
				return nil, errors.New("运维通知目录必须是绝对路径")
			}
			dir = filepath.Clean(dir)
			if e = w.Add(dir); e != nil {
				w.Close()
				return nil, e
			}
			directories[dir] = resource
		}
	}
	run, cancel := context.WithCancel(ctx)
	changes := make(chan []string)
	errorsOut := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(changes)
		defer close(errorsOut)
		defer w.Close()
		pending := map[string]bool{}
		for {
			var deliver chan []string
			var names []string
			if len(pending) > 0 {
				deliver = changes
				for name := range pending {
					names = append(names, name)
				}
				sort.Strings(names)
			}
			select {
			case <-run.Done():
				return
			case deliver <- names:
				pending = map[string]bool{}
			case e, ok := <-w.Errors:
				if ok {
					errorsOut <- e
				}
				return
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				if _, watched := directories[event.Name]; watched && event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
					errorsOut <- errors.New("运维账本目录已变更，请重新建立通知连接")
					return
				}
				if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 || !strings.HasSuffix(event.Name, ".json") || strings.HasPrefix(filepath.Base(event.Name), ".") {
					continue
				}
				if accept != nil && !accept(event.Name) {
					continue
				}
				if resource := directories[filepath.Dir(event.Name)]; resource != "" {
					pending[resource] = true
				}
			}
		}
	}()
	return &Subscription{Changes: changes, Errors: errorsOut, cancel: cancel, done: done}, nil
}
