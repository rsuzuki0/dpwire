# Digital Paper Goクライアント：プロジェクトワークフロー

## 1. 開発方針

本プロジェクトは、最初から全機能を完成させるのではなく、仕様保存、試験基盤、read-only通信、安全なwrite、workflow、同期、OS統合の順に拡張する。

各段階で以下を満たさなければ次へ進まない。

- API根拠が記録されている
- emulator testがある
- protocol golden testがある
- destructive operationにはfault testがある
- 未実装機能が正常終了しない
- public API互換性を不用意に壊さない
- 実機試験結果がcompatibility tableへ反映される

## 2. Phase一覧

| Phase | 目的 | 主な成果物 | 完了条件 |
|---|---|---|---|
| P0 | 仕様・試験基盤 | references固定、API catalog、emulator、CI | 実機なしでeval可能 |
| P1 | read-only利用 | auth、status、list、stat、download | 実機から安全に取得可能 |
| P2 | safe-write | upload、update、mkdir、copy、move、open | conflictと部分失敗を処理 |
| P3 | PDF-only実用版 | pairing、profile、safe delete、配布物 | PDF管理だけで日常使用可能 |
| P4 | soak / hardening | 実使用、bug修正、互換性記録 | core CLIとpublic APIが安定 |
| P5 | backup/sync | snapshot、plan/apply、watch | 注釈PDFを安全に保全 |
| P6 | OS統合 | macOS PDF Service、CUPS、mDNS | coreを変更せず統合可能 |
| P7 | optional機能 | screenshot、whiteboard、template、Wi-Fi | Capabilityごとに追加 |
| P8 | render workflow | `dp send`、Markdown、LaTeX、Tectonic | 安定したPDF core上で変換可能 |

## 3. Phase P0：仕様保存と試験基盤

### 3.1 作業

- `doc/endpoints.json` の取得とhash固定
- provenance記録
- Sony公式仕様の要点記録
- API operation catalog作成
- error code catalog作成
- model/firmware capability table作成
- Go module初期化
- directory skeleton作成
- architecture dependency check作成
- stateful emulator骨格作成
- crypto known-answer test作成
- CI workflow作成
- eval tool作成

P0のpairing範囲は登録APIの入口予約までとする。endpointは明示的な未実装
応答を返し、PIN、DH、HMAC、証明書発行を含む完全な登録処理はP3まで実装
しない。P0のcrypto KATは独立して検証可能なRFC 3394 AES-KWに限定する。

### 3.2 予約するディレクトリ

```text
render/
workflow/send/
workflow/backup/
workflow/sync/
workflow/watch/
workflow/print/
internal/wire/screen/
internal/wire/templates/
internal/wire/wifi/
```

未実装機能の公開commandや成功dummyは作らない。

### 3.3 完了条件

```sh
go run ./tools/eval -mode=ci
```

が実機なしで成功する。

## 4. Phase P1：read-only通信

### 4.1 実装順

1. profile JSON
2. Sony credential import
3. TLS transport
4. nonce/auth
5. session cookie parser
6. device status
7. firmware/model取得
8. document list
9. folder list
10. stat
11. download
12. pagination
13. Unicode path

### 4.2 禁止事項

- trust-all TLSを既定にしない
- 一覧件数を1300固定で取得しない
- local pathとremote pathを混用しない
- response bodyを無制限にmemoryへ読む
- private keyやcookieをlogへ出さない

### 4.3 実機試験

read-onlyのみ。

- firmware/model
- battery/storage
- root folder list
- deep folder list
- 日本語filename
- 100件超pagination
- PDF download
- ETag/hash/revision

## 5. Phase P2：safe-write

### 5.1 実装順

1. create folder
2. create document metadata
3. upload PDF
4. verify hash/revision
5. update with target revision
6. rename
7. move
8. copy
9. open/display
10. partial failure reporting

delete、split upload、部分失敗の自動cleanupはP2に含めない。削除は独立した
安全設計と明示承認を要し、split uploadは実機根拠を得てから追加する。

### 5.2 安全策

- 実機試験は利用者が承認した試験フォルダー内だけ
- destructive operation前にmanifest保存
- upload後にsize/hash/revision確認
- updateはrevision指定必須
- delete commandはP2では公開しない
- metadata作成後upload失敗時はPartialFailureError
- cleanupは実施結果を返す

### 5.3 fault injection

最低限：

- metadata作成後に通信断
- upload 50%で切断
- malformed JSON
- invalid cookie
- storage full
- conflict
- timeout
- server 500
- hash mismatch
- revision mismatch

