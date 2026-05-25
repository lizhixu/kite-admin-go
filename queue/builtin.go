package queue

import (
	"context"
	"fmt"
	"log"
	"time"
)

// init 进程启动时把示例 handler 注册进去（此时 DB 未必就绪，所以仅 setHandler；
// 实际的 Queue 行在 manager.Init() -> rehydrateHandlers() 里 FirstOrCreate）。
func init() {
	setHandler("demo.echo", func(ctx context.Context, payload string) (string, error) {
		return payload, nil
	})
	setHandler("demo.sleep", func(ctx context.Context, payload string) (string, error) {
		select {
		case <-time.After(time.Second):
			return fmt.Sprintf("slept 1s, payload=%s", payload), nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
	log.Println("Queue builtin handlers registered: demo.echo, demo.sleep")
}
