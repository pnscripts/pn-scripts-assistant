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
| Ollama (local model runner; setup installs release v0.40.11, SHA-256 of each Linux archive checked, and keeps a newer Ollama the user already has) | https://github.com/ollama/ollama | MIT, © Ollama |
| Chat models chosen from the user's hardware, for example `llama3.2:3b` (Meta) and `qwen2.5-coder:7b` (Alibaba Cloud) | Ollama model library | Llama 3.2 Community Licence (requires "Built with Llama" attribution and the Acceptable Use Policy when redistributed; here the user downloads it directly); Apache-2.0 |
| `nomic-embed-text` embedding model | Nomic AI via Ollama | Apache-2.0 |
| whisper.cpp (setup builds release v1.9.5, commit checked) and the Whisper small speech model (`ggml-small.bin` from Hugging Face `ggerganov/whisper.cpp` at commit 5359861c739e955e79d9a303bcbc70fb988958b1, SHA-256 checked) | https://github.com/ggml-org/whisper.cpp, OpenAI Whisper | whisper.cpp MIT, © The ggml authors; Whisper model MIT, © OpenAI |
| Piper text-to-speech (setup installs release 2023.11.14-2, SHA-256 checked) and three Piper voices (see "Voices and their credits" below); other voices are offered as choices | https://github.com/rhasspy/piper, https://huggingface.co/rhasspy/piper-voices | Piper MIT, © Michael Hansen; each voice has its own dataset licence, below. Voices with non-commercial or special terms must not be bundled in a release. |
| ONNX Runtime | https://github.com/microsoft/onnxruntime | MIT |
| ESCO classification (European Commission), optional occupation and skills data | https://esco.ec.europa.eu | Reuse under the European Commission's ESCO terms (attribution to the European Commission) |
| O*NET database (U.S. Department of Labor), optional | https://www.onetcenter.org/database.html | CC BY 4.0 (attribution to the U.S. Department of Labor, Employment and Training Administration; O*NET is a trademark of the USDOL/ETA; this product is not endorsed by them) |

## Voices and their credits

Setup downloads three Piper voices from `rhasspy/piper-voices` at commit 5041a7a49ae5b3a0f30e6837769eb03fdbb231bd, each checked by SHA-256. Their model cards name the dataset each was trained on and its licence; the credits below are given in the program too (Updates view, "Credits and licences", in English and Bulgarian, and in the list of voices).

| Voice | Used as | Dataset and credit | Licence |
|---|---|---|---|
| `en_GB-jenny_dioco-medium` | the robot (the neural voice with a machine timbre put on it) | The voice of **Jenny (Dioco)**, from the Jenny (Dioco) dataset, https://github.com/dioco-group/jenny-tts-dataset | The dataset's own terms: attribution is required in software that generates audio with it; the voice must be referred to as "Jenny", and where at all practical as "Jenny (Dioco)". Commercial use is permitted. The program calls the voice "Jenny (Dioco)" in its list of voices and names it in the robot's name. |
| `en_GB-alba-medium` | the woman | Alba speech corpus, Cassia Valentini-Botinhao and Junichi Yamagishi, University of Edinburgh, https://hdl.handle.net/10283/3270 | CC BY 4.0 |
| `en_GB-northern_english_male-medium` | the man | Crowdsourced high-quality UK and Ireland English Dialect speech data set, OpenSLR 83, © 2018, 2019 Google, Inc., https://www.openslr.org/83/ | CC BY-SA 4.0 |

All three model cards say the voice was fine-tuned from Piper's U.S. English `lessac` voice (medium), whose model card names the Blizzard 2013 Lessac data and its licence: https://www.cstr.ed.ac.uk/projects/blizzard/2013/lessac_blizzard2013/license.html
