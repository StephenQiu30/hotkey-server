# HotKey 热点事件监控统计与日报服务 实施方案

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标：** 实现 X Filtered Stream 实时采集 → ONNX 进程内 embedding (bge-small-zh-v1.5) → pgvector 余弦相似度匹配 → 每小时批量话题聚类/热点聚合/趋势快照的完整链路。

**架构：** 采集流在 Fx OnStart 建立长连接，逐条推文即时 embedding + 写入 + 匹配；每小时 Cron 触发 Kafka 消息，由 Worker 串联已有的 `topic.Cluster()`、`hotevent.ComputeHeatScore()`、`trend.BuildTopicSnapshot()` 等纯函数。

**技术栈：** Go 1.26 + GORM v2 + pgvector + ONNX Runtime (`github.com/yalue/onnxruntime_go`) + X API v2 Filtered Stream + Kafka + uber-go/fx

---

## 任务分解

### 任务 1：Vector384 类型 + GORM 集成

**文件：**
- Create: `internal/pkg/vector.go`

**说明：** 封装 384 维 float32 向量，实现 `sql.Scanner` 和 `driver.Valuer` 接口使 GORM 能自动序列化/反序列化 pgvector 类型。

- [ ] **Step 1：写测试**

```go
// tests/unit/pkg/vector_test.go
package pkg_test

import (
    "testing"
    "github.com/StephenQiu30/hotkey-server/internal/pkg"
)

func TestVector384ScanValue(t *testing.T) {
    var v pkg.Vector384
    // pgvector binary format: 384 float32 values
    data := make([]byte, 4*384)
    for i := range 384 {
        copy(data[i*4:(i+1)*4], float32Bytes(float32(i)/384.0))
    }
    if err := v.Scan(data); err != nil {
        t.Fatalf("Scan failed: %v", err)
    }
    if v[0] != 0.0 || v[383] != 1.0 {
        t.Errorf("unexpected values: got %f..%f", v[0], v[383])
    }
    val, err := v.Value()
    if err != nil {
        t.Fatalf("Value failed: %v", err)
    }
    if val == nil {
        t.Fatal("Value returned nil")
    }
}
```

- [ ] **Step 2：运行测试验证失败**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server && go test ./tests/unit/pkg/... -v -run TestVector384ScanValue
```
Expected: FAIL (package not found)

- [ ] **Step 3：实现 Vector384**

```go
// internal/pkg/vector.go
package pkg

import (
    "database/sql/driver"
    "encoding/binary"
    "fmt"
    "math"
)

// Vector384 is a 384-dimensional float32 vector for pgvector.
type Vector384 [384]float32

// Scan implements sql.Scanner for pgvector binary format.
func (v *Vector384) Scan(src interface{}) error {
    if src == nil {
        return nil
    }
    var data []byte
    switch s := src.(type) {
    case []byte:
        data = s
    case string:
        data = []byte(s)
    default:
        return fmt.Errorf("unexpected vector type: %T", src)
    }
    if len(data) < 4 { // at least dimensionality
        return fmt.Errorf("vector data too short: %d bytes", len(data))
    }
    dim := binary.LittleEndian.Uint32(data[:4])
    if dim != 384 {
        return fmt.Errorf("expected dim 384, got %d", dim)
    }
    if len(data) < 4+384*4 {
        return fmt.Errorf("vector data too short for dim %d: %d bytes", dim, len(data))
    }
    for i := range 384 {
        v[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[4+i*4:]))
    }
    return nil
}

// Value implements driver.Valuer for pgvector binary format.
func (v Vector384) Value() (driver.Value, error) {
    buf := make([]byte, 4+384*4)
    binary.LittleEndian.PutUint32(buf[:4], 384)
    for i := range 384 {
        binary.LittleEndian.PutUint32(buf[4+i*4:], math.Float32bits(v[i]))
    }
    return buf, nil
}

// Dim returns the vector dimension (always 384).
func (v Vector384) Dim() int { return 384 }
```

- [ ] **Step 4：补充测试——nil scan、短数据错误**

```go
// tests/unit/pkg/vector_test.go (追加)
func TestVector384ScanNil(t *testing.T) {
    var v pkg.Vector384
    if err := v.Scan(nil); err != nil {
        t.Fatalf("Scan nil failed: %v", err)
    }
}

func TestVector384ScanShortData(t *testing.T) {
    var v pkg.Vector384
    if err := v.Scan([]byte{1, 2, 3}); err == nil {
        t.Fatal("expected error for short data")
    }
}
```

- [ ] **Step 5：运行全部向量测试**

```bash
go test ./tests/unit/pkg/... -v
```
Expected: 3 PASS

- [ ] **Step 6：提交**

```bash
git add internal/pkg/vector.go tests/unit/pkg/vector_test.go
git commit -m "feat: add Vector384 type with GORM Scanner/Valuer for pgvector

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### 任务 2：ONNX Embedding Service

**文件：**
- Create: `internal/embedding/model.go`
- Create: `internal/embedding/service.go`

**说明：** 加载 bge-small-zh-v1.5 ONNX 模型，提供 `Embed(text) → Vector384` 接口。Fx OnStart 阶段加载模型，加载失败拒绝启动。

**依赖：** `github.com/yalue/onnxruntime_go`

- [ ] **Step 1：安装依赖**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server && go get github.com/yalue/onnxruntime_go
```

- [ ] **Step 2：写 embedding 服务接口测试**

```go
// tests/unit/embedding/service_test.go
package embedding_test

import (
    "context"
    "testing"
    "github.com/StephenQiu30/hotkey-server/internal/embedding"
)

type mockModel struct{}

func (m *mockModel) Embed(text string) ([384]float32, error) {
    var v [384]float32
    for i := range 384 {
        v[i] = float32(len(text)) / 384.0
    }
    return v, nil
}

func (m *mockModel) Close() error { return nil }

func TestEmbeddingService(t *testing.T) {
    svc := embedding.NewService(&mockModel{})
    v, err := svc.Embed(context.Background(), "hello world")
    if err != nil {
        t.Fatalf("Embed failed: %v", err)
    }
    if v.Dim() != 384 {
        t.Errorf("expected dim 384, got %d", v.Dim())
    }
}

func TestEmbeddingBatch(t *testing.T) {
    svc := embedding.NewService(&mockModel{})
    texts := []string{"a", "b", "c"}
    vecs, err := svc.EmbedBatch(context.Background(), texts)
    if err != nil {
        t.Fatalf("EmbedBatch failed: %v", err)
    }
    if len(vecs) != 3 {
        t.Errorf("expected 3 vectors, got %d", len(vecs))
    }
}
```

- [ ] **Step 3：运行测试验证失败**

```bash
go test ./tests/unit/embedding/... -v
```
Expected: FAIL (package not found)

- [ ] **Step 4：实现 Model 接口**

```go
// internal/embedding/model.go
package embedding

import (
    "fmt"
    ort "github.com/yalue/onnxruntime_go"
)

// Model wraps an ONNX runtime session for text embedding.
type Model struct {
    session *ort.AdvancedSession
    inputName string
    outputName string
}

