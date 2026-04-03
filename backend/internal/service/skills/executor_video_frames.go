package skills

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type videoFramesExecutor struct{}

func newVideoFramesExecutor() SkillExecutor { return &videoFramesExecutor{} }

func (e *videoFramesExecutor) CanHandle(in ExecuteInput) bool {
	skillID := strings.ToLower(strings.TrimSpace(in.SkillID))
	return strings.Contains(skillID, "video-frames")
}

func (e *videoFramesExecutor) Execute(ctx context.Context, in ExecuteInput) (map[string]interface{}, error) {
	videoPath := extractString(in.Payload, "video_path", "videoPath", "input_video", "source")
	videoPath = normalizeLocalPath(videoPath)
	if videoPath == "" {
		return nil, errors.New("video_path is required for video-frames skill")
	}
	info, err := os.Stat(videoPath)
	if err != nil || info.IsDir() {
		return nil, fmt.Errorf("video_path not found: %s", videoPath)
	}

	outputDir := normalizeLocalPath(extractString(in.Payload, "output_dir", "outputDir"))
	if outputDir == "" {
		outputDir = filepath.Join(os.TempDir(), "powerx", "skills", "video-frames", strings.TrimSpace(in.TraceID))
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output_dir failed: %w", err)
	}

	fps := extractFloat(in.Payload, 1.0, "fps", "frame_rate")
	if fps <= 0 {
		fps = 1.0
	}
	maxFrames := extractInt(in.Payload, 24, "max_frames", "maxFrames", "frame_limit")
	if maxFrames <= 0 {
		maxFrames = 24
	}
	format := strings.ToLower(strings.TrimSpace(extractString(in.Payload, "format", "image_format")))
	if format == "" {
		format = "jpg"
	}
	if format != "jpg" && format != "jpeg" && format != "png" && format != "webp" {
		return nil, fmt.Errorf("unsupported format %q, allowed: jpg/png/webp", format)
	}
	if format == "jpeg" {
		format = "jpg"
	}

	timeoutSec := extractInt(in.Payload, 120, "timeout_sec", "timeout")
	if timeoutSec <= 0 {
		timeoutSec = 120
	}
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	pattern := filepath.Join(outputDir, "frame_%05d."+format)
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-i", videoPath,
		"-vf", fmt.Sprintf("fps=%g", fps),
		"-frames:v", strconv.Itoa(maxFrames),
		pattern,
	}
	cmd := exec.CommandContext(execCtx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stdout = nil
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("ffmpeg extract frames failed: %s", msg)
	}

	files, err := filepath.Glob(filepath.Join(outputDir, "frame_*."+format))
	if err != nil {
		return nil, fmt.Errorf("glob output frames failed: %w", err)
	}
	sort.Strings(files)
	frames := make([]map[string]interface{}, 0, len(files))
	for i, file := range files {
		stat, statErr := os.Stat(file)
		size := int64(0)
		if statErr == nil {
			size = stat.Size()
		}
		frames = append(frames, map[string]interface{}{
			"index":      i + 1,
			"path":       file,
			"size_bytes": size,
		})
	}

	return map[string]interface{}{
		"skill_id":    in.SkillID,
		"version":     in.Version,
		"video_path":  videoPath,
		"output_dir":  outputDir,
		"frame_count": len(frames),
		"frames":      frames,
		"format":      format,
		"fps":         fps,
		"max_frames":  maxFrames,
	}, nil
}

func normalizeLocalPath(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "file://")
	return raw
}

func extractString(payload map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := payload[key]; ok {
			s := strings.TrimSpace(fmt.Sprintf("%v", v))
			if s != "" {
				return s
			}
		}
	}
	return ""
}

func extractInt(payload map[string]interface{}, fallback int, keys ...string) int {
	for _, key := range keys {
		v, ok := payload[key]
		if !ok {
			continue
		}
		switch n := v.(type) {
		case int:
			return n
		case int32:
			return int(n)
		case int64:
			return int(n)
		case float64:
			return int(n)
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
				return parsed
			}
		}
	}
	return fallback
}

func extractFloat(payload map[string]interface{}, fallback float64, keys ...string) float64 {
	for _, key := range keys {
		v, ok := payload[key]
		if !ok {
			continue
		}
		switch n := v.(type) {
		case float64:
			return n
		case float32:
			return float64(n)
		case int:
			return float64(n)
		case int64:
			return float64(n)
		case string:
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
				return parsed
			}
		}
	}
	return fallback
}

