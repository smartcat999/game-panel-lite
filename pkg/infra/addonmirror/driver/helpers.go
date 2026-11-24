package driver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// IsLatestAlias reports whether v represents a floating release alias rather than a concrete tag.
func IsLatestAlias(v string) bool {
	low := strings.ToLower(strings.TrimSpace(v))
	return low == "" || low == "latest" || low == "stable"
}

// InferArchiveFormat maps a release filename suffix to a normalized registry ArchiveFormat constant.
func InferArchiveFormat(fname string) string {
	low := strings.ToLower(fname)
	switch {
	case strings.HasSuffix(low, ".tar.gz") || strings.HasSuffix(low, ".tgz"):
		return ArchiveFormatTarGz
	case strings.HasSuffix(low, ".zip"):
		return ArchiveFormatZip
	case strings.HasSuffix(low, ".jar"):
		return ArchiveFormatJar
	case strings.HasSuffix(low, ".tmod"):
		return ArchiveFormatTMod
	default:
		return ArchiveFormatRaw
	}
}

// FirstNonEmpty returns the first trimmed non-empty string in vals.
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// CompareSemanticDesc compares two version strings in descending semantic order.
func CompareSemanticDesc(a, b string) int {
	pa, pb := splitSemTokens(a), splitSemTokens(b)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, errA := strconv.Atoi(pa[i])
		nb, errB := strconv.Atoi(pb[i])
		if errA == nil && errB == nil && na != nb {
			if na > nb {
				return -1
			}
			return 1
		}
		if pa[i] != pb[i] {
			if pa[i] > pb[i] {
				return -1
			}
			return 1
		}
	}
	if len(pa) != len(pb) {
		if len(pa) > len(pb) {
			return -1
		}
		return 1
	}
	return strings.Compare(b, a)
}

func splitSemTokens(v string) []string {
	v = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "v")
	return strings.FieldsFunc(v, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

// DedupeTrimmed trims whitespace, drops empty values, and preserves first-seen order.
func DedupeTrimmed(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, item := range in {
		clean := strings.TrimSpace(item)
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; !ok {
			seen[clean] = struct{}{}
			out = append(out, clean)
		}
	}
	return out
}

// CommitEntry holds normalized commit metadata returned by PaperMC / Purpur build APIs.
type CommitEntry struct {
	SHA         string `json:"sha"`
	Hash        string `json:"hash"`
	Message     string `json:"message"`
	Description string `json:"description"`
	Author      string `json:"author"`
}

// FormatCommitChangelog renders commits into concise Markdown bullet points.
func FormatCommitChangelog(commits []CommitEntry) string {
	lines := make([]string, 0, len(commits))
	for _, c := range commits {
		msg := strings.TrimSpace(FirstNonEmpty(c.Message, c.Description))
		if msg == "" {
			continue
		}
		firstLine := strings.SplitN(msg, "\n", 2)[0]
		sha := strings.TrimSpace(FirstNonEmpty(c.SHA, c.Hash))
		if len(sha) > 7 {
			sha = sha[:7]
		}
		if sha != "" {
			lines = append(lines, fmt.Sprintf("- `%s` %s", sha, firstLine))
		} else {
			lines = append(lines, "- "+firstLine)
		}
	}
	return strings.Join(lines, "\n")
}

// ParseRFC3339Ptr parses an optional RFC3339 timestamp string into UTC.
func ParseRFC3339Ptr(raw string) *time.Time {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

// ParseMillisPtr converts Unix milliseconds into an optional UTC *time.Time.
func ParseMillisPtr(ms int64) *time.Time {
	if ms <= 0 {
		return nil
	}
	utc := time.UnixMilli(ms).UTC()
	return &utc
}

// CacheHashKey builds a namespaced content-hashed Redis cache key for addonmirror queries.
func CacheHashKey(namespace, raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return "addonmirror:v2:" + namespace + ":" + hex.EncodeToString(sum[:16])
}

// ContainsString reports whether target is present in slice.
func ContainsString(slice []string, target string) bool {
	for _, item := range slice {
		if item == target {
			return true
		}
	}
	return false
}

// IsDigitsOnly reports whether s is non-empty and consists solely of ASCII decimal digits.
func IsDigitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// GitHubAvatarURL builds a canonical GitHub owner avatar URL on the allowlisted avatars host.
func GitHubAvatarURL(owner string) string {
	trimmed := strings.TrimSpace(owner)
	if trimmed == "" || strings.ContainsAny(trimmed, "/\\ \t\r\n?&#") {
		return ""
	}
	return DefaultGitHubAvatarBaseURL + "/" + trimmed + "?s=128"
}
