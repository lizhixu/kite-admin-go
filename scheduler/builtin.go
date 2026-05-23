package scheduler

import (
	"backend/config"
	"backend/models"
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// TaskFunc 是内置任务函数的签名，返回输出文本和错误
type TaskFunc func(ctx context.Context) (string, error)

var (
	funcRegistry = map[string]TaskFunc{}
	funcMu       sync.RWMutex
)

// RegisterFunc 注册一个内置任务函数；同名会覆盖。建议在 init() 中调用
func RegisterFunc(name string, fn TaskFunc) {
	funcMu.Lock()
	defer funcMu.Unlock()
	funcRegistry[name] = fn
}

func getFunc(name string) (TaskFunc, bool) {
	funcMu.RLock()
	defer funcMu.RUnlock()
	fn, ok := funcRegistry[name]
	return fn, ok
}

// FuncList 返回所有已注册的内置函数名（用于前端下拉选择）
func FuncList() []string {
	funcMu.RLock()
	defer funcMu.RUnlock()
	names := make([]string, 0, len(funcRegistry))
	for n := range funcRegistry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func init() {
	// 清理 30 天前的操作日志
	RegisterFunc("cleanup_old_syslog", func(_ context.Context) (string, error) {
		cutoff := time.Now().AddDate(0, 0, -30)
		res := config.DB.Where("create_time < ?", cutoff).Delete(&models.SysLog{})
		if res.Error != nil {
			return "", res.Error
		}
		return fmt.Sprintf("deleted %d syslog rows before %s", res.RowsAffected, cutoff.Format(time.RFC3339)), nil
	})

	// 清理 30 天前的任务执行日志
	RegisterFunc("cleanup_old_task_log", func(_ context.Context) (string, error) {
		cutoff := time.Now().AddDate(0, 0, -30)
		res := config.DB.Where("start_time < ?", cutoff).Delete(&models.TaskLog{})
		if res.Error != nil {
			return "", res.Error
		}
		return fmt.Sprintf("deleted %d task_log rows before %s", res.RowsAffected, cutoff.Format(time.RFC3339)), nil
	})
}
