# prompt

[![Go Reference](https://pkg.go.dev/badge/github.com/nao1215/prompt.svg)](https://pkg.go.dev/github.com/nao1215/prompt)
[![Go Report Card](https://goreportcard.com/badge/github.com/nao1215/prompt)](https://goreportcard.com/report/github.com/nao1215/prompt)
[![MultiPlatformUnitTest](https://github.com/nao1215/prompt/actions/workflows/unit_test.yml/badge.svg)](https://github.com/nao1215/prompt/actions/workflows/unit_test.yml)

[English](../../README.md) | [日本語](../ja/README.md) | [Русский](../ru/README.md) | [中文](../zh-cn/README.md) | [한국어](../ko/README.md) | [Español](../es/README.md) | **Français**

![logo](../img/logo-small.png)

**prompt** est une bibliothèque de prompt terminal simple pour Go qui fournit des interfaces de ligne de commande interactives puissantes. Cette bibliothèque est conçue comme un remplacement pour la bibliothèque non maintenue [c-bata/go-prompt](https://github.com/c-bata/go-prompt), en résolvant les problèmes critiques tout en ajoutant des fonctionnalités améliorées et un meilleur support multiplateforme.

![sample](../img/sample.png)

## 🎯 Pourquoi prompt ?

La bibliothèque originale [go-prompt](https://github.com/c-bata/go-prompt) n'a pas été maintenue depuis mars 2021, avec 286 issues ouvertes et de nombreux bugs critiques incluant :

- **Paniques de division par zéro** dans le rendu terminal (issue #277)
- **Problèmes de compatibilité avec le terminal Windows** (issue #285)
- **Fuites de descripteurs de fichier** dans la gestion de /dev/tty (issue #253)
- **Support TTY limité** au-delà de STDIN (issue #275)
- **Problèmes de réinitialisation des couleurs du terminal** à la sortie de l'application (issue #265)

Cette bibliothèque résout tous ces problèmes tout en fournissant une base de code plus simple et maintenable avec une couverture de tests complète.

## ✨ Fonctionnalités

- 🖥️ **Support Multiplateforme** - Fonctionne parfaitement sur Linux, macOS et Windows
- 🔍 **Autocomplétion Avancée** - Complétion par Tab avec correspondance floue et suggestions personnalisables
- 📚 **Historique des Commandes** - Navigation avec les touches fléchées, historique persistant et recherche inversée (Ctrl+R)
- ⌨️ **Raccourcis Clavier Riches** - Raccourcis complets incluant la navigation de style Emacs
- 🌈 **Thèmes de Couleur** - Schémas de couleur intégrés et thématisation personnalisable
- 📝 **Entrée Multilignes** - Support pour l'entrée multilignes avec navigation appropriée du curseur
- ⚡ **Haute Performance** - Rendu efficace et allocations minimales
- 🧪 **Tests Complets** - 54.5% de couverture de tests avec des tests multiplateformes étendus
- 🔧 **API Simple** - Conception d'API moderne et propre avec pattern d'options fonctionnelles
- 🛠️ **Gestion des Ressources** - Nettoyage approprié et pas de fuites de descripteurs de fichier

## 📦 Installation

```bash
go get github.com/nao1215/prompt
```

## 🔧 Exigences

- **Version Go** : 1.24 ou ultérieure
- **Systèmes d'Exploitation** :
  - Linux
  - macOS
  - Windows

## 🚀 Démarrage Rapide

### Utilisation de Base

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
                fmt.Println("Au revoir !")
                break
            }
            log.Printf("Erreur : %v\n", err)
            continue
        }

        if input == "exit" {
            break
        }
        fmt.Printf("Vous avez tapé : %s\n", input)
    }
}
```

### Avec Autocomplétion

```go
package main

import (
    "errors"
    "log"
    "github.com/nao1215/prompt"
)

func completer(d prompt.Document) []prompt.Suggestion {
    return []prompt.Suggestion{
        {Text: "help", Description: "Afficher le message d'aide"},
        {Text: "users", Description: "Lister tous les utilisateurs"},
        {Text: "groups", Description: "Lister tous les groupes"},
        {Text: "exit", Description: "Quitter le programme"},
    }
}

func main() {
    p, err := prompt.New("monapp> ",
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
        // Traiter les commandes...
    }
}
```

### Avec Historique et Fonctionnalités Avancées

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
    // Créer un prompt avec historique et timeout
    p, err := prompt.New(">>> ",
        prompt.WithMemoryHistory(100),
        prompt.WithColorScheme(prompt.ThemeDracula),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer p.Close()

    // Utiliser le contexte pour le support des timeouts
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    input, err := p.RunWithContext(ctx)
    if err == context.DeadlineExceeded {
        fmt.Println("Délai d'attente atteint")
        return
    }

    fmt.Printf("Entrée : %s\n", input)
}
```

## ⌨️ Raccourcis Clavier

La bibliothèque supporte des raccourcis clavier complets dès le départ :

| Touche | Action |
|--------|--------|
| Entrée | Soumettre l'entrée |
| Ctrl+C | Annuler et retourner ErrInterrupted |
| Ctrl+D | EOF quand le buffer est vide |
| ↑/↓ | Naviguer dans l'historique (ou lignes en mode multilignes) |
| ←/→ | Déplacer le curseur |
| Ctrl+A / Home | Aller au début de la ligne |
| Ctrl+E / End | Aller à la fin de la ligne |
| Ctrl+K | Supprimer du curseur à la fin de la ligne |
| Ctrl+U | Supprimer toute la ligne |
| Ctrl+W | Supprimer le mot vers l'arrière |
| Ctrl+R | Recherche inversée dans l'historique |
| Tab | Autocomplétion |
| Retour arrière | Supprimer le caractère vers l'arrière |
| Suppr | Supprimer le caractère vers l'avant |
| Ctrl+←/→ | Se déplacer par limites de mots |

## 🎨 Thèmes de Couleur

Thèmes de couleur intégrés :

```go
// Thèmes disponibles
prompt.ThemeDefault
prompt.ThemeDracula
prompt.ThemeNightOwl
prompt.ThemeMonokai
prompt.ThemeSolarizedDark
prompt.ThemeSolarizedLight

// Utilisation
p, err := prompt.New("$ ",
    prompt.WithColorScheme(prompt.ThemeDracula),
)
```

## ⚠️ Notes Importantes

### Sécurité des Threads
⚠️ **IMPORTANT** : Cette bibliothèque n'est **PAS thread-safe** :
- **NE PARTAGEZ PAS** les instances de prompt entre les goroutines
- **N'APPELEZ PAS** les méthodes de manière concurrente sur la même instance de prompt
- **N'APPELEZ PAS** `Close()` pendant que `Run()` est actif dans une autre goroutine
- Utilisez des instances de prompt séparées pour les opérations concurrentes si nécessaire

### Gestion des Ressources
- **Appelez toujours `Close()`** lorsque vous avez terminé avec un prompt pour éviter les fuites de ressources
- La méthode `Close()` peut être appelée plusieurs fois en toute sécurité
- Appelez `Close()` même si `Run()` ou `RunWithContext()` retourne une erreur

### Gestion des Erreurs
La bibliothèque fournit des types d'erreur spécifiques :
- `prompt.ErrEOF` : L'utilisateur a appuyé sur Ctrl+D avec un buffer vide
- `prompt.ErrInterrupted` : L'utilisateur a appuyé sur Ctrl+C
- `context.DeadlineExceeded` : Délai d'attente atteint (lors de l'utilisation du contexte)
- `context.Canceled` : Le contexte a été annulé

## 🧪 Tests

Exécuter la suite de tests :

```bash
make test    # Exécuter les tests avec couverture
make lint    # Exécuter le linter
make clean   # Nettoyer les fichiers générés
```

## 🤝 Contribuer

Les contributions sont les bienvenues ! Veuillez consulter le [Guide de Contribution](../../CONTRIBUTING.md) pour plus de détails.

### Exigences de Développement

- Go 1.24 ou ultérieur
- golangci-lint pour la qualité du code
- Tests multiplateformes sur Linux, macOS et Windows

## 💖 Soutien

Si vous trouvez ce projet utile, veuillez considérer :

- ⭐ Lui donner une étoile sur GitHub - cela aide les autres à découvrir le projet
- 💝 [Devenir sponsor](https://github.com/sponsors/nao1215) - votre soutien maintient le projet en vie et motive le développement continu

Votre soutien, que ce soit par des étoiles, des parrainages ou des contributions, est ce qui fait avancer ce projet. Merci !

## 📄 Licence

Ce projet est sous licence MIT - voir le fichier [LICENSE](../../LICENSE) pour les détails.