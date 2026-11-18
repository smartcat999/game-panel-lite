package runtime

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	restful "github.com/emicklei/go-restful/v3"
	"github.com/invopop/jsonschema"
)

// BuildOpenAPISpec generates an OpenAPI 3.0.3 document from all go-restful WebServices registered in the Container.
func (c *Container) BuildOpenAPISpec() map[string]any {
	paths := make(map[string]map[string]any)
	schemas := make(map[string]any)
	for _, ws := range c.restfulContainer.RegisteredWebServices() {
		for _, rt := range ws.Routes() {
			pathItem, ok := paths[rt.Path]
			if !ok {
				pathItem = make(map[string]any)
				paths[rt.Path] = pathItem
			}
			pathItem[strings.ToLower(rt.Method)] = buildOpenAPIRouteOperation(schemas, rt)
		}
	}
	return map[string]any{
		"openapi":    "3.0.3",
		"info":       map[string]any{"title": "GamePanel Lite Declarative API", "version": "v1"},
		"paths":      paths,
		"components": map[string]any{"schemas": schemas},
	}
}

func buildOpenAPIRouteOperation(schemas map[string]any, rt restful.Route) map[string]any {
	tag, _ := rt.Metadata["tag"].(string)
	op := map[string]any{"summary": rt.Doc}
	if tag != "" {
		op["tags"] = []string{tag}
	}
	if len(rt.ParameterDocs) > 0 {
		op["parameters"] = buildOpenAPIParameters(rt.ParameterDocs)
	}
	if rt.ReadSample != nil {
		schemaRef := registerSchema(schemas, rt.ReadSample)
		op["requestBody"] = map[string]any{
			"required": true,
			"content":  map[string]any{"application/json": map[string]any{"schema": schemaRef}},
		}
	}
	op["responses"] = buildOpenAPIResponses(schemas, rt)
	return op
}

func buildOpenAPIParameters(paramDocs []*restful.Parameter) []map[string]any {
	var params []map[string]any
	for _, p := range paramDocs {
		data := p.Data()
		inKind := "query"
		if data.Kind == restful.PathParameterKind {
			inKind = "path"
		}
		dataType := "string"
		if data.DataType != "" {
			dataType = data.DataType
		}
		paramSchema := map[string]any{"type": dataType}
		if data.DefaultValue != "" {
			if dataType == "integer" {
				if intVal, err := strconv.Atoi(data.DefaultValue); err == nil {
					paramSchema["default"] = intVal
				} else {
					paramSchema["default"] = data.DefaultValue
				}
			} else {
				paramSchema["default"] = data.DefaultValue
			}
		}
		params = append(params, map[string]any{
			"name": data.Name, "in": inKind, "description": data.Description, "required": data.Required, "schema": paramSchema,
		})
	}
	return params
}

func buildOpenAPIResponses(schemas map[string]any, rt restful.Route) map[string]any {
	responses := make(map[string]any)
	if len(rt.ResponseErrors) > 0 {
		for code, meta := range rt.ResponseErrors {
			respObj := map[string]any{"description": meta.Message}
			model := meta.Model
			if model == nil && (code >= 200 && code < 300) {
				model = rt.WriteSample
			}
			if model != nil {
				schemaRef := registerSchema(schemas, model)
				respObj["content"] = map[string]any{"application/json": map[string]any{"schema": schemaRef}}
			}
			responses[strconv.Itoa(code)] = respObj
		}
		return responses
	}
	if rt.WriteSample != nil {
		schemaRef := registerSchema(schemas, rt.WriteSample)
		responses["200"] = map[string]any{
			"description": "OK",
			"content":     map[string]any{"application/json": map[string]any{"schema": schemaRef}},
		}
		return responses
	}
	responses["200"] = map[string]any{"description": "OK"}
	return responses
}

func registerSchema(schemas map[string]any, sample any) map[string]any {
	r := &jsonschema.Reflector{AllowAdditionalProperties: true}
	s := r.Reflect(sample)
	if s == nil {
		return map[string]any{"type": "object"}
	}
	for name, def := range s.Definitions {
		if _, exists := schemas[name]; !exists {
			schemas[name] = schemaToOpenAPIMap(def)
		}
	}
	if s.Ref != "" {
		refName := strings.TrimPrefix(s.Ref, "#/$defs/")
		return map[string]any{"$ref": "#/components/schemas/" + refName}
	}
	return schemaToOpenAPIMap(s)
}

func schemaToOpenAPIMap(s *jsonschema.Schema) map[string]any {
	raw, err := json.Marshal(s)
	if err != nil {
		return map[string]any{"type": "object"}
	}
	raw = bytes.ReplaceAll(raw, []byte("#/$defs/"), []byte("#/components/schemas/"))
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{"type": "object"}
	}
	delete(out, "$schema")
	delete(out, "$id")
	delete(out, "$defs")
	return out
}
