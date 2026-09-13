// test-api は学習用のシンプルな TODO API サーバー。
// 外部ライブラリは使わず、標準ライブラリだけで組んでいる。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kakaz-one/test-api/internal/task"
)

func main() {
	// slog は Go 1.21 で標準入りした構造化ログ。
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	addr := ":" + envOr("PORT", "8080")

	mux := http.NewServeMux()
	task.NewHandler(task.NewStore()).Register(mux)

	srv := &http.Server{
		Addr:    addr,
		Handler: withLogging(mux),
		// タイムアウトを設定しないと、遅い（あるいは悪意のある）クライアントに
		// 接続を握られたままになる。本番サーバーでは必ず入れる項目。
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Ctrl-C（SIGINT）や SIGTERM を受け取ったら ctx がキャンセルされる。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ListenAndServe はブロックするので goroutine で動かし、
	// 結果はチャネル経由で受け取る。
	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", addr)
		// 正常な Shutdown 時は ErrServerClosed が返るので、これはエラー扱いしない。
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
		// 処理中のリクエストが終わるのを最大 10 秒待ってから終了する（graceful shutdown）。
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// withLogging は全リクエストのメソッド・パス・ステータス・所要時間を記録する
// ミドルウェア。「http.Handler を受け取って http.Handler を返す関数」という
// この形が Go のミドルウェアの定番パターン。
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start),
		)
	})
}

// statusRecorder は http.ResponseWriter をラップして、
// 書き込まれたステータスコードを覚えておくための型。
// 埋め込み（ResponseWriter）により、他のメソッドはそのまま使える。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// envOr は環境変数を読み、未設定ならデフォルト値を返す。
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
