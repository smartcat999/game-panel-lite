package mod

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const maxDSTModInfoBytes = 2 * 1024 * 1024

type DSTConfigChoice struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Value       any    `json:"value"`
}

type DSTConfigOption struct {
	Name        string            `json:"name"`
	Label       string            `json:"label"`
	Description string            `json:"description,omitempty"`
	Default     any               `json:"default"`
	Choices     []DSTConfigChoice `json:"choices"`
	Section     bool              `json:"section,omitempty"`
}

// ReadDSTConfigOptions reads the declarative configuration tables from a DST
// modinfo.lua without executing third-party Lua code.
func ReadDSTConfigOptions(path, locale string) ([]DSTConfigOption, error) {
	for _, candidate := range localizedDSTModInfoCandidates(path, locale) {
		options, err := readDSTConfigOptionsFile(candidate, locale)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if len(options) > 0 {
			return options, nil
		}
	}
	return readDSTConfigOptionsFile(path, locale)
}

func readDSTConfigOptionsFile(path, locale string) ([]DSTConfigOption, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("invalid modinfo.lua")
	}
	if info.Size() > maxDSTModInfoBytes {
		return nil, fmt.Errorf("modinfo.lua is too large")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseDSTConfigOptions(string(content), locale)
}

func localizedDSTModInfoCandidates(path, locale string) []string {
	code := strings.ToLower(strings.TrimSpace(locale))
	if separator := strings.IndexAny(code, "-_"); separator >= 0 {
		code = code[:separator]
	}
	if code == "" || code == "en" {
		return nil
	}
	for _, value := range code {
		if value < 'a' || value > 'z' {
			return nil
		}
	}
	dir := filepath.Dir(path)
	if code == "zh" {
		return []string{filepath.Join(dir, "modinfo_chs.lua"), filepath.Join(dir, "modinfo_zh.lua")}
	}
	return []string{filepath.Join(dir, "modinfo_"+code+".lua")}
}

var dstTableAssignment = regexp.MustCompile(`(?m)(?:^|[\r\n])[ \t]*(?:local[ \t]+)?(descs|options|configs|vars|configuration_options)[ \t]*=[ \t]*\{`)
var dstConfigExpressionAssignment = regexp.MustCompile(`(?m)(?:^|[\r\n])[ \t]*configuration_options[ \t]*=[ \t]*`)
var dstLocaleBooleanAssignment = regexp.MustCompile(`(?m)(?:^|[\r\n])[ \t]*local[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*=[ \t]*(locale[ \t]*==[^\r\n]+)`)

func ParseDSTConfigOptions(source, locale string) ([]DSTConfigOption, error) {
	source = selectDSTLocale(source, locale)
	wantedLocale := strings.ToLower(strings.TrimSpace(locale))
	if separator := strings.IndexAny(wantedLocale, "-_"); separator >= 0 {
		wantedLocale = wantedLocale[:separator]
	}
	env := map[string]luaValue{"L": {scalar: wantedLocale == "zh"}}
	for _, match := range dstLocaleBooleanAssignment.FindAllStringSubmatchIndex(source, -1) {
		name := source[match[2]:match[3]]
		condition := source[match[4]:match[5]]
		env[name] = luaValue{scalar: localeConditionMatches(condition, wantedLocale)}
	}
	matches := dstTableAssignment.FindAllStringSubmatchIndex(source, -1)
	for _, match := range matches {
		name := source[match[2]:match[3]]
		open := strings.LastIndex(source[match[0]:match[1]], "{") + match[0]
		parser := newLuaTableParser(source[open:], env)
		value, err := parser.parseTable()
		if err != nil {
			if name == "configuration_options" {
				return nil, fmt.Errorf("parse configuration_options: %w", err)
			}
			continue
		}
		env[name] = value
	}
	if _, ok := env["configuration_options"]; !ok {
		if match := dstConfigExpressionAssignment.FindStringIndex(source); match != nil {
			parser := newLuaTableParser(source[match[1]:], env)
			value, err := parser.parseValue()
			if err != nil {
				return nil, fmt.Errorf("parse configuration_options: %w", err)
			}
			env["configuration_options"] = value
		}
	}
	root, ok := env["configuration_options"].table()
	if !ok {
		return []DSTConfigOption{}, nil
	}
	result := make([]DSTConfigOption, 0, len(root.items))
	for _, item := range root.items {
		table, ok := item.table()
		if !ok {
			continue
		}
		name, _ := table.fields["name"].stringValue()
		label, _ := table.fields["label"].stringValue()
		description, _ := table.fields["hover"].stringValue()
		choicesTable, _ := table.fields["options"].table()
		choices := make([]DSTConfigChoice, 0)
		if choicesTable != nil {
			for _, rawChoice := range choicesTable.items {
				choice, ok := rawChoice.table()
				if !ok {
					continue
				}
				value, exists := choice.fields["data"]
				if !exists || !value.isScalar() {
					continue
				}
				choiceLabel, _ := choice.fields["description"].stringValue()
				choiceDescription, _ := choice.fields["hover"].stringValue()
				choices = append(choices, DSTConfigChoice{Label: choiceLabel, Description: choiceDescription, Value: value.scalar})
			}
		}
		defaultValue := table.fields["default"]
		if !defaultValue.isScalar() || len(choices) == 0 {
			continue
		}
		section := len(choices) == 1 && choices[0].Label == "" && description == ""
		if section && label != "" {
			result = append(result, DSTConfigOption{Name: fmt.Sprintf("__section_%d", len(result)), Label: label, Default: defaultValue.scalar, Choices: choices, Section: true})
			continue
		}
		if name == "" {
			continue
		}
		if label == "" {
			label = name
		}
		result = append(result, DSTConfigOption{Name: name, Label: label, Description: description, Default: defaultValue.scalar, Choices: choices, Section: section})
	}
	return result, nil
}

