package l4d2

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
)

const (
	vpkMagic           = uint32(0x55aa1234)
	vpkHeaderV1Size    = int64(12)
	vpkHeaderV2Size    = int64(28)
	vpkEntryTerminator = uint16(0xffff)
	vpkSelfContained   = uint16(0x7fff)
	maxVPKTreeBytes    = 32 << 20
	maxVPKMissionBytes = 256 << 10
)

var l4d2MissionMapRe = regexp.MustCompile(`(?im)(?:^|[\s{])(?:"map"|map)\s+(?:"([^"\s]+)"|([^\s"{}]+))`)

type vpkHeader struct {
	version    uint32
	treeSize   uint32
	headerSize int64
}

type vpkEntry struct {
	relPath      string
	archiveIndex uint16
	entryOffset  uint32
	entryLength  uint32
	preload      []byte
}

func extractVPKComponentsFromFile(filePath string) ([]vpkTextFile, []string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()
	hdr, tree, err := readVPKHeaderAndTree(f)
	if err != nil {
		return nil, nil, err
	}
	entries, err := parseVPKTree(tree)
	if err != nil {
		return nil, nil, err
	}
	missions, bsps := partitionVPKMapEntries(entries)
	dataOffset := hdr.headerSize + int64(hdr.treeSize)
	missionBodies := make([]vpkTextFile, 0, len(missions))
	for _, m := range missions {
		if body, ok := readVPKEntryAt(f, dataOffset, m); ok {
			missionBodies = append(missionBodies, vpkTextFile{relPath: m.relPath, content: body})
		}
	}
	return missionBodies, bsps, nil
}

func extractVPKMapsFromFile(filePath string) ([]string, error) {
	missions, bsps, err := extractVPKComponentsFromFile(filePath)
	if err != nil {
		return nil, err
	}
	return resolveVPKMaps(missions, bsps), nil
}

func extractVPKComponentsFromStream(r io.Reader) ([]vpkTextFile, []string, error) {
	_, tree, err := readVPKHeaderAndTree(r)
	if err != nil {
		return nil, nil, err
	}
	entries, err := parseVPKTree(tree)
	if err != nil {
		return nil, nil, err
	}
	missions, bsps := partitionVPKMapEntries(entries)
	missionBodies := readVPKMissionsFromStream(r, missions)
	return missionBodies, bsps, nil
}

func readVPKHeaderAndTree(r io.Reader) (vpkHeader, []byte, error) {
	var raw [vpkHeaderV2Size]byte
	if _, err := io.ReadFull(r, raw[:vpkHeaderV1Size]); err != nil {
		return vpkHeader{}, nil, err
	}
	magic := binary.LittleEndian.Uint32(raw[0:4])
	ver := binary.LittleEndian.Uint32(raw[4:8])
	treeSize := binary.LittleEndian.Uint32(raw[8:12])
	if magic != vpkMagic || (ver != 1 && ver != 2) {
		return vpkHeader{}, nil, errors.New("not a valid vpk header")
	}
	if treeSize == 0 || treeSize > maxVPKTreeBytes {
		return vpkHeader{}, nil, fmt.Errorf("invalid vpk tree size %d", treeSize)
	}
	hdr := vpkHeader{version: ver, treeSize: treeSize, headerSize: vpkHeaderV1Size}
	if ver == 2 {
		if _, err := io.ReadFull(r, raw[vpkHeaderV1Size:vpkHeaderV2Size]); err != nil {
			return vpkHeader{}, nil, err
		}
		hdr.headerSize = vpkHeaderV2Size
	}
	tree := make([]byte, treeSize)
	if _, err := io.ReadFull(r, tree); err != nil {
		return vpkHeader{}, nil, err
	}
	return hdr, tree, nil
}

func parseVPKTree(tree []byte) ([]vpkEntry, error) {
	pos := 0
	var out []vpkEntry
	for {
		ext, ok := readVPKCString(tree, &pos)
		if !ok {
			return nil, errors.New("truncated vpk extension")
		}
		if ext == "" {
			return out, nil
		}
		dirEntries, err := parseVPKDirsForExt(tree, &pos, ext)
		if err != nil {
			return nil, err
		}
		out = append(out, dirEntries...)
	}
}

func parseVPKDirsForExt(tree []byte, pos *int, ext string) ([]vpkEntry, error) {
	var out []vpkEntry
	for {
		dir, ok := readVPKCString(tree, pos)
		if !ok {
			return nil, errors.New("truncated vpk directory")
		}
		if dir == "" {
			return out, nil
		}
		for {
			name, ok := readVPKCString(tree, pos)
			if !ok {
				return nil, errors.New("truncated vpk filename")
			}
			if name == "" {
				break
			}
			entry, err := readVPKEntryRecord(tree, pos, formatVPKPath(ext, dir, name))
			if err != nil {
				return nil, err
			}
			out = append(out, entry)
		}
	}
}

