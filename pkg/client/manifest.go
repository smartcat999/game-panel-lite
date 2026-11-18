package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// ParseManifestFile reads and parses a YAML file from disk (supporting multi-document YAML).
func ParseManifestFile(filePath string) ([]metav1.Object, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read manifest file %q: %w", filePath, err)
	}
	return ParseManifestBytes(data)
}

// ParseManifestBytes parses raw YAML bytes containing one or more YAML documents separated by ---.
func ParseManifestBytes(data []byte) ([]metav1.Object, error) {
	return ParseManifests(bytes.NewReader(data))
}

// ParseManifests parses a stream of YAML documents into concrete metav1.Object instances.
func ParseManifests(r io.Reader) ([]metav1.Object, error) {
	dec := yaml.NewDecoder(r)
	var objects []metav1.Object
	for {
		var rawNode yaml.Node
		if err := dec.Decode(&rawNode); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("failed to parse YAML manifest: %w", err)
		}
		if rawNode.Kind == 0 || (rawNode.Kind == yaml.DocumentNode && len(rawNode.Content) == 0) {
			continue
		}
		obj, err := decodeSingleYAMLManifestNode(&rawNode)
		if err != nil {
			return nil, err
		}
		if obj != nil {
			objects = append(objects, obj)
		}
	}
	if len(objects) == 0 {
		return nil, errors.New("no valid Kubernetes/GamePanel objects found in manifest")
	}
	return objects, nil
}

func decodeSingleYAMLManifestNode(rawNode *yaml.Node) (metav1.Object, error) {
	var header struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	}
	if err := rawNode.Decode(&header); err != nil {
		return nil, fmt.Errorf("failed to decode object header: %w", err)
	}
	if header.Kind == "" {
		return nil, nil
	}
	info, err := ResolveGVK(header.APIVersion, header.Kind)
	if err != nil {
		info, err = ResolveKind(header.Kind)
		if err != nil {
			return nil, err
		}
	}
	obj := info.NewObject()
	var raw any
	if err := rawNode.Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode YAML node: %w", err)
	}
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	if err := json.Unmarshal(jsonBytes, obj); err != nil {
		return nil, fmt.Errorf("failed to decode %s manifest: %w", header.Kind, err)
	}
	obj.DefaultMeta()
	if strings.TrimSpace(obj.GetObjectMeta().Name) == "" {
		return nil, fmt.Errorf("invalid %s manifest: metadata.name is required", header.Kind)
	}
	return obj, nil
}
