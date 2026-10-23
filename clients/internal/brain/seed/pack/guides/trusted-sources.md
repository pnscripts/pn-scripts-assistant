# Trusted sources for models and tools

When something is missing, it is fetched only from the project's own official
source, and its checksum or signature is checked before it is used.

| What | Official source | Check |
| --- | --- | --- |
| Language models | Ollama library (ollama.com/library), pulled by Ollama | Ollama verifies each layer's sha256 digest |
| Godot | godotengine.org/download, github.com/godotengine/godot/releases | SHA512-SUMS.txt from the same release |
| Node.js | nodejs.org/dist | SHASUMS256.txt, signed by the release team |
| Python | python.org/downloads | the sha256 and signature published per file |
| Go | go.dev/dl | the sha256 listed for each file |
| Blender | blender.org/download | the sha256 file published with each release |
| MCP servers | the official MCP registry and the server's own repository | read the permissions it asks for before adding it |

Programs are installed for your user only, inside the brain's own tools folder
when possible, so they travel with a portable brain and never need an
administrator password. Anything that would need one is asked first.
