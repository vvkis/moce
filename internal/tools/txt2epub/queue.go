package txt2epub

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type jobView struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Title      string  `json:"title"`
	Status     string  `json:"status"`
	Step       string  `json:"step"`
	Progress   float64 `json:"progress"`
	Error      string  `json:"error,omitempty"`
	OutputPath string  `json:"outputPath,omitempty"`
	Format     string  `json:"format"`
	Elapsed    string  `json:"elapsed"`
	ETA        string  `json:"eta,omitempty"`
	Batch      string  `json:"batch"`
}

type storedJob struct {
	jobView
	Input    fileIn    `json:"input"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
	Estimate int       `json:"estimate"`
	cancel   context.CancelFunc
}

var (
	jobMu   sync.Mutex
	jobList []*storedJob
	jobData string
	pumping bool
)

// SetDataDir stores the queue under the portable data directory.
func SetDataDir(dir string) {
	jobMu.Lock()
	jobData = dir
	jobMu.Unlock()
	loadJobs()
}

func (t *Tool) jobs(action string, payload json.RawMessage) (any, error) {
	switch action {
	case "listJobs":
		return map[string]any{"jobs": listJobViews()}, nil
	case "clearJobs":
		clearFinished()
		return map[string]any{"jobs": listJobViews()}, nil
	case "cancelJob":
		var in struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, fmt.Errorf("请求格式不对")
		}
		cancelJob(in.ID)
		return map[string]any{"jobs": listJobViews()}, nil
	case "retryJob":
		var in struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, fmt.Errorf("请求格式不对")
		}
		if err := retryJob(t, in.ID); err != nil {
			return nil, err
		}
		return map[string]any{"jobs": listJobViews()}, nil
	case "enqueue":
		var in struct {
			Files []fileIn `json:"files"`
		}
		if err := json.Unmarshal(payload, &in); err != nil {
			return nil, fmt.Errorf("请求格式不对")
		}
		if len(in.Files) == 0 {
			return nil, fmt.Errorf("没有要排版的书")
		}
		batch := newID()
		var ids []string
		jobMu.Lock()
		for _, file := range in.Files {
			if file.Path != "" {
				file.DataBase64 = ""
			}
			title := trim(file.Title)
			if title == "" {
				title = file.Name
			}
			item := &storedJob{
				jobView: jobView{
					ID: batchID(batch), Name: file.Name, Title: title, Status: "queued",
					Step: "排队", Format: formatOf(file.Name), Batch: batch,
				},
				Input: file, Estimate: estimateSeconds(file),
			}
			item.ID = newID()
			jobList = append(jobList, item)
			ids = append(ids, item.ID)
		}
		jobMu.Unlock()
		saveJobs()
		t.pump()
		return map[string]any{"batch": batch, "ids": ids, "jobs": listJobViews()}, nil
	default:
		return nil, fmt.Errorf("未知操作：%s", action)
	}
}

func batchID(batch string) string { return batch }

func (t *Tool) pump() {
	jobMu.Lock()
	if pumping {
		jobMu.Unlock()
		return
	}
	pumping = true
	jobMu.Unlock()
	go func() {
		for {
			job := claimQueued()
			if job == nil {
				jobMu.Lock()
				pumping = false
				jobMu.Unlock()
				return
			}
			ctx, cancel := context.WithCancel(context.Background())
			jobMu.Lock()
			job.cancel = cancel
			job.Status = "extracting"
			job.Step = "提取正文"
			job.Progress = 0.12
			job.Started = time.Now()
			jobMu.Unlock()
			saveJobs()
			setActive(ctx)
			setHook(func(step string, frac float64) {
				jobMu.Lock()
				job.Status = step
				job.Step = stepLabel(step)
				job.Progress = frac
				jobMu.Unlock()
			})
			result, err := t.convert(job.Input)
			cancelled := ctx.Err() != nil || (err != nil && err.Error() == "已取消")
			setHook(nil)
			setActive(nil)
			cancel()
			jobMu.Lock()
			if cancelled {
				job.Status = "cancelled"
				job.Step = "已取消"
				job.Error = ""
			} else if err != nil {
				job.Status = "failed"
				job.Step = "失败"
				job.Error = err.Error()
			} else {
				job.Status = "done"
				job.Step = "完成"
				job.Progress = 1
				job.OutputPath = result.Path
			}
			job.Finished = time.Now()
			job.cancel = nil
			jobMu.Unlock()
			saveJobs()
		}
	}()
}

func claimQueued() *storedJob {
	jobMu.Lock()
	defer jobMu.Unlock()
	for _, job := range jobList {
		if job.Status == "queued" {
			return job
		}
	}
	return nil
}

func cancelJob(id string) {
	jobMu.Lock()
	defer jobMu.Unlock()
	for _, job := range jobList {
		if job.ID != id {
			continue
		}
		if job.Status == "queued" {
			job.Status = "cancelled"
			job.Step = "已取消"
			job.Finished = time.Now()
		} else if job.cancel != nil {
			job.cancel()
			job.Status = "cancelled"
			job.Step = "已取消"
		}
	}
	go saveJobs()
}

func retryJob(t *Tool, id string) error {
	jobMu.Lock()
	var found *storedJob
	for _, job := range jobList {
		if job.ID == id {
			found = job
		}
	}
	if found == nil {
		jobMu.Unlock()
		return fmt.Errorf("找不到这本书")
	}
	if found.Status != "failed" && found.Status != "cancelled" {
		jobMu.Unlock()
		return fmt.Errorf("这本书还在处理")
	}
	found.Status = "queued"
	found.Step = "排队"
	found.Progress = 0
	found.Error = ""
	found.OutputPath = ""
	found.Finished = time.Time{}
	jobMu.Unlock()
	saveJobs()
	t.pump()
	return nil
}

func clearFinished() {
	jobMu.Lock()
	var keep []*storedJob
	for _, job := range jobList {
		switch job.Status {
		case "queued", "extracting", "detecting", "packaging":
			keep = append(keep, job)
		}
	}
	jobList = keep
	jobMu.Unlock()
	saveJobs()
}

func listJobViews() []jobView {
	jobMu.Lock()
	defer jobMu.Unlock()
	out := make([]jobView, 0, len(jobList))
	now := time.Now()
	for i := len(jobList) - 1; i >= 0; i-- {
		job := jobList[i]
		view := job.jobView
		start := job.Started
		if start.IsZero() {
			start = now
		}
		elapsed := now.Sub(start)
		if !job.Finished.IsZero() {
			elapsed = job.Finished.Sub(job.Started)
		}
		view.Elapsed = formatElapsed(elapsed)
		if job.Status != "done" && job.Status != "failed" && job.Status != "cancelled" && job.Estimate > 0 {
			remain := job.Estimate - int(elapsed.Seconds())
			if job.Progress > 0 && job.Progress < 1 {
				remain = int(float64(job.Estimate) * (1 - job.Progress))
			}
			if remain < 1 {
				remain = 1
			}
			view.ETA = formatETA(remain)
		}
		out = append(out, view)
	}
	return out
}

func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	sec := int(d.Seconds())
	if sec < 60 {
		return fmt.Sprintf("用时 %d 秒", sec)
	}
	return fmt.Sprintf("用时 %d 分 %d 秒", sec/60, sec%60)
}

func formatETA(seconds int) string {
	minutes := (seconds + 59) / 60
	if minutes < 1 {
		minutes = 1
	}
	if minutes >= 60 {
		return fmt.Sprintf("约 %d 小时 %d 分", minutes/60, minutes%60)
	}
	return fmt.Sprintf("约 %d 分钟", minutes)
}

func estimateSeconds(in fileIn) int {
	n := len(in.DataBase64) * 3 / 4
	if in.Path != "" {
		if st, err := os.Stat(in.Path); err == nil {
			n = int(st.Size())
		}
	}
	sec := 6 + n/300000
	if sec < 6 {
		sec = 6
	}
	return sec
}

func stepLabel(step string) string {
	switch step {
	case "extracting":
		return "提取正文"
	case "detecting":
		return "识别章节"
	case "packaging":
		return "打包 EPUB"
	default:
		return step
	}
}

func setHook(fn func(string, float64)) {
	jobMu.Lock()
	progressHook = fn
	jobMu.Unlock()
}

var progressHook func(string, float64)
var activeCtx context.Context

func setActive(ctx context.Context) {
	jobMu.Lock()
	activeCtx = ctx
	jobMu.Unlock()
}

func aborted() error {
	jobMu.Lock()
	ctx := activeCtx
	jobMu.Unlock()
	if ctx != nil && ctx.Err() != nil {
		return fmt.Errorf("已取消")
	}
	return nil
}

func report(step string, frac float64) {
	jobMu.Lock()
	fn := progressHook
	jobMu.Unlock()
	if fn != nil {
		fn(step, frac)
	}
}

func saveJobs() {
	jobMu.Lock()
	dir := jobData
	snap := append([]*storedJob(nil), jobList...)
	jobMu.Unlock()
	if dir == "" {
		return
	}
	_ = os.MkdirAll(dir, 0o755)
	body, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "jobs.json"), body, 0o644)
}

func loadJobs() {
	jobMu.Lock()
	dir := jobData
	jobMu.Unlock()
	if dir == "" {
		return
	}
	body, err := os.ReadFile(filepath.Join(dir, "jobs.json"))
	if err != nil {
		return
	}
	var snap []*storedJob
	if json.Unmarshal(body, &snap) != nil {
		return
	}
	jobMu.Lock()
	for _, job := range snap {
		switch job.Status {
		case "extracting", "detecting", "packaging", "queued":
			job.Status = "queued"
			job.Step = "排队"
			job.Progress = 0
		}
		jobList = append(jobList, job)
	}
	jobMu.Unlock()
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