func selectDSTLocale(source, locale string) string {
	wanted := strings.ToLower(strings.TrimSpace(locale))
	if separator := strings.IndexAny(wanted, "-_"); separator >= 0 {
		wanted = wanted[:separator]
	}
	lines := strings.Split(source, "\n")
	selected := make([]string, 0, len(lines))
	inLocaleBlock := false
	keepBlock := false
	matchedBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inLocaleBlock && strings.HasPrefix(trimmed, "if locale") && strings.HasSuffix(trimmed, "then") {
			inLocaleBlock = true
			keepBlock = localeConditionMatches(trimmed, wanted)
			matchedBlock = keepBlock
			continue
		}
		if inLocaleBlock && strings.HasPrefix(trimmed, "elseif locale") && strings.HasSuffix(trimmed, "then") {
			keepBlock = !matchedBlock && localeConditionMatches(trimmed, wanted)
			matchedBlock = matchedBlock || keepBlock
			continue
		}
		if inLocaleBlock && trimmed == "else" {
			keepBlock = !matchedBlock
			matchedBlock = true
			continue
		}
		if inLocaleBlock && trimmed == "end" {
			inLocaleBlock = false
			keepBlock = false
			matchedBlock = false
			continue
		}
		if !inLocaleBlock || keepBlock {
			selected = append(selected, line)
		}
	}
	return strings.Join(selected, "\n")
}

func localeConditionMatches(condition, wanted string) bool {
	if wanted == "" || wanted == "en" {
		return false
	}
	quoted := `"` + wanted + `"`
	if strings.Contains(condition, quoted) {
		return true
	}
	return wanted == "zh" && (strings.Contains(condition, `"zht"`) || strings.Contains(condition, `"zhr"`))
}

type luaValue struct {
	scalar any
	ref    string
	tab    *luaTable
}

type luaTable struct {
	items  []luaValue
	fields map[string]luaValue
}

func (v luaValue) table() (*luaTable, bool) { return v.tab, v.tab != nil }
func (v luaValue) stringValue() (string, bool) {
	value, ok := v.scalar.(string)
	return value, ok
}
func (v luaValue) isScalar() bool { return v.tab == nil && v.ref == "" && v.scalar != nil }

type luaTokenKind int

const (
	luaEOF luaTokenKind = iota
	luaIdent
	luaString
	luaNumber
	luaSymbol
)

type luaToken struct {
	kind luaTokenKind
	text string
}

type luaTableParser struct {
	tokens []luaToken
	index  int
	env    map[string]luaValue
}

func newLuaTableParser(source string, env map[string]luaValue) *luaTableParser {
	return &luaTableParser{tokens: lexLuaTable(source), env: env}
}

