package apiclient

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// Console returns a slice of a build's console output starting at byte offset
// start. It uses the progressive log endpoint so the same call backs both a
// one-shot read and a follow loop: More reports whether the build is still
// emitting output, and NextStart is the offset to pass on the next call.
func (c *apiClient) Console(ctx context.Context, path, ref string, start int64) (*ConsoleChunk, error) {
	if start < 0 {
		start = 0
	}
	q := url.Values{"start": {strconv.FormatInt(start, 10)}}
	body, header, err := c.getText(ctx, jobPath(path)+"/"+buildRef(ref)+"/logText/progressiveText", q)
	if err != nil {
		return nil, err
	}
	chunk := &ConsoleChunk{
		Text:      string(body),
		More:      strings.EqualFold(header.Get("X-More-Data"), "true"),
		NextStart: start + int64(len(body)),
	}
	if sz := header.Get("X-Text-Size"); sz != "" {
		if n, perr := strconv.ParseInt(sz, 10, 64); perr == nil {
			chunk.NextStart = n
		}
	}
	return chunk, nil
}
