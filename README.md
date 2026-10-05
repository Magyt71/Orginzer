# Organization Moderator

A lightweight **desktop file auto-organizer** built with Go. It runs silently in your system tray, watches the folders you choose (your **Downloads** folder by default), and instantly sorts new files into categorized sub-folders based on their extension.

Stop digging through a messy Downloads folder — drop a file in, and it lands where it belongs.

---

## ✨ Features

- **System-tray app** — runs in the background; open/close the window from the tray menu.
- **Real-time watching** — uses [`fsnotify`](https://github.com/fsnotify/fsnotify) to detect new/modified files the moment they appear.
- **Smart file-lock handling** — polls until a download/write finishes (the file is no longer locked) before moving it, so partial files are never grabbed.
- **Auto-categorization by extension** — e.g. `.jpg → Image/`, `.mp4 → Video/`, `.pdf → Document/`, `.zip → Archive/`, `.exe → Programs/`.
- **Editable rules** — add/remove your own `extension → folder` mappings from the UI.
- **Custom destination** — organize into a single central folder, or sort inside each watched folder.
- **Clean Up Now** — one click to organize everything already sitting in the watched folders.
- **Undo last move** — restores the last batch of moved files (available within 30 seconds).
- **Duplicate-safe naming** — if a file already exists at the target, it's renamed `name(1).ext`, `name(2).ext`, …
- **Desktop notifications** — optional toast when a file is moved (via [`beeep`](https://github.com/gen2brain/beeep)).
- **Start with Windows** — optional auto-launch via the current-user registry `Run` key.
- **Persistent settings** — configuration and recent history are saved to `Settings.json`.
- **Modern dark UI** — built with the [Gio](https://gioui.org) immediate-mode GUI toolkit.

---

## 📦 Default categories

| Category  | Extensions                                  |
|-----------|---------------------------------------------|
| `Image`   | `.jpg` `.jpeg` `.png` `.gif` `.svg` `.webp` |
| `Video`   | `.mp4` `.mkv` `.avi`                        |
| `Document`| `.pdf` `.docx` `.txt`                       |
| `Archive` | `.zip` `.rar` `.7z`                         |
| `Programs`| `.exe` `.msi`                               |

You can change or extend these at any time from **Settings → File Types**.

---

## 🖥️ Screenshots

> Add screenshots here, e.g.:
>
> `![Main dashboard](docs/dashboard.png)`
> `![File types editor](docs/file-types.png)`

---

## 🚀 Getting started

### Prerequisites

- **Go 1.25+** — https://go.dev/dl/
- A C compiler is **not** required, but on Windows you may need **GCC/MinGW** only if you hit a cgo dependency. Standard builds work with the pure-Go toolchain.

### Run from source

```bash
# 1. Clone the repo
git clone https://github.com/YOUR_USERNAME/organization-moderator.git
cd organization-moderator

# 2. Fetch dependencies
go mod download

# 3. Run (a console window will appear while developing)
go run .
```

### Build a Windows executable (no console window)

```powershell
go build -ldflags "-H windowsgui -s -w" -o "OrganizationModerator.exe" .
```

The app icon and Windows manifest are embedded from `rsrc.syso`, which the Go toolchain links automatically.

> **Note:** the system-tray icon is loaded from `image/organization-chart.ico` at runtime. Keep the `image/` folder next to the `.exe` and launch from that directory (or embed it if you want a single portable file).

---

## ⚙️ Configuration

On first launch the app writes a `Settings.json` in its working folder. You can either edit it directly or use the in-app UI. A template is provided:

```bash
cp Settings.example.json Settings.json
```

```jsonc
{
  "watch_paths":      ["C:\\Users\\YOU\\Downloads"], // folders to monitor
  "target_folder":    { ".jpg": "Image", ... },       // extension → sub-folder
  "destination_path": "",                             // "" = sort inside each watched folder
  "notifications":    true,                           // desktop toasts
  "run_at_startup":   false,                          // launch with Windows
  "recent_moves":     []                              // move history (used by Undo)
}
```

> `Settings.json` holds machine-specific paths, so it is **gitignored**. Commit `Settings.example.json` instead.

---

## 🧭 How to use

1. Launch the app — a tray icon appears and the window opens.
2. **Start** begins watching the configured folders.
3. Drop/download a file into a watched folder → it's automatically moved into its category.
4. **Clean Up Now** to sort the existing contents in one pass.
5. **Undo** the last move if something was filed incorrectly.
6. **Settings** → manage folders, edit file types, set a destination, toggle notifications & startup.

---

## 🗂️ Project structure

| File               | Responsibility                                             |
|--------------------|------------------------------------------------------------|
| `main-desktop.go`  | Entry point, system tray, app lifecycle.                   |
| `gui.go`           | Gio UI: dashboard, settings, file-type editor, activity log.|
| `engine.go`        | Organizer core: watching, move/undo/clean, config logic.   |
| `theme.go`         | Colors and UI theme constants.                             |
| `DB.go`            | Persist/load configuration to/from `Settings.json`.        |
| `image/`           | Tray icon (`organization-chart.ico`).                      |

---

## 🛠️ Tech stack

- **Language:** Go
- **GUI:** [Gio](https://gioui.org)
- **File watching:** [`fsnotify`](https://github.com/fsnotify/fsnotify)
- **Notifications:** [`beeep`](https://github.com/gen2brain/beeep)
- **System tray:** [`getlantern/systray`](https://github.com/getlantern/systray)
- **Native folder picker:** [`sqweek/dialog`](https://github.com/sqweek/dialog)

---

## 🐛 Troubleshooting

- **Files aren't moving** — make sure you pressed **Start** and the folder is listed under *Watching Directories*.
- **"Access denied" during a move** — the file may still be in use; the engine retries until the lock is released.
- **Tray icon missing** — confirm the `image/` folder sits next to the executable.
- **Nothing on double-click** — use the tray menu's **Open The App** to show the window.

---

## 📄 License

No license file has been added yet. If you want it open source, add a `LICENSE` file (for example, MIT) at the repository root.

---

## 💡 Ideas / contributions

Contributions are welcome — open an issue first to discuss what you'd like to change.
