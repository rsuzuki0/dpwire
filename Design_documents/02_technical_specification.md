# Digital Paper Goクライアント：技術仕様

## 1. プロジェクト概要

正式名称：`Digital Paper`

リポジトリ名：`digitalpaper`

Author：Ryuji Suzuki

License：MIT

標準実行ファイル名：`dp`

補助実行ファイル名：

- `dp-sim`: Digital Paper APIエミュレータ

Go module：

```text
github.com/rsuzuki0/digitalpaper
```

## 2. 対象機器

初期対象：

- Sony DPT-RP1
- Sony DPT-CP1
- Fujitsu QUADERNO Gen.1

優先検証対象：

- Fujitsu QUADERNO Gen.2（FMVDP41 / FMVDP51）

将来互換性調査：

- Fujitsu QUADERNO Gen.3C
- 同系統APIを持つ派生機

機種・firmwareごとの差はCapability tableで管理する。

## 3. 対象プラットフォーム

module language version：Go 1.24

CIではGo 1.24に加え、その時点の新しいサポート版でも試験する。

必須：

- macOS arm64
- macOS amd64
- Linux amd64
- Linux arm64

将来：

- Windows amd64
- Windows arm64

Core binaryは可能な限り以下でbuildする。

```sh
CGO_ENABLED=0 go build -trimpath ./cmd/dp
```

## 4. 外部依存方針

production libraryのGo依存は原則以下に限定する。

```text
golang.org/x/crypto
golang.org/x/text
```

用途：

- PBKDF2等
- Unicode NFC正規化

CLI frameworkは使わず、標準 `flag` と小型command registryを用いる。

設定形式はJSONとする。

## 5. リポジトリ構成

```text
digitalpaper/
├── go.mod
├── go.sum
├── LICENSE
├── NOTICE
├── README.md
│
├── client.go
├── profile.go
├── capabilities.go
├── documents.go
├── folders.go
├── device.go
├── viewer.go
├── transfer.go
├── remote_path.go
├── errors.go
│
├── credentials/
│   ├── store.go
│   ├── file_store.go
│   ├── import_sony.go
│   └── keychain_darwin.go
│
├── discovery/
│   ├── discovery.go
│   ├── static.go
│   └── mdns.go
│
├── render/
│   ├── renderer.go
│   ├── artifact.go
│   ├── passthrough/
│   ├── pandoc/
│   ├── latexmk/
│   └── tectonic/
│
├── workflow/
│   ├── send/
│   ├── backup/
│   ├── sync/
│   ├── watch/
│   └── print/
│
├── dptest/
│   ├── emulator.go
│   ├── state.go
│   ├── registration.go
│   ├── faults.go
│   ├── recorder.go
│   └── fixtures.go
│
├── internal/
│   ├── wire/
│   │   ├── transport/
│   │   ├── auth/
│   │   ├── registration/
│   │   ├── documents/
│   │   ├── folders/
│   │   ├── device/
│   │   └── viewer/
│   ├── crypto/
│   │   └── aeskw/
│   ├── compat/
│   ├── atomicfile/
│   ├── redaction/
│   └── cli/
│
├── cmd/
│   ├── dp/
│   └── dp-sim/
│
├── spec/
│   ├── references/
│   │   ├── polaris-0.6.0.swagger.json
│   │   └── provenance.json
│   └── compat/
│       ├── operations.json
│       ├── models.json
│       └── error_codes.json
│
├── testdata/
│   ├── crypto/
│   ├── protocol/
│   ├── recordings/
│   ├── unicode/
│   └── pdf/
│
├── tools/
│   ├── eval/
│   ├── spec-check/
│   ├── arch-check/
│   ├── fixture-scrub/
│   └── interop-probe/
│
├── docs/
│   ├── architecture.md
│   ├── protocol.md
│   ├── compatibility.md
│   ├── testing.md
│   └── adr/
│
└── .github/
    └── workflows/
        ├── ci.yml
        ├── integration.yml
        └── release.yml
```

## 6. 公開Go API

### 6.1 Client

