package sanitizer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI removes ANSI escape color and formatting sequences from input.
func StripANSI(input string) string {
	return ansiRegex.ReplaceAllString(input, "")
}

// TruncateHeadTail keeps headLen at start and tailLen at end, truncating the middle if exceeded.
func TruncateHeadTail(input string, headLen int, tailLen int) string {
	if headLen < 0 {
		headLen = 0
	}
	if tailLen < 0 {
		tailLen = 0
	}
	if len(input) <= headLen+tailLen {
		return input
	}
	omitted := len(input) - headLen - tailLen
	return input[:headLen] + "\n\n[...OUTPUT TRUNCATED (" + strconv.Itoa(omitted) + " chars omitted)...]\n\n" + input[len(input)-tailLen:]
}

// CleanToolOutput runs StripANSI, strings.TrimSpace, and TruncateHeadTail on raw tool output.
func CleanToolOutput(raw string, headLen int, tailLen int) string {
	stripped := StripANSI(raw)
	trimmed := strings.TrimSpace(stripped)
	return TruncateHeadTail(trimmed, headLen, tailLen)
}

// ExtractJSON finds the substring from the first '{' to the last '}' in raw.
func ExtractJSON(raw string) (string, error) {
	start := strings.IndexByte(raw, '{')
	end := strings.LastIndexByte(raw, '}')
	if start == -1 || end == -1 || start >= end {
		return "", fmt.Errorf("no valid JSON object found in output")
	}
	return raw[start : end+1], nil
}

// ErrorDeduplicator tracks repeating consecutive execution errors to prevent token bloat.
type ErrorDeduplicator struct {
	lastError string
	count     int
}

// Process returns the output, or a deduplication warning if identical to the previous output.
func (d *ErrorDeduplicator) Process(cleanedOutput string) string {
	if d.lastError == cleanedOutput && cleanedOutput != "" {
		d.count++
		return fmt.Sprintf("[Error identik terulang (x%d). Ubah pendekatan Anda.]", d.count)
	}
	d.lastError = cleanedOutput
	d.count = 1
	return cleanedOutput
}
