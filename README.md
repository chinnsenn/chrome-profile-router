# Chrome Profile Router

> A high-performance, design-forward macOS default browser router for multi-profile Google Chrome, built with [MyGo](https://mygo.egoist.dev/).

---

## ✨ Features

- **Apple TV 3D Parallax Tilt**: 
  Smooth, cursor-tracking 3D spatial rotation powered by MyGo's GPU rendering and `AnimateTo` spring physics damping. No jitter, no tearing, pure tactile feel.
- **Dynamic Chrome Profile Discovery**:
  Directly parses `~/Library/Application Support/Google/Chrome/Local State` at startup. Automatically extracts your real 256×256 Google account avatars (`Google Profile Picture.png`) and filters out deleted ghost profiles.
- **Hero Destination Focus**:
  The clicked target URL is placed at the top of the visual hierarchy with prominent domain emphasis, letting you know where you are heading before picking an identity.
- **Adaptive 3-Way Color Themes**:
  Native segmented control supporting **Auto (Follow System)**, **Light (Apple Platinum)**, and **Dark (Deep Obsidian)** with zero emoji noise.
- **Keyboard Speed Dial**:
  - `1` – `9`: Instant one-touch launch with corresponding profile.
  - `↑` `↓` `←` `→`: 2D spatial navigation.
  - `Enter`: Open selected profile.
  - `Esc`: Dismiss immediately.
- **macOS Default Browser Compliant**:
  Configured with both `CFBundleURLTypes` (http/https) and `CFBundleDocumentTypes` (public.html), officially recognized by macOS System Settings as a default web browser.

---

## 🚀 Quick Start

### Prerequisites

- macOS 12.0+
- [Go](https://go.dev/) 1.27.3+

### Build & Deploy

Clone the repository and run the build script:

```bash
git clone https://github.com/chinnsenn/chrome-profile-router.git
cd chrome-profile-router

# Build and deploy to /Applications/Chrome Profile Router.app
./build.sh
```

### Set as Default Browser

1. Open macOS **System Settings** → **Desktop & Dock** (or **General** on older macOS versions).
2. Locate the **Default web browser** dropdown.
3. Select **Chrome Profile Router**.

Whenever an external application (Slack, WeChat, Terminal, Mail, Feishu, etc.) opens a link, **Chrome Profile Router** will spring into view, allowing you to route the link to the desired profile with a single keystroke.

---

## 🛠️ Architecture

- **Engine**: [MyGo](https://mygo.egoist.dev/) (`github.com/egoist/mygo`)
- **Graphics & Physics**: GPU Hardware-accelerated Metal rendering, `plugins/glass` (Liquid Glass), `AnimateTo` (Spring Damping).
- **Runtime**: Pure Go, zero Electron / Webview bloat, sub-10MB bundle size, instant cold-start.

---

## 📄 License

MIT © [Senn](https://github.com/chinnsenn)
