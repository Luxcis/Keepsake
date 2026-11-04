package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate 按文件名顺序执行未应用的迁移脚本，并记录到 _migrations。
// 简单可靠的一维版本管理：文件名即版本（0001_init.sql ...）。
func (s *Store) Migrate(ctx context.Context) (applied int, err error) {
	if _, err = s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS _migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return 0, fmt.Errorf("ensure _migrations: %w", err)
	}

	entries, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return 0, fmt.Errorf("glob migrations: %w", err)
	}
	sort.Strings(entries)

	rows, err := s.DB.QueryContext(ctx, `SELECT version FROM _migrations`)
	if err != nil {
		return 0, err
	}
	done := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return 0, err
		}
		done[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, name := range entries {
		version := strings.TrimPrefix(strings.TrimPrefix(name, "migrations/"), "")
		if done[version] {
			continue
		}
		raw, err := migrationsFS.ReadFile(name)
		if err != nil {
			return applied, fmt.Errorf("read %s: %w", name, err)
		}
		for i, stmt := range splitSQL(string(raw)) {
			if stmt == "" {
				continue
			}
			if _, err := s.DB.ExecContext(ctx, stmt); err != nil {
				return applied, fmt.Errorf("apply %s (statement %d): %w", name, i+1, err)
			}
		}
		if _, err := s.DB.ExecContext(ctx,
			`INSERT INTO _migrations (version, applied_at) VALUES (?, ?)`,
			version, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return applied, fmt.Errorf("record %s: %w", name, err)
		}
		applied++
	}
	return applied, nil
}

// splitSQL 把迁移脚本拆成单条语句。
// 我们的迁移 SQL 由仓库自己维护，只需正确处理单双引号字符串与 -- 注释。
func splitSQL(script string) []string {
	var stmts []string
	var b strings.Builder
	inS, inD := false, false // '...' 与 "..." 内的引号状态
	flush := func() {
		if s := strings.TrimSpace(b.String()); s != "" {
			stmts = append(stmts, s)
		}
		b.Reset()
	}
	lines := strings.Split(script, "\n")
	for _, ln := range lines {
		// 行级 -- 注释整行忽略；语句内不允许出现内联注释（保持迁移文件简单）。
		trimmed := strings.TrimSpace(ln)
		if strings.HasPrefix(trimmed, "--") {
			continue
		}
		for _, r := range ln {
			switch {
			case r == '\'' && !inD:
				inS = !inS
			case r == '"' && !inS:
				inD = !inD
			case r == ';' && !inS && !inD:
				flush()
				continue
			}
			b.WriteRune(r)
		}
		b.WriteByte('\n')
	}
	flush()
	return stmts
}
