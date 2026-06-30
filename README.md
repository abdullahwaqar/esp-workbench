# esp-workbench

TUI for ESP32 development with ESP-IDF.

## Features

- Auto-scans connected ESP32 devices on Linux, macOS, Windows
- Live streaming logs with color-coded severity (error / warn / success / system)
- One-key operations: build, flash, build+flash, monitor, erase
- Tab navigation between panels
- Chip detection via `esptool.py` — shows chip model and MAC address
- Auto-refresh every 5 seconds so hot-plugged devices appear
- Project context awareness — reads CMakeLists.txt, sdkconfig, components
- Automatic serial permission handling on Linux (dialout group check + temporary fix)
- Single binary

## Prerequisites

- Go 1.21+
- ESP-IDF installed with `idf.py` in `$PATH`
- `esptool.py` in `$PATH` (bundled with ESP-IDF)

## Building

```bash
# build the binary
make build

# run directly
make run

# install globally to $GOPATH/bin
make install

# format, lint, test, and build
make all

# development mode with race detector
make dev

# see all targets
make help
```

Manual build:

```bash
go build -o esp-workbench ./cmd/app
./esp-workbench
```

## Usage

```bash
# default: use current directory as project
esp-workbench

# specify a project path
esp-workbench /path/to/my-esp32-app
esp-workbench --project ~/projects/weather-station

# show version
esp-workbench --version
```

## Keybindings

| Key                    | Action                           |
| ---------------------- | -------------------------------- |
| `Tab` / `Shift+Tab`    | Cycle between panels             |
| `up` / `down` or `j/k` | Navigate device list             |
| `enter`                | Select device as active port     |
| `b`                    | Build (`idf.py build`)           |
| `f`                    | Flash (`idf.py -p <port> flash`) |
| `a`                    | Build + Flash in one shot        |
| `m`                    | Open serial monitor              |
| `e`                    | Erase entire flash               |
| `r`                    | Rescan devices                   |
| `l`                    | Clear log pane                   |
| `q` / `Ctrl+C`         | Quit                             |

## Project Detection

esp-workbench reads the project directory at startup and displays:

- **target chip** — from `sdkconfig` `CONFIG_IDF_TARGET` (most reliable), falling back to `CMakeLists.txt`
- **configured / not configured** — whether `sdkconfig` exists (i.e. `idf.py menuconfig` has been run)
- **component count** — number of subdirectories in `components/` that contain a `CMakeLists.txt`
- **partition table** — name of any `*partition*.csv` found in the project root
- **version** — from `project(...VERSION x.y.z)` in CMakeLists, or `CONFIG_APP_PROJECT_VER` in sdkconfig

If the directory is not a valid ESP-IDF project (missing `CMakeLists.txt` or `main/`), the header shows what is missing.

## Permission Handling (Linux)

If a device is not readable, esp-workbench:

1. Diagnoses the problem (not in dialout group vs wrong device permissions)
2. Attempts an automatic temporary fix via `pkexec chmod a+rw <port>`
3. Falls back to `sudo chmod a+rw <port>`
4. If all automatic fixes fail, prints the exact command to run manually

Permanent fix (run once, then log out and back in):

```bash
sudo usermod -aG dialout $USER
```

### Package Boundaries

- `espworkbench`: hardware and subprocess concerns (ports, chip detection, idf.py execution, permissions, project parsing)
- `tui`: UI state, rendering, event handling

### Format and Lint

```bash
make fmt    # go fmt
make lint   # go vet
make check  # fmt + lint + test
```

## Troubleshooting

**No devices found**

- Press `r` to manually trigger a scan
- Ensure `esptool.py` is in `$PATH`
- Check the USB cable and device connection

**Build fails with missing imports**

```bash
go mod tidy
go mod download
```

**Terminal rendering issues**

- Ensure the terminal supports ANSI colors
- Try resizing the window (layout reflows on resize)
- On Windows, use Windows Terminal rather than cmd.exe