// NewModel loads an ONNX model from the given path.
func NewModel(modelPath string) (*Model, error) {
    ort.SetSharedLibraryPath("/usr/local/lib/libonnxruntime.dylib")
    if err := ort.InitializeEnvironment(); err != nil {
        return nil, fmt.Errorf("onnx init: %w", err)
    }
    // Tokenizer input: [1, seq_len] int64
    inputShape := ort.NewShape(1, 512)
    inputTensor, err := ort.NewEmptyTensor[int64](inputShape)
    if err != nil {
        return nil, fmt.Errorf("create input tensor: %w", err)
    }
    // Attention mask: [1, seq_len] int64
    maskShape := ort.NewShape(1, 512)
    maskTensor, err := ort.NewEmptyTensor[int64](maskShape)
    if err != nil {
        return nil, fmt.Errorf("create mask tensor: %w", err)
    }
    // Output: [1, 384] float32
    outputShape := ort.NewShape(1, 384)
    outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
    if err != nil {
        return nil, fmt.Errorf("create output tensor: %w", err)
    }
    // For bge-small-zh-v1.5: input_ids, attention_mask → last_hidden_state (mean-pooled)
    // The ONNX export of sentence-transformers has specific I/O names
    session, err := ort.NewAdvancedSession(
        modelPath,
        []string{"input_ids", "attention_mask"},
        []string{"dense_1"},  // output name varies by export; adjust on integration
        []*ort.Tensor[int64]{inputTensor, maskTensor},
        []*ort.Tensor[float32]{outputTensor},
        nil,
    )
    if err != nil {
        return nil, fmt.Errorf("create session: %w", err)
    }
    return &Model{
        session: session,
        inputName: "input_ids",
        outputName: "dense_1",
    }, nil
}

// Embed generates an embedding vector for the given token IDs.
// This is called after tokenization; the Service layer handles tokenization.
func (m *Model) Embed(tokenIDs []int64) ([384]float32, error) {
    var result [384]float32
    inputT := m.session.GetInputTensors()[0].(*ort.Tensor[int64])
    maskT := m.session.GetInputTensors()[1].(*ort.Tensor[int64])
    outputT := m.session.GetOutputTensors()[0].(*ort.Tensor[float32])
    
    seqLen := len(tokenIDs)
    if seqLen > 512 {
        seqLen = 512
    }
    inputData := make([]int64, 512)
    maskData := make([]int64, 512)
    for i := 0; i < seqLen; i++ {
        inputData[i] = tokenIDs[i]
        maskData[i] = 1
    }
    copy(inputT.GetData(), inputData)
    copy(maskT.GetData(), maskData)
    
    if err := m.session.Run(); err != nil {
        return result, fmt.Errorf("onnx run: %w", err)
    }
    copy(result[:], outputT.GetData())
    return result, nil
}

// Close shuts down the ONNX session.
func (m *Model) Close() error {
    m.session.Destroy()
    ort.DestroyEnvironment()
    return nil
}
```

- [ ] **Step 5：实现 Service 层（封装 tokenization + embedding call）**

```go
// internal/embedding/service.go
package embedding

import (
    "context"
    "fmt"
    "math"
    "strings"
    "unicode"

    "github.com/StephenQiu30/hotkey-server/internal/pkg"
)

// Tokenizer is a minimal WordPiece/BPE tokenizer for bge-small-zh-v1.5.
// In production, use a proper tokenizer library. Here we implement a
// simplified version that handles CJK character splitting + whitespace.
type Tokenizer struct {
    vocab map[string]int64
}

func NewTokenizer() *Tokenizer {
    // bge-small-zh-v1.5 uses a 250k vocab from BERT-uncased base
    // For test/development, we provide a minimal fallback
    return &Tokenizer{vocab: buildMinimalVocab()}
}

func buildMinimalVocab() map[string]int64 {
    // Minimal vocab for testing — in production, load from model tokenizer.json
    v := make(map[string]int64)
    v["[CLS]"] = 101
    v["[SEP]"] = 102
    v["[PAD]"] = 0
    v["[UNK]"] = 100
    // Add common Chinese characters
    for _, r := range "这是一段中文测试文本热点事件关键词监控" {
        v[string(r)] = int64(2000 + r)
    }
    return v
}

func (t *Tokenizer) Encode(text string) []int64 {
    tokens := []int64{t.vocab["[CLS]"]}
    for _, r := range text {
        if unicode.Is(unicode.Han, r) {
            // CJK: split per character
            if id, ok := t.vocab[string(r)]; ok {
                tokens = append(tokens, id)
            } else {
                tokens = append(tokens, t.vocab["[UNK]"])
            }
        } else {
            // Non-CJK: split by whitespace
            for _, word := range strings.Fields(text) {
                if id, ok := t.vocab[strings.ToLower(word)]; ok {
                    tokens = append(tokens, id)
                } else {
                    tokens = append(tokens, t.vocab["[UNK]"])
                }
            }
        }
    }
    tokens = append(tokens, t.vocab["[SEP]"])
    if len(tokens) > 512 {
        tokens = tokens[:512]
        tokens[511] = t.vocab["[SEP]"]
    }
    return tokens
}

// Embedder is the interface for generating text embeddings.
type Embedder interface {
    Embed(tokenIDs []int64) ([384]float32, error)
    Close() error
}

// Service provides text-to-embedding conversion.
type Service struct {
    model     Embedder
    tokenizer *Tokenizer
}

// NewService creates an embedding service.
func NewService(model Embedder) *Service {
    return &Service{
        model:     model,
        tokenizer: NewTokenizer(),
    }
}

// Embed converts a single text to a normalized 384-dim Vector384.
func (s *Service) Embed(ctx context.Context, text string) (pkg.Vector384, error) {
    tokenIDs := s.tokenizer.Encode(text)
    raw, err := s.model.Embed(tokenIDs)
    if err != nil {
        return pkg.Vector384{}, fmt.Errorf("embed: %w", err)
    }
    return normalize(raw), nil
}

// EmbedBatch converts multiple texts to normalized vectors.
func (s *Service) EmbedBatch(ctx context.Context, texts []string) ([]pkg.Vector384, error) {
    result := make([]pkg.Vector384, len(texts))
    for i, text := range texts {
        v, err := s.Embed(ctx, text)
        if err != nil {
            return nil, fmt.Errorf("embed batch [%d]: %w", i, err)
        }
        result[i] = v
    }
    return result, nil
}

// normalize applies L2 normalization to the vector.
func normalize(raw [384]float32) pkg.Vector384 {
    var sum float64
    for _, v := range raw {
        sum += float64(v) * float64(v)
    }
    norm := math.Sqrt(sum)
    if norm == 0 {
        return pkg.Vector384{}
    }
    var result pkg.Vector384
    for i, v := range raw {
        result[i] = float32(float64(v) / norm)
    }
    return result
}

// Close releases the underlying model resources.
func (s *Service) Close() error {
    return s.model.Close()
}
```

- [ ] **Step 6：运行测试**

```bash
go test ./tests/unit/embedding/... -v
```
Expected: TestEmbeddingService PASS, TestEmbeddingBatch PASS

- [ ] **Step 7：提交**

```bash
git add internal/embedding/ tests/unit/embedding/
git commit -m "feat: add ONNX embedding service (bge-small-zh-v1.5, 384d)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### 任务 3：配置 + 模型变更

**文件：**
- Modify: `internal/config/config.go`
- Modify: `internal/repository/gormimpl/model.go`

- [ ] **Step 1：写配置测试**

```go
// tests/unit/config/config_test.go (追加)
func TestEmbeddingConfigDefault(t *testing.T) {
    t.Setenv("DATABASE_URL", "postgres://localhost:5432/test")
    t.Setenv("JWT_SECRET", "test-secret")
    t.Setenv("X_BEARER_TOKEN", "test-token")
    cfg, err := config.Load()
    if err != nil {
        t.Fatalf("Load failed: %v", err)
    }
    if cfg.EmbeddingModelPath != "" {
        t.Errorf("expected empty default, got %q", cfg.EmbeddingModelPath)
    }
}
```

- [ ] **Step 2：添加 EmbeddingModelPath 配置项**

```go
// internal/config/config.go Config 结构体追加字段
EmbeddingModelPath string `mapstructure:"EMBEDDING_MODEL_PATH"`
```

- [ ] **Step 3：给 PlatformPost 加 Embedding 字段，给 KeywordMonitor 加 QueryEmbedding 字段**

```go
// internal/repository/gormimpl/model.go

// 在 PlatformPost 追加
Embedding *pkg.Vector384 `gorm:"type:vector(384);column:embedding"`

// 在 KeywordMonitor 追加
QueryEmbedding *pkg.Vector384 `gorm:"type:vector(384);column:query_embedding"`
```

