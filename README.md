<p align="center">
  <img src="Design/Icon/png/videohelper-app-icon-256.png" width="96" height="96" alt="VideoHelper app icon">
</p>

<h1 align="center">VideoHelper</h1>

<p align="center"><strong>Find the moment you remember.</strong></p>

<p align="center">
  <a href="#quick-start">Quick start</a> ·
  <a href="#features">Features</a> ·
  <a href="#how-it-works">How it works</a> ·
  <a href="#docs">Docs</a> ·
  English · <a href="README.zh-CN.md">简体中文</a>
</p>

VideoHelper is a personal desktop video library that searches scenes, spoken words and sounds. Import a local file or video link, describe what you remember, and jump to the matching moment.

Your library, media and model inference stay on your computer. There is no account or hosted backend to set up.

## Why VideoHelper?

- Find a scene in footage without scrubbing through every file.
- Look up a line in a Chinese interview or recorded talk and play it in context.
- Search for a sound, such as applause or waves, across a project’s clips.

## Quick start

**The validated target is Apple Silicon Mac with macOS 13 or later.** Intel Macs are currently unsupported. Windows and Linux implementations have not yet passed real-device GUI and model validation.

There are no published binary releases yet. To build from source, install **Go 1.27.3**, **Node.js 24+** and a C toolchain. On macOS, use Xcode Command Line Tools.

```sh
git clone https://github.com/realOzine/videohelper.git
cd videohelper
npm --prefix web ci
make release mac
open Release/darwin-arm64/VideoHelper.app
```

On Mac, the main output is `Release/darwin-arm64/VideoHelper.app`. Double-click it in Finder to launch, or copy the whole `.app` to another compatible Apple Silicon Mac. Mac builds are locally signed and have not been notarized with a Developer ID.

### Build targets

`make release` builds for the current platform. Select one platform explicitly with:

| Native build host | Command | Local output |
| --- | --- | --- |
| Apple Silicon Mac | `make release mac` | `Release/darwin-arm64/VideoHelper.app` |
| Windows x64 | `make release windows` | `Release/windows-amd64/VideoHelper.exe` |
| Linux x64 | `make release linux` | `Release/linux-amd64/videohelper` |

SQLite uses CGO, so each target must be built on its own OS with the appropriate C toolchain; these commands do not cross-compile all platforms from a Mac. Only the Mac target has been validated; Windows and Linux targets still need real-device acceptance. These commands create local artifacts and do not upload a GitHub release.

For the existing single-main-executable workflow, use `make desktop` (or `make build`), then run `./bin/VideoHelper` on Mac. Both delivery formats include the UI, fonts and model-service source, but **do not include installed models or their runtime dependencies**.

### First launch

VideoHelper downloads private Python, media tools, model dependencies and weights into its application data directory. **The first setup needs internet access, reachable dependency/model hosts and sufficient disk space.** You do not need to install Python, FFmpeg or yt-dlp yourself. Later launches check and reuse cached files, then load the models; missing components or weights are downloaded when needed.

The window shows preparation progress and the selected device. You can browse the library and play already prepared videos while models load; new imports wait in a persistent queue.

The release `.app` starts directly. If you use the standalone Mac executable instead, it restores a small `.app` wrapper in the data directory and starts it through the system; the terminal launcher then exits while the app keeps running. Close the window or press `⌘Q` to quit.

1. Open **Library** and select local videos or paste a video link.
2. Wait for processing to finish, then describe a scene, line or sound in the search box.
3. Open a matching moment to play it. Use projects or search within one video to narrow the scope.

## Features

- **Three search channels.** Picture and speech are enabled by default. Sound is indexed too; enable it in **Models** when you want sound-based matches. Queries are passed to the models without automatic translation.
- **Local files and video links.** Use the native file picker or import a link through yt-dlp. Local imports copy the original into the library and leave your source file unchanged. Download support depends on the source site.
- **Moments you can play.** Results include timestamps, thumbnails and transcript excerpts where available. Jump to a hit, move between hits or scrub the timeline.
- **A manageable library.** Group videos into projects, search across the library or within a project/video, and restore deleted videos.
- **Resumable work.** View processing stages, cancel or retry tasks, and resume unfinished work after restarting the app.
- **Chinese and English UI.** Both languages share the same local library. The current speech transcription model supports Chinese.

## How it works

```text
Local video / video link
        ↓
Managed original + playback copy + thumbnails
        ↓
Picture · Chinese transcript · Sound indexes
        ↓
Text query → matching moments → jump to playback
```

The desktop shell uses [MyGo](https://github.com/egoist/mygo), Go and React. SQLite stores the library and task state; sqlite-vec searches the vectors. WeMM-Embedding-2B encodes video and transcript text, FunASR transcribes Chinese speech, and CLAP HTSAT unfused encodes sound. ASR and sound analysis retain the original audio.

WeMM selects MPS → CUDA → CPU using actual runtime checks; ASR and CLAP currently run on CPU. This detection does not establish support or performance on an unvalidated platform. See the [runtime guide](docs/desktop-runtime.md) for device selection, downloads and platform requirements.

On macOS, data lives in `~/Library/Application Support/VideoHelper` by default. `VIDEOHELPER_DATA_DIR` can select another directory. A single-file delivery still creates a database, managed media, model caches and private runtime files. Quit the app before backing up the database together with its media.

## Docs

The detailed guides are currently in Chinese:

- [Product design](docs/design.md) — scope, desktop behavior and data boundaries.
- [Implementation](docs/spec.md) — modules, local interfaces, tasks and retrieval.
- [Runtime setup](docs/desktop-runtime.md) — downloads, platforms, component licenses and explicit model tests.
- [Evaluation](Eval/README.md) — frozen datasets and historical evidence. Earlier server results do not establish desktop retrieval quality.
- [Cloud/server branch](https://github.com/realOzine/videohelper/tree/cloud) — the previous server implementation.

For development checks, run `make test-web`, `make test-go` and `make vet`. Apple Silicon Mac validation covers real-model import, indexing, search, playback and persistence; it does not establish cold-install reliability, long-video performance or retrieval quality.