func readVPKEntryRecord(tree []byte, pos *int, relPath string) (vpkEntry, error) {
	if *pos+18 > len(tree) {
		return vpkEntry{}, errors.New("truncated vpk entry header")
	}
	rec := tree[*pos : *pos+18]
	*pos += 18
	preloadBytes := int(binary.LittleEndian.Uint16(rec[4:6]))
	archiveIdx := binary.LittleEndian.Uint16(rec[6:8])
	entryOff := binary.LittleEndian.Uint32(rec[8:12])
	entryLen := binary.LittleEndian.Uint32(rec[12:16])
	if binary.LittleEndian.Uint16(rec[16:18]) != vpkEntryTerminator {
		return vpkEntry{}, errors.New("invalid vpk entry terminator")
	}
	if *pos+preloadBytes > len(tree) {
		return vpkEntry{}, errors.New("truncated vpk preload data")
	}
	var preload []byte
	if preloadBytes > 0 {
		preload = tree[*pos : *pos+preloadBytes]
	}
	*pos += preloadBytes
	return vpkEntry{
		relPath:      relPath,
		archiveIndex: archiveIdx,
		entryOffset:  entryOff,
		entryLength:  entryLen,
		preload:      preload,
	}, nil
}

func readVPKCString(buf []byte, pos *int) (string, bool) {
	if *pos >= len(buf) {
		return "", false
	}
	idx := bytes.IndexByte(buf[*pos:], 0)
	if idx < 0 {
		return "", false
	}
	s := string(buf[*pos : *pos+idx])
	*pos += idx + 1
	return s, true
}

func formatVPKPath(ext, dir, name string) string {
	var b strings.Builder
	if dir != "" && dir != " " {
		b.WriteString(strings.Trim(dir, "/"))
		b.WriteByte('/')
	}
	if name != " " {
		b.WriteString(name)
	}
	if ext != "" && ext != " " {
		b.WriteByte('.')
		b.WriteString(ext)
	}
	return b.String()
}

type vpkTextFile struct {
	relPath string
	content string
}

func partitionVPKMapEntries(entries []vpkEntry) ([]vpkEntry, []string) {
	var missions []vpkEntry
	var bsps []string
	for _, e := range entries {
		if isL4D2MissionTextRel(e.relPath) {
			missions = append(missions, e)
		} else if stem := extractBSPMapStem(e.relPath); stem != "" && !slices.Contains(bsps, stem) {
			bsps = append(bsps, stem)
		}
	}
	return missions, bsps
}

func readVPKEntryAt(ra io.ReaderAt, dataOffset int64, e vpkEntry) (string, bool) {
	var buf bytes.Buffer
	buf.Write(e.preload)
	if e.entryLength > 0 && e.archiveIndex == vpkSelfContained && buf.Len() < maxVPKMissionBytes {
		need := min(int64(e.entryLength), int64(maxVPKMissionBytes-buf.Len()))
		sr := io.NewSectionReader(ra, dataOffset+int64(e.entryOffset), need)
		_, _ = io.Copy(&buf, sr)
	}
	if buf.Len() == 0 {
		return "", false
	}
	return stripUTF8BOM(buf.String()), true
}

func readVPKMissionsFromStream(r io.Reader, missions []vpkEntry) []vpkTextFile {
	out := make([]vpkTextFile, 0, len(missions))
	var inline []vpkEntry
	for _, m := range missions {
		if m.entryLength > 0 && m.archiveIndex == vpkSelfContained && len(m.preload) < maxVPKMissionBytes {
			inline = append(inline, m)
			continue
		}
		if len(m.preload) > 0 {
			out = append(out, vpkTextFile{relPath: m.relPath, content: stripUTF8BOM(string(m.preload))})
		}
	}
	sort.Slice(inline, func(i, j int) bool { return inline[i].entryOffset < inline[j].entryOffset })
	var cursor int64
	for _, m := range inline {
		body, nextCursor, ok := readNextStreamSpan(r, cursor, m)
		cursor = nextCursor
		if ok {
			out = append(out, vpkTextFile{relPath: m.relPath, content: body})
		} else if len(m.preload) > 0 {
			out = append(out, vpkTextFile{relPath: m.relPath, content: stripUTF8BOM(string(m.preload))})
		}
	}
	return out
}

