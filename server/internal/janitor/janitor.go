// Package janitor 承担后台补偿任务：删除照片时 COS 对象的异步清理（计划 §4.1）。
// 数据库已先删关联；这里仅重试删除对象，成功（含 404 幂等）后关闭任务，失败保留计数与错误。
package janitor

import (
	"context"
	"log/slog"
	"time"

	"keepsake/internal/cosutil"
	"keepsake/internal/store"
)

type Janitor struct {
	st       *store.Store
	cos      cosutil.Backend
	interval time.Duration
}

func New(st *store.Store, cos cosutil.Backend) *Janitor {
	return &Janitor{st: st, cos: cos, interval: time.Minute}
}

// Run 阻塞执行循环；ctx 取消即退出。启动时立即跑一轮，之后按 interval。
func (j *Janitor) Run(ctx context.Context) {
	logger := slog.Default()
	j.sweep(ctx, logger)
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.sweep(ctx, logger)
		}
	}
}

// sweep 处理 pending 任务单轮。单批上限 50，避免爬占磁盘与外呼配额。
func (j *Janitor) sweep(ctx context.Context, logger *slog.Logger) {
	tasks, err := j.st.PendingDeleteTasks(ctx, 50)
	if err != nil {
		logger.Error("janitor scan", "err", err)
		return
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}
		if derr := j.cos.Delete(task.COSKey); derr != nil {
			// 任意外部错误：记录一轮失败，等下一轮重试（计划 §4.1 补偿语义）。
			failureMsg := derr.Error()
			if len(failureMsg) > 200 {
				failureMsg = failureMsg[:200]
			}
			if ferr := j.st.RecordTaskFailure(ctx, task.ID, failureMsg); ferr != nil {
				logger.Error("janitor record failure", "task", task.ID, "err", ferr)
			}
			continue
		}
		if err := j.st.CompleteDeleteTask(ctx, task.ID); err != nil {
			logger.Error("janitor complete", "task", task.ID, "err", err)
		} else {
			logger.Info("cos object cleaned", "task", task.ID)
		}
	}
}
