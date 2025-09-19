# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.0.4] - 2025-01-22

### Fixed
- **Multi-line cursor positioning ([307ee32](https://github.com/nao1215/prompt/commit/307ee32))**: Completely fixed cursor and input character positioning issues in multi-line mode
  - Fixed cursor positioning calculation for continuation lines to start from line beginning (column 0)
  - Eliminated progressive character drift that caused input characters to move rightward over time
  - Simplified position calculations by removing complex prefix-based indentation logic
  - Added explicit carriage return (`\r`) and line clear (`\x1b[K`) for continuation lines to ensure proper line start positioning
  - Resolved visual misalignment between cursor position and actual character input location

### Enhanced
- **Multi-line input reliability**: Continuation lines now consistently start from line beginning without complex position calculations
- **User experience**: Eliminated confusing cursor/input position discrepancies that made multi-line editing difficult
- **Code maintainability**: Simplified renderer logic by removing error-prone position calculations for continuation lines

### Technical Improvements
- **Renderer simplification**: Updated `positionCursor` function to use simple line-start positioning for continuation lines
- **Consistent behavior**: Both cursor positioning and character rendering now follow the same simple rules
- **Cross-platform reliability**: Removed Unicode and terminal-specific positioning edge cases
- **Performance**: Eliminated complex calculations that could cause cumulative positioning errors

## [0.0.3] - 2025-01-21

### Fixed
- **Multi-line history navigation ([b160784](https://github.com/nao1215/prompt/commit/b160784))**: Fixed display position issues when navigating through multi-line command history
  - Improved `clearPreviousLines` function to properly clear multi-line content
  - Enhanced line count tracking for accurate terminal positioning
  - Fixed cursor position management for multi-line input navigation

- **Terminal line wrapping calculation ([b160784](https://github.com/nao1215/prompt/commit/b160784))**: Improved handling of long input lines that wrap across multiple terminal lines
  - Added `calculateRenderedLines` function to accurately count rendered lines
  - Accounts for terminal width when calculating line wrapping
  - Fixed prompt duplication issues when text wraps to next line
  - Properly handles prefix length in line wrapping calculations

### Technical Improvements
- **Renderer enhancements**: Added terminal interface to renderer for dynamic size detection
- **Terminal width awareness**: Renderer now considers terminal width for accurate line wrapping
- **Line counting accuracy**: More precise calculation of actual rendered lines vs logical lines
- **State management**: Improved tracking of rendered line count for better screen clearing

## [0.0.2] - 2025-01-20

### Fixed
- **Completion suggestion scrolling ([994b558](https://github.com/nao1215/prompt/commit/994b558))**: Fixed infinite scrolling bug when navigating through completion suggestions beyond the visible range
  - Implemented proper scroll boundaries to prevent selection from continuing into empty fields
  - Added offset-based rendering system for smooth scrolling through large suggestion lists
  - Maximum 10 suggestions displayed at once with proper up/down navigation
- **Terminal boundary display issues**: Fixed completion suggestions jumping to screen top when displayed at terminal bottom
  - Improved ANSI escape sequence handling for terminal edge cases
  - Enhanced cursor positioning to avoid terminal boundary artifacts
- **Cursor flickering during completion ([994b558](https://github.com/nao1215/prompt/commit/994b558))**: Eliminated excessive cursor movement during suggestion navigation
  - Implemented cursor hiding during suggestion display with `\x1b[?25l`/`\x1b[?25h`
  - Optimized rendering to minimize cursor position updates
  - Added state management to track suggestion display status
- **Suggestion list persistence**: Fixed completion suggestions not clearing after TAB selection
  - Implemented comprehensive screen clearing with `\x1b[0J` escape sequence
  - Added proper state transition handling between suggestion display and normal input
  - Enhanced cleanup of suggestion rendering areas

### Enhanced
- **Scroll test example**: Updated autocomplete example with 23+ commands and 15+ items to demonstrate scrolling functionality
  - Added comprehensive test scenarios for suggestion scrolling
  - Included detailed README with testing instructions
  - Improved user experience validation tools

### Technical Improvements
- **Renderer architecture**: Enhanced separation between cursor management and suggestion rendering
- **State management**: Improved tracking of suggestion display state with `suggestionsActive` flag
- **Screen clearing**: More robust terminal content clearing with multiple fallback strategies
- **Cross-platform compatibility**: Better handling of terminal differences across operating systems

## [0.1.0] - 2025-09-18

### Added
- **Initial implementation of modern prompt library ([45519e9](https://github.com/nao1215/prompt/commit/45519e9))**: Complete rewrite of go-prompt with improved architecture and cross-platform support
- **Functional options API pattern**: Clean, extensible configuration using `WithCompleter`, `WithMemoryHistory`, etc.
- **Cross-platform terminal support**: Enhanced Windows compatibility via mattn/go-colorable, native Unix support
- **Resource management**: Proper cleanup with Close() method and defer patterns to prevent file descriptor leaks
- **Error recovery mechanisms**: Fixed critical divide-by-zero panics and improved error handling
- **Comprehensive testing framework**: Test-driven development with >80% coverage target
- **Multi-language documentation**: Support for Chinese (zh-cn) documentation
- **Sponsor integration**: GitHub Sponsors support for project sustainability

### Fixed
- **Divide by zero panics** in terminal rendering logic that plagued original go-prompt
- **File descriptor leaks** in /dev/tty handling through proper resource management
- **Windows terminal compatibility issues** with ANSI escape sequences
- **Terminal color reset issues** on application exit
- **Memory leaks and race conditions** through improved concurrency design

### Changed
- **Simplified API design**: Reduced from 50+ public APIs to 5 core types for better usability
- **Modernized architecture**: Clear separation of concerns between input, output, rendering, and completion
- **Interface-based design**: Enhanced testability and extensibility through proper abstractions
- **Performance optimizations**: Efficient diff-based terminal updates and minimal memory allocations

### Technical Details
- **Thread Safety**: Library is explicitly NOT thread-safe by design for performance
- **Platform Support**: Linux, macOS, Windows with native terminal capabilities
- **Unicode Support**: Full UTF-8 character handling including wide characters
- **Development Tools**: Makefile with test, lint, clean, and tools targets

---

## Project Context

This project is a modern replacement for the unmaintained go-prompt library (github.com/c-bata/go-prompt), addressing 286 open issues and critical bugs that have existed since March 2021.

### Migration from go-prompt
- **Drop-in replacement**: Designed for easy migration from original go-prompt
- **API compatibility**: Maintains familiar patterns while improving reliability
- **Performance improvements**: Better memory usage and rendering efficiency
- **Enhanced cross-platform support**: Robust Windows, macOS, and Linux compatibility

### Sponsors
Support this project: https://github.com/sponsors/nao1215