func readNextStreamSpan(r io.Reader, cursor int64, m vpkEntry) (string, int64, bool) {
	off := int64(m.entryOffset)
	if off < cursor {
		return "", cursor, false
	}
	if gap := off - cursor; gap > 0 {
		if _, err := io.CopyN(io.Discard, r, gap); err != nil {
			return "", cursor, false
		}
		cursor = off
	}
	var buf bytes.Buffer
	buf.Write(m.preload)
	need := min(int64(m.entryLength), int64(maxVPKMissionBytes-buf.Len()))
	if need > 0 {
		n, _ := io.CopyN(&buf, r, need)
		cursor += n
	}
	return stripUTF8BOM(buf.String()), cursor, buf.Len() > 0
}

func isL4D2MissionTextRel(rel string) bool {
	low := strings.ToLower(strings.TrimLeft(strings.ReplaceAll(rel, "\\", "/"), "./"))
	base := path.Base(low)
	return path.Dir(low) == "missions" && strings.HasSuffix(base, ".txt") && !strings.HasPrefix(base, "credits")
}

func extractBSPMapStem(rel string) string {
	low := strings.ToLower(strings.TrimLeft(strings.ReplaceAll(rel, "\\", "/"), "./"))
	if path.Dir(low) != "maps" || !strings.HasSuffix(low, ".bsp") {
		return ""
	}
	return strings.TrimSuffix(path.Base(strings.TrimSpace(rel)), path.Ext(rel))
}

func resolveVPKMaps(missions []vpkTextFile, bsps []string) []string {
	sort.Slice(missions, func(i, j int) bool { return missions[i].relPath < missions[j].relPath })
	var missionMaps []string
	for _, m := range missions {
		for _, mapCode := range parseL4D2MissionMaps(m.content) {
			if !slices.Contains(missionMaps, mapCode) {
				missionMaps = append(missionMaps, mapCode)
			}
		}
	}
	if len(bsps) == 0 {
		return missionMaps
	}
	return filterAndOrderMapsByBSP(missionMaps, bsps)
}

func filterAndOrderMapsByBSP(missionMaps, bsps []string) []string {
	bspByLower := make(map[string]string, len(bsps))
	for _, b := range bsps {
		key := strings.ToLower(b)
		if _, exists := bspByLower[key]; !exists {
			bspByLower[key] = b
		}
	}
	out := make([]string, 0, len(bsps))
	seen := make(map[string]bool, len(bsps))
	for _, m := range missionMaps {
		key := strings.ToLower(m)
		if actual, ok := bspByLower[key]; ok && !seen[key] {
			seen[key] = true
			out = append(out, actual)
		}
	}
	sortedBSPs := slices.Clone(bsps)
	sort.Strings(sortedBSPs)
	for _, b := range sortedBSPs {
		key := strings.ToLower(b)
		if !seen[key] {
			seen[key] = true
			out = append(out, b)
		}
	}
	return out
}

func parseL4D2MissionMaps(raw string) []string {
	cleaned := stripKVLineComments(stripUTF8BOM(raw))
	if !strings.Contains(strings.ToLower(cleaned), "modes") {
		return nil
	}
	var out []string
	for _, match := range l4d2MissionMapRe.FindAllStringSubmatch(cleaned, -1) {
		val := strings.TrimSpace(match[1])
		if val == "" && len(match) > 2 {
			val = strings.TrimSpace(match[2])
		}
		val = sanitizeL4D2MapName(val)
		if val != "" && !slices.Contains(out, val) {
			out = append(out, val)
		}
	}
	return out
}

func sanitizeL4D2MapName(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.ContainsAny(trimmed, "/\\ \t\r\n") {
		return ""
	}
	if strings.HasSuffix(strings.ToLower(trimmed), ".bsp") {
		trimmed = trimmed[:len(trimmed)-4]
	}
	return trimmed
}

func stripKVLineComments(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lines[i] = stripLineCommentOutsideQuotes(line)
	}
	return strings.Join(lines, "\n")
}

func stripLineCommentOutsideQuotes(line string) string {
	inQuotes := false
	escaped := false
	for i := range len(line) {
		c := line[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inQuotes {
			escaped = true
			continue
		}
		if c == '"' {
			inQuotes = !inQuotes
			continue
		}
		if !inQuotes && c == '/' && i+1 < len(line) && line[i+1] == '/' {
			return line[:i]
		}
	}
	return line
}

func stripUTF8BOM(s string) string {
	return strings.TrimPrefix(s, "\xef\xbb\xbf")
}
