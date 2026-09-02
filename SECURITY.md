# Security Policy

## Reporting

If you believe you found a vulnerability, email
[`n.chika156@gmail.com`](mailto:n.chika156@gmail.com). Please do not open a
public GitHub issue first.

Include the affected version or commit, reproduction steps, the impact, and any
workaround you already found. Valid reports are acknowledged and handled as
priority work; follow-up questions come if the reproduction is missing details.

## Supported Versions

Security fixes are provided for the latest published release series only.

| Version | Supported | Notes |
|---------|-----------|-------|
| `0.0.x` | Yes | Current published release series as of September 2, 2026 |

Security fixes are not backported to unsupported release series. When the next
series is published, support moves to it.

## What the library touches

prompt reads the terminal, writes escape sequences to it, and, when
`WithFileHistory` is used, reads and writes one file. That file holds what the
user typed, so a file this library creates is created readable by its owner
alone, whatever the umask, and a directory it creates is created without
world access. A file or directory that already existed keeps the mode it had,
and a backup is the file renamed, so it keeps the file's mode. Nothing is sent
anywhere, and no other file is opened.
