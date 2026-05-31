package scheduler

import (
	"backend/config"
	"backend/models"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	TypeHTTP  = "HTTP"
	TypeShell = "SHELL"
	TypeFunc  = "FUNC"

	statusSuccess = "SUCCESS"
	statusFailed  = "FAILED"
	statusTimeout = "TIMEOUT"
)

// stepLevel 步骤级别
const (
	lvInfo  = "INFO"
	lvWarn  = "WARN"
	lvError = "ERROR"
)

// taskStep 任务执行过程中的一个步骤
type taskStep struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// stepRecorder 线程安全地收集执行步骤
type stepRecorder struct {
	mu    sync.Mutex
	steps []taskStep
}

func (r *stepRecorder) add(level, format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps = append(r.steps, taskStep{
		Time:    time.Now(),
		Level:   level,
		Message: fmt.Sprintf(format, args...),
	})
}

func (r *stepRecorder) marshal() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, _ := json.Marshal(r.steps)
	return string(b)
}

func run(task models.Task, trigger string) {
	start := time.Now()
	tl := models.TaskLog{
		TaskID:    task.ID,
		TaskName:  task.Name,
		Trigger:   trigger,
		StartTime: start,
	}

	rec := &stepRecorder{}
	rec.add(lvInfo, "任务开始 [%s] 触发方式=%s 类型=%s", task.Name, trigger, task.Type)

	timeout := time.Duration(task.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	rec.add(lvInfo, "设置超时 %s", timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var (
		output string
		err    error
	)
	switch task.Type {
	case TypeHTTP:
		output, err = runHTTP(ctx, task, rec)
	case TypeShell:
		output, err = runShell(ctx, task, rec)
	case TypeFunc:
		output, err = runFunc(ctx, task, rec)
	default:
		err = fmt.Errorf("unknown task type: %s", task.Type)
		rec.add(lvError, "未知任务类型: %s", task.Type)
	}

	end := time.Now()
	tl.EndTime = end
	tl.Duration = end.Sub(start).Milliseconds()
	tl.Output = output

	switch {
	case err == nil:
		tl.Status = statusSuccess
		rec.add(lvInfo, "执行成功，耗时 %d ms", tl.Duration)
	case ctx.Err() == context.DeadlineExceeded:
		tl.Status = statusTimeout
		tl.Error = err.Error()
		rec.add(lvError, "执行超时: %s", err.Error())
	default:
		tl.Status = statusFailed
		tl.Error = err.Error()
		rec.add(lvError, "执行失败: %s", err.Error())
	}

	tl.Steps = rec.marshal()
	config.DB.Create(&tl)

	updates := map[string]any{"last_run_at": start}
	if s := Default(); s != nil {
		if next, ok := s.nextRunFor(task.ID); ok {
			updates["next_run_at"] = next
		}
	}
	config.DB.Model(&models.Task{}).Where("id = ?", task.ID).Updates(updates)
}

func runHTTP(ctx context.Context, task models.Task, rec *stepRecorder) (string, error) {
	method := task.HttpMethod
	if method == "" {
		method = http.MethodGet
	}
	rec.add(lvInfo, "构造 HTTP 请求 %s %s", method, task.Command)

	var body io.Reader
	if task.HttpBody != "" {
		body = bytes.NewBufferString(task.HttpBody)
		rec.add(lvInfo, "携带请求体 %d 字节", len(task.HttpBody))
	}
	req, err := http.NewRequestWithContext(ctx, method, task.Command, body)
	if err != nil {
		rec.add(lvError, "构造请求失败: %s", err.Error())
		return "", err
	}
	if task.HttpHeaders != "" {
		var headers map[string]string
		if jerr := json.Unmarshal([]byte(task.HttpHeaders), &headers); jerr == nil {
			for k, v := range headers {
				req.Header.Set(k, v)
			}
			rec.add(lvInfo, "设置请求头 %d 个", len(headers))
		} else {
			rec.add(lvWarn, "请求头 JSON 解析失败，已忽略: %s", jerr.Error())
		}
	}

	sendAt := time.Now()
	rec.add(lvInfo, "发送请求 …")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		rec.add(lvError, "请求失败: %s", err.Error())
		return "", err
	}
	defer resp.Body.Close()
	rec.add(lvInfo, "收到响应 status=%d 耗时=%d ms", resp.StatusCode, time.Since(sendAt).Milliseconds())

	buf, _ := io.ReadAll(resp.Body)
	rec.add(lvInfo, "读取响应体 %d 字节", len(buf))

	out := fmt.Sprintf("HTTP %d\n%s", resp.StatusCode, string(buf))
	if resp.StatusCode >= 400 {
		return out, fmt.Errorf("http status %d", resp.StatusCode)
	}
	return out, nil
}

func runShell(ctx context.Context, task models.Task, rec *stepRecorder) (string, error) {
	// 安全检查：拒绝包含危险字符的命令，防止命令注入
	dangerousChars := []string{"|", "&", ";", "`", "$(", ">", "<", "\n", "\r"}
	for _, d := range dangerousChars {
		if strings.Contains(task.Command, d) {
			rec.add(lvError, "命令包含不安全字符: %q", d)
			return "", fmt.Errorf("command contains unsafe character: %q", d)
		}
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		rec.add(lvInfo, "Windows 平台，调用 cmd /C")
		cmd = exec.CommandContext(ctx, "cmd", "/C", task.Command)
	} else {
		rec.add(lvInfo, "Unix 平台，调用 sh -c")
		cmd = exec.CommandContext(ctx, "sh", "-c", task.Command)
	}
	rec.add(lvInfo, "执行命令: %s", task.Command)

	runAt := time.Now()
	out, err := cmd.CombinedOutput()
	rec.add(lvInfo, "命令结束，耗时 %d ms 输出 %d 字节", time.Since(runAt).Milliseconds(), len(out))
	return string(out), err
}

func runFunc(ctx context.Context, task models.Task, rec *stepRecorder) (string, error) {
	rec.add(lvInfo, "查找内置函数: %s", task.Command)
	fn, ok := getFunc(task.Command)
	if !ok {
		rec.add(lvError, "函数未注册: %s", task.Command)
		return "", fmt.Errorf("function not registered: %s", task.Command)
	}
	rec.add(lvInfo, "调用内置函数 …")
	runAt := time.Now()
	out, err := fn(ctx)
	rec.add(lvInfo, "函数返回，耗时 %d ms", time.Since(runAt).Milliseconds())
	return out, err
}
