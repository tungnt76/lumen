package encode

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPick(t *testing.T) {
	if got := len(Pick(1080)); got != 3 {
		t.Errorf("1080p source: want 3 rungs, got %d", got)
	}
	if got := len(Pick(720)); got != 2 {
		t.Errorf("720p source: want 2 rungs, got %d", got)
	}
	if got := Pick(241); len(got) != 1 || got[0].Height != 240 {
		t.Errorf("tiny source: got %+v", got)
	}
}

func encodeSample(t *testing.T, withAudio bool) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp4")
	args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=size=1280x720:rate=24:duration=8"}
	if withAudio {
		args = append(args, "-f", "lavfi", "-i", "sine=frequency=440:duration=8", "-shortest")
	}
	args = append(args, "-pix_fmt", "yuv420p", src)
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("make sample: %v %s", err, out)
	}

	var progress []int
	out := filepath.Join(dir, "hls")
	names, err := Run(context.Background(), src, out, func(p int) { progress = append(progress, p) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, ",") != "360p,720p" {
		t.Errorf("renditions: %v", names)
	}
	master, err := os.ReadFile(filepath.Join(out, "master.m3u8"))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if !strings.Contains(string(master), n+"/index.m3u8") {
			t.Errorf("master playlist missing %s:\n%s", n, master)
		}
		segs, _ := filepath.Glob(filepath.Join(out, n, "seg_*.ts"))
		if len(segs) < 2 {
			t.Errorf("%s: want >= 2 segments, got %d", n, len(segs))
		}
	}
	if len(progress) == 0 {
		t.Error("no progress reported")
	}
}

func TestRunWithAudio(t *testing.T)  { encodeSample(t, true) }
func TestRunSilentFilm(t *testing.T) { encodeSample(t, false) }

func TestAudio(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.wav")
	if out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi",
		"-i", "sine=frequency=220:duration=5", "-ac", "2", src).CombinedOutput(); err != nil {
		t.Fatalf("make sample: %v %s", err, out)
	}
	for _, speech := range []bool{true, false} {
		dst := filepath.Join(dir, "out.m4a")
		secs, err := Audio(context.Background(), src, dst, speech)
		if err != nil {
			t.Fatal(err)
		}
		if secs != 5 {
			t.Errorf("speech=%v: duration %d, want 5", speech, secs)
		}
		probe, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_name,channels", "-of", "csv=p=0", dst).Output()
		if err != nil {
			t.Fatal(err)
		}
		want := "aac,2"
		if speech {
			want = "aac,1"
		}
		if got := strings.TrimSpace(string(probe)); got != want {
			t.Errorf("speech=%v: stream %q, want %q", speech, got, want)
		}
	}
	if _, err := Audio(context.Background(), filepath.Join(dir, "missing.wav"), filepath.Join(dir, "x.m4a"), true); err == nil {
		t.Error("missing source encoded")
	}
}