```go
type Client struct {
    // unexported
}

func NewClient(profile DeviceProfile, opts ...Option) (*Client, error)

func (c *Client) Documents() *DocumentsService
func (c *Client) Folders()   *FoldersService
func (c *Client) Device()    *DeviceService
func (c *Client) Viewer()    *ViewerService
```

### 6.2 共通要件

すべての通信関数は以下を満たす。

- 第1引数は `context.Context`
- timeout/cancelを尊重
- global state禁止
- Clientごとにsession、TLS、profileを保持
- typed errorを返す
- revision、hash、ETagを失わない
- read/writeを明示的に区別

### 6.3 DeviceProfile

```go
type DeviceProfile struct {
    Name              string
    Address           string
    Model             string
    Firmware          string
    ClientID          string
    PrivateKeyRef     string
    DeviceCAPEM       []byte
    CertificateSHA256 string
}
```

### 6.4 RemotePath

```go
type RemotePath struct {
    normalized string
}
```

制約：

- separatorは `/`
- NFCへ正規化
- `..` 禁止
- NUL禁止
- empty segment規則を明示
- URL生成時はsegment単位escape
- ローカルpath APIとの混用を禁止

## 7. Capability仕様

```go
type Capability string

const (
    CapabilityDocuments     Capability = "documents"
    CapabilitySplitUpload   Capability = "split-upload"
    CapabilityScreenCapture Capability = "screen-capture"
    CapabilityWhiteboard    Capability = "whiteboard"
    CapabilityWiFiConfig    Capability = "wifi-config"
)
```

機種・firmware・実装状況ごとにCapabilityを返す。

未対応時は必ずtyped errorを返す。

```go
var ErrUnsupported = errors.New("digitalpaper: capability unsupported")

type UnsupportedError struct {
    Capability Capability
    Model      string
    Firmware   string
}
```

未実装処理が `nil` を返すことは禁止。

## 8. TLS仕様

接続mode：

1. `VerifyDeviceCA`
2. `PinCertificate`
3. `InsecureExplicit`

通常は `VerifyDeviceCA`。

既存credential import時にdevice CAがない場合は、ユーザー確認付きTOFUでfingerprintを記録可能とする。

`InsecureExplicit` はcommand-line明示時のみ許可し、通常設定の既定値にはできない。

## 9. 認証仕様

### 9.1 通常認証

1. nonce取得
2. RSA-SHA256署名
3. client IDおよび署名送信
4. Credentials cookie取得
5. session保持

### 9.2 新規登録

必要な暗号機能：

- DH
- PBKDF2-SHA256
- HMAC-SHA256
- AES Key Wrap RFC 3394
- RSA-2048
- X.509
- PEM
- Base64
- secure random

Java BigInteger互換byte表現を専用関数で実装する。

### 9.3 credential保存

interface：

```go
type Store interface {
    Load(ctx context.Context, profile string) (Credentials, error)
    Save(ctx context.Context, profile string, c Credentials) error
    Delete(ctx context.Context, profile string) error
    List(ctx context.Context) ([]string, error)
}
```

初期実装：

- permission 0600のportable file store
- atomic write
- backup generation

将来：

- macOS Keychain adapter

## 10. 文書API仕様

### 10.1 Read

- List
- Stat
- Find
- Download
- GetMetadata
- GetRevision
- GetHash
- GetETag

### 10.2 Write

- CreateDocumentMetadata
- UploadContent
- UpdateContent
- Move
- Copy
- Rename
- Delete
- CreateFolder

### 10.3 Pagination

- limit既定値：100
- offsetによる完全走査
- countと取得件数の不一致をerrorまたはwarningで報告
- 1300件固定取得に依存しない

### 10.4 Upload transaction

```text
Create metadata
      ↓
Upload content
      ↓
Verify revision/hash
      ↓
Return committed result
```

部分失敗：

```go
type PartialFailureError struct {
    Operation  string
    DocumentID string
    Cause      error
    Cleanup    CleanupStatus
}
```

### 10.5 Update conflict

更新時は `target_revision` を使用する。

競合時：

```go
type ConflictError struct {
    Path             RemotePath
    ExpectedRevision int64
    ActualRevision   int64
}
```

暗黙の上書きは禁止。

