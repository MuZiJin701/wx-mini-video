# Repository Slimming and Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep `wx-mini-video` small, user-focused, and reliable for Windows users who need to capture and download media loaded by PC WeChat mini programs.

**Architecture:** Preserve the current single-binary Go TUI architecture. Reduce repository noise around docs, distribution artifacts, and old product-specific names before adding new behavior.

**Tech Stack:** Go, Bubble Tea/Lipgloss, Echo proxy adapter, Windows system proxy/certificate APIs, ffmpeg.

## Global Constraints

- Do not restore old video-account, web-server, Docker, bat/ps1 launcher, or app-specific Qiming-only flows.
- Do not commit `dist/`, `downloads/`, screenshots, posters, logs, generated exe files, or ffmpeg binaries.
- Build and publish Windows zip artifacts through GitHub Releases, not Git history.
- After code changes run `go test -count=1 ./...` and `go vet ./...`.

---

### Task 1: Finish Repository Slimming

**Files:**
- Modify: `.gitignore`
- Delete: `.dockerignore`
- Keep local-only: `dist/`, `downloads/`, `poster/`, screenshots

**Interfaces:**
- Produces: a repository where only source, tests, build script, and durable docs are tracked.

- [x] Remove `.dockerignore` because Docker is not a supported build or distribution path.
- [x] Ignore screenshots and poster output so promotional/local QA artifacts do not enter commits.
- [x] Keep `dist/` ignored; use Releases for zip distribution.
- [ ] In a later cleanup pass, verify whether `pkg/system/fs.go`, `pkg/system/util.go`, and `pkg/platform/*` are still used; delete them only if `rg` and tests prove no references.

### Task 2: Normalize Documentation

**Files:**
- Create: `AGENTS.md`
- Create: `wx-mini-video-knowledge.md`
- Create: `docs/superpowers/plans/2026-07-20-repository-slimming-and-optimization.md`
- Move: old Qiming docs into `docs/archive/`
- Modify: `README.md`

**Interfaces:**
- Produces: stable project entry points for agents, users, and future implementers.

- [x] Make `docs/` root contain only `archive/` and `superpowers/`.
- [x] Put historical Qiming-specific docs under `docs/archive/`.
- [x] Document current boundaries in `AGENTS.md`.
- [x] Keep user-facing README short and release-oriented.
- [x] Keep reusable architecture and troubleshooting knowledge in `wx-mini-video-knowledge.md`.

### Task 3: Next Optimization Work

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/miniprogram/candidate.go`
- Modify: `internal/minidownload/downloader.go`
- Test: matching `*_test.go` files

**Interfaces:**
- Produces: better resource identification and more recoverable downloads.

- [ ] Add an optional detail view for the selected resource showing full URL, source URL, headers summary, and local cache path without polluting the main list.
- [ ] Add title extraction from JSON fields near media URLs when available, storing it on `Candidate` as a display-only field.
- [ ] Add resumable direct downloads using temporary `.part` files and atomic rename on completion.
- [ ] Add a download history file under `downloads/history.jsonl` with completed path, source host, size, and timestamp.
- [ ] Add a build verification script that checks zip contents exactly match `README.md`, `wx-mini-video.exe`, and `wx-mini-video.yaml`.

### Task 4: Release Process

**Files:**
- Modify: `README.md`
- Optional create: `docs/superpowers/plans/YYYY-MM-DD-release-checklist.md`

**Interfaces:**
- Produces: repeatable Windows release workflow.

- [ ] Run `go test -count=1 ./...`.
- [ ] Run `go vet ./...`.
- [ ] Run `cmd /c build\build.bat windows`.
- [ ] Inspect zip contents with `tar -tf dist\wx-mini-video-windows-amd64.zip`.
- [ ] Upload zip to GitHub Releases with a short changelog.
