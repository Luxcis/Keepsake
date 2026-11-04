// Package store 负责SQLite 连接管理与数据访问。
// 显式 SQL + 参数化查询（计划 §3 技术选型：避免过度 ORM）。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // 纯 Go 驱动，注册名为 "sqlite"，避免 CGO 跨平台问题
)

type Store struct {
	DB *sql.DB
}

// ErrNotFound：业务查询未命中（含“未发布对公共不可见”的情况）。
var ErrNotFound = errors.New("resource not found")

// Open 打开（或创建）SQLite 数据库，并启用计划要求的关键 pragma。
// WAL 提升读并发；busy_timeout 保证短事务写入竞争时不立刻报错；
// foreign_keys 打开级联与约束；synchronous=NORMAL 兼顾安全与性能。
func Open(ctx context.Context, dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// 计划 §3.2：连接池保守配置；SQLite 单机单写入节点。
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(0)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return &Store{DB: db}, nil
}

func (s *Store) Close() error { return s.DB.Close() }

// Ping 用于健康检查。
func (s *Store) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.DB.PingContext(ctx)
}
