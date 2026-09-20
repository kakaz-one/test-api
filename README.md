# test-api

GoでAPIを作ってみる — 標準ライブラリだけで書いた学習用の TODO API。

外部ライブラリはゼロ（`go.mod` に依存なし）。データはメモリ上に持つので、サーバーを再起動すると消えます。

## 起動

```bash
go run .            # http://localhost:8080 で待ち受け
PORT=3000 go run .  # ポートを変える
```

## テスト

```bash
go test ./...        # テスト実行
go test -v ./...     # どのテストが走ったか表示
go vet ./...         # 静的チェック
gofmt -l .           # 整形されていないファイルを列挙（何も出なければ OK）
```

## エンドポイント

| メソッド | パス | 説明 | 成功時 |
| --- | --- | --- | --- |
| GET | `/health` | 死活確認 | 200 |
| GET | `/tasks` | 一覧取得 | 200 |
| POST | `/tasks` | 作成 | 201 + `Location` ヘッダ |
| GET | `/tasks/{id}` | 1 件取得 | 200 |
| PATCH | `/tasks/{id}` | 部分更新 | 200 |
| DELETE | `/tasks/{id}` | 削除 | 204（ボディ無し） |

タスクの JSON:

```json
{ "id": 1, "title": "Go を学ぶ", "done": false, "created_at": "2026-09-12T13:16:02.056Z" }
```

エラー時は `{"error": "メッセージ"}` を返します（400 / 404 / 405 / 500）。

## 使い方（curl）

```bash
# 作成
curl -i -X POST localhost:8080/tasks -d '{"title":"Go を学ぶ"}'

# 一覧
curl localhost:8080/tasks

# 1 件取得
curl localhost:8080/tasks/1

# 完了にする（指定しなかった項目は変わらない）
curl -X PATCH localhost:8080/tasks/1 -d '{"done":true}'

# タイトルを変える
curl -X PATCH localhost:8080/tasks/1 -d '{"title":"Go を極める"}'

# 削除
curl -i -X DELETE localhost:8080/tasks/1
```

## ファイル構成

```
main.go                        サーバー起動、ミドルウェア、graceful shutdown
internal/task/task.go          Task 構造体とバリデーション
internal/task/store.go         メモリ上の保存処理（sync.RWMutex で排他）
internal/task/handler.go       HTTP ハンドラ、ルーティング、JSON 入出力
internal/task/handler_test.go  ハンドラのテスト
```

`internal/` 配下のパッケージは、このモジュールの外からは import できないという Go のルールがあります。外部に公開したくない実装を置く場所です。

## コードの中で見どころになるポイント

- **ルーティング** — Go 1.22 から `http.ServeMux` が `"GET /tasks/{id}"` の形式を書けるようになり、パス変数は `r.PathValue("id")` で取れます。外部ルーターなしでこの規模の API は組めます。
- **PATCH とポインタ** — `updateRequest` の項目は `*string` / `*bool`。ポインタにすると「キーが無かった（nil）」と「明示的に `false` が来た」を区別できます。値型だとゼロ値と区別がつきません。
- **排他制御** — HTTP ハンドラは複数の goroutine から同時に呼ばれるため、`map` をそのまま触ると data race になります。`go test -race ./...` で検出できます。
- **エラーの扱い** — ドメイン層は `ErrNotFound` などの値を返し、`errors.Is` で判定して HTTP ステータスへ翻訳します（`writeStoreError`）。ドメイン層が HTTP を知らずに済む形。
- **ミドルウェア** — 「`http.Handler` を受け取って `http.Handler` を返す関数」が Go の定番パターン（`withLogging`）。
- **テスト** — `httptest.NewRequest` + `httptest.NewRecorder` で、ポートを開かずにハンドラを直接叩けます。`TestErrorResponses` は Go で定番のテーブルドリブンテスト。

## 次にやると学びが増えること

1. `go test -race ./...` を試す（`Store` の mutex を外すと何が起きるか見る）
2. `GET /tasks?done=true` のような絞り込みクエリを足す
3. 保存先をメモリから SQLite やファイルに差し替える（`Store` をインターフェースにする練習）
4. `PUT /tasks/{id}`（全項目の置き換え）を実装して PATCH との違いを体感する
5. 認証ヘッダを見るミドルウェアを足す
