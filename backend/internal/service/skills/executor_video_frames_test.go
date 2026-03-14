package skills

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVideoFramesExecutor_Execute(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found in PATH")
	}

	workDir := t.TempDir()
	videoPath := filepath.Join(workDir, "sample.mp4")
	outputDir := filepath.Join(workDir, "frames")

	cmd := exec.Command(
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-f", "lavfi",
		"-i", "testsrc=duration=1:size=160x120:rate=8",
		videoPath,
	)
	require.NoError(t, cmd.Run())

	executor := newVideoFramesExecutor()
	result, err := executor.Execute(context.Background(), ExecuteInput{
		TraceID: "trace-video-frames-unit",
		SkillID: "skill.builtin.video-frames",
		Version: "1.0.0",
		Payload: map[string]interface{}{
			"video_path": videoPath,
			"output_dir": outputDir,
			"fps":        4,
			"max_frames": 3,
			"format":     "jpg",
		},
	})
	require.NoError(t, err)
	require.Equal(t, 3, result["frame_count"])

	entries, readErr := os.ReadDir(outputDir)
	require.NoError(t, readErr)
	require.GreaterOrEqual(t, len(entries), 3)
}
