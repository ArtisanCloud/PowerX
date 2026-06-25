package manager

import (
	"context"
	"errors"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

func TestPostEnablePluginHookReceivesPluginAndAPIBaseURL(t *testing.T) {
	var gotPlugin plugin_mgr.Plugin
	var gotBaseURL string
	hook := PostEnablePluginHook(func(ctx context.Context, plugin plugin_mgr.Plugin, apiBaseURL string) error {
		gotPlugin = plugin
		gotBaseURL = apiBaseURL
		return nil
	})

	err := hook(context.Background(), plugin_mgr.Plugin{ID: "com.powerx.plugin.mediax-studio", Version: "1.0.0"}, "http://127.0.0.1:18080")
	if err != nil {
		t.Fatalf("hook err=%v", err)
	}
	if gotPlugin.ID != "com.powerx.plugin.mediax-studio" || gotPlugin.Version != "1.0.0" {
		t.Fatalf("plugin=%+v", gotPlugin)
	}
	if gotBaseURL != "http://127.0.0.1:18080" {
		t.Fatalf("apiBaseURL=%q", gotBaseURL)
	}
}

func TestPostEnablePluginHookPropagatesDiscoveryError(t *testing.T) {
	want := errors.New("discovery failed")
	hook := PostEnablePluginHook(func(ctx context.Context, plugin plugin_mgr.Plugin, apiBaseURL string) error {
		return want
	})
	if err := hook(context.Background(), plugin_mgr.Plugin{ID: "com.powerx.plugin.mediax-studio"}, "http://127.0.0.1:18080"); !errors.Is(err, want) {
		t.Fatalf("err=%v, want %v", err, want)
	}
}
