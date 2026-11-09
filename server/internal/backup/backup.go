// Package backup 生成站点全量备份包：SQLite 一致性快照 + 上传对象 + 清单。
//
// 备份在后台 goroutine 中执行，HTTP 请求只负责启动任务；同一进程同时只允许
// 一个任务。备份先写入 .tmp 文件，成功后原子重命名，失败或进程重启时清理，
// 不会把半成品暴露为可用备份。
package backup

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"keepsake/internal/cosutil"
)

// ManifestSchema 是 manifest.json 的格式版本。
const ManifestSchema = "keepsake.backup.v1"

// 备份任务阶段，供管理端展示。
const (
	StageSnapshot = "snapshot"
	StageListing  = "listing"
	StagePacking  = "packing"
)

var (
	// ErrRunning 表示已有备份任务在执行。
	ErrRunning = errors.New("backup already running")
	// ErrInvalidName 表示备份名称不符合生成规则。
	ErrInvalidName = errors.New("invalid backup name")
	// ErrNotFound 表示备份文件不存在。
	ErrNotFound = errors.New("backup not found")
)

// 备份文件名由服务端生成：keepsake-backup-20060102-150405.zip（同秒冲突时追加 -2）。
var namePattern = regexp.MustCompile(`^keepsake-backup-[0-9]{8}-[0-9]{6}(-[0-9]+)?\.zip$`)

// Options 是 Service 的构造参数。
type Options struct {
	// Dir 是备份文件目录；必须位于持久卷上。
	Dir string
	// DBName 是包内数据库文件名（取配置的 DB_FILE，默认 journey.db）。
	DBName string
	// Env 是运行环境（dev/production），写入清单。
	Env string
	// Driver 是存储驱动（local/cos/none），写入清单。
	Driver string
	// Snapshot 生成数据库一致性快照；通常为 Store.SnapshotTo。
	Snapshot func(ctx context.Context, dst string) error
	// Objects 提供上传对象枚举与读取；nil 表示无文件可备份。
	Objects cosutil.ObjectSource
	// Now 可注入时钟，测试用。
	Now func() time.Time
	// Log 为结构化日志；为空时使用 slog.Default()。
	Log *slog.Logger
}

// Service 负责备份任务的启动、状态查询与文件管理。
type Service struct {
	dir      string
	dbName   string
	env      string
	driver   string
	snapshot func(ctx context.Context, dst string) error
	objects  cosutil.ObjectSource
	now      func() time.Time
	log      *slog.Logger

	mu      sync.Mutex
	running *job
	last    *LastResult
}

type job struct {
	name     string
	progress Progress
}

