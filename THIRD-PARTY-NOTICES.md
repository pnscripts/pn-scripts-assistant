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
| Piper text-to-speech (setup installs release 2023.11.14-2, SHA-256 checked) and three Piper voices trained from scratch on public domain recordings (see "Voices and their credits" below); voices the user adds are offered as choices | https://github.com/rhasspy/piper, https://huggingface.co/rhasspy/piper-voices | Piper MIT, © Michael Hansen; each voice has its own dataset licence, below. Voices with non-commercial, research-only or special terms, or fine-tuned from such a voice, must not be bundled or installed by setup. |
| ONNX Runtime (runs the "Answer only me" model; release 1.31.0, SHA-256 checked) | https://github.com/microsoft/onnxruntime | MIT |
| "Answer only me" speaker model: CAM++ from the 3D-Speaker project, trained on VoxCeleb (ModelScope `iic/speech_campplus_sv_en_voxceleb_16k`), as converted to ONNX in Hugging Face `csukuangfj/speaker-embedding-models` at commit 0743f301363dec56491a490f6d6cbc9d67f9a3bf, SHA-256 checked | https://github.com/modelscope/3D-Speaker | Apache-2.0 (the model card on ModelScope). The VoxCeleb data it was trained on is offered for research under CC BY 4.0, the recordings staying with their owners (mm.kaist.ac.kr/datasets/voxceleb, read 2026-11-10); the program downloads the model only when "Answer only me" is first turned on |
| ESCO classification (European Commission), optional occupation and skills data | https://esco.ec.europa.eu | Reuse under the European Commission's ESCO terms (attribution to the European Commission) |
| O*NET database (U.S. Department of Labor), optional | https://www.onetcenter.org/database.html | CC BY 4.0 (attribution to the U.S. Department of Labor, Employment and Training Administration; O*NET is a trademark of the USDOL/ETA; this product is not endorsed by them) |

## Voices and their credits

Setup downloads three Piper voices from `rhasspy/piper-voices` at commit 5041a7a49ae5b3a0f30e6837769eb03fdbb231bd, each checked by SHA-256. Each was trained from scratch, not fine-tuned from another voice, on recordings in the public domain, so the whole of what each voice is made from may be used for any purpose, commercial use included. Nothing is owed; they are credited anyway, in the program too (Updates view, "Credits and licences", in English and Bulgarian).

| Voice | Used as | Made by, from what | Licence |
|---|---|---|---|
| `en_GB-cori-medium` | the robot (the neural voice with a machine timbre put on it), the default | **Cori**, a British woman's voice, trained from scratch by Bryce Beattie on about 24 hours of LibriVox recordings (model card at the pinned commit; https://brycebeattie.com/files/tts/) | public domain (LibriVox donates every recording to the public domain: https://librivox.org/pages/public-domain/) |
| `en_US-ljspeech-medium` | the woman | **LJ Speech**, an American woman's voice, trained from scratch by Bryce Beattie on the LJ Speech dataset by Keith Ito (https://keithito.com/LJ-Speech-Dataset/), LibriVox recordings of 1884 to 1964 texts | public domain ("There are no restrictions on its use") |
| `en_US-norman-medium` | the man | **Norman**, an American man's voice, trained from scratch by Bryce Beattie on about 15.5 hours of LibriVox recordings | public domain |

The voices installed before 1.0.0 (`en_GB-jenny_dioco-medium`, `en_GB-alba-medium`, `en_GB-northern_english_male-medium`) are no longer installed or spoken with: each model card says the voice was fine-tuned from Piper's U.S. English `lessac` voice, whose recordings (Blizzard 2013 Lessac data) are licensed "exclusively for Research Purposes only": https://www.cstr.ed.ac.uk/projects/blizzard/2013/lessac_blizzard2013/license.html. A copy updated from an older version keeps their files until the person removes them, and never uses them.

The Bulgarian voice is espeak-ng's own (GPL-3.0, installed from the system's packages), which is rule-based and not trained on recordings. Piper's Bulgarian voice `bg_BG-dimitar-medium` is not installed by setup: its model card also says it was fine-tuned from `lessac`. A Piper voice somebody adds themselves is offered as they add it, under its own licence, except the three withdrawn medium voices by their file names (`en_GB-jenny_dioco-medium`, `en_GB-alba-medium`, `en_GB-northern_english_male-medium`), which are skipped.
