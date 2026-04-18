# screener

A terminal UI for mirroring and controlling Android devices from your desktop using [scrcpy](https://github.com/Genymobile/scrcpy) and ADB.  
Works over USB, local Wi-Fi, and Tailscale — so you can control your phone from anywhere.

---

## What it does

screener sits in your terminal and gives you a keyboard-driven interface to:

- See all your Android devices (USB and wireless) at a glance
- Launch scrcpy sessions with one keypress
- Pair new devices wirelessly without touching a command line
- Save devices with nicknames and custom connection profiles
- Use Tailscale to reach your phone even when you're not home

---

## Requirements

You need three things installed and in your `PATH` before screener will work:

| Tool | Purpose | Install |
|------|---------|---------|
| `adb` | Talks to your Android device | [Android SDK Platform-Tools](https://developer.android.com/studio/releases/platform-tools) |
| `scrcpy` | Does the actual mirroring | [github.com/Genymobile/scrcpy](https://github.com/Genymobile/scrcpy) |
| A terminal | Where screener runs | Any desktop terminal emulator |

On Linux you also need a display server running (X11 or Wayland). Run screener from a desktop terminal like Kitty, GNOME Terminal, or Alacritty — not over SSH without X forwarding.

On Windows and macOS, the native windowing system is used automatically — no extra setup needed.

---

## Install

**Download a binary** from the [Releases](https://github.com/NamasteJasutin/screener/releases) page and put it somewhere in your `PATH`.

Or build from source (requires Go 1.22+):

```bash
git clone https://github.com/NamasteJasutin/screener.git
cd screener
go build -o screener ./cmd/screener
```

---

## Setting up your phone (first time)

screener connects over Wireless Debugging, a feature built into Android 11 and newer.  
Follow these steps once per phone — after that, screener reconnects automatically.

### Step 1 — Enable Developer Options

1. Open **Settings → About phone**
2. Find **Build number** and tap it **7 times rapidly**
3. You'll see: *"You are now a developer!"*

Already done? Skip to Step 2.

### Step 2 — Enable Wireless Debugging

1. Open **Settings → Developer options**
2. Scroll down and toggle **Wireless debugging** ON
3. Confirm the prompt if it appears
4. Your phone now shows its IP address and a port number — keep this screen open

### Step 3 — Pair with screener

1. On the Wireless Debugging screen, tap **"Pair device with pairing code"**
2. Note the pairing address, port, and the 6-digit code
3. Open screener and press **`P`** to open the pairing dialog
4. Fill in the IP address, the **pairing port** (from the popup), and the 6-digit code
5. Fill in the **connect port** shown on the main Wireless Debugging screen (different from pairing port)
6. Press **Enter** — screener will pair, connect, and save the device automatically

After pairing, screener remembers the device. Next time you open screener it will reconnect on its own.

### The port changes every time

Android assigns a new connect port every time your phone reconnects to Wi-Fi. If screener says "Connection refused", press **Enter** on the device — you'll be asked for the new port. Check your phone's Wireless Debugging screen, type it in, and screener reconnects and launches immediately.

---

## Connecting over Tailscale

Tailscale lets you reach your phone from anywhere — from work, travelling, or a different network entirely.

1. Install [Tailscale](https://tailscale.com) on both your phone and your computer
2. Log in to the same Tailscale account on both devices
3. In the Tailscale app on your phone, enable **"Accept routes"**

Once both are on the same tailnet, screener automatically detects the Tailscale IP (`100.x.x.x`) and adds it as an endpoint. Connecting works exactly the same way from then on.

---

## Using screener

```
screener [--version] [--help] [--config <path>] [--log <path>] [--debug]
```

### Keyboard reference

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate devices |
| `Enter` | Connect and launch scrcpy |
| `p` | Assign a launch profile to the selected device |
| `e` | Edit device nickname and connection details |
| `D` | Delete device (asks for confirmation) |
| `r` | Refresh device list |
| `P` | Pair a new device wirelessly |
| `B` | Step-by-step first-time setup guide |
| `O` | Edit launch options for the active profile |
| `N` | New profile |
| `d` | Duplicate active profile |
| `F` | Set active profile as default |
| `1`–`5` | Toggle flags (screen-off, stay-awake, H265, audio, camera) |
| `+` / `-` | Increase / decrease video bitrate |
| `[` / `]` | Cycle colour theme |
| `W` | Show compatibility warnings |
| `?` | Help screen |
| `q` | Quit |

### Profiles

A profile is a saved set of launch settings: resolution, bitrate, codec, display mode, and more.  
You can have as many profiles as you want — for example, a low-bandwidth one for Tailscale and a high-quality one for local Wi-Fi.

Press `p` on a device to assign a profile to it. That profile will be used automatically every time you launch from that device.

### Config and log files

| File | Default location |
|------|-----------------|
| Config | `~/.config/screener/config.json` |
| Log | `~/.local/state/screener/screener.log` |

Override with `--config` and `--log` flags.

---

## USB connection

Plug in your phone over USB, enable **USB debugging** in Developer Options, and accept the authorisation prompt on your phone. screener detects it automatically on the next refresh (`r`).

---

## Troubleshooting

**"adb not found"** — install Android SDK Platform-Tools and make sure `adb` is in your `PATH`.

**"scrcpy not found"** — install scrcpy from [its GitHub page](https://github.com/Genymobile/scrcpy).

**Device shows as offline** — press `r` to refresh, or press Enter to attempt reconnection.

**Connection refused** — the port changed. Press Enter on the device, enter the new port from your phone's Wireless Debugging screen.

**Fingerprint mismatch** — the device pairing record is stale. screener removes it automatically and asks you to re-pair with `P`.

**No window opens (Linux only)** — make sure you're running screener from a desktop terminal. `DISPLAY` or `WAYLAND_DISPLAY` must be set.

---

## License

MIT