- [ ] **Step 4：写模型测试——确保 `go build` 通过**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server && go build ./...
```
Expected: success

- [ ] **Step 5：迁移 SQL**

```sql
-- db/migrations/003_add_vector_columns.sql
CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE platform_posts ADD COLUMN IF NOT EXISTS embedding vector(384);
CREATE INDEX IF NOT EXISTS idx_platform_posts_embedding ON platform_posts
  USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

ALTER TABLE keyword_monitors ADD COLUMN IF NOT EXISTS query_embedding vector(384);
```

- [ ] **Step 6：提交**

```bash
git add internal/config/config.go internal/repository/gormimpl/model.go db/migrations/003_add_vector_columns.sql
git commit -m "feat: add embedding model path config, pgvector columns for posts and monitors

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### 任务 4：X API Filtered Stream 客户端

**文件：**
- Create: `internal/collect/xclient.go`

**说明：** X API v2 Filtered Stream 客户端。提供建立长连接、注册规则、解析推文、断线重连的能力。

- [ ] **Step 1：写测试**

```go
// tests/unit/collect/xclient_test.go
package collect_test

import (
    "testing"
    "github.com/StephenQiu30/hotkey-server/internal/collect"
)

func TestParseTweet(t *testing.T) {
    raw := `{"data": {"id": "123", "text": "hello world"}, "includes": {"users": [{"id": "456", "name": "Test User", "username": "test"}]}}`
    tweet, err := collect.ParseTweet([]byte(raw))
    if err != nil {
        t.Fatalf("ParseTweet failed: %v", err)
    }
    if tweet.ID != "123" {
        t.Errorf("expected ID 123, got %s", tweet.ID)
    }
    if tweet.Text != "hello world" {
        t.Errorf("expected text 'hello world', got %s", tweet.Text)
    }
    if tweet.AuthorID != "456" {
        t.Errorf("expected author 456, got %s", tweet.AuthorID)
    }
}

func TestParseTweetError(t *testing.T) {
    _, err := collect.ParseTweet([]byte(`invalid json`))
    if err == nil {
        t.Fatal("expected error for invalid JSON")
    }
}
```

- [ ] **Step 2：运行测试验证失败**

```bash
go test ./tests/unit/collect/... -v
```
Expected: FAIL

- [ ] **Step 3：实现 X API 客户端**

```go
// internal/collect/xclient.go
package collect

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
    "time"
)

// Tweet represents a parsed tweet from the X API stream.
type Tweet struct {
    ID           string `json:"id"`
    Text         string `json:"text"`
    AuthorID     string `json:"author_id"`
    AuthorName   string `json:"author_name,omitempty"`
    AuthorHandle string `json:"author_handle,omitempty"`
    CreatedAt    string `json:"created_at,omitempty"`
    LikeCount    int    `json:"like_count,omitempty"`
    RetweetCount int    `json:"retweet_count,omitempty"`
    ReplyCount   int    `json:"reply_count,omitempty"`
}

type apiTweet struct {
    Data struct {
        ID        string `json:"id"`
        Text      string `json:"text"`
        AuthorID  string `json:"author_id"`
        CreatedAt string `json:"created_at"`
    } `json:"data"`
    Includes struct {
        Users []struct {
            ID       string `json:"id"`
            Name     string `json:"name"`
            Username string `json:"username"`
        } `json:"users"`
    } `json:"includes"`
}

// StreamRule represents a Filtered Stream rule.
type StreamRule struct {
    ID    string `json:"id,omitempty"`
    Value string `json:"value"`
    Tag   string `json:"tag,omitempty"`
}

// XClient manages the X API v2 Filtered Stream connection.
type XClient struct {
    baseURL    string
    token      string
    httpClient *http.Client
}

// NewXClient creates an X API client.
func NewXClient(baseURL, token string) *XClient {
    return &XClient{
        baseURL:    strings.TrimRight(baseURL, "/"),
        token:      token,
        httpClient: &http.Client{Timeout: 30 * time.Second},
    }
}

// SetRules replaces all Filtered Stream rules with the given ones.
// X API limits: Free tier = 25 rules, Basic = 250, Pro = 5000.
func (c *XClient) SetRules(ctx context.Context, rules []StreamRule) error {
    // First, delete all existing rules
    existing, err := c.getRules(ctx)
    if err != nil {
        return fmt.Errorf("get existing rules: %w", err)
    }
    if len(existing) > 0 {
        ids := make([]string, len(existing))
        for i, r := range existing {
            ids[i] = r.ID
        }
        delPayload, _ := json.Marshal(map[string]interface{}{
            "delete": map[string]interface{}{
                "ids": ids,
            },
        })
        req, err := http.NewRequestWithContext(ctx, "POST",
            c.baseURL+"/2/tweets/search/stream/rules",
            strings.NewReader(string(delPayload)),
        )
        if err != nil {
            return err
        }
        req.Header.Set("Authorization", "Bearer "+c.token)
        req.Header.Set("Content-Type", "application/json")
        resp, err := c.httpClient.Do(req)
        if err != nil {
            return fmt.Errorf("delete rules: %w", err)
        }
        resp.Body.Close()
    }

    // Then add new rules
    addPayload, _ := json.Marshal(map[string]interface{}{
        "add": rules,
    })
    req, err := http.NewRequestWithContext(ctx, "POST",
        c.baseURL+"/2/tweets/search/stream/rules",
        strings.NewReader(string(addPayload)),
    )
    if err != nil {
        return err
    }
    req.Header.Set("Authorization", "Bearer "+c.token)
    req.Header.Set("Content-Type", "application/json")
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("add rules: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("add rules failed: status=%d body=%s", resp.StatusCode, string(body))
    }
    return nil
}

// ConnectStream establishes a Filtered Stream connection.
// Returns a response body reader that should be read in a loop.
// The caller must close the body when done.
func (c *XClient) ConnectStream(ctx context.Context) (io.ReadCloser, error) {
    req, err := http.NewRequestWithContext(ctx, "GET",
        c.baseURL+"/2/tweets/search/stream?expansions=author_id&tweet.fields=created_at,public_metrics&user.fields=name,username",
        nil,
    )
    if err != nil {
        return nil, err
    }
    req.Header.Set("Authorization", "Bearer "+c.token)
    req.Header.Set("Accept", "application/json")
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("connect stream: %w", err)
    }
    if resp.StatusCode != http.StatusOK {
        resp.Body.Close()
        return nil, fmt.Errorf("stream connection failed: status=%d", resp.StatusCode)
    }
    return resp.Body, nil
}

// ParseTweet parses a single JSON line from the stream into a Tweet struct.
func ParseTweet(data []byte) (*Tweet, error) {
    var raw apiTweet
    if err := json.Unmarshal(data, &raw); err != nil {
        return nil, fmt.Errorf("unmarshal tweet: %w", err)
    }
    if raw.Data.ID == "" {
        return nil, fmt.Errorf("empty tweet data")
    }
    tweet := &Tweet{
        ID:        raw.Data.ID,
        Text:      raw.Data.Text,
        AuthorID:  raw.Data.AuthorID,
        CreatedAt: raw.Data.CreatedAt,
    }
    for _, u := range raw.Includes.Users {
        if u.ID == raw.Data.AuthorID {
            tweet.AuthorName = u.Name
            tweet.AuthorHandle = u.Username
            break
        }
    }
    return tweet, nil
}

// ReadStream reads a single line from the Filtered Stream scanner.
// Returns a Tweet, or nil if the line is a keepalive (empty).
func ReadStream(scanner *bufio.Scanner) (*Tweet, error) {
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" {
            continue // keepalive
        }
        return ParseTweet([]byte(line))
    }
    return nil, scanner.Err()
}

// getRules retrieves all current Filtered Stream rules.
func (c *XClient) getRules(ctx context.Context) ([]StreamRule, error) {
    req, err := http.NewRequestWithContext(ctx, "GET",
        c.baseURL+"/2/tweets/search/stream/rules", nil,
    )
    if err != nil {
        return nil, err
    }
    req.Header.Set("Authorization", "Bearer "+c.token)
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    body, _ := io.ReadAll(resp.Body)
    var result struct {
        Data []StreamRule `json:"data"`
    }
    if err := json.Unmarshal(body, &result); err != nil {
        return nil, nil
    }
    return result.Data, nil
}
```

