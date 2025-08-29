// internal/server/agent/catalog/watch.go
package catalog

//
//import (
//	"log"
//
//	"github.com/fsnotify/fsnotify"
//)
//
//func WatchDir(dir string) {
//	w, _ := fsnotify.NewWatcher()
//	_ = w.Add(dir)
//	go func() {
//		for ev := range w.Events {
//			if ev.Has(fsnotify.Create) || ev.Has(fsnotify.Write) || ev.Has(fsnotify.Remove) {
//				reg, err := Load(dir)
//				if err == nil {
//					Global = reg
//					log.Println("[catalog] reloaded")
//				}
//			}
//		}
//	}()
//}
