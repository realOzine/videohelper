package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	desktopruntime "github.com/ozine/videohelper/internal/desktop/runtime"
	"github.com/ozine/videohelper/internal/desktop/store"
	"github.com/ozine/videohelper/internal/media"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/timeline"
)

func (p *Processor) process(ctx context.Context, j store.Job) error {
	ctx, cancel := context.WithTimeout(ctx, 24*time.Hour)
	defer cancel()
	cp := checkpoint{}
	if err := json.Unmarshal(j.Progress, &cp); err != nil {
		return err
	}
	if cp.Identity == nil {
		cp.Identity = maps.Clone(p.identity)
		cp.Done = map[string]bool{}
		cp.Next = map[string]int{}
		cp.Stages = map[string]StageStatus{}
	}
	if !maps.Equal(cp.Identity, p.identity) || cp.Done == nil || cp.Next == nil || cp.Stages == nil {
		return fmt.Errorf("模型版本已改变，请重新建立索引: %w", store.ErrConflict)
	}
	v, err := p.store.Video(ctx, j.VideoID)
	if err != nil {
		return err
	}
	if v.DeletedAt != nil {
		return store.ErrStale
	}
	if v.SourceKey == "" {
		if err := p.save(ctx, j, &cp, "fetch", "running", 0, 0, nil); err != nil {
			return err
		}
		key, err := p.fetch(ctx, j, v)
		if err != nil {
			return err
		}
		if err := p.save(ctx, j, &cp, "fetch", "done", 1, 1, func(tx *store.Tx) error { return tx.SetSource(ctx, v.ID, key) }); err != nil {
			return err
		}
		v, err = p.store.Video(ctx, v.ID)
		if err != nil {
			return err
		}
	}
	if !cp.Done["probe"] {
		if err := p.save(ctx, j, &cp, "probe", "running", 0, 0, nil); err != nil {
			return err
		}
		source, err := p.store.MediaPath(v.SourceKey)
		if err != nil {
			return err
		}
		info, err := p.media.Probe(ctx, source)
		if err != nil {
			return err
		}
		if err := p.save(ctx, j, &cp, "probe", "done", 1, 1, func(tx *store.Tx) error {
			return tx.UpdateMedia(ctx, v.ID, store.MediaInfo{DurationMs: info.DurationMs, HasVideo: info.HasVideo, HasAudio: info.HasAudio})
		}); err != nil {
			return err
		}
		v, err = p.store.Video(ctx, v.ID)
		if err != nil {
			return err
		}
	}
	if v.DurationMs == nil || v.HasVideo == nil || v.HasAudio == nil {
		return store.ErrInvalid
	}
	if !cp.Done["prepare_media"] {
		if err := p.save(ctx, j, &cp, "prepare_media", "running", 0, 0, nil); err != nil {
			return err
		}
		info, err := p.prepare(ctx, j, v)
		if err != nil {
			return err
		}
		if err := p.save(ctx, j, &cp, "prepare_media", "done", 1, 1, func(tx *store.Tx) error { return tx.UpdateMedia(ctx, v.ID, info) }); err != nil {
			return err
		}
		v, err = p.store.Video(ctx, v.ID)
		if err != nil {
			return err
		}
	}
	if *v.HasAudio && !cp.Done["asr"] {
		if err := p.transcribe(ctx, j, &cp, v); err != nil {
			return err
		}
	}
	for _, route := range p.routes {
		if !eligible(v, route.channel) || cp.Done["embed:"+route.channel] {
			continue
		}
		if err := p.embed(ctx, j, &cp, v, route); err != nil {
			return err
		}
	}
	if err := p.save(ctx, j, &cp, "finalize", "done", 1, 1, nil); err != nil {
		return err
	}
	return p.store.CompleteJob(ctx, j.ID, j.Attempt, nil)
}

func eligible(v store.Video, channel string) bool {
	if channel == "visual" {
		return v.HasVideo != nil && *v.HasVideo
	}
	return v.HasAudio != nil && *v.HasAudio
}

func runBase(j store.Job) string {
	return store.VideoKey(j.VideoID, fmt.Sprintf("runs/%d-%d", j.ID, j.Attempt))
}
func (p *Processor) publish(ctx context.Context, key, file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	return p.store.WriteFile(ctx, key, f)
}

type boundedOutput struct {
	data  []byte
	limit int
}

func (b *boundedOutput) Write(data []byte) (int, error) {
	n := len(data)
	if len(b.data)+n > b.limit {
		return 0, errors.New("command output exceeds limit")
	}
	b.data = append(b.data, data...)
	return n, nil
}

