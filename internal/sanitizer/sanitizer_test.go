package sanitizer

import (
	"strings"
	"testing"
)

func TestStripANSI(t *testing.T) {
	input := "\x1b[31mError:\x1b[0m File \x1b[1mnot found\x1b[0m"
	expected := "Error: File not found"
	got := StripANSI(input)
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}

	cleanInput := "plain text with no color"
	if StripANSI(cleanInput) != cleanInput {
		t.Errorf("expected %q to remain unchanged", cleanInput)
	}
}

func TestTruncateHeadTail(t *testing.T) {
	shortText := "short text"
	if got := TruncateHeadTail(shortText, 10, 10); got != shortText {
		t.Errorf("expected untruncated text %q, got %q", shortText, got)
	}

	head := "HEAD_START_"
	middle := strings.Repeat("M", 50)
	tail := "_TAIL_END"
	longText := head + middle + tail

	headLen := len(head)
	tailLen := len(tail)
	truncated := TruncateHeadTail(longText, headLen, tailLen)

	if !strings.HasPrefix(truncated, head) {
		t.Errorf("expected prefix %q, got %q", head, truncated)
	}
	if !strings.HasSuffix(truncated, tail) {
		t.Errorf("expected suffix %q, got %q", tail, truncated)
	}
	if !strings.Contains(truncated, "[...OUTPUT TRUNCATED (50 chars omitted)...]") {
		t.Errorf("expected truncation marker with omitted count, got %q", truncated)
	}
}

func TestCleanToolOutput(t *testing.T) {
	head := strings.Repeat("A", 10)
	middle := strings.Repeat("B", 100)
	tail := strings.Repeat("C", 10)
	raw := "   \x1b[32m" + head + middle + tail + "\x1b[0m   \n"

	cleaned := CleanToolOutput(raw, 10, 10)

	// Check ANSI stripped
	if strings.Contains(cleaned, "\x1b") {
		t.Errorf("cleaned output still contains ANSI escape: %q", cleaned)
	}
	// Check trimmed
	if strings.HasPrefix(cleaned, " ") || strings.HasSuffix(cleaned, "\n") {
		t.Errorf("cleaned output was not trimmed properly: %q", cleaned)
	}
	// Check truncation
	if !strings.HasPrefix(cleaned, head) {
		t.Errorf("expected prefix %q, got %q", head, cleaned)
	}
	if !strings.HasSuffix(cleaned, tail) {
		t.Errorf("expected suffix %q, got %q", tail, cleaned)
	}
	if !strings.Contains(cleaned, "[...OUTPUT TRUNCATED (100 chars omitted)...]") {
		t.Errorf("expected truncation marker, got %q", cleaned)
	}
}

func TestExtractJSON(t *testing.T) {
	// 1. Markdown wrapped JSON
	inputMarkdown := "Berikut respon saya:\n```json\n{\"thought\":\"ok\",\"action\":\"bash\",\"input\":\"ls\"}\n```\nSemoga membantu."
	expected := `{"thought":"ok","action":"bash","input":"ls"}`

	extracted, err := ExtractJSON(inputMarkdown)
	if err != nil {
		t.Fatalf("ExtractJSON failed: %v", err)
	}
	if extracted != expected {
		t.Errorf("expected %q, got %q", expected, extracted)
	}

	// 2. Pure JSON
	pureJSON := `{"action":"final_answer","input":"done"}`
	extracted, err = ExtractJSON(pureJSON)
	if err != nil {
		t.Fatalf("ExtractJSON failed on pure JSON: %v", err)
	}
	if extracted != pureJSON {
		t.Errorf("expected %q, got %q", pureJSON, extracted)
	}

	// 3. Invalid inputs without JSON brackets or malformed
	invalidInputs := []string{
		"no json here at all",
		"only { open bracket",
		"only } close bracket",
		"reversed }{ brackets",
	}
	for _, inv := range invalidInputs {
		if _, err := ExtractJSON(inv); err == nil {
			t.Errorf("expected error for invalid input %q, got nil", inv)
		}
	}
}

func TestErrorDeduplicator(t *testing.T) {
	dedup := &ErrorDeduplicator{}

	errMsg := "command not found: foobar"

	// Call 1: First occurrence should return original message
	res1 := dedup.Process(errMsg)
	if res1 != errMsg {
		t.Errorf("first call expected %q, got %q", errMsg, res1)
	}

	// Call 2: Identical second occurrence
	res2 := dedup.Process(errMsg)
	expected2 := "[Error identik terulang (x2). Ubah pendekatan Anda.]"
	if res2 != expected2 {
		t.Errorf("second call expected %q, got %q", expected2, res2)
	}

	// Call 3: Identical third occurrence
	res3 := dedup.Process(errMsg)
	expected3 := "[Error identik terulang (x3). Ubah pendekatan Anda.]"
	if res3 != expected3 {
		t.Errorf("third call expected %q, got %q", expected3, res3)
	}

	// Call 4: Different message resets counter
	newErrMsg := "permission denied"
	res4 := dedup.Process(newErrMsg)
	if res4 != newErrMsg {
		t.Errorf("new error expected %q, got %q", newErrMsg, res4)
	}

	// Call 5: Repeating new message
	res5 := dedup.Process(newErrMsg)
	expected5 := "[Error identik terulang (x2). Ubah pendekatan Anda.]"
	if res5 != expected5 {
		t.Errorf("repeat new error expected %q, got %q", expected5, res5)
	}
}
