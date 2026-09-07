package docker

import "strings"

func cleanLogLines(data []byte) []string {
	var lines []string
	idx := 0
	for idx < len(data) {
		if idx+8 <= len(data) && (data[idx] == 1 || data[idx] == 2 || data[idx] == 0) && data[idx+1] == 0 && data[idx+2] == 0 && data[idx+3] == 0 {
			frameLen := int(data[idx+4])<<24 | int(data[idx+5])<<16 | int(data[idx+6])<<8 | int(data[idx+7])
			idx += 8
			if frameLen > 0 && idx+frameLen <= len(data) {
				chunk := string(data[idx : idx+frameLen])
				for _, line := range strings.Split(chunk, "\n") {
					line = strings.TrimRight(line, "\r")
					if strings.TrimSpace(line) != "" {
						lines = append(lines, line)
					}
				}
				idx += frameLen
				continue
			}
		}
		// Fallback line by line
		chunk := string(data[idx:])
		for _, line := range strings.Split(chunk, "\n") {
			line = strings.TrimRight(line, "\r")
			if strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		}
		break
	}
	return lines
}