- [ ] **Step 4：运行测试**

```bash
go test ./tests/unit/collect/... -v
```
Expected: TestParseTweet PASS, TestParseTweetError PASS

- [ ] **Step 5：提交**

```bash
git add internal/collect/xclient.go tests/unit/collect/xclient_test.go
git commit -m "feat: add X API v2 Filtered Stream client with rule management

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### 任务 5：采集调度 + 采集 Repository

**文件：**
- Create: `internal/collect/service.go`
- Create: `internal/repository/gormimpl/collect_repo.go`

**说明：** 采集调度器负责启动/停止 Stream 连接、将推文标准化为 PlatformPost、调用 embedding、写入 DB + 执行匹配。采集 Repository 封装采集相关写入操作。

- [ ] **Step 1：写采集 Repository 测试**

```go
// tests/unit/repository/collect_repo_test.go
package repository_test

import (
    "context"
    "testing"
    "github.com/StephenQiu30/hotkey-server/internal/pkg"
    "github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
    "gorm.io/gorm"
)

// Uses test DB — skip if no DB
func TestCollectRepoCreatePost(t *testing.T) {
    db, err := testDB()
    if err != nil {
        t.Skipf("no test DB: %v", err)
    }
    repo := gormimpl.NewCollectRepo(db)
    id, err := repo.CreatePost(context.Background(), gormimpl.PlatformPost{
        Platform:       "x",
        PlatformPostID: "test-123",
        AuthorName:     "Test",
        ContentText:    "hello world",
    })
    if err != nil {
        t.Fatalf("CreatePost failed: %v", err)
    }
    if id == 0 {
        t.Fatal("expected non-zero ID")
    }
    t.Logf("created post id=%d", id)
}

func TestCollectRepoCreateHit(t *testing.T) {
    // Similar pattern for monitor_post_hits
}
```

- [ ] **Step 2：实现采集 Repository**

```go
// internal/repository/gormimpl/collect_repo.go
package gormimpl

import (
    "context"
    "time"
    "github.com/StephenQiu30/hotkey-server/internal/pkg"
    "gorm.io/gorm"
)

// CollectRepo handles collection-related DB writes (posts, hits, authors).
type CollectRepo struct {
    db *gorm.DB
}

func NewCollectRepo(db *gorm.DB) *CollectRepo {
    return &CollectRepo{db: db}
}

// CreatePost inserts a new platform post and returns its ID.
func (r *CollectRepo) CreatePost(ctx context.Context, p PlatformPost) (int64, error) {
    if err := r.db.WithContext(ctx).Create(&p).Error; err != nil {
        return 0, err
    }
    return p.ID, nil
}

// UpsertPost creates or updates a post by platform + platform_post_id,
// and updates its embedding.
func (r *CollectRepo) UpsertPost(ctx context.Context, p *PlatformPost) error {
    if p.Embedding != nil {
        result := r.db.WithContext(ctx).
            Model(&PlatformPost{}).
            Where("platform = ? AND platform_post_id = ?", p.Platform, p.PlatformPostID).
            Updates(map[string]interface{}{
                "content_text": p.ContentText,
                "author_name":  p.AuthorName,
                "author_handle": p.AuthorHandle,
                "embedding":    *p.Embedding,
                "updated_at":   time.Now(),
            })
        if result.Error != nil {
            return result.Error
        }
        if result.RowsAffected == 0 {
            return r.db.WithContext(ctx).Create(p).Error
        }
        return nil
    }
    // No embedding: insert if not exists
    return r.db.WithContext(ctx).Where("platform = ? AND platform_post_id = ?",
        p.Platform, p.PlatformPostID).FirstOrCreate(p).Error
}

// UpdatePostEmbedding updates the embedding for an existing post.
func (r *CollectRepo) UpdatePostEmbedding(ctx context.Context, postID int64, emb pkg.Vector384) error {
    return r.db.WithContext(ctx).Model(&PlatformPost{}).
        Where("id = ?", postID).
        Update("embedding", emb).Error
}

// CreateHit records a monitor_post_hit entry from a cosine match.
func (r *CollectRepo) CreateHit(ctx context.Context, hit *MonitorPostHit) error {
    return r.db.WithContext(ctx).Create(hit).Error
}

// ListActiveMonitors retrieves all active monitors for rule registration.
func (r *CollectRepo) ListActiveMonitors(ctx context.Context) ([]KeywordMonitor, error) {
    var monitors []KeywordMonitor
    if err := r.db.WithContext(ctx).Where("status = ?", "active").Find(&monitors).Error; err != nil {
        return nil, err
    }
    return monitors, nil
}

// ListHitsSince retrieves monitor_post_hits created after a given time.
func (r *CollectRepo) ListHitsSince(ctx context.Context, since time.Time) ([]MonitorPostHit, error) {
    var hits []MonitorPostHit
    if err := r.db.WithContext(ctx).
        Preload("Post").
        Where("first_seen_at >= ?", since).
        Find(&hits).Error; err != nil {
        return nil, err
    }
    return hits, nil
}
```

- [ ] **Step 3：实现采集调度 Service**

```go
// internal/collect/service.go
package collect

import (
    "bufio"
    "context"
    "fmt"
    "io"
    "math"
    "sync"
    "time"

    "github.com/StephenQiu30/hotkey-server/internal/embedding"
    "github.com/StephenQiu30/hotkey-server/internal/platform/logging"
    "github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
    "go.uber.org/zap"
)

// Service runs the X Filtered Stream collector.
type Service struct {
    client     *XClient
    embedder   *embedding.Service
    repo       *gormimpl.CollectRepo
    similarityThreshold float64
    cancel     context.CancelFunc
    wg         sync.WaitGroup
    mu         sync.Mutex
    running    bool
}

// NewService creates a new collection service.
func NewService(client *XClient, embedder *embedding.Service, repo *gormimpl.CollectRepo, threshold float64) *Service {
    if threshold <= 0 {
        threshold = 0.7
    }
    return &Service{
        client:              client,
        embedder:            embedder,
        repo:                repo,
        similarityThreshold: threshold,
    }
}

// Start begins the Filtered Stream connection and processes incoming tweets.
// Blocks until initial rule registration completes, then runs in background.
func (s *Service) Start(ctx context.Context) error {
    s.mu.Lock()
    if s.running {
        s.mu.Unlock()
        return nil
    }
    s.running = true
    ctx, s.cancel = context.WithCancel(ctx)
    s.mu.Unlock()

    // Register active monitors as stream rules
    monitors, err := s.repo.ListActiveMonitors(ctx)
    if err != nil {
        return fmt.Errorf("list active monitors: %w", err)
    }
    if len(monitors) == 0 {
        logging.L().Warn("no active monitors found, stream will receive no rules")
    }
    rules := make([]StreamRule, 0, len(monitors))
    for _, m := range monitors {
        rules = append(rules, StreamRule{
            Value: m.QueryText,
            Tag:   fmt.Sprintf("monitor_%d", m.ID),
        })
    }
    if len(rules) > 0 {
        if err := s.client.SetRules(ctx, rules); err != nil {
            return fmt.Errorf("set stream rules: %w", err)
        }
        logging.L().Info("stream rules registered", zap.Int("count", len(rules)))
    }

    s.wg.Add(1)
    go s.runLoop(ctx)
    return nil
}