## 6. Phase P3：PDF-only実用版

P3は機能拡張ではなく、PDFだけを長期間日常使用できる完成版を作る段階と
する。Markdown、LaTeX、Pandoc、latexmk、Tectonic、HTML変換は含めない。

### 6.1 実装要素

- `ls`、`ls -l`、`file/stat`、`get/put`、`cp/mv`、`mkdir`
- revisionを確認するsafe deleteとempty-only `rmdir`
- profile作成、既存Sony credentialの明示的import
- direct connectionとrelay connectionの明確なprofile化
- multi-profile
- DH key exchange
- Java BigInteger互換
- PBKDF2
- HMAC
- AES-KW
- RSA key generation
- CSR/certificate処理
- PIN input
- device CA保存
- credential atomic save
- macOS arm64 / amd64およびLinux用binary
- checksum、install、upgrade、recovery手順

### 6.2 試験

- RFC test vectors
- Java signed-byte edge case
- 256 byte / 257 byte公開値
- wrong PIN
- repeated registration
- interrupted registration
- invalid device response
- corrupt credential store
- delete対象のrevision変化
- non-empty folderの`rmdir`拒否
- CLI end-to-end
- release archive再現性

### 6.3 完了条件

純正Digital Paper Appを使わず、新しいMacから登録でき、PDFについて
list、stat、upload、download、copy、move、deleteができる。配布binaryと
checksumからinstallでき、実使用期間へ移行できる。

## 7. Phase P4：soak / hardening

P3 releaseをしばらく日常使用し、実機差、sleep/reconnect、競合、部分失敗、
CLIの分かりにくさを記録して修正する。この期間はpublic APIとCLIの破壊的な
機能追加を避ける。PDF coreが安定したと判断するまでrendererへ進まない。

## 8. Phase P8：`dp send` workflow（最後に実施）

このphaseはP3 releaseとP4 soakの完了後まで着手しない。

### 7.1 CLI設計

```sh
dp send notes.md --to "Inbox" --open
```

```sh
dp send paper.tex --renderer latexmk --to "Papers" --open=1
```

```sh
cat memo.md | dp send - --format markdown --name memo.pdf --to "Received" --open
```

### 7.2 Renderer実装順

1. PDF pass-through
2. Pandoc Markdown
3. latexmk
4. Tectonic
5. stdin spool
6. `dp render`
7. renderer configuration
8. custom renderer plugin方式の検討

### 7.3 安全策

- isolated temp directory
- shellを介さない
- output PDF validation
- file size limit
- timeout
- temporary file cleanup
- `--keep-pdf` option
- renderer versionをlogへ記録

### 7.4 pipeline command

個別commandも提供する。

```sh
dp render notes.md --output notes.pdf
dp put notes.pdf "Inbox/notes.pdf"
dp open "Inbox/notes.pdf"
```

```sh
dp render notes.md --output - | dp put - "Inbox/notes.pdf" --open
```

## 9. Phase P5：backupとsync

### 8.1 Backup

- device manifest取得
- local manifest保存
- revision/hash比較
- changed PDFのみdownload
- date-stamped snapshot
- annotation済みPDF優先保護
- metadata JSON保存

### 8.2 Sync

```sh
dp sync plan ~/DigitalPaper .
dp sync apply <plan-id>
```

planには次を含める。

- source hash
- destination hash
- source revision
- destination revision
- operation
- conflict理由
- generated timestamp
- profile ID

apply時にはplan生成後の変更を再確認する。

### 8.3 既定policy

- delete伝播なし
- conflict自動解決なし
- device側変更をsnapshot
- local/device両方が変更された場合は双方保存
- dry-run表示

## 10. Phase P6：OS統合

### 9.1 macOS PDF Service

macOS側は通信コードを持たず、`dp send` を呼ぶだけにする。

候補：

- Finder Quick Action
- Print PDF Workflow
- Shortcuts
- Automator互換workflow

### 9.2 CUPS

CUPS backendはstdin PDFを受け取り、`dp send` へ渡す薄いadapterとする。

通信、認証、retry、upload処理をCUPS側へ複製しない。

### 9.3 mDNS

static IPを最初の標準とする。

mDNSは後付けDiscovery interfaceとして実装する。

```go
type Discovery interface {
    Discover(ctx context.Context) ([]Candidate, error)
}
```

## 11. Phase P7：optional機能

候補：

- screenshot
- whiteboard
- note template
- Wi-Fi設定
- owner/configuration
- Bluetooth/USB network helper
- FUSE

各機能は、以下が揃った場合のみ公開する。

