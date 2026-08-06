# Digital Paper Goクライアント：検討経緯と設計判断

## 1. 目的

Sony Digital Paper DPT-RP1 / DPT-CP1は、純正Digital Paper Appの更新停止により、将来のmacOS更新、証明書処理、ネットワーク探索、Electronランタイム、OS権限制御などの変化で利用不能になる可能性が高い。

本プロジェクトの目的は、純正アプリへの依存を除去し、Sony Digital Paper本体の文書管理APIへ直接アクセスする、長期保守可能なクライアント基盤を構築することである。

単なる既存Python実装の翻訳ではなく、次を満たす新規実装とする。

- Goによる単一実行ファイル配布
- 通信ライブラリとCLIの分離
- 他アプリへ組み込み可能な公開Go API
- 実機なしで自動評価できるエミュレータ
- 段階的な機能拡張
- Markdown / LaTeX / PDFからDigital Paperへ送信する統合ワークフロー
- macOS、Linux、将来的にはWindowsにも移植可能な構造
- root化やファームウェア改造を通常運用から隔離
- Sony DPTと互換性のあるFujitsu QUADERNO世代をCapability単位で扱う構造

## 2. 調査対象

### 2.1 主要参照実装

- `janten/dpt-rp1-py`
  - Python製の通信ライブラリおよびCLI
  - 通常認証、新規登録、文書操作、同期、FUSE等を実装
  - ペアリング暗号処理と実機固有quirkの重要な参照元

- `DPT-RP1/DigitalPaperApp`
  - Java製の広範な実装
  - CLI、CUPS、FUSE、whiteboard、画面取得、設定、root等を収録
  - `doc/endpoints.json` にPolaris 2.0 Digital Paper Control InterfaceのSwagger形式資料を収録

- `chrigro/dptrp1_manager`
  - `dpt-rp1-py`上に同期・管理機能を追加
  - 同期設計とUXの参考

- `cristobaltapia/dptrp1-virtual-printer`
  - CUPS / Tea4CUPS / Python CLIを組み合わせた旧仮想プリンタ
  - 現在はアーカイブ済み

- `cristobaltapia/dpt-rp1-cups`
  - 上記の後継CUPS backend
  - 印刷→PDF→Digital Paperというワークフローの参考

### 2.2 ファームウェア・root関連

- `HappyZ/dpt-tools`
- `HappyZ/dpt-tools issue #146`
- `Antiparadox/Sony-Digital-Paper-Hack`

これらは、Quadernoファームウェア移植、純正アプリ改造、root化、任意ファームウェア投入などの研究資料として価値がある。

ただし、純正アプリの代替という目的に対しては危険性が高く、通常版クライアントとは完全に分離する。

## 3. 原典の優先順位

仕様決定時は次の順に信頼する。

1. 実機応答と実機試験
2. Sony公式Help Guideおよび公開仕様
3. Polaris 2.0 API資料 (`doc/endpoints.json`)
4. Java版DigitalPaperAppの実装
5. `dpt-rp1-py` の実装
6. manager / CUPS / virtual-printerのワークフロー
7. root / firmware hack資料

既存コードが動作していることは参考になるが、設計や安全性まで正しいことを意味しない。

## 4. Pythonを基礎にしない理由

`dpt-rp1-py` は有用な参照実装だが、長期運用基盤としては次の問題がある。

- Python本体のバージョン依存
- pip、venv、setuptools等の環境依存
- 多数の第三者パッケージ
- TLS、暗号、FUSE、mDNSなどの依存差
- OS更新によるパッケージ破損
- Alpha段階のプロジェクト構造
- 通信、CLI、同期、FUSE等が密結合
- TLS証明書検証を無効化している

Pythonコードは、以下を知るための参照資料としてのみ使う。

- ペアリングの暗号手順
- 非標準cookieの扱い
- Java `BigInteger.toByteArray()` 互換処理
- APIの実機quirk
- 既存鍵ファイル形式

Pythonのクラス構造、同期ロジック、依存パッケージ構成は移植しない。

## 5. 言語選定

### 5.1 Goを採用する理由

- 単一実行ファイルとして配布可能
- `CGO_ENABLED=0` で共有ライブラリ依存を抑制できる
- HTTP、TLS、RSA、AES、JSON、multipart等が標準ライブラリで揃う
- macOS arm64 / amd64、Linux、Windowsへクロスコンパイル可能
- コードの可読性と修正容易性が高い
- Rustより依存木と実装複雑性を抑えやすい
- CよりTLS、JSON、Unicode、メモリ安全性の保守負担が小さい
- Rubyのように実行時ランタイムやgem環境を必要としない

### 5.2 他言語を採用しない理由

#### Ruby

Pythonより読みやすく移植しやすいが、Ruby本体、gem、OpenSSL extension、Homebrew環境等への依存が残る。

#### Rust

安全性は高いが、本案件では非同期、trait、crate依存、型複雑性が保守負担になりやすい。日常的なRust開発者がいない限りGoの方が適切。

#### C / C++

libcurl、OpenSSL、JSON、Unicode、CLI、multipart等の外部依存とABI保守が必要。単一バイナリを作れても、ソースとビルド環境の保守性が悪い。

## 6. 最重要の設計判断