func (p *Processor) fetch(ctx context.Context, j store.Job, v store.Video) (string, error) {
	dir, err := p.store.WorkDir()
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	// shortcut: temporary downloads can exceed the final-file limit, add a disk budget when needed.
	const maxBytes = int64(20 << 30)
	args := []string{"--ignore-config", "--no-plugin-dirs", "--no-playlist", "--no-progress", "--no-warnings", "--restrict-filenames", "--no-part", "--max-filesize", strconv.FormatInt(maxBytes, 10), "--ffmpeg-location", filepath.Dir(p.cfg.Tools.FFmpeg), "--paths", dir, "--output", "source.%(ext)s", "--print", "after_move:filepath"}
	if p.cfg.Tools.Deno != "" {
		args = append(args, "--js-runtimes", "deno:"+p.cfg.Tools.Deno)
	}
	args = append(args, "--", v.SourceURL)
	var output = boundedOutput{limit: 64 << 10}
	var stderr = boundedOutput{limit: 64 << 10}
	if err := desktopruntime.RunCommand(ctx, p.cfg.Tools.YTDLP, args, &output, &stderr); err != nil {
		return "", fmt.Errorf("下载失败: %w: %s", err, strings.TrimSpace(string(stderr.data)))
	}
	lines := strings.Split(strings.TrimSpace(string(output.data)), "\n")
	source := strings.TrimSpace(lines[len(lines)-1])
	if !filepath.IsAbs(source) {
		source = filepath.Join(dir, source)
	}
	rel, err := filepath.Rel(dir, source)
	if err != nil || !filepath.IsLocal(rel) || filepath.Dir(rel) != "." {
		return "", store.ErrInvalid
	}
	info, err := os.Lstat(source)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxBytes {
		return "", store.ErrInvalid
	}
	ext := strings.ToLower(filepath.Ext(source))
	if len(ext) > 12 || !safeExtension(ext) {
		ext = ".bin"
	}
	key := runBase(j) + "/source" + ext
	if err := p.publish(ctx, key, source); err != nil {
		return "", err
	}
	return key, nil
}

