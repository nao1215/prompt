# prompt

[![Go Reference](https://pkg.go.dev/badge/github.com/nao1215/prompt.svg)](https://pkg.go.dev/github.com/nao1215/prompt)
[![Go Report Card](https://goreportcard.com/badge/github.com/nao1215/prompt)](https://goreportcard.com/report/github.com/nao1215/prompt)
[![MultiPlatformUnitTest](https://github.com/nao1215/prompt/actions/workflows/unit_test.yml/badge.svg)](https://github.com/nao1215/prompt/actions/workflows/unit_test.yml)

[English](../../README.md) | [日本語](../ja/README.md) | [Русский](../ru/README.md) | [中文](../zh-cn/README.md) | [한국어](../ko/README.md) | **Español** | [Français](../fr/README.md)

![logo](../img/logo-small.png)

**prompt** es una biblioteca de prompts de terminal simple para Go que proporciona interfaces de línea de comandos interactivas y potentes. Esta biblioteca está diseñada como un reemplazo para la biblioteca no mantenida [c-bata/go-prompt](https://github.com/c-bata/go-prompt), abordando problemas críticos mientras añade funcionalidad mejorada y mejor soporte multiplataforma.

![sample](../img/sample.png)

## 🎯 ¿Por qué prompt?

La biblioteca original [go-prompt](https://github.com/c-bata/go-prompt) no ha sido mantenida desde marzo de 2021, con 286 issues abiertos y numerosos errores críticos incluyendo:

- **Panics de división por cero** en renderizado de terminal (issue #277)
- **Problemas de compatibilidad con terminal de Windows** (issue #285)
- **Fugas de descriptores de archivo** en manejo de /dev/tty (issue #253)
- **Soporte TTY limitado** más allá de STDIN (issue #275)
- **Problemas de reseteo de colores de terminal** al salir de la aplicación (issue #265)

Esta biblioteca aborda todos estos problemas mientras proporciona una base de código más simple y mantenible con cobertura de pruebas integral.

## ✨ Características

- 🖥️ **Soporte Multiplataforma** - Funciona perfectamente en Linux, macOS y Windows
- 🔍 **Autocompletado Avanzado** - Completado con Tab con coincidencia difusa y sugerencias personalizables
- 📚 **Historial de Comandos** - Navegación con teclas de flecha, historial persistente y búsqueda inversa (Ctrl+R)
- ⌨️ **Combinaciones de Teclas Avanzadas** - Atajos completos incluyendo navegación estilo Emacs
- 🌈 **Temas de Color** - Esquemas de color incorporados y tematización personalizable
- 📝 **Entrada Multilínea** - Soporte para entrada multilínea con navegación apropiada del cursor
- ⚡ **Alto Rendimiento** - Renderizado eficiente y asignaciones mínimas
- 🧪 **Pruebas Integrales** - 54.5% de cobertura de pruebas con testing multiplataforma extensivo
- 🔧 **API Simple** - Diseño de API moderno y limpio con patrón de opciones funcionales
- 🛠️ **Gestión de Recursos** - Limpieza apropiada y sin fugas de descriptores de archivo

## 📦 Instalación

```bash
go get github.com/nao1215/prompt
```

## 🔧 Requisitos

- **Versión de Go**: 1.24 o posterior
- **Sistemas Operativos**:
  - Linux
  - macOS
  - Windows

## 🚀 Inicio Rápido

### Uso Básico

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
                fmt.Println("¡Adiós!")
                break
            }
            log.Printf("Error: %v\n", err)
            continue
        }

        if input == "exit" {
            break
        }
        fmt.Printf("Escribiste: %s\n", input)
    }
}
```

### Con Autocompletado

```go
package main

import (
    "errors"
    "log"
    "github.com/nao1215/prompt"
)

func completer(d prompt.Document) []prompt.Suggestion {
    return []prompt.Suggestion{
        {Text: "help", Description: "Mostrar mensaje de ayuda"},
        {Text: "users", Description: "Listar todos los usuarios"},
        {Text: "groups", Description: "Listar todos los grupos"},
        {Text: "exit", Description: "Salir del programa"},
    }
}

func main() {
    p, err := prompt.New("miapp> ",
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
        // Manejar comandos...
    }
}
```

### Con Historial y Características Avanzadas

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
    // Crear prompt con historial y timeout
    p, err := prompt.New(">>> ",
        prompt.WithMemoryHistory(100),
        prompt.WithColorScheme(prompt.ThemeDracula),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer p.Close()

    // Usar contexto para soporte de timeout
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    input, err := p.RunWithContext(ctx)
    if err == context.DeadlineExceeded {
        fmt.Println("Tiempo de espera agotado")
        return
    }

    fmt.Printf("Entrada: %s\n", input)
}
```

## ⌨️ Combinaciones de Teclas

La biblioteca soporta combinaciones de teclas completas desde el inicio:

| Tecla | Acción |
|-------|--------|
| Enter | Enviar entrada |
| Ctrl+C | Cancelar y devolver ErrInterrupted |
| Ctrl+D | EOF cuando buffer está vacío |
| ↑/↓ | Navegar historial (o líneas en modo multilínea) |
| ←/→ | Mover cursor |
| Ctrl+A / Home | Mover al inicio de línea |
| Ctrl+E / End | Mover al final de línea |
| Ctrl+K | Eliminar desde cursor al final de línea |
| Ctrl+U | Eliminar línea completa |
| Ctrl+W | Eliminar palabra hacia atrás |
| Ctrl+R | Búsqueda inversa en historial |
| Tab | Autocompletado |
| Backspace | Eliminar carácter hacia atrás |
| Delete | Eliminar carácter hacia adelante |
| Ctrl+←/→ | Mover por límites de palabra |

## 🎨 Temas de Color

Temas de color incorporados:

```go
// Temas disponibles
prompt.ThemeDefault
prompt.ThemeDracula
prompt.ThemeNightOwl
prompt.ThemeMonokai
prompt.ThemeSolarizedDark
prompt.ThemeSolarizedLight

// Uso
p, err := prompt.New("$ ",
    prompt.WithColorScheme(prompt.ThemeDracula),
)
```

## ⚠️ Notas Importantes

### Seguridad de Hilos
⚠️ **IMPORTANTE**: Esta biblioteca **NO es segura para hilos**:
- **NO** comparta instancias de prompt entre goroutines
- **NO** llame métodos concurrentemente en la misma instancia de prompt
- **NO** llame `Close()` mientras `Run()` está activo en otra goroutine
- Use instancias de prompt separadas para operaciones concurrentes si es necesario

### Gestión de Recursos
- **Siempre llame `Close()`** cuando termine con un prompt para prevenir fugas de recursos
- El método `Close()` es seguro de llamar múltiples veces
- Llame `Close()` incluso si `Run()` o `RunWithContext()` devuelve un error

### Manejo de Errores
La biblioteca proporciona tipos de error específicos:
- `prompt.ErrEOF`: Usuario presionó Ctrl+D con buffer vacío
- `prompt.ErrInterrupted`: Usuario presionó Ctrl+C
- `context.DeadlineExceeded`: Tiempo de espera alcanzado (al usar contexto)
- `context.Canceled`: Contexto fue cancelado

## 🧪 Pruebas

Ejecutar la suite de pruebas:

```bash
make test    # Ejecutar pruebas con cobertura
make lint    # Ejecutar linter
make clean   # Limpiar archivos generados
```

## 🤝 Contribuir

¡Las contribuciones son bienvenidas! Por favor vea la [Guía de Contribución](../../CONTRIBUTING.md) para más detalles.

### Requisitos de Desarrollo

- Go 1.24 o posterior
- golangci-lint para calidad de código
- Pruebas multiplataforma en Linux, macOS y Windows

## 💖 Apoyo

Si encuentra útil este proyecto, por favor considere:

- ⭐ Darle una estrella en GitHub - ayuda a otros a descubrir el proyecto
- 💝 [Convertirse en patrocinador](https://github.com/sponsors/nao1215) - su apoyo mantiene el proyecto vivo y motiva el desarrollo continuo

Su apoyo, ya sea a través de estrellas, patrocinios o contribuciones, es lo que impulsa este proyecto hacia adelante. ¡Gracias!

## 📄 Licencia

Este proyecto está licenciado bajo la Licencia MIT - vea el archivo [LICENSE](../../LICENSE) para detalles.