func (p *luaTableParser) parseTable() (luaValue, error) {
	if !p.accept("{") {
		return luaValue{}, fmt.Errorf("expected table")
	}
	table := &luaTable{fields: map[string]luaValue{}}
	for p.peek().kind != luaEOF && p.peek().text != "}" {
		if p.accept(",") || p.accept(";") {
			continue
		}
		if p.peek().kind == luaIdent && p.peekNext().text == "=" {
			key := p.take().text
			p.take()
			value, err := p.parseValue()
			if err != nil {
				return luaValue{}, err
			}
			table.fields[key] = value
		} else {
			value, err := p.parseValue()
			if err != nil {
				return luaValue{}, err
			}
			table.items = append(table.items, value)
		}
		p.accept(",")
		p.accept(";")
	}
	if !p.accept("}") {
		return luaValue{}, fmt.Errorf("unterminated table")
	}
	return luaValue{tab: table}, nil
}

func (p *luaTableParser) parseValue() (luaValue, error) {
	value, err := p.parsePrimary()
	if err != nil {
		return luaValue{}, err
	}
	if p.peek().text != "and" {
		return value, nil
	}
	p.take()
	whenTrue, err := p.parseValue()
	if err != nil {
		return luaValue{}, err
	}
	if !p.accept("or") {
		return luaValue{}, fmt.Errorf("expected or in conditional expression")
	}
	whenFalse, err := p.parseValue()
	if err != nil {
		return luaValue{}, err
	}
	if truthyLuaValue(value) {
		return whenTrue, nil
	}
	return whenFalse, nil
}

func (p *luaTableParser) parsePrimary() (luaValue, error) {
	token := p.take()
	switch token.kind {
	case luaString:
		return luaValue{scalar: token.text}, nil
	case luaNumber:
		value, err := strconv.ParseFloat(token.text, 64)
		if err != nil {
			return luaValue{}, err
		}
		return luaValue{scalar: value}, nil
	case luaIdent:
		if token.text == "true" {
			return luaValue{scalar: true}, nil
		}
		if token.text == "false" {
			return luaValue{scalar: false}, nil
		}
		path := token.text
		for p.accept(".") {
			next := p.take()
			if next.kind != luaIdent {
				return luaValue{}, fmt.Errorf("invalid reference")
			}
			path += "." + next.text
		}
		resolved := p.resolve(path)
		for p.accept("[") {
			key, err := p.parseValue()
			if err != nil || !p.accept("]") {
				return luaValue{}, fmt.Errorf("invalid table index")
			}
			resolved = indexLuaValue(resolved, key, path)
		}
		if p.accept("(") {
			args := make([]luaValue, 0, 3)
			for p.peek().kind != luaEOF && p.peek().text != ")" {
				arg, err := p.parseValue()
				if err != nil {
					return luaValue{}, err
				}
				args = append(args, arg)
				if !p.accept(",") {
					break
				}
			}
			if !p.accept(")") {
				return luaValue{}, fmt.Errorf("unterminated function call")
			}
			return luaConfigCall(path, args), nil
		}
		return resolved, nil
	case luaSymbol:
		if token.text == "{" {
			p.index--
			return p.parseTable()
		}
		if token.text == "(" {
			value, err := p.parseValue()
			if err != nil || !p.accept(")") {
				return luaValue{}, fmt.Errorf("unterminated parenthesized value")
			}
			return value, nil
		}
	}
	return luaValue{}, fmt.Errorf("unsupported value %q", token.text)
}

func indexLuaValue(value, key luaValue, fallback string) luaValue {
	table, ok := value.table()
	if !ok {
		return luaValue{ref: fallback}
	}
	if name, ok := key.stringValue(); ok {
		if item, exists := table.fields[name]; exists {
			return item
		}
	}
	if number, ok := key.scalar.(float64); ok && number >= 1 && int(number) <= len(table.items) {
		return table.items[int(number)-1]
	}
	return luaValue{ref: fallback}
}

func truthyLuaValue(value luaValue) bool {
	if boolean, ok := value.scalar.(bool); ok {
		return boolean
	}
	return value.scalar != nil || value.tab != nil
}

