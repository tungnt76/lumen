package encode

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
)

// AudioArgs builds the ffmpeg arguments that turn any audio (or the audio of a video) into a
// loudness-normalised AAC .m4a. Speech is mono at 48 kbps (about 22 MB per hour, so the
// R2 free tier holds a lot of audiobooks); music is stereo at 128 kbps.
func AudioArgs(src, dst string, speech bool) []string {
	bitrate, channels := "128k", "2"
	if speech {
		bitrate, channels = "48k", "1"
	}
	return []string{"-hide_banner", "-nostats", "-loglevel", "error", "-y", "-i", src, "-vn",
		"-af", "loudnorm=I=-16:TP=-1.5:LRA=11", "-ar", "44100",
		"-c:a", "aac", "-b:a", bitrate, "-ac", channels, "-movflags", "+faststart", dst}
}

// Audio encodes src into dst (an .m4a path) and returns the result's duration in whole seconds.
func Audio(ctx context.Context, src, dst string, speech bool) (int, error) {
	if out, err := exec.CommandContext(ctx, "ffmpeg", AudioArgs(src, dst, speech)...).CombinedOutput(); err != nil {
		return 0, fmt.Errorf("ffmpeg: %w: %s", err, lastLine(string(out)))
	}
	return Duration(ctx, dst)
}

// Duration reads a media file's duration in whole seconds (rounded).
func Duration(ctx context.Context, path string) (int, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-print_format", "json", "-show_format", path).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}
	var r struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return 0, err
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(r.Format.Duration), 64)
	if err != nil {
		return 0, fmt.Errorf("ffprobe: no duration for %s", path)
	}
	return int(math.Round(d)), nil
}