- API根拠
- Capability登録
- emulator実装
- protocol test
- 実機試験
- CLI help
- documentation

## 12. Git workflow

### 11.1 Branch

- `main`: 常にeval成功
- feature branch: `feature/<issue>-<name>`
- bugfix branch: `fix/<issue>-<name>`
- protocol investigation: `probe/<issue>-<name>`

### 11.2 Commit

一commitは原則一責務。

例：

```text
spec: record Polaris upload endpoint
transport: add pinned-certificate verification
auth: parse Credentials cookie
emulator: inject revision conflict
cli: add dp stat
```

### 11.3 Pull request条件

- issueへの参照
- API根拠
- test追加
- compat catalog更新
- security impact
- destructive operation有無
-実機試験の要否

## 13. ADR

設計判断は `docs/adr/` に保存する。

例：

```text
0001-use-go.md
0002-single-module.md
0003-no-trust-all-tls.md
0004-plan-apply-sync.md
0005-no-dummy-success.md
0006-static-ip-first.md
0007-external-renderers.md
```

各ADRは以下を含む。

- Context
- Decision
- Alternatives
- Consequences
- Status
- Date

## 14. Compatibility workflow

実機で確認した結果は `spec/compat/models.json` に追加する。

```json
{
  "model": "DPT-RP1",
  "firmware": "1.6.02",
  "verified": [
    "auth.session",
    "documents.list",
    "documents.download"
  ],
  "failed": [],
  "notes": []
}
```

operation status：

```text
documented
implemented
emulated
device-verified
deprecated
experimental
unsupported
```

## 15. CI workflow

### 14.1 Pull request CI

- gofmt
- go vet
- staticcheck
- architecture check
- spec check
- unit test
- protocol golden test
- emulator integration
- short fuzz
- race test
- cross-build

### 14.2 Nightly

- longer fuzz
- dependency vulnerability scan
- full coverage
- all supported GOOS/GOARCH build
- fixture consistency

### 14.3 Release

- clean checkout
- exact version tag
- reproducible build settings
- checksums
- macOS arm64/amd64
- Linux arm64/amd64
- Windows builds when supported
- release notes
- compatibility table

## 16. Eval command

```sh
go run ./tools/eval -mode=ci
```

mode：

```text
ci
developer
nightly
device
release
```

### 15.1 Report

```text
artifacts/eval/report.json
artifacts/eval/junit.xml
artifacts/eval/coverage.out
```

reportには次を含める。

- Go version
- OS/arch
- commit
- spec hash
- tests
- coverage
- race result
- fuzz duration
- cross-build result
- device profile（秘密情報を除く）

## 17. 実機試験workflow

実行例：

```sh
go run ./tools/eval -mode=device -profile rp1-lab
```

または：

```sh
DPT_INTEGRATION=1 \
DPT_PROFILE=rp1-lab \
go test -tags=integration ./...
```

制約：

- test namespace外を変更しない
- run IDごとにdirectoryを分ける
- start/end manifestを保存
-失敗時にcleanup commandを表示
- root folderや既存文書を削除しない

## 18. セキュリティworkflow

- credential、cookie、PIN、private keyをlog禁止
- fixture保存前にscrub toolを通す
- device certificateは公開情報として扱えるがprofile ID等を必要に応じてredact
- temp file permissionを制限
- external rendererにuntrusted argumentを渡さない
- `--insecure` 使用時は明示警告
- destructive commandにはprofile名とtarget pathを表示

## 19. Release policy

### 18.1 Versioning

Semantic Versioningを使用する。

- `v0.x`: public API変更可能
- `v1.0`: core library、auth、document operations、send workflowが安定

### 18.2 v1.0条件

- read/write API安定
- pairing安定
- profile/credential移行安定
- emulator coverage十分
- DPT-RP1実機検証
- DPT-CP1実機または明示的な未検証表示
- `dp send` PDF/Markdown/LaTeX
- backup
- sync plan/apply
- macOS arm64 binary
- protocol、security、recovery documentation

## 20. プロジェクトの初手

最初の実装作業は次の順とする。

1. repository作成
2. LICENSE / NOTICE
3. original spec保存
4. provenance hash
5. Go module
6. directory skeleton
7. ADR 0001–0007
8. compatibility catalog
9. emulator minimal HTTPS server
10. eval tool
11. P0 crypto KAT（AES-KW。完全pairing testはP3）
12. RemotePath
13. TLS transport
14. existing credential import
15. read-only auth

通信実装より先に、仕様、試験、依存規則、失敗表現を確定する。