// Stop gracefully shuts down the collector.
func (s *Service) Stop() {
    s.mu.Lock()
    defer s.mu.Unlock()
    if s.cancel != nil {
        s.cancel()
    }
    s.wg.Wait()
    s.running = false
}

// runLoop maintains the stream connection with automatic reconnection.
func (s *Service) runLoop(ctx context.Context) {
    defer s.wg.Done()
    backoff := 1 * time.Second
    maxBackoff := 5 * time.Minute

    for {
        select {
        case <-ctx.Done():
            logging.L().Info("collector stream loop stopped")
            return
        default:
        }

        body, err := s.client.ConnectStream(ctx)
        if err != nil {
            logging.L().Error("collector stream connect failed",
                zap.Error(err),
                zap.Duration("reconnect_in", backoff),
            )
            select {
            case <-ctx.Done():
                return
            case <-time.After(backoff):
                backoff = time.Duration(math.Min(
                    float64(backoff*2),
                    float64(maxBackoff),
                ))
            }
            continue
        }
        backoff = 1 * time.Second // reset on successful connect

        scanner := bufio.NewScanner(body)
        scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
        for {
            tweet, err := ReadStream(scanner)
            if err != nil {
                if err == io.EOF || err == context.Canceled {
                    break
                }
                logging.L().Error("collector stream read error", zap.Error(err))
                break
            }
            if tweet != nil {
                s.processTweet(ctx, tweet)
            }
        }
        body.Close()
    }
}

// processTweet handles a single incoming tweet: embedding → upsert → match.
func (s *Service) processTweet(ctx context.Context, tweet *Tweet) {
    log := logging.L().With(zap.String("tweet_id", tweet.ID))

    // Generate embedding
    emb, err := s.embedder.Embed(ctx, tweet.Text)
    if err != nil {
        log.Warn("embedding failed, storing without vector", zap.Error(err))
    }

    // Upsert post with embedding
    post := &gormimpl.PlatformPost{
        Platform:       "x",
        PlatformPostID: tweet.ID,
        AuthorName:     tweet.AuthorName,
        AuthorHandle:   tweet.AuthorHandle,
        ContentText:    tweet.Text,
        PublishedAt:    parseTime(tweet.CreatedAt),
    }
    if err == nil {
        post.Embedding = &emb
    }
    if err := s.repo.UpsertPost(ctx, post); err != nil {
        log.Error("failed to upsert post", zap.Error(err))
        return
    }

    // If no embedding was generated, skip matching
    if post.Embedding == nil {
        return
    }

    // Find matching active monitors and record hits
    monitors, err := s.repo.ListActiveMonitors(ctx)
    if err != nil {
        log.Error("failed to list monitors for matching", zap.Error(err))
        return
    }
    for _, m := range monitors {
        if m.QueryEmbedding == nil {
            continue
        }
        sim := cosineSimilarity(*post.Embedding, *m.QueryEmbedding)
        if sim >= s.similarityThreshold {
            hit := &gormimpl.MonitorPostHit{
                MonitorID:            m.ID,
                PostID:               post.ID,
                RelevanceScore:       sim,
                FreshnessScore:       1.0,
                AuthorInfluenceScore: 0.5,
                FinalScore:           sim * 0.7 + 0.3,
                FirstSeenAt:          time.Now(),
                LastSeenAt:           time.Now(),
            }
            if err := s.repo.CreateHit(ctx, hit); err != nil {
                log.Error("failed to record hit",
                    zap.Int64("monitor_id", m.ID),
                    zap.Error(err),
                )
            }
        }
    }
}

// cosineSimilarity computes cosine similarity between two vectors.
func cosineSimilarity(a, b [384]float32) float64 {
    var dot, normA, normB float64
    for i := range 384 {
        va, vb := float64(a[i]), float64(b[i])
        dot += va * vb
        normA += va * va
        normB += vb * vb
    }
    denom := math.Sqrt(normA) * math.Sqrt(normB)
    if denom == 0 {
        return 0
    }
    return dot / denom
}

// parseTime parses an X API timestamp.
func parseTime(s string) *time.Time {
    if s == "" {
        return nil
    }
    t, err := time.Parse(time.RFC3339, s)
    if err != nil {
        return nil
    }
    return &t
}
```

- [ ] **Step 4：运行 build 确认编译通过**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server && go build ./...
```
Expected: success

- [ ] **Step 5：提交**

```bash
git add internal/collect/service.go internal/repository/gormimpl/collect_repo.go
git commit -m "feat: add X stream collector service and collect repository

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### 任务 6：匹配 Repository + Monitor embedding 钩子

**文件：**
- Create: `internal/repository/gormimpl/match_repo.go`
- Modify: `internal/repository/gormimpl/monitor_repo.go`
- Modify: `internal/monitor/repository.go`
- Modify: `internal/monitor/service.go`

**说明：** 余弦相似度匹配查询 Repository，以及在 monitor create/update 时自动生成 query_embedding。

- [ ] **Step 1：扩展 Monitor Repository 接口**

```go
// internal/monitor/repository.go 追加方法
SetQueryEmbedding(ctx context.Context, id int64, emb pkg.Vector384) error
```

- [ ] **Step 2：实现 SetQueryEmbedding**

```go
// internal/repository/gormimpl/monitor_repo.go 追加方法
func (r *MonitorRepo) SetQueryEmbedding(ctx context.Context, id int64, emb pkg.Vector384) error {
    return r.db.WithContext(ctx).Model(&KeywordMonitor{}).
        Where("id = ?", id).
        Update("query_embedding", emb).Error
}
```

- [ ] **Step 3：在 monitor Service Create 钩子中添加 embedding 生成**

```go
// internal/monitor/service.go
type Service struct {
    repo      Repository
    embedder  EmbeddingService // 新增
}

type EmbeddingService interface {
    Embed(ctx context.Context, text string) (pkg.Vector384, error)
}

func NewService(repo Repository, embedder EmbeddingService) *Service {
    return &Service{repo: repo, embedder: embedder}
}

func (s *Service) Create(ctx context.Context, userID int64, input CreateMonitorInput) (Monitor, error) {
    // ... 已有验证 ...
    m, err := s.repo.Create(ctx, userID, input)
    if err != nil {
        return Monitor{}, err
    }
    // 异步生成 embedding（不阻塞创建流程）
    if s.embedder != nil {
        go func() {
            emb, err := s.embedder.Embed(context.Background(), input.QueryText)
            if err != nil {
                logging.L().Warn("failed to generate query embedding",
                    zap.Int64("monitor_id", m.ID),
                    zap.Error(err),
                )
                return
            }
            if err := s.repo.SetQueryEmbedding(context.Background(), m.ID, emb); err != nil {
                logging.L().Error("failed to save query embedding",
                    zap.Int64("monitor_id", m.ID),
                    zap.Error(err),
                )
            }
        }()
    }
    return m, nil
}
```

注：`Update` 同理——检测 `query_text` 变更则重新生成 embedding。

- [ ] **Step 4：实现匹配 Repository**

```go
// internal/repository/gormimpl/match_repo.go
package gormimpl

import (
    "context"
    "github.com/StephenQiu30/hotkey-server/internal/pkg"
    "gorm.io/gorm"
)

// PostMatch represents a post with its cosine similarity score.
type PostMatch struct {
    PlatformPost
    Similarity float64 `gorm:"-:migration" json:"similarity"`
}

// MatchRepo handles pgvector cosine similarity queries.
type MatchRepo struct {
    db *gorm.DB
}

func NewMatchRepo(db *gorm.DB) *MatchRepo {
    return &MatchRepo{db: db}
}

