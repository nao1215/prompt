# prompt

[![Go Reference](https://pkg.go.dev/badge/github.com/nao1215/prompt.svg)](https://pkg.go.dev/github.com/nao1215/prompt)
[![Go Report Card](https://goreportcard.com/badge/github.com/nao1215/prompt)](https://goreportcard.com/report/github.com/nao1215/prompt)
[![MultiPlatformUnitTest](https://github.com/nao1215/prompt/actions/workflows/unit_test.yml/badge.svg)](https://github.com/nao1215/prompt/actions/workflows/unit_test.yml)

[English](../../README.md) | **日本語** | [Русский](../ru/README.md) | [中文](../zh-cn/README.md) | [한국어](../ko/README.md) | [Español](../es/README.md) | [Français](../fr/README.md)

![logo](../img/logo-small.png)

**prompt** は、強力なインタラクティブコマンドラインインターフェースを提供するGo用のシンプルなターミナルプロンプトライブラリです。このライブラリは、メンテナンスされていない [c-bata/go-prompt](https://github.com/c-bata/go-prompt) ライブラリの代替として設計されており、重要な問題を解決しながら機能を強化し、クロスプラットフォームサポートを向上させています。

![sample](../img/demo.gif)

## 🎯 なぜpromptなのか？

元の [go-prompt](https://github.com/c-bata/go-prompt) ライブラリは2021年3月からメンテナンスされておらず、286個のオープンイシューと以下のような数多くの重要なバグがあります：

- **ターミナルレンダリングでのゼロ除算パニック** (issue #277)
- **Windowsターミナル互換性問題** (issue #285)
- **/dev/tty処理でのファイルディスクリプタリーク** (issue #253)
- **STDIN以外の限定的なTTYサポート** (issue #275)
- **アプリケーション終了時のターミナル色リセット問題** (issue #265)

このライブラリは、これらすべての問題を解決しながら、包括的なテストカバレッジを持つ、よりシンプルで保守しやすいコードベースを提供します。

## ✨ 機能

- 🖥️ **クロスプラットフォームサポート** - Linux、macOS、Windowsでシームレスに動作
- 🔍 **高度なオートコンプリート** - ファジーマッチングとカスタマイズ可能な候補によるTabコンプリート
- 📚 **コマンド履歴** - 矢印キーでのナビゲーション、永続化履歴、逆検索 (Ctrl+R)
- ⌨️ **豊富なキーバインディング** - Emacs風ナビゲーションを含む包括的なショートカット
- 🌈 **カラーテーマ** - 組み込みカラースキームとカスタマイズ可能なテーマ
- 📝 **マルチライン入力** - 適切なカーソルナビゲーションによるマルチライン入力サポート
- ⚡ **高性能** - 効率的なレンダリングと最小限のアロケーション
- 🧪 **包括的テスト** - 広範なクロスプラットフォームテストによる54.5%のテストカバレッジ
- 🔧 **シンプルなAPI** - 関数オプションパターンによるクリーンでモダンなAPI設計
- 🛠️ **リソース管理** - 適切なクリーンアップとファイルディスクリプタリークなし

## 📦 インストール

```bash
go get github.com/nao1215/prompt
```

## 🔧 要件

- **Goバージョン**: 1.24以降
- **オペレーティングシステム**:
  - Linux
  - macOS
  - Windows

## 🚀 クイックスタート

### 基本的な使用方法

```go
package main

import (
    "errors"
    "fmt"
    "log"
    "github.com/nao1215/prompt"
)

func main() {
    p, err := prompt.New("$ ")
    if err != nil {
        log.Fatal(err)
    }
    defer p.Close()

    for {
        input, err := p.Run()
        if err != nil {
            if errors.Is(err, prompt.ErrEOF) {
                fmt.Println("さようなら！")
                break
            }
            log.Printf("エラー: %v\n", err)
            continue
        }

        if input == "exit" {
            break
        }
        fmt.Printf("入力されました: %s\n", input)
    }
}
```

### オートコンプリート付き

```go
package main

import (
    "errors"
    "log"
    "github.com/nao1215/prompt"
)

func completer(d prompt.Document) []prompt.Suggestion {
    return []prompt.Suggestion{
        {Text: "help", Description: "ヘルプメッセージを表示"},
        {Text: "users", Description: "すべてのユーザーを一覧表示"},
        {Text: "groups", Description: "すべてのグループを一覧表示"},
        {Text: "exit", Description: "プログラムを終了"},
    }
}

func main() {
    p, err := prompt.New("myapp> ",
        prompt.WithCompleter(completer),
        prompt.WithColorScheme(prompt.ThemeNightOwl),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer p.Close()

    for {
        input, err := p.Run()
        if err != nil {
            if errors.Is(err, prompt.ErrEOF) {
                break
            }
            continue
        }

        if input == "exit" {
            break
        }
        // コマンドを処理...
    }
}
```

### 履歴と高度な機能付き

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"
    "github.com/nao1215/prompt"
)

func main() {
    // 履歴とタイムアウト付きプロンプトを作成
    p, err := prompt.New(">>> ",
        prompt.WithMemoryHistory(100),
        prompt.WithColorScheme(prompt.ThemeDracula),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer p.Close()

    // タイムアウトサポートのためのコンテキストを使用
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    input, err := p.RunWithContext(ctx)
    if err == context.DeadlineExceeded {
        fmt.Println("タイムアウトに達しました")
        return
    }

    fmt.Printf("入力: %s\n", input)
}
```

## ⌨️ キーバインディング

ライブラリは最初から包括的なキーバインディングをサポートしています：

| キー | アクション |
|------|-----------|
| Enter | 入力を送信 |
| Ctrl+C | キャンセルしてErrInterruptedを返す |
| Ctrl+D | バッファが空の時にEOF |
| ↑/↓ | 履歴をナビゲート（マルチラインモードでは行） |
| ←/→ | カーソルを移動 |
| Ctrl+A / Home | 行の最初に移動 |
| Ctrl+E / End | 行の最後に移動 |
| Ctrl+K | カーソルから行末まで削除 |
| Ctrl+U | 行全体を削除 |
| Ctrl+W | 後ろの単語を削除 |
| Ctrl+R | 履歴逆検索 |
| Tab | オートコンプリート |
| Backspace | 後ろの文字を削除 |
| Delete | 前の文字を削除 |
| Ctrl+←/→ | 単語境界で移動 |

## 🎨 カラーテーマ

組み込みカラーテーマ：

```go
// 利用可能なテーマ
prompt.ThemeDefault
prompt.ThemeDracula
prompt.ThemeNightOwl
prompt.ThemeMonokai
prompt.ThemeSolarizedDark
prompt.ThemeSolarizedLight

// 使用方法
p, err := prompt.New("$ ",
    prompt.WithColorScheme(prompt.ThemeDracula),
)
```

## ⚠️ 重要な注意事項

### スレッドセーフティ
⚠️ **重要**: このライブラリは**スレッドセーフではありません**:
- **ゴルーチン間でプロンプトインスタンスを共有しないでください**
- **同じプロンプトインスタンスでメソッドを同時に呼び出さないでください**
- **別のゴルーチンで`Run()`がアクティブな間に`Close()`を呼び出さないでください**
- 必要に応じて、並行操作には別々のプロンプトインスタンスを使用してください

### リソース管理
- **プロンプトが完了したら必ず`Close()`を呼び出して**リソースリークを防いでください
- `Close()`メソッドは複数回呼び出しても安全です
- `Run()`や`RunWithContext()`がエラーを返す場合でも`Close()`を呼び出してください

### エラーハンドリング
ライブラリは特定のエラータイプを提供します：
- `prompt.ErrEOF`: ユーザーがバッファが空の状態でCtrl+Dを押した
- `prompt.ErrInterrupted`: ユーザーがCtrl+Cを押した
- `context.DeadlineExceeded`: タイムアウトに達した（コンテキスト使用時）
- `context.Canceled`: コンテキストがキャンセルされた

## 🧪 テスト

テストスイートを実行：

```bash
make test    # カバレッジ付きテストを実行
make lint    # リンターを実行
make clean   # 生成されたファイルをクリーンアップ
```

## 🤝 貢献

貢献を歓迎します！詳細については[貢献ガイド](../../CONTRIBUTING.md)を参照してください。

### 開発要件

- Go 1.24以降
- コード品質のためのgolangci-lint
- Linux、macOS、Windowsでのクロスプラットフォームテスト

## 💖 サポート

このプロジェクトが役立つと思われる場合は、以下をご検討ください：

- ⭐ GitHubでスターを付ける - 他の人がプロジェクトを発見するのに役立ちます
- 💝 [スポンサーになる](https://github.com/sponsors/nao1215) - あなたのサポートがプロジェクトを維持し、継続的な開発を促進します

スター、スポンサーシップ、貢献を通じたあなたのサポートが、このプロジェクトを前進させるものです。ありがとうございます！

## 📄 ライセンス

このプロジェクトはMITライセンスの下でライセンスされています - 詳細については[LICENSE](../../LICENSE)ファイルを参照してください。