package mod

import (
	"strings"
	"testing"
)

func TestReadMetadataParsesTModHeader(t *testing.T) {
	data := append([]byte("TMOD"), binaryString("2025.9.3.3")...)
	data = append(data, make([]byte, 20+256)...)
	data = append(data, 0x79, 0x9a, 0x05, 0x00)
	data = append(data, binaryString("RecipeBrowser")...)
	data = append(data, binaryString("0.12G")...)

	metadata, err := ReadMetadata(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Name != "RecipeBrowser" || metadata.Version != "0.12G" || metadata.TModLoaderVersion != "2025.9.3.3" {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
}

func binaryString(value string) []byte {
	return append([]byte{byte(len(value))}, []byte(value)...)
}