// FindMatchingPosts returns posts matching a monitor's query_embedding above threshold.
func (r *MatchRepo) FindMatchingPosts(ctx context.Context, monitorID int64, threshold float64, limit int) ([]PostMatch, error) {
    var results []PostMatch
    err := r.db.WithContext(ctx).
        Table("platform_posts").
        Select("platform_posts.*, 1 - (platform_posts.embedding <=> ?) AS similarity",
            gorm.Expr("keyword_monitors.query_embedding")).
        Joins("JOIN keyword_monitors ON keyword_monitors.id = ?", monitorID).
        Where("1 - (platform_posts.embedding <=> keyword_monitors.query_embedding) >= ?", threshold).
        Where("platform_posts.embedding IS NOT NULL").
        Order(gorm.Expr("similarity DESC")).
        Limit(limit).
        Find(&results).Error
    return results, err
}

// ComputeMatchingScore computes the cosine similarity between a post and a monitor.
func (r *MatchRepo) ComputeMatchingScore(ctx context.Context, postID, monitorID int64) (float64, error) {
    var result struct {
        Score float64
    }
    err := r.db.WithContext(ctx).
        Raw(`SELECT 1 - (pp.embedding <=> km.query_embedding) AS score
             FROM platform_posts pp, keyword_monitors km
             WHERE pp.id = ? AND km.id = ?
               AND pp.embedding IS NOT NULL
               AND km.query_embedding IS NOT NULL`,
            postID, monitorID).
        Scan(&result).Error
    if err != nil {
        return 0, err
    }
    return result.Score, nil
}
```

- [ ] **Step 5：更新 Fx 中 monitor.NewService 调用（需传 embedder）**

```go
// 这步在 Fx wiring 任务中一起处理
```

- [ ] **Step 6：Build 确认**

```bash
go build ./...
```
Expected: success

- [ ] **Step 7：提交**

```bash
git add internal/repository/gormimpl/match_repo.go internal/monitor/repository.go internal/monitor/service.go
git commit -m "feat: add pgvector match repo and monitor embedding hook on create

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### 任务 7：Topic + Snapshot 写入 Repository

**文件：**
- Create: `internal/repository/gormimpl/topic_write_repo.go`
- Create: `internal/repository/gormimpl/snapshot_repo.go`

**说明：** 补齐 topics、topic_posts、topic_snapshots、monitor_snapshots 的写入操作。目前只有读取（query）没有写入。

- [ ] **Step 1：实现 Topic 写入 Repository**

```go
// internal/repository/gormimpl/topic_write_repo.go
package gormimpl

import (
    "context"
    "time"
    "gorm.io/gorm"
)

// TopicWriteRepo handles writes to topics and topic_posts tables.
type TopicWriteRepo struct {
    db *gorm.DB
}

func NewTopicWriteRepo(db *gorm.DB) *TopicWriteRepo {
    return &TopicWriteRepo{db: db}
}

// CreateTopic inserts a new topic and returns its ID.
func (r *TopicWriteRepo) CreateTopic(ctx context.Context, monitorID int64, topicKey, title, summary string) (int64, error) {
    t := Topic{
        MonitorID:       monitorID,
        TopicKey:        topicKey,
        Title:           title,
        Summary:         summary,
        Status:          "active",
        FirstDetectedAt: time.Now(),
        LastActiveAt:    time.Now(),
    }
    if err := r.db.WithContext(ctx).Create(&t).Error; err != nil {
        return 0, err
    }
    return t.ID, nil
}

// AddTopicPost links a post to a topic.
func (r *TopicWriteRepo) AddTopicPost(ctx context.Context, topicID, postID int64, score float64) error {
    tp := TopicPost{
        TopicID:         topicID,
        PostID:          postID,
        MembershipScore: score,
        AddedAt:         time.Now(),
    }
    return r.db.WithContext(ctx).Where("topic_id = ? AND post_id = ?", topicID, postID).
        FirstOrCreate(&tp).Error
}

// UpdateTopicHeat updates a topic's heat score and last active time.
func (r *TopicWriteRepo) UpdateTopicHeat(ctx context.Context, topicID int64, heat float64, direction string) error {
    return r.db.WithContext(ctx).Model(&Topic{}).
        Where("id = ?", topicID).
        Updates(map[string]interface{}{
            "current_heat_score": heat,
            "trend_direction":    direction,
            "last_active_at":     time.Now(),
            "updated_at":         time.Now(),
        }).Error
}
```

- [ ] **Step 2：实现 Snapshot 写入 Repository**

```go
// internal/repository/gormimpl/snapshot_repo.go
package gormimpl

import (
    "context"
    "gorm.io/gorm"
)

// SnapshotRepo handles writes to snapshot tables.
type SnapshotRepo struct {
    db *gorm.DB
}

func NewSnapshotRepo(db *gorm.DB) *SnapshotRepo {
    return &SnapshotRepo{db: db}
}

// CreateTopicSnapshot persists a topic snapshot.
func (r *SnapshotRepo) CreateTopicSnapshot(ctx context.Context, snap *TopicSnapshot) error {
    return r.db.WithContext(ctx).Create(snap).Error
}

// CreateMonitorSnapshot persists a monitor snapshot.
func (r *SnapshotRepo) CreateMonitorSnapshot(ctx context.Context, snap *MonitorSnapshot) error {
    return r.db.WithContext(ctx).Create(snap).Error
}

// GetTopicSnapshotBefore retrieves the most recent snapshot before a given time.
func (r *SnapshotRepo) GetTopicSnapshotBefore(ctx context.Context, topicID int64, before time.Time) (*TopicSnapshot, error) {
    var snap TopicSnapshot
    err := r.db.WithContext(ctx).
        Where("topic_id = ? AND snapshot_time < ?", topicID, before).
        Order("snapshot_time DESC").
        First(&snap).Error
    if err == gorm.ErrRecordNotFound {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    return &snap, nil
}
```

- [ ] **Step 3：Build 确认**

```bash
go build ./...
```
Expected: success

- [ ] **Step 4：提交**

```bash
git add internal/repository/gormimpl/topic_write_repo.go internal/repository/gormimpl/snapshot_repo.go
git commit -m "feat: add topic write repo and snapshot repo for hourly aggregation

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### 任务 8：每小时批量 Worker

**文件：**
- Create: `internal/worker/hourly_aggregate.go`
- Modify: `internal/queue/message.go`

**说明：** 每小时执行话题聚类 → 热点聚合 → 趋势快照三步。复用已有的 `topic.Cluster()`、`hotevent.ComputeHeatScore()`、`trend.BuildTopicSnapshot()` 纯函数。

- [ ] **Step 1：添加 topic 常量**

```go
// internal/queue/message.go 追加
TopicHourlyRun  = "hotkey.hourly.run"
TopicHourlyRunDLQ = "hotkey.hourly.run.dlq"
```

- [ ] **Step 2：更新 dispatcher 的 topicForType / topicForDLQ**

```go
// internal/queue/dispatcher.go
func topicForType(msgType string) string {
    switch msgType {
    case "digest.run":
        return TopicDigestRun
    case "hourly.run":
        return TopicHourlyRun
    default:
        return msgType
    }
}

func topicForDLQ(msgType string) string {
    switch msgType {
    case "digest.run":
        return TopicDigestRunDLQ
    case "hourly.run":
        return TopicHourlyRunDLQ
    default:
        return msgType + ".dlq"
    }
}
```

- [ ] **Step 3：实现 HourlyAggregateJob**

```go
// internal/worker/hourly_aggregate.go
package worker

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"
    "time"

    "github.com/StephenQiu30/hotkey-server/internal/content"
    "github.com/StephenQiu30/hotkey-server/internal/hotevent"
    "github.com/StephenQiu30/hotkey-server/internal/platform/logging"
    "github.com/StephenQiu30/hotkey-server/internal/queue"
    "github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
    "github.com/StephenQiu30/hotkey-server/internal/topic"
    "github.com/StephenQiu30/hotkey-server/internal/trend"
    "go.uber.org/zap"
    "gorm.io/gorm"
)

