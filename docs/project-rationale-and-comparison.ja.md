# DPWireを独自開発する理由

この文書では、このGo実装を独自に開発した経緯と意義、Sony純正Digital
Paper Appおよび`dpt-rp1-py`との違い、設計要件を説明します。
[English version](project-rationale-and-comparison.md)もあります。

比較確認日：2026年8月6日。

## 結論

Sonyは2026年3月31日をもってDPT-RP1とDPT-CP1の修理・サポートを終了し、
Digital Paper Appと本体ファームウェアの提供も終了したと公式に発表して
います。独立して保守できるclientがあれば、vendor applicationの提供終了後も
動作する本体を継続利用できます。

一方、オープンソースの[`dpt-rp1-py`](https://github.com/janten/dpt-rp1-py)
は、既に実用的なPythonライブラリとCLIを提供しています。これは先駆的で重要な
成果であり、現在も活動しています。確認した最新版には2026年7月13日付で、
断続的に生じる登録失敗の修正が取り込まれています。機能には、同期、FUSE mount、
Wi-Fi設定なども含まれます。

DPWireは、長期保守可能で他のGoアプリへ組み込める通信ライブラリ、実行時依存の
ない単一`dp` binary、厳格なTLS識別、
破壊的操作の安全制約、実機なしで動くprotocol emulator、再現可能な配布、
機種・firmware別の実機検証記録を提供するための独立実装です。

`dp` CLIは、UNIXのfile操作やFTPに慣れた利用者が、新しい独自操作体系を覚える
負担を抑え、既知のcommandとpath表現で本体の基本機能をできるだけ違和感なく
活用できるように設計しています。このfilesystem-like interfaceはcommand UIで
あり、filesystem mountではありません。

新規pairingでは`dp`が本体へ直接接続し、新しいRSA client identityを生成します。
本体自身が返すdevice certificateを検証して完全一致pinとして保存します。これに
よりmacOSとLinuxで運用できます。Sonyが公開したdesktop AppはWindows版とmacOS版
でした。

## 公平な比較

| 比較項目 | Sony Digital Paper App | `dpt-rp1-py` | DPWire / `dp` |
|---|---|---|---|
| 位置付け | Sony純正の利用者向けGUIと公式workflow | コミュニティ製Python library、CLI、同期、FUSE mount | 独立したGo library、Unix/FTP風CLI、protocol emulator |
| 現在の状態 | 2026年3月31日にSonyのサポートと提供が終了 | 確認した2026年7月13日版で活動中のcommunity project | pre-releaseの独立project。PDF core完成、DPT-RP1実機検証済み |
| sourceとlicense | 純正application。community向けopen source projectではない | MIT licenseのopen source | MIT licenseのopen source |
| install・実行時依存 | 旧純正installerとdesktop runtime。現在Sonyからは提供されない | `pip`で導入するPython 3 package。現行metadataには10個のruntime依存 | 対応環境ごとの静的Go binary一個。実行時にPython、package manager、Sony Appは不要 |
| desktop OS | Sonyの最終公開資料はWindows版とmacOS版のみ。Linux版はない | upstreamはWindows、Linux、macOSでの試験を掲げる | macOS/Linuxのarm64/amd64 binary。Linux自動build済み、Linux実機USB pairingは未検証 |
| 対象機種 | Sony DPT-RP1、DPT-CP1 | upstreamはSony DPT-RP1/CP1とFujitsu QUADERNO対応を掲げる | 本projectで実機確認済みなのはDPT-RP1。DPT-CP1と全QUADERNO世代は明示的に未検証 |
| 登録・資格情報 | 純正pairingとSony管理の資格情報 | PINによる新規登録、またはSony資格情報の再利用 | PINによる新規登録、named profile、明示的Sony資格情報import。新規鍵はSony領域外へ保存 |
| 接続方法 | 純正アプリがUSB・network接続を管理 | 自動探索または明示address。upstreamはWi-Fi、Bluetooth、USBを説明 | direct/relayを明示したnamed profile。`digitalpaper.local`経由のUSB direct接続を実機検証済み |
| 日常のPDF操作 | 純正GUIによる転送、同期、印刷workflow | list、upload、download、copy、move、delete、display、syncを含む広いCLI | `ls`、`file`/`stat`、`get`、`put`、`cp`、`mv`、`mkdir`、安全制約付き`rm`/`rmdir`、`open` |
| 拡張workflow | 純正GUI統合と公式sync workflow | sync、FUSE、Wi-Fi管理、template、設定、screenshot、firmware関連command | sync、backup、renderer、CUPS、OS統合はPDF coreのsoak後に実装予定 |
| pathの見せ方 | 本体UIはprotocol内部の`Document/` rootを見せない | upstream CLIでは通常`Document/...`を使用 | filesystem-like CLI UIが`Document/`を拒否して隠し、本体rootからのpathとして扱う |
| TLSによる本体識別 | 純正内部動作。保守可能なopen clientとして外部監査できない | 確認版はHTTPS sessionの証明書検証を無効化 | trust-all modeなし。通常のCA/hostname検証、またはDPT-RP1のSANなし証明書に対するleaf SHA-256完全一致pin |
| 削除・競合の安全性 | 純正GUI側の対話的動作 | 直接delete APIを呼ぶ。upstreamのsync説明は、新しいfileが古いfileを追加警告なしで上書きすると明記 | PDF削除に直前取得revision必須。`rmdir`は空を確認し、recursive force deleteを明示的に無効化 |
| 自動評価 | Sony社内の旧試験は再現可能な公開suiteではない | 確認版にはtest fileやprotocol emulatorが収録されず、GitHub workflowはPython package公開用 | auth・pairing・document emulator、fault injection、unit/integration、race、architecture/spec検査、cross-build |
| 配布と復旧 | Sony管理。サポート終了とともにdownload提供終了 | PyPIまたはsourceからinstall | 再現可能binary/source archive、manifest、SHA-256、owner-only profile、install・復旧文書 |
| 現在もっとも適する用途 | 既にinstall済みで、現在のOS上でも正常に動くlegacy環境 | 広い成熟機能が必要で、Python環境を受け入れられる場合 | 小さく実機確認済みのPDF core、Go組込み、安全境界、再現可能配布を重視する場合 |

## 独自開発に至った経緯

### 1. 純正アプリ依存が継続利用上のriskになった

Digital Paper本体は、用途が明確で単純だからこそ長く使えます。本体の寿命は、
古いOS、installer、証明書処理、vendor配布に依存するdesktop applicationより
長くなり得ます。Sonyのサポート終了によって、この不一致は現実の
問題になりました。動作する本体があっても、純正applicationとfirmwareはもう
提供されません。

### 2. communityの先行成果が代替可能性を実証した

`dpt-rp1-py`は、直接登録、認証、文書操作、同期、探索、mountを実証しました。
Java版[`DigitalPaperApp`](https://github.com/DPT-RP1/DigitalPaperApp)は調査範囲を
さらに広げ、Polaris 2.0 endpoint資料の翻訳を保存しました。

現在の`dpt-rp1-py`の登録修正も、その価値を示す具体例です。本体がJava
`BigInteger`として生成する256/257 byte表現を、認証transcript内では受信した
raw byteのまま保持する必要があります。本Go実装も同じwire上の必要動作を独立に
実装し、試験しています。

### 3. production環境に対する要求が異なった

production環境にはGoとself-contained executableを採用します。将来のnative
applicationから再利用できる公開通信libraryも必要でした。

そこで独立設計の新しい実装を選びました。公式文書、保存された
Polaris API資料、community code、実機観察を突き合わせますが、公開API、package
境界、error model、path model、資格情報保存、emulator、release工程はこの
projectの要件から設計しています。

## 独自開発の意義

### hardwareを使い続けられる

protocolを文書化したclientにより、正常な読書・筆記hardwareを回復可能な方法で
継続利用できます。

### 純正アプリから運用を独立できる

Sony App終了後も、新規pairingした資格情報によるdirect認証が成功しています。
client keyは`dp`自身が生成して独立保存し、本体が返すcertificateからTLS完全一致
pinを確立します。

### 通信libraryを他のapplicationへ組み込める

公開Go packageはCLI、profile、将来のrenderer、OS統合から分離されています。
macOS application、print adapter、その他のtoolが、CLI subprocessを介さず同じ
認証clientを利用できます。

### compatibilityの検証levelを記録できる

原典記載、emulator動作、実機動作を同じ「対応済み」とは扱いません。model、
firmware、日付、transport、redact済み結果が記録されたoperationだけを
`device-verified`へ昇格します。firmware固有quirkも全機種へ暗黙に一般化せず、
compatibility recordへ残します。

### 長期保守時の事故を減らせる

破壊的commandは狭く、errorは明示的で、response sizeには上限があります。
TLS trustは検証方式に固定し、資格情報はowner-only権限でatomicに保存されます。
将来用commandは明示的errorを返します。再現可能archiveとsource bundleにより、
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

現在の範囲は、PDFだけを日常利用するrelease candidateです。Sony DPT-RP1
firmware `1.6.50.14130`では、新規pairing、Sony App
終了後のdirect認証、status、listing、recursive listing、download、
upload/replacement、本体内copy/move、folder作成、安全制約付きdelete、viewer openを
実機検証済みです。試験中、本体は
一貫してUSB接続でした。新規pairingと独立認証はUSB Ethernet endpointへ直接接続し、
それ以前の文書操作試験はSony Appのloopback relayを使用しました。他機種と延期機能は
明示的に未検証・未実装として残します。

## 原典と謝辞

本projectでは安全性について可能な限り試験しています。すべての利用は利用者自身の
責任です。保証と責任の条件はMIT Licenseに記載しています。

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
creditします。
