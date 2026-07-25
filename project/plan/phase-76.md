# Phase 76 — `toolkit`: `Read` refuses non-text files

*Realizes design Decision 28 (`Read`/`Write`/`Edit` semantics) — the content-sniffing slice only; D28's existing ids are already realized.*

Fixes a silent-corruption bug in the shipped `toolkit.Read`: it currently does `os.ReadFile` then `string(content)`, so a PDF or an image returns megabytes of binary decoded as text into the conversation.

Observable end state: `toolkit/read.go` sniffs the file's content type with `net/http.DetectContentType` and returns an error for anything not detected as text, naming the detected type and returning no content. The rejection is broad — every non-text type, not just PDFs and images — and detection is by content, so an extensionless UTF-8 file still reads normally and a binary named `.txt` is still refused. The error message is generic: it names the path and the detected type and does **not** name any extraction tool, because `toolkit` does not import `ocr` and cannot know whether such a tool is registered in the running harness.

Scope guard: this phase touches only `Read`. `Write`, `Edit`, and D28's other behaviors are unchanged, and their existing tagged tests must stay green.

**Done when** the suite is green (`go build ./...` and `go test ./...` both exit 0, per design Conventions), D28's previously-realized ids remain covered and passing, and each id below is covered by a clearly-named test in `toolkit/*_test.go` tagged with the bare id:

- R-VGH5-GE0Q — `Read` of a PDF errors naming `application/pdf` and returns no content.
- R-VHP1-U5RF — `Read` of a PNG and of a JPEG each error naming the detected image type, returning no content.
- R-VIWY-7XI4 — `Read` of an extensionless UTF-8 text file succeeds and returns its exact contents.
- R-VK4U-LP8T — `Read` of a `.txt` file whose bytes are binary is rejected, proving the extension does not override sniffing.
