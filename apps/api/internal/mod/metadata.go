package mod

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"unicode/utf8"
)

type Metadata struct {
	Name              string
	Version           string
	TModLoaderVersion string
}

func Inspect(path string) (Metadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return Metadata{}, err
	}
	defer file.Close()
	return ReadMetadata(file)
}

func ReadMetadata(reader io.Reader) (Metadata, error) {
	var magic [4]byte
	if _, err := io.ReadFull(reader, magic[:]); err != nil {
		return Metadata{}, err
	}
	if string(magic[:]) != "TMOD" {
		return Metadata{}, fmt.Errorf("invalid tmod header")
	}
	tmodVersion, err := readBinaryString(reader)
	if err != nil {
		return Metadata{}, err
	}
	if _, err := io.CopyN(io.Discard, reader, 20+256); err != nil {
		return Metadata{}, err
	}
	var dataLength uint32
	if err := binary.Read(reader, binary.LittleEndian, &dataLength); err != nil {
		return Metadata{}, err
	}
	name, err := readBinaryString(reader)
	if err != nil {
		return Metadata{}, err
	}
	version, err := readBinaryString(reader)
	if err != nil {
		return Metadata{}, err
	}
	return Metadata{Name: name, Version: version, TModLoaderVersion: tmodVersion}, nil
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