### 6.1 翻訳ではなく再設計

本プロジェクトは `dpt-rp1-py` のGo翻訳ではない。

- API仕様
- Sony公式動作
- 実機挙動
- 各参照実装

を突き合わせ、通信契約を再構成する。

### 6.2 通信ライブラリを独立させる

公開Go packageとして、他アプリから利用できるようにする。

CLI、同期、PDF変換、CUPS、macOS統合から独立させる。

依存方向は次の一方向に固定する。

```text
internal/wire
      ↓
public Go library
      ↓
workflow
      ↓
CLI / OS integration
```

### 6.3 一つのGo moduleから開始する

通信ライブラリとCLIを別リポジトリや別moduleに分けると、リリース番号、互換性、CI、修正同期が増える。

最初は一つのmoduleにまとめる。

ただし、ライブラリ側からCLIやworkflowをimportすることは禁止する。

moduleは `github.com/rsuzuki0/digitalpaper`、CLIは `dp`、emulatorは
`dp-sim` とする。製品系列をmodule名へ埋め込まず、vendor、model、firmware、
Capabilityの組合せで互換性を管理する。

### 6.4 未実装機能はdummy成功にしない

将来拡張用に空関数を置いて `nil` を返すことは禁止する。

代わりに、次を使用する。

- Capability宣言
- 明示的な `ErrUnsupported`
- 未実装commandをhelpへ出さない
- issue番号と実機検証条件をコメントに記載
- ソースディレクトリのみ先に予約

### 6.5 安全性を既存実装より改善する

既存PythonおよびJava実装のtrust-all TLSは採用しない。

通常は次を使う。

- 登録時に取得したdevice CA
- 証明書fingerprint固定
- 明示的な `--insecure` のみ例外

### 6.6 同期はplan/apply方式

同期は自動的に上書きしない。

```text
dp sync plan
dp sync apply
```

の二段階に分ける。

- 削除は既定で伝播しない
- conflictは自動解決しない
- revisionとhashを使用
- apply前に本体側をsnapshot
- plan後の変更は再検証

## 7. 通信仕様上の重要事項

### 7.1 通常認証

概略は以下。

1. 本体からnonce取得
2. client private keyでRSA-SHA256署名
3. client IDと署名を `/auth` へ送信
4. `Credentials` cookieを取得
5. 以後のAPIへsession cookieを付加

cookie形式が一般的な実装と異なる可能性があるため、独自parserとfixture試験を持つ。

### 7.2 新規ペアリング

必要要素は以下。

- Diffie-Hellman
- PBKDF2-SHA256
- HMAC-SHA256
- AES Key Wrap RFC 3394
- RSA-2048鍵生成
- X.509 / PEM
- PIN入力
- Base64
- Java BigInteger byte表現互換

Java側の公開値が符号bitのため257 byteになる場合を保持しなければ、約半数でHMACが一致しない可能性がある。

### 7.3 文書操作

- 文書一覧
- pagination
- metadata登録
- multipart upload
- download
- ETag
- file hash
- revision
- `target_revision`
- split upload
- conflict error

を最初からデータモデルに含める。

### 7.4 path処理

ローカルpathとDigital Paper内pathを同じ型にしない。

Digital Paper側には専用 `RemotePath` 型を設ける。

- `/` 固定
- NFC正規化
- `..` 禁止
- NUL禁止
- segment単位escape
- `filepath.Join` 禁止

### 7.5 uploadの部分失敗

metadata作成後、file uploadが失敗すると孤児文書が残る可能性がある。

正常・異常を明確に分ける。

```text
Create metadata
      ↓
Upload content
      ↓
Verify revision/hash
      ↓
Commit result
```

途中失敗時は `PartialFailureError` を返し、document IDとcleanup結果を保持する。

## 8. ワークフロー統合の考え方

通信CLIの中心commandを `dp send` とする。

```sh
dp send notes.md --to "Inbox" --open
```

```sh
dp send paper.tex --renderer latexmk --to "Papers" --open=1
```

```sh
cat memo.md | dp send - --format markdown --name memo.pdf --to "Received" --open
```

内部は次のpipelineとする。

```text
input
  ↓
format detection
  ↓
renderer selection
  ↓
isolated temporary workspace
  ↓
PDF validation
  ↓
size/hash calculation
  ↓
upload
  ↓
optional display
  ↓
cleanup
```

Go本体にMarkdownやTeX engineを内蔵しない。

- PDF: pass-through
- Markdown: Pandoc adapter
- LaTeX: latexmk / Tectonic adapter

外部programはshell文字列で実行せず、`exec.CommandContext` に引数を分けて渡す。

## 9. 非目標

通常版では以下を扱わない。

- root化
- 任意firmware投入
- Quaderno firmware移植
- ADB有効化
- bootloader操作
- 非公式system partition変更

必要になった場合は `dp-lab` または別リポジトリへ隔離する。

## 10. 最終方針

本プロジェクトは、以下の五本柱で進める。

1. Python翻訳ではなくAPI仕様の再構成
2. 公開Go通信ライブラリの独立
3. CLI、変換、同期、OS統合の分離
4. Capabilityと明示的unsupported errorによる安全な拡張
5. stateful emulatorと自動evalを最初から構築
