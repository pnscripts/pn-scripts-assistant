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

Speech, language and voice models, datasets and the ONNX Runtime library are not part of this repository: the program downloads them at install or run time from their publishers (see `clients/internal/preflight/`), under the publishers' own licences, listed below.

## Shipped beside the program in the Windows downloads

| File | Component | Licence |
|---|---|---|
| `WebView2Loader.dll` | Microsoft Edge WebView2 SDK loader (x64), from the NuGet package `Microsoft.Web.WebView2` 1.0.4258.31, SHA-256 of the package checked at build time (`scripts/build-packages.sh`) | BSD-style licence by Microsoft Corporation, shipped beside it as `WebView2Loader-LICENSE.txt` |

The WebView2 runtime itself is not shipped: it is part of Windows 11 and is installed on almost every Windows 10, kept up to date by Microsoft.

## Models and data the program downloads on the user's machine

Nothing below is stored in this repository or in the release files. The program downloads each item from its publisher when the user installs or enables the matching feature; each keeps its publisher's licence.

| Item | Publisher / source | Licence |
|---|---|---|
| Ollama (local model runner) | https://github.com/ollama/ollama | MIT |
| Chat models chosen from the user's hardware, for example `llama3.2:3b` (Meta) and `qwen2.5-coder:7b` (Alibaba Cloud) | Ollama model library | Llama 3.2 Community Licence (requires "Built with Llama" attribution and the Acceptable Use Policy when redistributed; here the user downloads it directly); Apache-2.0 |
| `nomic-embed-text` embedding model | Nomic AI via Ollama | Apache-2.0 |
| whisper.cpp and the Whisper `ggml-small` speech model | https://github.com/ggml-org/whisper.cpp, OpenAI Whisper | MIT |
| Piper text-to-speech and Piper voices (for example en_GB-alba-medium, bg_BG-dimitar-medium; other English voices are offered as choices) | https://github.com/rhasspy/piper, https://huggingface.co/rhasspy/piper-voices | Piper MIT; each voice has its own dataset licence in its model card (alba CC BY 4.0, dimitar CC0; lessac, amy and jenny_dioco have their own dataset terms, see the cards). Voices with non-commercial or special terms must not be bundled in a release. |
| ONNX Runtime | https://github.com/microsoft/onnxruntime | MIT |
| ESCO classification (European Commission), optional occupation and skills data | https://esco.ec.europa.eu | Reuse under the European Commission's ESCO terms (attribution to the European Commission) |
| O*NET database (U.S. Department of Labor), optional | https://www.onetcenter.org/database.html | CC BY 4.0 (attribution to the U.S. Department of Labor, Employment and Training Administration; O*NET is a trademark of the USDOL/ETA; this product is not endorsed by them) |
