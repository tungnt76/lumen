// Package encode turns a source video into an adaptive HLS ladder with ffmpeg.
package encode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Rung is one quality level of the ladder.
type Rung struct {
	Height   int
	VBitrate string // target video bitrate
	MaxRate  string
	BufSize  string
}

// Ladder from lowest to highest. Rungs above the source height are skipped (no upscaling).
var Ladder = []Rung{
	{360, "800k", "856k", "1200k"},
	{720, "2800k", "2996k", "4200k"},
	{1080, "5000k", "5350k", "7500k"},
}

type Probe struct {
	Duration float64
	Height   int
	HasAudio bool
}

// ProbeFile reads duration, video height and whether there is an audio stream.
func ProbeFile(ctx context.Context, path string) (Probe, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-print_format", "json",
		"-show_format", "-show_streams", path).Output()
	if err != nil {
		return Probe{}, fmt.Errorf("ffprobe: %w", err)
	}
	var r struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			CodecType string `json:"codec_type"`
			Height    int    `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return Probe{}, err
	}
	p := Probe{}
	p.Duration, _ = strconv.ParseFloat(r.Format.Duration, 64)
	for _, s := range r.Streams {
		switch s.CodecType {
		case "video":
			if s.Height > p.Height {
				p.Height = s.Height
			}
		case "audio":
			p.HasAudio = true
		}
	}
	if p.Height == 0 {
		return p, fmt.Errorf("no video stream found")
	}
	return p, nil
}

// Pick returns the rungs to encode for a source of the given height.
func Pick(sourceHeight int) []Rung {
	var out []Rung
	for _, r := range Ladder {
		if r.Height <= sourceHeight {
			out = append(out, r)
		}
	}
	if len(out) == 0 { // tiny source: one rung at its own height (even number)
		h := sourceHeight - sourceHeight%2
		out = []Rung{{h, "600k", "642k", "900k"}}
	}
	return out
}

// Args builds the ffmpeg arguments that write outDir/master.m3u8 and outDir/v{N}/.
func Args(src, outDir string, rungs []Rung, hasAudio bool) []string {
	n := len(rungs)
	var fc strings.Builder
	fmt.Fprintf(&fc, "[0:v]split=%d", n)
	for i := range rungs {
		fmt.Fprintf(&fc, "[s%d]", i)
	}
	for i, r := range rungs {
		fmt.Fprintf(&fc, ";[s%d]scale=w=-2:h=%d[v%d]", i, r.Height, i)
	}

	args := []string{"-hide_banner", "-nostats", "-y", "-i", src, "-filter_complex", fc.String()}
	var streamMap []string
	for i, r := range rungs {
		args = append(args,
			"-map", fmt.Sprintf("[v%d]", i),
			fmt.Sprintf("-c:v:%d", i), "libx264",
			fmt.Sprintf("-b:v:%d", i), r.VBitrate,
			fmt.Sprintf("-maxrate:v:%d", i), r.MaxRate,
			fmt.Sprintf("-bufsize:v:%d", i), r.BufSize,
		)
		if hasAudio {
			streamMap = append(streamMap, fmt.Sprintf("v:%d,a:%d,name:%dp", i, i, r.Height))
		} else {
			streamMap = append(streamMap, fmt.Sprintf("v:%d,name:%dp", i, r.Height))
		}
	}
	if hasAudio {
		for range rungs {
			args = append(args, "-map", "0:a:0")
		}
		args = append(args, "-c:a", "aac", "-b:a", "128k", "-ac", "2")
	}
	args = append(args,
		"-preset", "veryfast", "-profile:v", "main", "-pix_fmt", "yuv420p",
		// Keyframe every 2s at typical frame rates so segments cut cleanly.
		"-force_key_frames", "expr:gte(t,n_forced*2)", "-sc_threshold", "0",
		"-f", "hls", "-hls_time", "6", "-hls_playlist_type", "vod",
		"-hls_flags", "independent_segments",
		"-hls_segment_filename", filepath.Join(outDir, "%v", "seg_%04d.ts"),
		"-master_pl_name", "master.m3u8",
		"-var_stream_map", strings.Join(streamMap, " "),
		"-progress", "pipe:1",
		filepath.Join(outDir, "%v", "index.m3u8"),
	)
	return args
}

// Run encodes src into outDir, calling onProgress with 0-99 as it goes.
// It returns the rendition names, e.g. ["360p","720p"].
func Run(ctx context.Context, src, outDir string, onProgress func(pct int)) ([]string, error) {
	p, err := ProbeFile(ctx, src)
	if err != nil {
		return nil, err
	}
	rungs := Pick(p.Height)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", Args(src, outDir, rungs, p.HasAudio)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = &limitedWriter{b: &stderr, max: 8 << 10}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	last := -1
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok || k != "out_time_us" || p.Duration <= 0 {
			continue
		}
		us, err := strconv.ParseFloat(v, 64)
		if err != nil {
			continue
		}
		pct := int(us / 1e6 / p.Duration * 100)
		if pct > 99 {
			pct = 99
		}
		if pct != last && onProgress != nil {
			last = pct
			onProgress(pct)
		}
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, lastLine(stderr.String()))
	}

	names := make([]string, len(rungs))
	for i, r := range rungs {
		names[i] = fmt.Sprintf("%dp", r.Height)
	}
	return names, nil
}

type limitedWriter struct {
	b   *strings.Builder
	max int
}

// Write keeps only the tail of stderr so errors stay readable.
func (w *limitedWriter) Write(p []byte) (int, error) {
	w.b.Write(p)
	if w.b.Len() > w.max {
		s := w.b.String()
		w.b.Reset()
		w.b.WriteString(s[len(s)-w.max:])
	}
	return len(p), nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		return s[i+1:]
	}
	return s
}