func luaConfigCall(path string, args []luaValue) luaValue {
	if path == "option" {
		table := &luaTable{fields: map[string]luaValue{}}
		if len(args) > 0 {
			table.fields["description"] = args[0]
		}
		if len(args) > 1 {
			table.fields["data"] = args[1]
		}
		if len(args) > 2 {
			table.fields["hover"] = args[2]
		}
		return luaValue{tab: table}
	}
	if strings.HasSuffix(path, ".largeLabel") && len(args) > 0 {
		choice := luaValue{tab: &luaTable{fields: map[string]luaValue{
			"description": {scalar: ""},
			"data":        {scalar: false},
		}}}
		return luaValue{tab: &luaTable{fields: map[string]luaValue{
			"name":    {scalar: ""},
			"label":   args[0],
			"default": {scalar: false},
			"options": {tab: &luaTable{items: []luaValue{choice}, fields: map[string]luaValue{}}},
		}}}
	}
	return luaValue{ref: path}
}

func (p *luaTableParser) resolve(path string) luaValue {
	parts := strings.Split(path, ".")
	value, ok := p.env[parts[0]]
	if !ok {
		return luaValue{ref: path}
	}
	for _, part := range parts[1:] {
		table, ok := value.table()
		if !ok {
			return luaValue{ref: path}
		}
		value, ok = table.fields[part]
		if !ok {
			return luaValue{ref: path}
		}
	}
	return value
}

func (p *luaTableParser) peek() luaToken {
	if p.index >= len(p.tokens) {
		return luaToken{kind: luaEOF}
	}
	return p.tokens[p.index]
}

func (p *luaTableParser) peekNext() luaToken {
	if p.index+1 >= len(p.tokens) {
		return luaToken{kind: luaEOF}
	}
	return p.tokens[p.index+1]
}

func (p *luaTableParser) take() luaToken {
	token := p.peek()
	if p.index < len(p.tokens) {
		p.index++
	}
	return token
}

func (p *luaTableParser) accept(text string) bool {
	if p.peek().text != text {
		return false
	}
	p.index++
	return true
}

func lexLuaTable(source string) []luaToken {
	tokens := make([]luaToken, 0, len(source)/4)
	for i := 0; i < len(source); {
		r := rune(source[i])
		if unicode.IsSpace(r) {
			i++
			continue
		}
		if strings.HasPrefix(source[i:], "--[[") {
			if end := strings.Index(source[i+4:], "]]"); end >= 0 {
				i += end + 6
				continue
			}
			break
		}
		if strings.HasPrefix(source[i:], "--") {
			if end := strings.IndexByte(source[i:], '\n'); end >= 0 {
				i += end + 1
				continue
			}
			break
		}
		if source[i] == '\'' || source[i] == '"' {
			quote := source[i]
			i++
			var value strings.Builder
			for i < len(source) && source[i] != quote {
				if source[i] == '\\' && i+1 < len(source) {
					decoded, _, _, err := strconv.UnquoteChar(source[i:i+2], quote)
					if err == nil {
						value.WriteRune(decoded)
						i += 2
						continue
					}
				}
				value.WriteByte(source[i])
				i++
			}
			if i < len(source) {
				i++
			}
			tokens = append(tokens, luaToken{kind: luaString, text: value.String()})
			continue
		}
		if strings.HasPrefix(source[i:], "[[") {
			end := strings.Index(source[i+2:], "]]")
			if end < 0 {
				break
			}
			tokens = append(tokens, luaToken{kind: luaString, text: source[i+2 : i+2+end]})
			i += end + 4
			continue
		}
		if isLuaIdentStart(source[i]) {
			start := i
			for i < len(source) && isLuaIdentPart(source[i]) {
				i++
			}
			tokens = append(tokens, luaToken{kind: luaIdent, text: source[start:i]})
			continue
		}
		if source[i] == '-' || source[i] >= '0' && source[i] <= '9' {
			start := i
			i++
			for i < len(source) && ((source[i] >= '0' && source[i] <= '9') || source[i] == '.') {
				i++
			}
			tokens = append(tokens, luaToken{kind: luaNumber, text: source[start:i]})
			continue
		}
		tokens = append(tokens, luaToken{kind: luaSymbol, text: source[i : i+1]})
		i++
	}
	return append(tokens, luaToken{kind: luaEOF})
}

func isLuaIdentStart(value byte) bool {
	return value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}
func isLuaIdentPart(value byte) bool { return isLuaIdentStart(value) || value >= '0' && value <= '9' }
