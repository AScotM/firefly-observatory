# Firefly Observatory

A generative, standard-library-only Go terminal artwork based on coupled oscillators, spatial neighborhoods, luminous trails, and optional Linux telemetry.

## Features

- Hundreds to thousands of independently moving agents
- Kuramoto-inspired local phase coupling and synchronization metrics
- Toroidal geometry and spatial-hash neighbor queries
- Six motion modes: `flock`, `drift`, `orbit`, `breathe`, `swarm`, `rain`
- Seven palettes: `aurora`, `ember`, `moon`, `forest`, `ocean`, `violet`, `mono`
- ANSI truecolor or ASCII terminal renderer, additive glow, persistent trails
- Optional `/proc/stat`, `/proc/meminfo`, and `/proc/diskstats` sampling on Linux
- Deterministic seeds, JSON state snapshots, JSONL timeline recording
- Offline PPM image rendering, text rendering, and JSON inspection reports
- Graceful Ctrl+C shutdown with terminal restoration
- No external Go modules, network calls, or storage modifications

## Build

Go 1.23 or newer is recommended.

```bash
go build -o firefly-observatory main.go
./firefly-observatory version
```

## Quick start

```bash
./firefly-observatory run --agents 500 --palette aurora --fps 20
./firefly-observatory run --mode breathe --palette moon --trails 0.91
./firefly-observatory run --mode orbit --palette ember --agents 700
./firefly-observatory run --mode drift --palette forest --telemetry
```

Press Ctrl+C to exit. The application uses the alternate screen and hides the cursor while rendering, restoring both on normal shutdown and handled termination signals.

## Commands

| Command | Description |
| --- | --- |
| `run` | Animated terminal observatory |
| `render` | Simulate and export a still image as binary PPM or ASCII text |
| `inspect` | Simulate and print or save a JSON summary |
| `palettes` | List available color palettes |
| `modes` | List movement modes |
| `version` | Print build environment and version |

## Examples

```bash
./firefly-observatory run --seed 42 --frames 400 --snapshot state.json --record timeline.jsonl
./firefly-observatory run --restore state.json --frames 300 --snapshot continued.json
./firefly-observatory render --width 160 --height 90 --frames 600 --output night.ppm
./firefly-observatory render --width 100 --height 35 --frames 240 --output night.txt
./firefly-observatory inspect --agents 1000 --frames 500 --output analysis.json
./firefly-observatory run --headless --frames 1000 --telemetry
```

PPM is a simple lossless RGB image format; ImageMagick can convert it with `magick night.ppm night.png` if installed.

## Parameters

`--agents`, `--fps`, `--width`, `--height`, `--seed`, `--palette`, `--mode`, `--coupling`, `--radius`, `--speed`, `--noise`, `--trails`, `--density`, `--wind`, `--gravity`, `--bloom`, `--duration`, `--frames`, `--telemetry`, `--record`, `--snapshot`, `--restore`, `--headless`, `--ascii`, `--stats`, `--quiet`, `--output`.

Use `./firefly-observatory run --help` for defaults and detailed descriptions. The default dimensions follow the terminal when available; specify `--width` and `--height` to produce repeatable geometry.

## Model

Each agent has a phase, natural frequency, velocity, location, family, and luminous energy. For nearby agents, phase evolves according to a local coupling term:

```
d(theta_i)/dt = omega_i + K / max(1, N_i) * sum_j sin(theta_j - theta_i)
```

The synchronization order parameter is the magnitude of the average unit phase vector. Values approach 1 when oscillators align and 0 when phases are dispersed. Motion and oscillator dynamics are coupled indirectly through the neighborhood graph. Glow is accumulated in a fading RGB field before rendering.

## Limitations

This is generative art, not a physically exact firefly biology simulator. The spatial index is reconstructed every step. The snapshot saves agents and configuration but not the trail framebuffer or random generator state; restoring therefore does not guarantee pixel-identical replay. Seeded runs with the same dimensions and options are reproducible without telemetry. Linux telemetry is optional, and unavailable counters resolve to zero. `--telemetry` is sampled from the host's visible `/proc` namespace and should not be interpreted as a comprehensive physical hardware monitor. PPM images are single frames, not video.

Large terminal sizes, high FPS, or thousands of agents can consume significant CPU. Start with 200–500 agents on modest systems.

## License

GNU General Public License version 3. See `LICENSE`.