## 11. Renderer仕様

```go
type Renderer interface {
    Name() string
    Probe(ctx context.Context) (RendererInfo, error)
    Supports(format Format) bool
    Render(ctx context.Context, req Request) (*Artifact, error)
}
```

```go
type Artifact struct {
    Path      string
    Size      int64
    SHA256    [32]byte
    MediaType string
    Filename  string
}
```

対応：

- PDF pass-through
- Markdown via Pandoc
- LaTeX via latexmk
- LaTeX via Tectonic
- HTMLは将来

外部program実行には `exec.CommandContext` を用い、shell展開を使用しない。

## 12. CLI仕様

### 12.1 基本command

```text
dp profile
dp auth
dp device
dp ls
dp stat
dp get
dp put
dp mkdir
dp mv
dp cp
dp rm
dp open
dp send
dp render
dp backup
dp sync
dp watch
```

### 12.2 例

```sh
dp ls "Document"
```

```sh
dp put file.pdf "Document/Inbox/file.pdf"
```

```sh
dp send notes.md --to "Document/Inbox" --open
```

```sh
dp send paper.tex --renderer latexmk --to "Document/Papers" --open=1
```

```sh
cat memo.md | dp send - --format markdown --name memo.pdf --to "Document/Received" --open
```

### 12.3 stdin

stdin入力は一時ファイルへspoolする。

理由：

- Content-Length確定
- hash算出
- retry
- PDF validation
- renderer入力

## 13. Sync仕様

command：

```sh
dp sync plan ~/DigitalPaper "Document"
dp sync apply <plan-id>
```

plan項目：

```text
UPLOAD
DOWNLOAD
CONFLICT
UNCHANGED
DELETE-CANDIDATE
```

既定動作：

- deleteを自動伝播しない
- conflictを自動上書きしない
- revisionとhashを利用
- apply前に本体変更をsnapshot
- plan生成後の変更を再検証
- annotation済みPDFを保護

## 14. エミュレータ仕様

実行ファイル：`dp-sim`

機能：

- HTTP registration service
- HTTPS API service
- device CA
- PIN state machine
- nonce/session cookie
- document/folder tree
- revision increment
- ETag
- file hash
- split upload
- storage limit
- firmware capability
- fault injection

P0ではstateful HTTPS server、文書木、TLS、fault injectionおよび登録APIの
入口のみを作る。登録endpointは明示的な未実装応答を返し、成功dummyには
しない。PIN state machine、DH、証明書発行を含む完全なpairingはP3で一体と
して実装する。

例：

```sh
dp-sim \
  --fault upload-disconnect:50% \
  --fault auth-cookie-malformed \
  --fault storage-full-after=100M
```

## 15. 自動評価仕様

実行：

```sh
go run ./tools/eval -mode=ci
```

内容：

1. gofmt
2. go vet
3. static analysis
4. architecture dependency check
5. spec compatibility check
6. unit tests
7. crypto KAT
8. protocol golden tests
9. emulator integration tests
10. race detector
11. fuzz tests
12. coverage check
13. cross-platform build
14. CLI end-to-end tests
15. report生成

aggregate statement coverageはP0で最低60%を要求し、phaseの進行に合わせて
閾値を引き上げる。閾値を下げる変更には理由を記録する。

生成物：

```text
artifacts/eval/report.json
artifacts/eval/junit.xml
artifacts/eval/coverage.out
```

## 16. Spec管理

参照資料は `spec/references` に保存し、変更禁止。

```json
{
  "source": "DPT-RP1/DigitalPaperApp doc/endpoints.json",
  "commit": "<commit hash>",
  "sha256": "<hash>",
  "retrieved": "2026-08-06",
  "classification": "community-preserved translation",
  "authoritative": false,
  "license_note": "See NOTICE"
}
```

実装状態は `spec/compat/operations.json` で管理する。

status：

```text
documented
implemented
emulated
device-verified
deprecated
experimental
unsupported
```

## 17. 通常版から除外する機能

- firmware改造
- root
- ADB
- bootloader
- system partition変更
- Quaderno firmware移植

必要時は別binary、別module、または別repositoryへ隔離する。
