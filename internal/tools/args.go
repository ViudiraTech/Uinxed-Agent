package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Tool arguments are written by the model, and a model brings the tool schema
// it was trained on rather than the one we just sent: the same bash call is
// `command` in Claude Code and `cmd` here, `file_path` there and `path` here,
// `old_string`/`new_string` versus `old`/`new`. Rejecting a call because the
// field was named the other way turned a correct command into
// `invalid tool arguments: json: unknown field "command"`: an error card in the
// transcript and a wasted round trip, since the model has no reason to prefer
// one spelling and often retries with the same one.
//
// Decoding therefore folds the spellings models actually emit onto the
// canonical field and ignores keys no field declares. Display code that reads
// arguments straight from the raw call (the collapsed transcript line, the
// approval prompt) lists the alias spellings beside the canonical ones itself,
// because it sees what the model sent rather than what a tool decoded. Required values are still
// enforced by each tool, with the name the schema uses — that message is what
// tells the model how to resend the call.
var argAliases = map[string]string{
	"command":         "cmd",
	"file_path":       "path",
	"filepath":        "path",
	"old_string":      "old",
	"new_string":      "new",
	"old_str":         "old",
	"new_str":         "new",
	"old_text":        "old",
	"new_text":        "new",
	"glob":            "include", // Claude Code's Grep names the file filter glob.
	"timeout_seconds": "timeout",
}

// decode unmarshals tool arguments tolerantly: alias spellings are folded onto
// their canonical names, and keys the target struct does not declare are
// ignored instead of failing the whole call.
func decode(raw json.RawMessage, v any) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		raw = []byte("{}")
	}
	normalized, err := normalizeArgs(raw)
	if err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	if err := json.Unmarshal(normalized, v); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	return nil
}

// normalizeArgs rewrites alias keys onto their canonical names. The canonical
// key wins when the model sent both spellings, so a half-edited call cannot
// silently prefer the stale one.
func normalizeArgs(raw json.RawMessage) (json.RawMessage, error) {
	var in map[string]json.RawMessage
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage, len(in))
	changed := false
	for k, val := range in {
		canon, ok := argAliases[strings.ToLower(k)]
		if !ok {
			out[k] = val
			continue
		}
		if _, taken := in[canon]; taken {
			continue
		}
		out[canon] = val
		changed = true
	}
	if !changed {
		return raw, nil
	}
	return json.Marshal(out)
}
