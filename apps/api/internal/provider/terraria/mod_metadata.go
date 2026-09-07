package terraria

import (
	"encoding/binary"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func (TModLoaderProvider) InspectMod(reader io.Reader) (domain.ModMetadata, error) {
	var magic [4]byte
	if _, err := io.ReadFull(reader, magic[:]); err != nil {
		return domain.ModMetadata{}, err
	}
	if string(magic[:]) != "TMOD" {
		return domain.ModMetadata{}, fmt.Errorf("invalid tmod header")
	}
	tmodVersion, err := readBinaryString(reader)
	if err != nil {
		return domain.ModMetadata{}, err
	}
	if _, err := io.CopyN(io.Discard, reader, 20+256); err != nil {
		return domain.ModMetadata{}, err
	}
	var dataLength uint32
	if err := binary.Read(reader, binary.LittleEndian, &dataLength); err != nil {
		return domain.ModMetadata{}, err
	}
	name, err := readBinaryString(reader)
	if err != nil {
		return domain.ModMetadata{}, err
	}
	version, err := readBinaryString(reader)
	if err != nil {
		return domain.ModMetadata{}, err
	}
	return domain.ModMetadata{Name: name, Version: version, LoaderVersion: tmodVersion}, nil
}

func readBinaryString(reader io.Reader) (string, error) {
	length, err := read7BitEncodedInt(reader)
	if err != nil {
		return "", err
	}
	if length < 0 || length > 4096 {
		return "", fmt.Errorf("invalid string length %d", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return "", err
	}
	if !utf8.Valid(buf) {
		return "", fmt.Errorf("invalid string encoding")
	}
	return string(buf), nil
}

func read7BitEncodedInt(reader io.Reader) (int, error) {
	var result int
	var shift uint
	var one [1]byte
	for i := 0; i < 5; i++ {
		if _, err := io.ReadFull(reader, one[:]); err != nil {
			return 0, err
		}
		result |= int(one[0]&0x7f) << shift
		if one[0]&0x80 == 0 {
			return result, nil
		}
		shift += 7
	}
	return 0, fmt.Errorf("invalid 7-bit encoded integer")
}
