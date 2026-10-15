# Third-party notices

PN Scripts Assistant is © 2026 ПН СКРИПТС ЕООД (PN Scripts) and released under the MIT licence (see `LICENSE`).
It also contains or links third-party software, which keeps its own licence.

## Files committed in this repository

| Path | Component | Licence |
|---|---|---|
| `clients/internal/stage/three.module.js` | three.js, © 2010-2024 Three.js Authors | MIT |

## Go modules compiled into the binaries

Direct dependencies (full list with indirect ones in `clients/go.mod` / `clients/go.sum`):

| Module | Licence |
|---|---|
| github.com/yalue/onnxruntime_go | MIT |
| golang.org/x/sys | BSD-3-Clause |
| modernc.org/sqlite | BSD-3-Clause |

The Go standard library is © The Go Authors, BSD-3-Clause.

Speech, language and voice models and the ONNX Runtime library are not part of this repository: the program downloads them at install or run time from their publishers (see `clients/internal/preflight/`), under the publishers' own licences.
