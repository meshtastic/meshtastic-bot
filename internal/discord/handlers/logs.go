package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode/utf8"
)

// logFileLimit is how much of one attached file is read.
const logFileLimit = 256 << 10

var logHTTP = &http.Client{Timeout: 10 * time.Second}

// logFile is an attachment as the issue shows it: its text, or a note saying
// why it was left out.
type logFile struct {
	Name    string
	Content string
	Note    string
}

var textExtensions = map[string]bool{".txt": true, ".log": true, ".json": true, ".csv": true, ".yaml": true, ".yml": true}

func isTextFile(f AttachedFile) bool {
	ct := strings.ToLower(f.ContentType)
	return strings.HasPrefix(ct, "text/") || strings.HasPrefix(ct, "application/json") ||
		textExtensions[strings.ToLower(path.Ext(f.Name))]
}

// fetchLogs downloads the reporter's text attachments. Discord's attachment
// links expire, so the text itself goes in the issue rather than a link.
//
// room is the characters left for the logs as formatLog renders them, fences
// and wrappers included; a log that does not fit is cut to fit, and an entry
// with no room even for its note is dropped.
func fetchLogs(files []AttachedFile, room int) []logFile {
	out := make([]logFile, 0, len(files))
	for _, f := range files {
		entry := logFile{Name: f.Name}
		if !isTextFile(f) {
			entry.Note = "not included; only text files can be attached from Discord, so add it in a comment"
		} else if text, truncated, err := downloadText(f.URL); err != nil {
			log.Printf("Could not download an attached log: %v", err)
			entry.Note = "could not be downloaded"
		} else {
			entry.Content = text
			if truncated {
				entry.Note = "truncated"
			}
			entry = fitLog(entry, room)
		}
		if size := renderedSize(entry); size <= room {
			out = append(out, entry)
			room -= size
		}
	}
	return out
}

func renderedSize(l logFile) int { return utf8.RuneCountInString(formatLog(l)) }

// fitLog cuts a log's text until its rendered entry fits room. Cutting can
// only shorten the fence, so the overhead measured on the full text bounds it.
func fitLog(l logFile, room int) logFile {
	size := renderedSize(l)
	if size <= room {
		return l
	}
	cut := logFile{Name: l.Name, Content: l.Content, Note: "truncated"}
	keep := room - (renderedSize(cut) - utf8.RuneCountInString(l.Content))
	if keep <= 0 {
		return logFile{Name: l.Name, Note: "not included; the issue has no room for more logs"}
	}
	cut.Content = string([]rune(l.Content)[:keep])
	return cut
}

func downloadText(url string) (text string, truncated bool, err error) {
	resp, err := logHTTP.Get(url)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, logFileLimit+1))
	if err != nil {
		return "", false, err
	}
	if len(data) > logFileLimit {
		data, truncated = data[:logFileLimit], true
	}
	if !utf8.Valid(data) {
		// A cut can split the last character; anything else is not text.
		trimmed := []byte(strings.ToValidUTF8(string(data), ""))
		if !truncated || len(data)-len(trimmed) > utf8.UTFMax {
			return "", false, fmt.Errorf("not UTF-8 text")
		}
		data = trimmed
	}
	return string(data), truncated, nil
}
