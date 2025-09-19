# prompt

[![Go Reference](https://pkg.go.dev/badge/github.com/nao1215/prompt.svg)](https://pkg.go.dev/github.com/nao1215/prompt)
[![Go Report Card](https://goreportcard.com/badge/github.com/nao1215/prompt)](https://goreportcard.com/report/github.com/nao1215/prompt)
[![MultiPlatformUnitTest](https://github.com/nao1215/prompt/actions/workflows/unit_test.yml/badge.svg)](https://github.com/nao1215/prompt/actions/workflows/unit_test.yml)

[English](../../README.md) | [日本語](../ja/README.md) | [Русский](../ru/README.md) | **中文** | [한국어](../ko/README.md) | [Español](../es/README.md) | [Français](../fr/README.md)

![logo](../img/logo-small.png)

**prompt** 是一个为 Go 提供强大交互式命令行界面的简单终端提示库。该库被设计为不再维护的 [c-bata/go-prompt](https://github.com/c-bata/go-prompt) 库的替代品，解决了关键问题的同时添加了增强功能和更好的跨平台支持。

![sample](../img/demo.gif)

## 🎯 为什么选择 prompt？

原始的 [go-prompt](https://github.com/c-bata/go-prompt) 库自2021年3月以来就没有维护，有286个开放问题和众多关键错误，包括：

- **终端渲染中的除零恐慌** (问题 #277)
- **Windows 终端兼容性问题** (问题 #285)
- **/dev/tty 处理中的文件描述符泄露** (问题 #253)
- **STDIN 之外的有限 TTY 支持** (问题 #275)
- **应用程序退出时的终端颜色重置问题** (问题 #265)

该库解决了所有这些问题，同时提供了具有全面测试覆盖的更简单、更可维护的代码库。

## ✨ 特性

- 🖥️ **跨平台支持** - 在 Linux、macOS 和 Windows 上无缝工作
- 🔍 **高级自动补全** - 具有模糊匹配和可自定义建议的 Tab 补全
- 📚 **命令历史** - 使用方向键导航、持久历史和反向搜索 (Ctrl+R)
- ⌨️ **丰富的键绑定** - 包括 Emacs 风格导航的全面快捷键
- 🌈 **颜色主题** - 内置颜色方案和可自定义主题
- 📝 **多行输入** - 支持具有适当光标导航的多行输入
- ⚡ **高性能** - 高效渲染和最小分配
- 🧪 **全面测试** - 54.5% 测试覆盖率，具有广泛的跨平台测试
- 🔧 **简单 API** - 采用函数选项模式的清洁、现代 API 设计
- 🛠️ **资源管理** - 适当的清理和没有文件描述符泄露

## 📦 安装

```bash
go get github.com/nao1215/prompt
```

## 🔧 要求

- **Go 版本**: 1.24 或更高
- **操作系统**:
  - Linux
  - macOS
  - Windows

## 🚀 快速开始

### 基本用法

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
                fmt.Println("再见！")
                break
            }
            log.Printf("错误: %v\n", err)
            continue
        }

        if input == "exit" {
            break
        }
        fmt.Printf("您输入了: %s\n", input)
    }
}
```

### 带自动补全

```go
package main

import (
    "errors"
    "log"
    "github.com/nao1215/prompt"
)

func completer(d prompt.Document) []prompt.Suggestion {
    return []prompt.Suggestion{
        {Text: "help", Description: "显示帮助信息"},
        {Text: "users", Description: "列出所有用户"},
        {Text: "groups", Description: "列出所有组"},
        {Text: "exit", Description: "退出程序"},
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
        // 处理命令...
    }
}
```

### 带历史和高级功能

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
    // 创建带历史和超时的提示
    p, err := prompt.New(">>> ",
        prompt.WithMemoryHistory(100),
        prompt.WithColorScheme(prompt.ThemeDracula),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer p.Close()

    // 使用上下文支持超时
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    input, err := p.RunWithContext(ctx)
    if err == context.DeadlineExceeded {
        fmt.Println("达到超时")
        return
    }

    fmt.Printf("输入: %s\n", input)
}
```

## ⌨️ 键绑定

该库从一开始就支持全面的键绑定：

| 键 | 动作 |
|----|------|
| Enter | 提交输入 |
| Ctrl+C | 取消并返回 ErrInterrupted |
| Ctrl+D | 缓冲区为空时的 EOF |
| ↑/↓ | 导航历史（多行模式下的行） |
| ←/→ | 移动光标 |
| Ctrl+A / Home | 移动到行首 |
| Ctrl+E / End | 移动到行尾 |
| Ctrl+K | 从光标删除到行尾 |
| Ctrl+U | 删除整行 |
| Ctrl+W | 向后删除单词 |
| Ctrl+R | 历史反向搜索 |
| Tab | 自动补全 |
| Backspace | 向后删除字符 |
| Delete | 向前删除字符 |
| Ctrl+←/→ | 按单词边界移动 |

## 🎨 颜色主题

内置颜色主题：

```go
// 可用主题
prompt.ThemeDefault
prompt.ThemeDracula
prompt.ThemeNightOwl
prompt.ThemeMonokai
prompt.ThemeSolarizedDark
prompt.ThemeSolarizedLight

// 用法
p, err := prompt.New("$ ",
    prompt.WithColorScheme(prompt.ThemeDracula),
)
```

## ⚠️ 重要说明

### 线程安全性
⚠️ **重要**: 该库**不是线程安全的**:
- **不要**在 goroutine 之间共享提示实例
- **不要**在同一个提示实例上并发调用方法
- **不要**在另一个 goroutine 中 `Run()` 活跃时调用 `Close()`
- 如果需要，对并发操作使用单独的提示实例

### 资源管理
- **完成提示后始终调用 `Close()`** 以防止资源泄露
- `Close()` 方法可以安全地多次调用
- 即使 `Run()` 或 `RunWithContext()` 返回错误也要调用 `Close()`

### 错误处理
该库提供特定的错误类型：
- `prompt.ErrEOF`: 用户在缓冲区为空时按下 Ctrl+D
- `prompt.ErrInterrupted`: 用户按下 Ctrl+C
- `context.DeadlineExceeded`: 达到超时（使用上下文时）
- `context.Canceled`: 上下文被取消

## 🧪 测试

运行测试套件：

```bash
make test    # 运行带覆盖率的测试
make lint    # 运行代码检查
make clean   # 清理生成的文件
```

## 🤝 贡献

欢迎贡献！请查看[贡献指南](../../CONTRIBUTING.md)了解详情。

### 开发要求

- Go 1.24 或更高版本
- 用于代码质量的 golangci-lint
- 在 Linux、macOS 和 Windows 上进行跨平台测试

## 💖 支持

如果您觉得这个项目有用，请考虑：

- ⭐ 在 GitHub 上给它加星 - 这有助于其他人发现这个项目
- 💝 [成为赞助者](https://github.com/sponsors/nao1215) - 您的支持让项目保持活力并激励持续开发

您通过星标、赞助或贡献的支持，是推动这个项目前进的动力。谢谢！

## 📄 许可证

该项目在 MIT 许可证下授权 - 有关详细信息，请参阅 [LICENSE](../../LICENSE) 文件。