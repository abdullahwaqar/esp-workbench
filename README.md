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

| Key                    | Action                                          |
| ---------------------- | ----------------------------------------------- |
| `Tab` / `Shift+Tab`    | Cycle between panels                            |
| `up` / `down` or `j/k` | Navigate device list                            |
| `enter`                | Select device as active port                    |
| `b`                    | Build (`idf.py build`)                          |
| `f`                    | Flash (`idf.py -p <port> flash`)                |
| `a`                    | Build + Flash in one shot                       |
| `m`                    | Open serial monitor                             |
| `e`                    | Erase entire flash                              |
| `x`                    | Browse and flash an existing binary             |
| `p`                    | Read and visualize the device's partition table |
| `r`                    | Rescan devices                                  |
| `l`                    | Clear log pane                                  |
| `q` / `Ctrl+C`         | Quit                                            |

### Flash an existing binary (`x`)

Opens a file browser starting in the project's `build/` directory (or the
project root if no build exists yet). Navigate with `up/down`, descend into
a directory with `enter`, go back up with `backspace`. If `flasher_args.json`
is present in the current directory, a `[full flash]` entry appears at the
top — selecting it flashes every component at the exact addresses idf.py
computed, in one shot. Selecting any `.bin` file or `[full flash]` does not
flash immediately: it opens a confirmation screen showing the file, the
flash address, and the target device, requiring a second deliberate `enter`
before anything is written. `esc` cancels at any point.

### Visualize partitions (`p`)

Reads the partition table directly off the connected device — not from your
project files, from the chip itself — and renders it as a proportional usage
bar plus a detail list (name, type, offset, size, percentage of flash). Also
detects the physical flash chip size via `esptool.py flash_id` so the bar
reflects real usage, including free space. Press `r` inside this view to
re-read, `esc` to go back.

## Project Detection

esp-workbench reads the project directory at startup and displays:

- **target chip** — from `sdkconfig` `CONFIG_IDF_TARGET` (most reliable), falling back to `CMakeLists.txt`
- **configured / not configured** — whether `sdkconfig` exists (i.e. `idf.py menuconfig` has been run)
- **component count** — number of subdirectories in `components/` that contain a `CMakeLists.txt`
- **partition table** — name of any `*partition*.csv` found in the project root
- **version** — from `project(...VERSION x.y.z)` in CMakeLists, or `CONFIG_APP_PROJECT_VER` in sdkconfig

If the directory is not a valid ESP-IDF project (missing `CMakeLists.txt` or `main/`), the header shows what is missing.

Note: the `partitions: *.csv` field above only names a custom partition CSV
checked into the project — it does not read the device. For the actual,
currently-flashed partition layout on the chip, use `p` (see Keybindings).

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

## Development

### Code Style

- Descriptive variable names throughout (`model` not `m`, `stringBuilder` not `sb`)
- Explicit package imports, no dot imports
- Comments explain why, not what
- Lowercase UI text throughout

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
