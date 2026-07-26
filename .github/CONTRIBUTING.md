# Contributing to HyperWolf

First off, thanks for taking the time to contribute! 🎉

The following is a set of guidelines for contributing to HyperWolf. These are mostly guidelines, not rules — use your best judgment, and feel free to propose changes to this document.

---

## Table of Contents

1. [Code of Conduct](#code-of-conduct)
2. [Getting Started](#getting-started)
3. [How to Contribute](#how-to-contribute)
   - [Reporting Bugs](#reporting-bugs)
   - [Suggesting Features](#suggesting-features)
   - [Submitting Pull Requests](#submitting-pull-requests)
4. [Development Setup](#development-setup)
5. [Style Guide](#style-guide)
6. [Project Structure](#project-structure)

---

## Code of Conduct

This project and everyone participating in it is governed by our [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior by opening an issue.

---

## Getting Started

- **GitHub Issues** — The primary place for bug reports, feature requests, and discussions.
- **DOCS.md** — Start here for the full architecture and API reference.
- **README.md** — Quick start and usage guide.

---

## How to Contribute

### Reporting Bugs

Before submitting a bug report, please check that the issue hasn't already been reported. If you find a **closed** issue that seems related, open a new one and link to it.

**When filing a bug report, include as much detail as possible:**

- **Steps to reproduce** — What did you do? What happened?
- **Expected behavior** — What should have happened?
- **Screenshots** — If applicable
- **Environment** — OS, Go version, DERO node type (local/remote), HyperWolf version
- **Logs** — Check `~/.hyperwolf/hyperwolf.log` for relevant output

Use the [Bug Report template](ISSUE_TEMPLATE/bug_report.md).

### Suggesting Features

Feature requests are welcome! Tell us:

- **What problem are you trying to solve?**
- **How would this feature work?** (Feel free to sketch it out)
- **Is there a workaround you're currently using?**

Use the [Feature Request template](ISSUE_TEMPLATE/feature_request.md).

### Submitting Pull Requests

1. **Fork the repository** and create your branch from `main`.
2. **Run tests** — Make sure existing tests pass (`go test ./...`).
3. **Add tests** — For new features or bug fixes, include tests.
4. **Update documentation** — If you change the API or add features, update `DOCS.md` and/or the README.
5. **Format your code** — Run `go fmt ./...` before committing.
6. **Write a good commit message** — See the [style guide](#commit-messages) below.
7. **Open a pull request** — Fill out the [PR template](PULL_REQUEST_TEMPLATE.md).

---

## Development Setup

```bash
# Clone your fork
git clone https://github.com/ArcaneSphere/HyperWolf
cd HyperWolf

# Build
CGO_ENABLED=0 go build -o hyperwolf .

# Run
./hyperwolf

# Format code
go fmt ./...

# Tidy dependencies
go mod tidy
```

### Prerequisites

- Go 1.26+
- A DERO daemon (local or remote)
- Optional: a local DERO testnet node for development

---

## Style Guide

### Go Code

- Format with `go fmt` before every commit.
- Follow [Go's official style guide](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments).
- Use meaningful variable names. Avoid single-letter names except for loop indices.
- Prefer `sync.RWMutex` over `sync.Mutex` when reads significantly outnumber writes.
- Handle errors explicitly. Don't use `_` to discard errors unless you've documented why.

### Commit Messages

- **Subject line:** Capitalized, imperative mood, ≤ 72 characters
- **Body:** Wrap at 72 characters, explain *what* and *why*, not *how*

```
Add TELA DocShard reconstruction for fragmented apps

Previously, fragmented datashard-based apps would fail to load because
the proxy only handled standard TELA clone directories. This adds the
downloadAndReconstructShards flow to reassemble fragments on disk
before serving.
```

### HTML / CSS / JS

- Vanilla JS only (no frameworks, no build step).
- Use `async/await` over raw promises.
- CSS variables for theming — add new variables to `:root` in `style.css`.

---

## Project Structure

```
hyperwolf/
├── main.go                  # Entry point
├── internal/
│   ├── state/               # Thread-safe app state
│   ├── daemon/              # DERO RPC client
│   ├── indexer/             # HyperGnomon sync manager
│   ├── router/              # HTTP server + WebSocket hub
│   ├── tela/                # TELA reverse proxy
│   └── desktop/             # Desktop install/uninstall
├── tray/                    # System tray implementation
├── web/                     # Frontend (SPA shell, JS, CSS)
├── assets/                  # Screenshots and project assets
└── DOCS.md                  # Technical documentation
```

---

## Questions?

Open a [GitHub Discussion](../../discussions) or an issue. We're happy to help!