func safeExtension(ext string) bool {
	if len(ext) < 2 || ext[0] != '.' {
		return false
	}
	for _, c := range ext[1:] {
		if c < 'a' || c > 'z' {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

func (p *Processor) prepare(ctx context.Context, j store.Job, v store.Video) (store.MediaInfo, error) {
	info := store.MediaInfo{DurationMs: *v.DurationMs, HasVideo: *v.HasVideo, HasAudio: *v.HasAudio}
	dir, err := p.store.WorkDir()
	if err != nil {
		return info, err
	}
	defer os.RemoveAll(dir)
	source, err := p.store.MediaPath(v.SourceKey)
	if err != nil {
		return info, err
	}
	normalized := filepath.Join(dir, "normalized_video.mp4")
	if err := p.media.Transcode(ctx, source, normalized, media.TranscodeOptions{}); err != nil {
		return info, err
	}
	info.NormalizedVideoKey = runBase(j) + "/normalized_video.mp4"
	if err := p.publish(ctx, info.NormalizedVideoKey, normalized); err != nil {
		return info, err
	}
	if *v.HasVideo {
		files, layout, err := p.media.Sprites(ctx, normalized, filepath.Join(dir, "sprites"), timeline.Cells(*v.DurationMs), 320, 10, 10)
		if err != nil {
			return info, err
		}
		for i, file := range files {
			if err := p.publish(ctx, path.Dir(info.NormalizedVideoKey)+"/sprites/"+strconv.Itoa(i)+".jpg", file); err != nil {
				return info, err
			}
		}
		info.Thumbs = &layout
	}
	return info, nil
}

func (p *Processor) transcribe(ctx context.Context, j store.Job, cp *checkpoint, v store.Video) error {
	chunk, overlap := 60000, 5000
	if p.asr.MaxWindowMs < chunk+2*overlap {
		chunk = p.asr.MaxWindowMs
		overlap = 0
	}
	if chunk <= 0 {
		return store.ErrInvalid
	}
	total := (*v.DurationMs + chunk - 1) / chunk
	if len(cp.RawKeys) > total || cp.Next["asr"] != len(cp.RawKeys) {
		return store.ErrInvalid
	}
	if err := p.save(ctx, j, cp, "asr", "running", len(cp.RawKeys), total, nil); err != nil {
		return err
	}
	var sentences = []store.Sentence{}
	language := "zh"
	if !slices.Contains(p.asr.Languages, language) {
		language = p.asr.Languages[0]
	}
	for i := 0; i < total; i++ {
		start, end := i*chunk, min((i+1)*chunk, *v.DurationMs)
		request := mlclient.TranscribeRequest{Model: p.asr.ID, URI: "local://media/" + v.SourceKey, Language: language, StartMs: max(0, start-overlap), EndMs: min(*v.DurationMs, end+overlap)}
		var response mlclient.TranscribeResponse
		if i < len(cp.RawKeys) {
			f, err := p.store.OpenFile(cp.RawKeys[i])
			if err != nil {
				return err
			}
			err = json.NewDecoder(io.LimitReader(f, 32<<20)).Decode(&response)
			f.Close()
			if err != nil {
				return err
			}
		} else {
			var err error
			response, err = p.ml.Transcribe(ctx, p.asrEndpoint, request)
			if err != nil {
				return err
			}
			data, err := json.Marshal(response)
			if err != nil {
				return err
			}
			key := runBase(j) + "/asr/chunk-" + strconv.Itoa(i) + ".json"
			if err := p.store.WriteFile(ctx, key, bytes.NewReader(data)); err != nil {
				return err
			}
			cp.RawKeys = append(cp.RawKeys, key)
			cp.Next["asr"] = len(cp.RawKeys)
			if err := p.save(ctx, j, cp, "asr", "running", len(cp.RawKeys), total, nil); err != nil {
				return err
			}
		}
		if err := mlclient.ValidateTranscript(request, response, p.asr); err != nil {
			return err
		}
		for _, sentence := range response.Sentences {
			// shortcut: sentence-start ownership may repeat boundary speech, revisit after real long-video validation.
			if sentence.StartMs < start || sentence.StartMs >= end {
				continue
			}
			item := store.Sentence{StartMs: sentence.StartMs, EndMs: sentence.EndMs, Text: sentence.Text, Model: p.asr.ID + "." + strings.TrimPrefix(p.asr.Fingerprint, "sha256:")}
			for _, w := range sentence.Words {
				// ASR windows overlap; ValidateTranscript checks ordering within each response.
				item.Words = append(item.Words, store.Word{W: w.W, S: w.S, E: w.E})
			}
			if len(sentences) > 0 && sentences[len(sentences)-1].StartMs == item.StartMs {
				last := &sentences[len(sentences)-1]
				last.EndMs = max(last.EndMs, item.EndMs)
				last.Text += item.Text
				last.Words = append(last.Words, item.Words...)
			} else {
				sentences = append(sentences, item)
			}
		}
	}
	return p.save(ctx, j, cp, "asr", "done", total, total, func(tx *store.Tx) error { return tx.ReplaceSentences(ctx, v.ID, sentences) })
}

func (p *Processor) inputs(ctx context.Context, v store.Video, r route) ([]mlclient.EmbedInput, []timeline.Span, error) {
	inputs := []mlclient.EmbedInput{}
	spans := []timeline.Span{}
	if r.channel == "speech" {
		sentences, err := p.store.Sentences(ctx, v.ID, nil)
		if err != nil {
			return nil, nil, err
		}
		plain := make([]timeline.Sentence, len(sentences))
		for i, s := range sentences {
			plain[i] = timeline.Sentence{Span: timeline.Span{StartMs: s.StartMs, EndMs: s.EndMs}, Text: s.Text}
		}
		for _, s := range timeline.MergeSentences(plain, 8, 120) {
			inputs = append(inputs, mlclient.EmbedInput{Text: s.Text})
			spans = append(spans, s.Span)
		}
	} else {
		spans = timeline.Cells(*v.DurationMs)
		key := v.NormalizedVideoKey
		if r.channel == "audio" {
			key = v.SourceKey
		}
		for _, span := range spans {
			ref := &mlclient.MediaRef{URI: "local://media/" + key, StartMs: span.StartMs, EndMs: span.EndMs}
			input := mlclient.EmbedInput{Video: ref}
			if r.channel == "audio" {
				input = mlclient.EmbedInput{Audio: ref}
			}
			inputs = append(inputs, input)
		}
	}
	return inputs, spans, nil
}

func (p *Processor) embed(ctx context.Context, j store.Job, cp *checkpoint, v store.Video, r route) error {
	stage := "embed:" + r.channel
	inputs, spans, err := p.inputs(ctx, v, r)
	if err != nil {
		return err
	}
	next := cp.Next[stage]
	if next < 0 || next > len(inputs) {
		return store.ErrInvalid
	}
	var clear func(*store.Tx) error
	if next == 0 {
		clear = func(tx *store.Tx) error { return tx.DeleteVectors(ctx, v.ID, r.channel, r.localID) }
	}
	if err := p.save(ctx, j, cp, stage, "running", next, len(inputs), clear); err != nil {
		return err
	}
	batch := min(8, r.info.MaxBatch)
	for start := next; start < len(inputs); start += batch {
		end := min(start+batch, len(inputs))
		response, err := p.ml.Embed(ctx, mlclient.EmbedRequest{Space: r.info.ID, Role: "document", Inputs: inputs[start:end]})
		if err != nil {
			return err
		}
		rows := make([]store.VectorRow, end-start)
		for i, vec := range response.Vectors {
			rows[i] = store.VectorRow{VideoID: v.ID, SpaceID: r.localID, Fingerprint: r.info.Fingerprint, Channel: r.channel, Seg: r.seg, Span: spans[start+i], Vec: vec}
		}
		cp.Next[stage] = end
		if err := p.save(ctx, j, cp, stage, "running", end, len(inputs), func(tx *store.Tx) error { return tx.PutVectors(ctx, rows) }); err != nil {
			return err
		}
	}
	return p.save(ctx, j, cp, stage, "done", len(inputs), len(inputs), nil)
}
