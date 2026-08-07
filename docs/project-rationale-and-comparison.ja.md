# Digital Paperを独自開発する理由

この文書では、このGo実装を独自に開発した経緯と意義、Sony純正Digital
Paper Appおよび`dpt-rp1-py`との違い、設計要件を説明します。
[English version](project-rationale-and-comparison.md)もあります。

比較確認日：2026年8月6日。

## 結論

Sonyは2026年3月31日をもってDPT-RP1とDPT-CP1の修理・サポートを終了し、
Digital Paper Appと本体ファームウェアの提供も終了したと公式に発表して
います。動作する本体を今後も使うには、入手できずサポートもないデスクトップ
アプリだけに依存し続けることは、長期的な運用方法になりません。

一方、オープンソースの[`dpt-rp1-py`](https://github.com/janten/dpt-rp1-py)
は、既に実用的なPythonライブラリとCLIを提供しています。これは先駆的で重要な
成果であり、現在も活動しています。確認した最新版には2026年7月13日付で、
断続的に生じる登録失敗の修正が取り込まれています。また、同期、FUSE mount、
Wi-Fi設定など、本プロジェクトが意図的にまだ実装していない機能も備えています。

したがってDigital Paperは、「最初の代替クライアント」でも、既存ツールを全機能で
置き換えるものでもありません。目的が異なります。長期保守可能で他のGoアプリへ
組み込める通信ライブラリ、実行時依存のない単一`dp` binary、厳格なTLS識別、
破壊的操作の安全制約、実機なしで動くprotocol emulator、再現可能な配布、
機種・firmware別の実機検証記録を提供するための独立実装です。

Sony Appを呼び出したり、内部で利用したりするwrapperではありません。新規pairing
には、Sony Appが作成・保存したclient ID、private key、certificate fileのどれも
不要です。`dp`が新しいRSA client identityを生成し、本体自身が返すdevice
certificateを検証して完全一致pinとして保存します。Sonyがdesktop Appを提供した
のはWindowsとmacOSであり、Linux版はありませんでした。この独立性はLinuxでも
利用できるclientを作るうえで重要です。

## 公平な比較

| 比較項目 | Sony Digital Paper App | `dpt-rp1-py` | Digital Paper / `dp` |
|---|---|---|---|
| 位置付け | Sony純正の利用者向けGUIと公式workflow | コミュニティ製Python library、CLI、同期、FUSE mount | 独立したGo library、Unix/FTP風CLI、protocol emulator |
| 現在の状態 | 2026年3月31日にSonyのサポートと提供が終了 | 確認した2026年7月13日版で活動中のcommunity project | pre-releaseの独立project。P3 PDF core完成、DPT-RP1実機検証済み |
| sourceとlicense | 純正application。community向けopen source projectではない | MIT licenseのopen source | MIT licenseのopen source |
| install・実行時依存 | 旧純正installerとdesktop runtime。現在Sonyからは提供されない | `pip`で導入するPython 3 package。現行metadataには10個のruntime依存 | 対応環境ごとの静的Go binary一個。実行時にPython、package manager、Sony Appは不要 |
| desktop OS | Sonyの最終公開資料はWindows版とmacOS版のみ。Linux版はない | upstreamはWindows、Linux、macOSでの試験を掲げる | macOS/Linuxのarm64/amd64 binary。Linux自動build済み、Linux実機USB pairingは未検証 |
| 対象機種 | Sony DPT-RP1、DPT-CP1 | upstreamはSony DPT-RP1/CP1とFujitsu QUADERNO対応を掲げる | 本projectで実機確認済みなのはDPT-RP1。DPT-CP1と全QUADERNO世代は明示的に未検証 |
| 登録・資格情報 | 純正pairingとSony管理の資格情報 | PINによる新規登録、またはSony資格情報の再利用 | PINによる新規登録、named profile、明示的Sony資格情報import。新規鍵はSony領域外へ保存 |
| 接続方法 | 純正アプリがUSB・network接続を管理 | 自動探索または明示address。upstreamはWi-Fi、Bluetooth、USBを説明 | direct/relayを明示したnamed profile。`digitalpaper.local`経由のUSB direct接続を実機検証済み |
| 日常のPDF操作 | 純正GUIによる転送、同期、印刷workflow | list、upload、download、copy、move、delete、display、syncを含む広いCLI | `ls`、`file`/`stat`、`get`、`put`、`cp`、`mv`、`mkdir`、安全制約付き`rm`/`rmdir`、`open` |
| P3 PDF core以外 | 純正GUI統合と公式sync workflow | sync、FUSE、Wi-Fi管理、template、設定、screenshot、firmware関連command | 現在は意図的に未実装。sync、backup、renderer、CUPS、OS統合はsoak後へ延期 |
| pathの見せ方 | 本体UIはprotocol内部の`Document/` rootを見せない | upstream CLIでは通常`Document/...`を使用 | filesystem-like CLI UIが`Document/`を拒否して隠し、本体rootからのpathとして扱う |
| TLSによる本体識別 | 純正内部動作。保守可能なopen clientとして外部監査できない | 確認版はHTTPS sessionの証明書検証を無効化 | trust-all modeなし。通常のCA/hostname検証、またはDPT-RP1のSANなし証明書に対するleaf SHA-256完全一致pin |
| 削除・競合の安全性 | 純正GUI側の対話的動作 | 直接delete APIを呼ぶ。upstreamのsync説明は、新しいfileが古いfileを追加警告なしで上書きすると明記 | PDF削除に直前取得revision必須。`rmdir`は空を確認し、recursive force deleteを明示的に無効化 |
| 自動評価 | Sony社内の旧試験は再現可能な公開suiteではない | 確認版にはtest fileやprotocol emulatorが収録されず、GitHub workflowはPython package公開用 | auth・pairing・document emulator、fault injection、unit/integration、race、architecture/spec検査、cross-build |
| 配布と復旧 | Sony管理。サポート終了とともにdownload提供終了 | PyPIまたはsourceからinstall | 再現可能binary/source archive、manifest、SHA-256、owner-only profile、install・復旧文書 |
| 現在もっとも適する用途 | 既にinstall済みで、現在のOS上でも正常に動くlegacy環境 | 広い成熟機能が必要で、Python環境を受け入れられる場合 | 小さく実機確認済みのPDF core、Go組込み、安全境界、再現可能配布を重視する場合 |

## この比較が主張しないこと

- `dp`は現時点で`dpt-rp1-py`の上位互換ではありません。特にsync、FUSE、
  Wi-Fi管理commandはありません。
- DPT-RP1での実機結果は、DPT-CP1やQUADERNOの互換性を証明しません。
  それぞれを別に検証します。
- 単一binaryであるだけで安全になるわけではありません。TLS方針、変更・削除の
  guard、response上限、試験、復旧可能なrelease工程が重要な差です。
- SonyやFujitsuの公式な推奨・保証はありません。hardware修理、firmware保守、
  vendor supportを代替するものでもありません。
- 公開されたcodeと文書を確認した比較であり、先行projectのmaintainerの努力や
  品質を一括評価するものではありません。

## 独自開発に至った経緯

### 1. 純正アプリ依存が継続利用上のriskになった

Digital Paper本体は、用途が明確で単純だからこそ長く使えます。本体の寿命は、
古いOS、installer、証明書処理、vendor配布に依存するdesktop applicationより
長くなり得ます。Sonyのサポート終了によって、この不一致は仮定ではなく現実の
問題になりました。動作する本体があっても、純正applicationとfirmwareはもう
提供されません。

### 2. communityの先行成果が代替可能性を実証した

`dpt-rp1-py`は、直接登録、認証、文書操作、同期、探索、mountを実証しました。
Java版[`DigitalPaperApp`](https://github.com/DPT-RP1/DigitalPaperApp)は調査範囲を
さらに広げ、Polaris 2.0 endpoint資料の翻訳を保存しました。これらは歴史から
消すべき競合ではなく、本projectの重要な参照元・先行成果です。

現在の`dpt-rp1-py`の登録修正も、その価値を示す具体例です。本体がJava
`BigInteger`として生成する256/257 byte表現を、認証transcript内では受信した
raw byteのまま保持する必要があります。本Go実装も同じwire上の必要動作を独立に
実装し、試験しています。

### 3. production環境に対する要求が異なった

想定利用者は普段Python applicationを保守せず、Digital Paperの運用をPython、
`pip`、virtual environment、多数のpackageへ依存させたくありませんでした。
また、一つのCLIだけでなく、将来のnative applicationから再利用できる通信基盤が
必要でした。

そこで逐語的なGo移植ではなく、新しい実装を選びました。公式文書、保存された
Polaris API資料、community code、実機観察を突き合わせますが、公開API、package
境界、error model、path model、資格情報保存、emulator、release工程はこの
projectの要件から設計しています。

## 独自開発の意義

### hardwareを使い続けられる

desktop applicationのサポート終了だけを理由に、正常な読書・筆記hardwareを
e-wasteにする必要はありません。protocolを文書化したclientがあれば、所有者は
回復可能な方法で利用を継続できます。

### 純正アプリから運用を独立できる

Sony App終了後も、新規pairingした資格情報によるdirect認証が成功しています。
新しいidentityは独立保存され、Sony Appのprivate workspaceを参照も変更も
しません。Sony Appのinstall、起動、またはSony Appによる資格情報fileの作成は
不要です。client keyは`dp`自身が生成し、本体が返すcertificateからTLS完全一致
pinを確立します。

### 通信libraryを他のapplicationへ組み込める

公開Go packageはCLI、profile、将来のrenderer、OS統合から分離されています。
macOS application、print adapter、その他のtoolが、CLI subprocessを介さず同じ
認証clientを利用できます。

### 想像ではなく検証levelを記録できる

原典記載、emulator動作、実機動作を同じ「対応済み」とは扱いません。model、
firmware、日付、transport、redact済み結果が記録されたoperationだけを
`device-verified`へ昇格します。firmware固有quirkも全機種へ暗黙に一般化せず、
compatibility recordへ残します。

### 長期保守時の事故を減らせる

破壊的commandは狭く、errorは明示的で、response sizeには上限があります。
TLS trustを全面無効化できず、資格情報はowner-only権限でatomicに保存されます。
将来用commandをdummy成功させません。再現可能archiveとsource bundleにより、
最初の開発machineへの依存も減らします。

## 設計要件の概要

1. **単一binaryで配布する。** macOS/Linuxのarm64/amd64を、production用language
   runtimeやCGO共有libraryなしで動かす。
2. **Go通信libraryを独立させる。** protocol clientはCLI parsing、profile保存、
   sync、renderer、OS統合へ依存しない。
3. **本体へ直接認証する。** 新規PIN pairing、既存資格情報import、named device、
   Sony Appなしの運用に対応する。
4. **TLSで相手を識別する。** 通常はCA/hostname検証を使い、legacy deviceの制約は
   記録された完全一致certificate pinで扱う。
5. **既知のcommandを自然に使えるようにする。** `ls`、`cp`、`mv`、`mkdir`、
   `get`、`put`を採用し、protocol内部の`Document/`を隠す。
6. **不可逆変更をguardする。** 既定で上書きせず、PDF削除にはcurrent revisionを
   必須とし、root削除を拒否し、空folderの非recursive削除だけを許可する。
7. **実機なしで試験する。** 登録、認証、PDF操作、pagination、障害、競合を
   emulateし、実機検証とは別に記録する。
8. **protocol根拠を保存する。** reference revisionとchecksumを固定し、
   compatibility catalogをmachine-readableにし、差異を文書化する。
9. **再現可能にreleaseする。** cross-build、race test、checksum、source同梱、
   toolchainとcommit記録、install・rollback・資格情報復旧手順を用意する。
10. **段階的に拡張する。** PDF coreを実使用で安定させてから、sync、backup、
    CUPS、Markdown、LaTeX、Tectonicへ進む。

## 現在の範囲

P3はPDFだけを日常利用するrelease candidateであり、万能device managerの完成版では
ありません。Sony DPT-RP1 firmware `1.6.50.14130`では、新規pairing、Sony App
終了後のdirect認証、status、listing、download、upload/replacement、本体内
copy/move、folder作成、安全制約付きdeleteを実機検証済みです。試験中、本体は
一貫してUSB接続でした。新規pairingと独立認証はUSB Ethernet endpointへ直接接続し、
それ以前の文書操作試験はSony Appのloopback relayを使用しました。viewer openは
emulator試験のみです。他機種と延期機能は明示的に未検証・未実装として残します。

## 原典と謝辞

- [Sony：「デジタルペーパー」およびDigital Paper Appサポート終了](https://www.sony.jp/digital-paper/info2/20240628.html)
- [Sony Help Guide：computerから文書を転送](https://helpguide.sony.net/dpt/rp1/v1/en/contents/TP0001178322.html)
- [Sony最後のDigital Paper App update：Windows版・macOS版](https://www.sony.jp/digital-paper/update/#20240628)
- [確認版`dpt-rp1-py` README](https://github.com/janten/dpt-rp1-py/blob/9dda9d9a16c20477bd19374866e2095705765f96/README.md)
- [`dpt-rp1-py` package metadataと依存関係](https://github.com/janten/dpt-rp1-py/blob/9dda9d9a16c20477bd19374866e2095705765f96/setup.json)
- [`dpt-rp1-py`のTLS session設定](https://github.com/janten/dpt-rp1-py/blob/9dda9d9a16c20477bd19374866e2095705765f96/dptrp1/dptrp1.py#L163-L164)
- [`dpt-rp1-py`の256/257 byte登録修正](https://github.com/janten/dpt-rp1-py/commit/9dda9d9a16c20477bd19374866e2095705765f96)
- [Java DigitalPaperAppと保存されたendpoint翻訳](https://github.com/DPT-RP1/DigitalPaperApp)
- repository内のprotocol provenance：`spec/references/provenance.json`
- repository内の実機検証記録：`spec/compat/models.json`

`dpt-rp1-py`とJava版DigitalPaperAppは、それぞれのcontributorによるMIT licenseの
projectです。その成果をここ、およびrepositoryのprovenance recordで明示的に
creditします。本Digital Paper projectはSonyおよびFujitsuから独立しており、
提携、推奨、支援を受けたものではありません。