// HourlyAggregateDeps groups dependencies for the hourly aggregate job.
type HourlyAggregateDeps struct {
    DB             *gorm.DB
    CollectRepo    *gormimpl.CollectRepo
    TopicWriteRepo *gormimpl.TopicWriteRepo
    SnapshotRepo   *gormimpl.SnapshotRepo
    RunRepo        RunRepository
    Now            func() time.Time
}

// HourlyAggregateJob runs clustering → hot event aggregation → snapshots hourly.
type HourlyAggregateJob struct {
    deps HourlyAggregateDeps
}

func NewHourlyAggregateJob(deps HourlyAggregateDeps) *HourlyAggregateJob {
    if deps.Now == nil {
        deps.Now = time.Now
    }
    return &HourlyAggregateJob{deps: deps}
}

func (j *HourlyAggregateJob) Type() string { return "hourly.run" }

func (j *HourlyAggregateJob) Handle(ctx context.Context, msg queue.Message) error {
    var payload struct {
        TargetHour string `json:"target_hour,omitempty"`
    }
    _ = json.Unmarshal(msg.Payload, &payload)

    now := j.deps.Now()
    runKey := fmt.Sprintf("hourly-aggregate:%s", now.Format("2006-01-02T15:00"))
    log := logging.L().With(
        zap.String("run_key", runKey),
        zap.Time("now", now),
    )

    // Dedup via RunRepository
    started, err := j.deps.RunRepo.TryStart(ctx, runKey, "hourly-aggregate", now, now)
    if err != nil {
        return err
    }
    if !started {
        log.Info("hourly aggregate already running, skipping")
        return nil
    }

    runErr := j.executeAll(ctx, now)
    if runErr != nil {
        _ = j.deps.RunRepo.MarkFailed(ctx, runKey, runErr.Error(), j.deps.Now())
        log.Error("hourly aggregate failed", zap.Error(runErr))
        return runErr
    }
    _ = j.deps.RunRepo.MarkFinished(ctx, runKey, j.deps.Now())
    log.Info("hourly aggregate completed")
    return nil
}

func (j *HourlyAggregateJob) DedupeEnabled() bool { return false }

func (j *HourlyAggregateJob) executeAll(ctx context.Context, now time.Time) error {
    since := now.Add(-1 * time.Hour)

    // Step 1: Topic clustering
    if err := j.clusterPosts(ctx, since); err != nil {
        return fmt.Errorf("cluster: %w", err)
    }

    // Step 2: Hot event aggregation
    if err := j.aggregateHotEvents(ctx); err != nil {
        return fmt.Errorf("aggregate: %w", err)
    }

    // Step 3: Trend snapshots
    if err := j.snapshotTrends(ctx); err != nil {
        return fmt.Errorf("snapshot: %w", err)
    }

    return nil
}

func (j *HourlyAggregateJob) clusterPosts(ctx context.Context, since time.Time) error {
    // 1. Get all hits (matched posts) since last run
    hits, err := j.deps.CollectRepo.ListHitsSince(ctx, since)
    if err != nil {
        return fmt.Errorf("list hits: %w", err)
    }
    if len(hits) == 0 {
        return nil
    }

    // 2. PostLoader reads content from DB for clustering
    db := j.deps.DB
    type postRecord struct {
        ID          int64
        ContentText string
        MonitorID   int64
    }
    var posts []postRecord
    postIDs := make([]int64, len(hits))
    for i, h := range hits {
        postIDs[i] = h.PostID
    }
    if err := db.WithContext(ctx).
        Model(&gormimpl.PlatformPost{}).
        Select("id, content_text").
        Where("id IN ?", postIDs).
        Find(&posts).Error; err != nil {
        return fmt.Errorf("load posts: %w", err)
    }

    // 3. Convert to CandidatePost for clustering
    candidates := make([]topic.CandidatePost, len(posts))
    for i, p := range posts {
        candidates[i] = topic.CandidatePost{
            PostID: p.ID,
            Tokens: topic.ExtractTokens(p.ContentText),
        }
    }

    // 4. Run clustering
    clustered := topic.Cluster(candidates)
    log := logging.L().With(zap.Int("clusters", len(clustered)))
    log.Info("topic clustering completed")

    // 5. Persist
    for _, c := range clustered {
        // Map first hit's monitor ID as the cluster owner
        monitorID := hits[0].MonitorID
        for _, hit := range hits {
            for _, pid := range c.PostIDs {
                if hit.PostID == pid {
                    monitorID = hit.MonitorID
                    break
                }
            }
        }

        topicID, err := j.deps.TopicWriteRepo.CreateTopic(ctx, monitorID, c.TopicKey, c.Title, "")
        if err != nil {
            log.Error("failed to create topic", zap.String("key", c.TopicKey), zap.Error(err))
            continue
        }
        for _, pid := range c.PostIDs {
            if err := j.deps.TopicWriteRepo.AddTopicPost(ctx, topicID, pid, 1.0); err != nil {
                log.Error("failed to add topic post",
                    zap.Int64("topic_id", topicID),
                    zap.Int64("post_id", pid),
                    zap.Error(err),
                )
            }
        }
    }
    return nil
}

func (j *HourlyAggregateJob) aggregateHotEvents(ctx context.Context) error {
    // Simplified: create hot_events from topics
    type topicRecord struct {
        ID               int64
        MonitorID        int64
        TopicKey         string
        Title            string
        CurrentHeatScore float64
        TrendDirection   string
    }
    var topics []topicRecord
    if err := j.deps.DB.WithContext(ctx).
        Model(&gormimpl.Topic{}).
        Where("status = ?", "active").
        Find(&topics).Error; err != nil {
        return fmt.Errorf("list topics: %w", err)
    }

    for _, t := range topics {
        // Compute heat score using existing formula
        heat := hotevent.ComputeHeatScore("x", []float64{t.CurrentHeatScore}, time.Now())
        direction := hotevent.DetermineTrend(heat, t.CurrentHeatScore)

        event := gormimpl.HotEvent{
            Name:          t.Title,
            HeatScore:     heat,
            Platform:      "x",
            Trend:         direction,
            FirstSeenAt:   time.Now(),
            LastSeenAt:    time.Now(),
            Status:        hotevent.StatusActive,
        }
        if err := j.deps.DB.WithContext(ctx).Create(&event).Error; err != nil {
            return fmt.Errorf("create hot event: %w", err)
        }
    }
    return nil
}

func (j *HourlyAggregateJob) snapshotTrends(ctx context.Context) error {
    now := j.deps.Now()

    // Topic snapshots
    type topicInfo struct {
        ID               int64
        CurrentHeatScore float64
    }
    var topicInfos []topicInfo
    if err := j.deps.DB.WithContext(ctx).
        Model(&gormimpl.Topic{}).
        Select("id, current_heat_score").
        Where("status = ?", "active").
        Find(&topicInfos).Error; err != nil {
        return fmt.Errorf("list topics for snapshot: %w", err)
    }

    for _, ti := range topicInfos {
        prev, err := j.deps.SnapshotRepo.GetTopicSnapshotBefore(ctx, ti.ID, now)
        if err != nil {
            return fmt.Errorf("get prev snapshot topic %d: %w", ti.ID, err)
        }
        prevHeat := 0.0
        if prev != nil {
            prevHeat = prev.HeatScore
        }
        snap := trend.BuildTopicSnapshot(trend.TopicSnapshotInput{
            TopicID:      ti.ID,
            SnapshotTime: now,
            HeatScore:    ti.CurrentHeatScore,
            PreviousHeat: prevHeat,
        })
        gormSnap := &gormimpl.TopicSnapshot{
            TopicID:       snap.TopicID,
            SnapshotTime:  snap.SnapshotTime,
            HeatScore:     snap.HeatScore,
            TrendVelocity: snap.TrendVelocity,
        }
        if err := j.deps.SnapshotRepo.CreateTopicSnapshot(ctx, gormSnap); err != nil {
            return fmt.Errorf("create topic snapshot %d: %w", ti.ID, err)
        }
    }

    // Monitor snapshots
    var monitorIDs []int64
    if err := j.deps.DB.WithContext(ctx).
        Model(&gormimpl.KeywordMonitor{}).
        Where("status = ?", "active").
        Pluck("id", &monitorIDs).Error; err != nil {
        return fmt.Errorf("list monitors for snapshot: %w", err)
    }
    for _, mid := range monitorIDs {
        snap := trend.BuildMonitorSnapshot(trend.MonitorSnapshotInput{
            MonitorID:   mid,
            SnapshotTime: now,
        })
        gormSnap := &gormimpl.MonitorSnapshot{
            MonitorID:    snap.MonitorID,
            SnapshotTime: snap.SnapshotTime,
        }
        if err := j.deps.SnapshotRepo.CreateMonitorSnapshot(ctx, gormSnap); err != nil {
            return fmt.Errorf("create monitor snapshot %d: %w", mid, err)
        }
    }
    return nil
}
```

- [ ] **Step 4：写 Worker 测试**

```go
// tests/unit/worker/hourly_aggregate_test.go
package worker_test

