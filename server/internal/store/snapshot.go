package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SnapshotTo 用 SQLite 的 VACUUM INTO 生成一致性数据库快照。
//
// 与直接复制 .db/-wal/-shm 不同，VACUUM INTO 在读取事务内生成紧凑副本，
// 备份期间业务写入不会被阻塞，快照也不依赖 WAL 伴生文件。
// dst 必须不存在；父目录会自动创建。写入失败时清理目标文件。
func (s *Store) SnapshotTo(ctx context.Context, dst string) error {
	if dst == "" {
		return fmt.Errorf("snapshot: destination is required")
	}
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("snapshot: destination %s already exists", dst)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("snapshot: stat destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("snapshot: create destination dir: %w", err)
	}

	// VACUUM INTO 的目标是 SQL 字面量；路径来自服务端配置与生成的文件名，
	// 这里只做单引号转义，避免拼接出非法 SQL。
	target := strings.ReplaceAll(dst, "'", "''")
	if _, err := s.DB.ExecContext(ctx, "VACUUM INTO '"+target+"'"); err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("snapshot: vacuum into: %w", err)
	}
	return nil
}
