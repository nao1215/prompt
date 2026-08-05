# prompt

[![Go Reference](https://pkg.go.dev/badge/github.com/nao1215/prompt.svg)](https://pkg.go.dev/github.com/nao1215/prompt)
[![Go Report Card](https://goreportcard.com/badge/github.com/nao1215/prompt)](https://goreportcard.com/report/github.com/nao1215/prompt)
[![MultiPlatformUnitTest](https://github.com/nao1215/prompt/actions/workflows/unit_test.yml/badge.svg)](https://github.com/nao1215/prompt/actions/workflows/unit_test.yml)

[English](../../README.md) | [日本語](../ja/README.md) | **Русский** | [中文](../zh-cn/README.md) | [한국어](../ko/README.md) | [Español](../es/README.md) | [Français](../fr/README.md)

![logo](../img/logo-small.png)

**prompt** — это простая библиотека терминальных приглашений для Go, предоставляющая мощные интерактивные интерфейсы командной строки. Эта библиотека разработана как замена неподдерживаемой библиотеки [c-bata/go-prompt](https://github.com/c-bata/go-prompt), решая критические проблемы и добавляя улучшенные функции и лучшую кроссплатформенную поддержку.

![sample](../img/demo.gif)

## 🎯 Почему prompt?

Оригинальная библиотека [go-prompt](https://github.com/c-bata/go-prompt) не поддерживается с марта 2021 года, имеет 286 открытых проблем и множество критических ошибок, включая:

- **Паники деления на ноль** в рендеринге терминала (проблема #277)
- **Проблемы совместимости с терминалом Windows** (проблема #285)
- **Утечки файловых дескрипторов** при обработке /dev/tty (проблема #253)
- **Ограниченная поддержка TTY** за пределами STDIN (проблема #275)
- **Проблемы сброса цветов терминала** при выходе из приложения (проблема #265)

Эта библиотека решает все эти проблемы, предоставляя более простую и поддерживаемую кодовую базу с комплексным покрытием тестами.

## ✨ Возможности

- 🖥️ **Кроссплатформенная поддержка** - Беспроблемно работает на Linux, macOS и Windows
- 🔍 **Продвинутое автодополнение** - Tab-дополнение с нечетким поиском и настраиваемыми предложениями
- 📚 **История команд** - Навигация с помощью стрелок, постоянная история и обратный поиск (Ctrl+R)
- ⌨️ **Богатые клавиатурные привязки** - Комплексные горячие клавиши, включая навигацию в стиле Emacs
- 🌈 **Цветовые темы** - Встроенные цветовые схемы и настраиваемые темы
- 📝 **Многострочный ввод** - Поддержка многострочного ввода с правильной навигацией курсора
- ⚡ **Высокая производительность** - Эффективный рендеринг и минимальные аллокации
- 🧪 **Комплексное тестирование** - 54.5% покрытие тестами с обширным кроссплатформенным тестированием
- 🔧 **Простое API** - Чистый, современный дизайн API с паттерном функциональных опций
- 🛠️ **Управление ресурсами** - Правильная очистка и отсутствие утечек файловых дескрипторов

## 📦 Установка

```bash
go get github.com/nao1215/prompt
```

## 🔧 Требования

- **Версия Go**: 1.24 или новее
- **Операционные системы**:
  - Linux
  - macOS
  - Windows

## 🚀 Быстрый старт

### Базовое использование

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
                fmt.Println("До свидания!")
                break
            }
            log.Printf("Ошибка: %v\n", err)
            continue
        }

        if input == "exit" {
            break
        }
        fmt.Printf("Вы ввели: %s\n", input)
    }
}
```

### С автодополнением

```go
package main

import (
    "errors"
    "log"
    "github.com/nao1215/prompt"
)

func completer(d prompt.Document) []prompt.Suggestion {
    return []prompt.Suggestion{
        {Text: "help", Description: "Показать справочное сообщение"},
        {Text: "users", Description: "Список всех пользователей"},
        {Text: "groups", Description: "Список всех групп"},
        {Text: "exit", Description: "Выйти из программы"},
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
        // Обработка команд...
    }
}
```

### С историей и продвинутыми функциями

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
    // Создание приглашения с историей и таймаутом
    p, err := prompt.New(">>> ",
        prompt.WithMemoryHistory(100),
        prompt.WithColorScheme(prompt.ThemeDracula),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer p.Close()

    // Использование контекста для поддержки таймаутов
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    input, err := p.RunWithContext(ctx)
    if err == context.DeadlineExceeded {
        fmt.Println("Достигнут таймаут")
        return
    }

    fmt.Printf("Ввод: %s\n", input)
}
```

## ⌨️ Клавиатурные привязки

Библиотека поддерживает комплексные клавиатурные привязки из коробки:

| Клавиша | Действие |
|---------|----------|
| Enter | Отправить ввод |
| Ctrl+C | Отбросить текущую строку и вернуть ErrInterrupted |
| Ctrl+D | EOF когда буфер пуст |
| ↑/↓ | Навигация по истории (или строкам в многострочном режиме) |
| ←/→ | Движение курсора |
| Ctrl+A / Home | Переместиться к началу строки |
| Ctrl+E / End | Переместиться к концу строки |
| Ctrl+K | Удалить от курсора до конца строки |
| Ctrl+U | Удалить всю строку |
| Ctrl+W | Удалить слово назад |
| Ctrl+R | Обратный поиск по истории |
| Tab | Автодополнение |
| Backspace | Удалить символ назад |
| Delete | Удалить символ вперед |
| Ctrl+←/→ | Движение по границам слов |
| Esc | Закрыть список автодополнения |

## 🎨 Цветовые темы

Встроенные цветовые темы:

```go
// Доступные темы
prompt.ThemeDefault
prompt.ThemeDracula
prompt.ThemeNightOwl
prompt.ThemeMonokai
prompt.ThemeSolarizedDark
prompt.ThemeSolarizedLight

// Использование
p, err := prompt.New("$ ",
    prompt.WithColorScheme(prompt.ThemeDracula),
)
```

## ⚠️ Важные замечания

### Потокобезопасность
⚠️ **ВАЖНО**: Эта библиотека **НЕ является потокобезопасной**:
- **НЕ делитесь** экземплярами приглашений между горутинами
- **НЕ вызывайте** методы одновременно на одном экземпляре приглашения
- **НЕ вызывайте** `Close()` пока `Run()` активен в другой горутине
- При необходимости используйте отдельные экземпляры приглашений для одновременных операций

### Управление ресурсами
- **Всегда вызывайте `Close()`** когда закончили с приглашением, чтобы предотвратить утечки ресурсов
- Метод `Close()` безопасно вызывать несколько раз
- Вызывайте `Close()` даже если `Run()` или `RunWithContext()` возвращают ошибку

### Обработка ошибок
Библиотека предоставляет специфические типы ошибок:
- `prompt.ErrEOF`: Пользователь нажал Ctrl+D с пустым буфером
- `prompt.ErrInterrupted`: Пользователь нажал Ctrl+C
- `context.DeadlineExceeded`: Достигнут таймаут (при использовании контекста)
- `context.Canceled`: Контекст был отменен

## 🧪 Тестирование

Запуск тестового набора:

```bash
make test    # Запустить тесты с покрытием
make lint    # Запустить линтер
make clean   # Очистить сгенерированные файлы
```

## 🤝 Вклад в проект

Вклады приветствуются! Пожалуйста, смотрите [Руководство по вкладу](../../CONTRIBUTING.md) для подробностей.

### Требования для разработки

- Go 1.24 или новее
- golangci-lint для качества кода
- Кроссплатформенное тестирование на Linux, macOS и Windows

## 💖 Поддержка

Если вы находите этот проект полезным, пожалуйста, рассмотрите:

- ⭐ Поставить звезду на GitHub - это помогает другим обнаружить проект
- 💝 [Стать спонсором](https://github.com/sponsors/nao1215) - ваша поддержка поддерживает проект и мотивирует продолжение разработки

Ваша поддержка, будь то через звезды, спонсорство или вклады, это то, что движет этот проект вперед. Спасибо!

## 📄 Лицензия

Этот проект лицензирован под лицензией MIT - смотрите файл [LICENSE](../../LICENSE) для подробностей.