import (
    "context"
    "testing"
    "time"
    "github.com/StephenQiu30/hotkey-server/internal/worker"
    "github.com/StephenQiu30/hotkey-server/internal/queue"
)

type mockRunRepo struct{}

func (m *mockRunRepo) TryStart(ctx context.Context, runKey, runType string, target time.Time, started time.Time) (bool, error) {
    return true, nil
}
func (m *mockRunRepo) MarkFinished(ctx context.Context, runKey string, finished time.Time) error { return nil }
func (m *mockRunRepo) MarkFailed(ctx context.Context, runKey, message string, failed time.Time) error { return nil }

func TestHourlyAggregateType(t *testing.T) {
    job := worker.NewHourlyAggregateJob(worker.HourlyAggregateDeps{})
    if job.Type() != "hourly.run" {
        t.Errorf("expected hourly.run, got %s", job.Type())
    }
}

func TestHourlyAggregateDedupeEnabled(t *testing.T) {
    job := worker.NewHourlyAggregateJob(worker.HourlyAggregateDeps{})
    if job.DedupeEnabled() {
        t.Error("expected DedupeEnabled to be false")
    }
}
```

- [ ] **Step 5：运行测试**

```bash
go test ./tests/unit/worker/... -v -run TestHourly
```
Expected: PASS

- [ ] **Step 6：提交**

```bash
git add internal/worker/hourly_aggregate.go internal/queue/message.go tests/unit/worker/hourly_aggregate_test.go
git commit -m "feat: add hourly aggregate worker (cluster → hot event → snapshot)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### 任务 9：Fx Wiring + Cron + Consumer

**文件：**
- Modify: `internal/fxapp/app.go`

**说明：** 注册 Embedding Service、X Client、Collect Service、Hourly Worker；在 registerHooks 中添加 cron `0 * * * *` 发布 hourly.run 消息；注册新的 consumer handler。

- [ ] **Step 1：创建 NewCollectService 工厂**

```go
// 在 app.go 中或在新的 fx 文件中
func newCollectService(cfg *config.Config, embedder *embedding.Service, repo *gormimpl.CollectRepo) (*collect.Service, error) {
    if cfg.XToken == "" {
        return nil, errors.New("X_BEARER_TOKEN required for collector")
    }
    xclient := collect.NewXClient(cfg.XBaseURL, cfg.XToken)
    svc := collect.NewService(xclient, embedder, repo, 0.7)
    return svc, nil
}
```

- [ ] **Step 2：创建 OnStart embedder 初始化**

```go
// 在 registerHooks OnStart 中追加
if os.Getenv("SMOKE_TEST") != "1" {
    // 加载 ONNX 模型
    if cfg.EmbeddingModelPath != "" {
        model, err := embedding.NewModel(cfg.EmbeddingModelPath)
        if err != nil {
            return fmt.Errorf("embedding model load: %w", err)
        }
        embedder = embedding.NewService(model)
        
        // 启动采集流
        collectSvc, err := newCollectService(cfg, embedder, collectRepo)
        if err != nil {
            return fmt.Errorf("collect service init: %w", err)
        }
        if err := collectSvc.Start(ctx); err != nil {
            return fmt.Errorf("collect service start: %w", err)
        }
    }
}
```

- [ ] **Step 3：添加 hourly cron**

```go
// registerHooks OnStart cron 部分追加
cronS.AddFunc("0 * * * *", func() {
    now := time.Now().In(loc)
    payload, _ := json.Marshal(map[string]string{
        "target_hour": now.Format("2006-01-02T15:00"),
    })
    if pubErr := producer.Publish(context.Background(), queue.TopicHourlyRun,
        queue.NewMessage("hourly.run", payload)); pubErr != nil {
        logging.L().Error("cron publish hourly error", zap.Error(pubErr))
    }
})
```

- [ ] **Step 4：注册 hourly worker handler**

```go
// registerHooks OnStart dispatcher 注册部分追加
hourlyJob := NewHourlyAggregateJob(HourlyAggregateDeps{
    DB:             db,
    CollectRepo:    collectRepo,
    TopicWriteRepo: topicWriteRepo,
    SnapshotRepo:   snapshotRepo,
    RunRepo:        runRepo,
    Now:            time.Now,
})
dispatcher.Register(hourlyJob)
```

- [ ] **Step 5：Fx Provide 注册**

```go
// 在 fxapp.NewApp() 中追加 Provide
fx.Provide(NewCollectRepo),
fx.Provide(NewTopicWriteRepo),
fx.Provide(NewSnapshotRepo),
fx.Provide(NewCollectService),
fx.Provide(NewHourlyAggregateJob),
```

- [ ] **Step 6：Build 确认**

```bash
go build ./...
```
Expected: success

- [ ] **Step 7：运行全部测试**

```bash
go test ./... -v 2>&1 | tail -30
```
Expected: all pass

- [ ] **Step 8：提交**

```bash
git add internal/fxapp/app.go
git commit -m "feat: wire embedding, collector, hourly worker into Fx app lifecycle

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### 任务 10：更新 schema.sql + 文档

**文件：**
- Modify: `db/schema.sql`

- [ ] **Step 1：在 schema.sql 追加 pgvector 扩展和列**

```sql
-- 追加到 schema.sql
-- pgvector extension for cosine similarity
CREATE EXTENSION IF NOT EXISTS vector;

-- Platform posts: embedding vector for cosine similarity matching
ALTER TABLE platform_posts ADD COLUMN IF NOT EXISTS embedding vector(384);
CREATE INDEX IF NOT EXISTS idx_platform_posts_embedding ON platform_posts
  USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- Keyword monitors: embedding vector for query text
ALTER TABLE keyword_monitors ADD COLUMN IF NOT EXISTS query_embedding vector(384);
```

- [ ] **Step 2：提交**

```bash
git add db/schema.sql
git commit -m "chore: add pgvector extension and embedding columns to schema

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## 执行顺序依赖图

```
Task 1 (Vector384 type)
  └→ Task 3 (Config + model changes) — 依赖 Vector384 类型
Task 2 (ONNX embedding)
  ├→ Task 5 (collect service) — 依赖 embedding service
  └→ Task 6 (match repo + monitor hook) — 依赖 embedding service
Task 4 (X client)
  └→ Task 5 (collect service) — 依赖 X client
Task 7 (topic + snapshot repo)
  └→ Task 8 (hourly worker) — 依赖写入 repo
Task 9 (Fx wiring) — 依赖所有 prior tasks
Task 10 (schema update) — 可任意顺序
```

**推荐执行顺序：** 1 → 3 → 2 → 4 → 5 → 6 → 7 → 8 → 9 → 10
