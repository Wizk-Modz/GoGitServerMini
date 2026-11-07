// GoGitServerMini là git server tự host tối giản: Smart HTTP + quản lý repo qua web.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"gogitservermini/internal/metastore"
	"gogitservermini/internal/repostore"
	"gogitservermini/internal/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("máy chủ dừng do lỗi", "err", err)
		os.Exit(1)
	}
}

// run đọc cờ, dựng handler rồi chạy HTTP server tới khi nhận tín hiệu dừng.
func run() error {
	bind := flag.String("b", "127.0.0.1", "địa chỉ IP lắng nghe")
	port := flag.Int("p", 8080, "cổng lắng nghe")
	reposDir := flag.String("repos", "./repos", "thư mục chứa các bare repo")
	metaFile := flag.String("metadata", "", "file JSON lưu metadata repo (mặc định <repos>/metadata.json)")
	maxBody := flag.Int64("max-body", 512<<20, "giới hạn kích thước body cho mỗi request git (byte)")
	flag.Parse()

	if *port < 1 || *port > 65535 {
		return fmt.Errorf("cổng không hợp lệ: %d", *port)
	}
	// Mặc định đặt metadata cạnh các repo để đổi -repos là đổi cả hai.
	if *metaFile == "" {
		*metaFile = filepath.Join(*reposDir, "metadata.json")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	store, err := repostore.New(*reposDir)
	if err != nil {
		return err
	}
	meta, err := metastore.Open(*metaFile)
	if err != nil {
		return err
	}
	handler, err := web.New(store, meta, *maxBody, logger)
	if err != nil {
		return fmt.Errorf("khởi tạo web: %w", err)
	}

	addr := net.JoinHostPort(*bind, strconv.Itoa(*port))
	server := &http.Server{
		Addr:              addr,
		Handler:           logRequests(logger, handler.Routes()),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("máy chủ đang chạy", "url", "http://"+addr, "repos", store.Root, "metadata", meta.Path())
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
		return nil
	case <-ctx.Done():
		logger.Info("đang dừng máy chủ")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("tắt máy chủ: %w", err)
	}
	return nil
}

// logRequests ghi log method, đường dẫn, status và thời gian xử lý.
func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start).Round(time.Millisecond),
		)
	})
}

// statusRecorder ghi nhớ status code đã ghi ra client.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

// WriteHeader ghi status một lần duy nhất.
func (s *statusRecorder) WriteHeader(code int) {
	if s.written {
		return
	}
	s.status = code
	s.written = true
	s.ResponseWriter.WriteHeader(code)
}

// Write đảm bảo status mặc định 200 được ghi nhận.
func (s *statusRecorder) Write(p []byte) (int, error) {
	if !s.written {
		s.WriteHeader(http.StatusOK)
	}
	return s.ResponseWriter.Write(p)
}

// Flush chuyển tiếp khả năng flush cho streaming.
func (s *statusRecorder) Flush() {
	if flusher, ok := s.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Unwrap cho phép http.ResponseController truy cập writer gốc.
func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}
