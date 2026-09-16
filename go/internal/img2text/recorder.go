// T37 debug transcript recording for per-image img2text sessions.
//
// preview.img2text_all turns on one transcript per analysed image under
// <finally>/progress_items/sessions/<md>/<图file>.jsonl, in the same JSONL
// format the session engine writes, so the session preview renders them
// like any other session. The recorder hangs off AIClient (per-task
// struct copy, see runner.go) and sees every completed request/response
// exchange of that image's analysis — initial call, format-fix call,
// repair rounds — because the img2text conversation grows monotonically
// (each call replays all previous messages), a simple "messages seen so
// far" cursor reconstructs the wire transcript without touching the
// processor's internals.
package img2text

import (
	"encoding/json"
	"path/filepath"
	"sync"

	"mineru-tools/internal/session"
)

// ExchangeRecorder writes one image's exchanges to a session-engine
// transcript file. One recorder per image task; safe for the sequential
// calls one task makes (the mutex covers the shared-client edge case).
type ExchangeRecorder struct {
	mu   sync.Mutex
	tr   *session.TranscriptWriter
	seen int // messages of previous requests already persisted
}

// NewExchangeRecorder opens (creating if needed) the transcript for one
// image task. label names the session in the preview sidebar.
func NewExchangeRecorder(path, label, model, system string) (*ExchangeRecorder, error) {
	tr, err := session.NewTranscript(path)
	if err != nil {
		return nil, err
	}
	if err := tr.AppendMeta("img2text", label, model, system, nil); err != nil {
		_ = tr.Close()
		return nil, err
	}
	return &ExchangeRecorder{tr: tr}, nil
}

// Close flushes and closes the transcript.
func (r *ExchangeRecorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tr.Close()
}

// Record persists one completed exchange. req.Messages[:seen] were
// already written by previous calls; the tail is new (tool receipts,
// fix prompts), then the assistant reply and its usage line.
func (r *ExchangeRecorder) Record(req *ChatRequest, resp *ChatResponse) {
	r.mu.Lock()
	defer r.mu.Unlock()
	msgs := toSessionMessages(req.Messages)
	for i := r.seen; i < len(msgs); i++ {
		// Best effort: a transcript hiccup must not kill image analysis.
		_ = r.tr.Append(msgs[i])
	}
	r.seen = len(msgs)
	if len(resp.Choices) > 0 {
		_ = r.tr.Append(toSessionMessages([]ChatMessage{resp.Choices[0].Message})[0])
	}
	if resp.Usage != nil {
		reasoning := 0
		if resp.Usage.CompletionTokensDetails != nil {
			reasoning = resp.Usage.CompletionTokensDetails.ReasoningTokens
		}
		_ = r.tr.AppendUsage(session.UsageRecord{
			Model:        req.Model,
			Stream:       resp.Streamed,
			PromptTokens: resp.Usage.PromptTokens,
			Completion:   resp.Usage.CompletionTokens,
			Reasoning:    reasoning,
			Duration:     resp.Elapsed,
		})
	}
}

// toSessionMessages converts img2text messages to the session engine's
// transcript shape. The structs are wire-identical (Content any, same
// ToolCall layout); a JSON round-trip keeps the conversion honest if one
// side ever grows a field.
func toSessionMessages(in []ChatMessage) []session.ChatMessage {
	raw, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	out := make([]session.ChatMessage, 0, len(in))
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

// transcriptPathFor returns the debug transcript path of one image task:
// <progressRoot>/sessions/<mdName>/<图file>.jsonl.
func transcriptPathFor(progressRoot, mdName, imgRel string) string {
	base := filepath.Base(imgRel)
	return filepath.Join(progressRoot, "sessions", mdName, base+".jsonl")
}