// Item 是备份列表中的一项。
type Item struct {
	Name      string    `json:"name"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

// Progress 是当前任务的进度快照。
type Progress struct {
	StartedAt        time.Time `json:"started_at"`
	Stage            string    `json:"stage"`
	ProcessedBytes   int64     `json:"processed_bytes"`
	TotalBytes       int64     `json:"total_bytes"`
	ProcessedObjects int       `json:"processed_objects"`
	TotalObjects     int       `json:"total_objects"`
}

// LastResult 保留最近一次任务的结果，供页面展示失败原因。
type LastResult struct {
	Name       string    `json:"name"`
	Status     string    `json:"status"` // completed / failed
	FinishedAt time.Time `json:"finished_at"`
	Error      string    `json:"error,omitempty"`
}

// Status 是管理端状态接口的响应体。
type Status struct {
	Running *Progress   `json:"running"`
	Last    *LastResult `json:"last"`
	Items   []Item      `json:"items"`
}

// Manifest 是备份包内的 manifest.json。
type Manifest struct {
	Schema        string               `json:"schema"`
	CreatedAt     time.Time            `json:"created_at"`
	AppEnv        string               `json:"app_env"`
	StorageDriver string               `json:"storage_driver"`
	Database      DatabaseInfo         `json:"database"`
	Files         FilesInfo            `json:"files"`
	Objects       []cosutil.ObjectInfo `json:"objects"`
	Excludes      []string             `json:"excludes"`
}

// DatabaseInfo 描述包内数据库文件。
type DatabaseInfo struct {
	File  string `json:"file"`
	Bytes int64  `json:"bytes"`
}

// FilesInfo 描述包内上传对象统计。
type FilesInfo struct {
	Prefix string `json:"prefix"`
	Count  int    `json:"count"`
	Bytes  int64  `json:"bytes"`
}

var defaultExcludes = []string{
	"Redis 会话与限流数据",
	".env 与环境/部署配置",
	"备份目录自身",
}

// New 创建备份服务，创建目录并清理上次进程残留的临时文件。
func New(o Options) (*Service, error) {
	if strings.TrimSpace(o.Dir) == "" {
		return nil, errors.New("backup: dir is required")
	}
	if o.Snapshot == nil {
		return nil, errors.New("backup: snapshot is required")
	}
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return nil, fmt.Errorf("backup: resolve dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("backup: create dir: %w", err)
	}
	dbName := filepath.Base(strings.TrimSpace(o.DBName))
	if dbName == "" || dbName == "." || dbName == string(filepath.Separator) {
		dbName = "journey.db"
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	log := o.Log
	if log == nil {
		log = slog.Default()
	}
	s := &Service{
		dir:      dir,
		dbName:   dbName,
		env:      o.Env,
		driver:   o.Driver,
		snapshot: o.Snapshot,
		objects:  o.Objects,
		now:      now,
		log:      log,
	}
	s.cleanupTemp()
	return s, nil
}

// Dir 返回备份目录的绝对路径。
func (s *Service) Dir() string { return s.dir }

// cleanupTemp 清理上次进程中断留下的临时快照与压缩包。
func (s *Service) cleanupTemp() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		s.log.Warn("backup: read dir for cleanup", "dir", s.dir, "err", err)
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), ".tmp-") {
			continue
		}
		p := filepath.Join(s.dir, e.Name())
		if err := os.Remove(p); err != nil {
			s.log.Warn("backup: remove stale temp file", "path", p, "err", err)
		} else {
			s.log.Info("backup: removed stale temp file", "path", p)
		}
	}
}

// Start 启动一次备份任务；已有任务运行时返回 ErrRunning。
func (s *Service) Start() (string, error) {
	s.mu.Lock()
	if s.running != nil {
		s.mu.Unlock()
		return "", ErrRunning
	}
	now := s.now()
	name := s.pickNameLocked(now)
	s.running = &job{
		name: name,
		progress: Progress{
			StartedAt: now.UTC(),
			Stage:     StageSnapshot,
		},
	}
	s.mu.Unlock()

	go s.execute(name)
	return name, nil
}

// pickNameLocked 生成不与现有文件冲突的备份名；调用方需持有 s.mu。
func (s *Service) pickNameLocked(now time.Time) string {
	base := "keepsake-backup-" + now.Format("20060102-150405")
	candidate := base + ".zip"
	for i := 2; ; i++ {
		_, err := os.Stat(filepath.Join(s.dir, candidate))
		if err == nil {
			candidate = fmt.Sprintf("%s-%d.zip", base, i)
			continue
		}
		if os.IsNotExist(err) {
			return candidate
		}
		// 无法确认是否存在（如权限错误）时不再猜测，交给写包阶段返回真实错误。
		return candidate
	}
}

// Status 返回当前任务、最近结果与备份列表。
func (s *Service) Status() (Status, error) {
	items, err := s.List()
	if err != nil {
		return Status{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Items: items}
	if s.running != nil {
		p := s.running.progress
		st.Running = &p
	}
	if s.last != nil {
		l := *s.last
		st.Last = &l
	}
	return st, nil
}

// List 扫描备份目录，按创建时间倒序返回合法备份文件。
func (s *Service) List() ([]Item, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("backup: list dir: %w", err)
	}
	items := make([]Item, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !namePattern.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("backup: stat %s: %w", e.Name(), err)
		}
		items = append(items, Item{Name: e.Name(), SizeBytes: info.Size(), CreatedAt: info.ModTime().UTC()})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].Name > items[j].Name
		}
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

// Open 打开一个备份文件供下载；调用方负责 Close。
func (s *Service) Open(name string) (*os.File, os.FileInfo, error) {
	p, err := s.resolve(name)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, fmt.Errorf("backup: open %s: %w", name, err)
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, ErrNotFound
	}
	return f, info, nil
}

// Delete 删除一个备份文件。
func (s *Service) Delete(name string) error {
	p, err := s.resolve(name)
	if err != nil {
		return err
	}
	info, err := os.Lstat(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("backup: stat %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return ErrNotFound
	}
	if err := os.Remove(p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("backup: delete %s: %w", name, err)
	}
	return nil
}

// resolve 只接受服务端生成的备份名，避免目录穿越。
func (s *Service) resolve(name string) (string, error) {
	if !namePattern.MatchString(name) {
		return "", ErrInvalidName
	}
	return filepath.Join(s.dir, name), nil
}

func (s *Service) execute(name string) {
	tmpZip := filepath.Join(s.dir, ".tmp-"+name)
	tmpDB := filepath.Join(s.dir, ".tmp-db-"+strings.TrimSuffix(name, ".zip")+".db")
	result := LastResult{Name: name, Status: "failed"}

	defer func() {
		_ = os.Remove(tmpDB)
		_ = os.Remove(tmpZip)
		if rec := recover(); rec != nil {
			result.Error = fmt.Sprintf("备份任务异常: %v", rec)
			s.log.Error("backup: panic", "name", name, "panic", rec)
		}
		result.FinishedAt = s.now().UTC()
		s.mu.Lock()
		s.running = nil
		s.last = &result
		s.mu.Unlock()
	}()

	ctx := context.Background()

	// 1) 数据库一致性快照。
	if err := s.snapshot(ctx, tmpDB); err != nil {
		result.Error = "生成数据库快照失败: " + err.Error()
		s.log.Error("backup: snapshot failed", "name", name, "err", err)
		return
	}
	dbInfo, err := os.Stat(tmpDB)
	if err != nil {
		result.Error = "读取数据库快照失败: " + err.Error()
		s.log.Error("backup: stat snapshot failed", "name", name, "err", err)
		return
	}

	// 2) 枚举上传对象（本地遍历 / COS 分页）。
	s.setStage(name, StageListing)
	var objects []cosutil.ObjectInfo
	if s.objects != nil {
		objects, err = s.objects.ListObjects(ctx)
		if err != nil {
			result.Error = "枚举上传文件失败: " + err.Error()
			s.log.Error("backup: list objects failed", "name", name, "err", err)
			return
		}
	}
	var totalBytes int64
	for _, obj := range objects {
		totalBytes += obj.Size
	}
	s.setTotals(name, len(objects), dbInfo.Size()+totalBytes)

	// 3) 打包：清单 + 数据库 + 上传对象。
	s.setStage(name, StagePacking)
	if err := s.writeArchive(ctx, name, tmpZip, tmpDB, dbInfo.Size(), objects); err != nil {
		result.Error = "生成备份包失败: " + err.Error()
		s.log.Error("backup: write archive failed", "name", name, "err", err)
		return
	}
	if err := os.Rename(tmpZip, filepath.Join(s.dir, name)); err != nil {
		result.Error = "保存备份文件失败: " + err.Error()
		s.log.Error("backup: rename archive failed", "name", name, "err", err)
		return
	}
	result.Status = "completed"
	s.log.Info("backup: completed", "name", name, "objects", len(objects), "bytes", dbInfo.Size()+totalBytes)
}

// writeArchive 将清单、数据库快照与全部对象写入 zip。
func (s *Service) writeArchive(ctx context.Context, name, dst, dbPath string, dbSize int64, objects []cosutil.ObjectInfo) (err error) {
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	defer func() {
		if cerr := zw.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close zip: %w", cerr)
		}
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close archive: %w", cerr)
		}
	}()

	manifestObjects := make([]cosutil.ObjectInfo, 0, len(objects))
	var filesBytes int64
	for _, obj := range objects {
		manifestObjects = append(manifestObjects, obj)
		filesBytes += obj.Size
	}
	manifest := Manifest{
		Schema:        ManifestSchema,
		CreatedAt:     s.now().UTC(),
		AppEnv:        s.env,
		StorageDriver: s.driver,
		Database:      DatabaseInfo{File: s.dbName, Bytes: dbSize},
		Files:         FilesInfo{Prefix: "uploads/", Count: len(objects), Bytes: filesBytes},
		Objects:       manifestObjects,
		Excludes:      append([]string(nil), defaultExcludes...),
	}
	header, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := writeZipBytes(zw, "manifest.json", header, zip.Deflate); err != nil {
		return err
	}

	if err := copyZipFile(zw, s.dbName, dbPath, zip.Deflate); err != nil {
		return err
	}
	s.addProgress(name, dbSize, 0)

	for _, obj := range objects {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry, err := objectEntryName(obj.Key)
		if err != nil {
			return err
		}
		rc, err := s.objects.OpenObject(ctx, obj.Key)
		if err != nil {
			return fmt.Errorf("读取对象 %s: %w", obj.Key, err)
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: entry, Method: zip.Store, Modified: s.now()})
		if err != nil {
			_ = rc.Close()
			return fmt.Errorf("创建对象条目 %s: %w", obj.Key, err)
		}
		n, copyErr := io.Copy(w, rc)
		closeErr := rc.Close()
		if copyErr != nil {
			return fmt.Errorf("写入对象 %s: %w", obj.Key, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("关闭对象 %s: %w", obj.Key, closeErr)
		}
		s.addProgress(name, n, 1)
	}
	return nil
}

// objectEntryName 把对象 key 映射为包内路径，并拒绝目录穿越。
func objectEntryName(key string) (string, error) {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return "", fmt.Errorf("非法对象 key %q", key)
	}
	clean := path.Clean(key)
	if clean != key || clean == "." || clean == ".." {
		return "", fmt.Errorf("非法对象 key %q", key)
	}
	for _, seg := range strings.Split(clean, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("非法对象 key %q", key)
		}
	}
	return "uploads/" + clean, nil
}

func writeZipBytes(zw *zip.Writer, name string, data []byte, method uint16) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

func copyZipFile(zw *zip.Writer, name, src string, method uint16) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method})
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	if _, err := io.Copy(w, f); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

func (s *Service) setStage(name, stage string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running != nil && s.running.name == name {
		s.running.progress.Stage = stage
	}
}

func (s *Service) setTotals(name string, objects int, bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running != nil && s.running.name == name {
		s.running.progress.TotalObjects = objects
		s.running.progress.TotalBytes = bytes
	}
}

func (s *Service) addProgress(name string, bytes int64, objects int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running != nil && s.running.name == name {
		s.running.progress.ProcessedBytes += bytes
		s.running.progress.ProcessedObjects += objects
	}
}
