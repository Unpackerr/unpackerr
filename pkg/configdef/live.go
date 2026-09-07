package configdef

import (
	"bytes"
	"encoding"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// RenderMode selects example (generate/first-run) vs live (save) TOML output.
type RenderMode int

const (
	// RenderExample writes YAML defaults and commented list templates.
	RenderExample RenderMode = iota
	// RenderLive writes live struct values and comments keys still at YAML default.
	RenderLive
)

// RenderOpts controls TOML rendering.
type RenderOpts struct {
	Mode RenderMode
	// Persist is a list of param names (`listen_addr`) or `section.param`
	// (`webserver.listen_addr`) that are always written live, even at default.
	Persist []string
}

type persistSet map[string]struct{}

func newPersistSet(keys []string) persistSet {
	out := make(persistSet, len(keys))
	for _, key := range keys {
		if key != "" {
			out[key] = struct{}{}
		}
	}

	return out
}

func (p persistSet) has(section section, name string) bool {
	if p == nil {
		return false
	}

	if _, exists := p[name]; exists {
		return true
	}

	_, exists := p[string(section)+"."+name]

	return exists
}

// RenderTOML renders a configuration file from definitions.yml.
// A nil live value always produces example output.
func (c *Config) RenderTOML(live any, opts RenderOpts) string {
	if live == nil || opts.Mode != RenderLive {
		return c.ExampleTOML()
	}

	return c.renderLive(live, newPersistSet(opts.Persist))
}

func (c *Config) renderLive(live any, persist persistSet) string {
	root := derefValue(reflect.ValueOf(live))

	var buf bytes.Buffer

	for _, name := range c.Order {
		header := c.Sections[name]
		if header == nil {
			continue
		}

		if c.Defs[name] != nil {
			buf.WriteString(c.renderDefinedLive(name, root, persist))
			continue
		}

		sectionVal := root
		if !header.NoHeader {
			sectionVal, _ = fieldByTOML(root, string(name))
		}

		if header.Kind == list {
			buf.WriteString(header.renderListLive(name, sectionVal, persist))
			continue
		}

		buf.WriteString(header.makeSectionLive(name, false, sectionVal, persist))
	}

	return buf.String()
}

func (c *Config) renderDefinedLive(name section, root reflect.Value, persist persistSet) string {
	var buf bytes.Buffer

	for _, item := range c.DefOrder[name] {
		def := c.Defs[name][item]
		if def == nil {
			continue
		}

		header := createDefinedSection(def, c.Sections[name], item)
		sectionVal, _ := fieldByTOML(root, string(item))
		buf.WriteString(header.renderListLive(item, sectionVal, persist))
	}

	return buf.String()
}

func (h *Header) renderListLive(name section, live reflect.Value, persist persistSet) string {
	live = derefValue(live)
	if !live.IsValid() || live.Kind() != reflect.Slice || live.Len() == 0 {
		return h.makeSection(name, false, false)
	}

	var buf bytes.Buffer

	for idx := range live.Len() {
		buf.WriteString(h.makeSectionLive(name, true, derefValue(live.Index(idx)), persist))
	}

	return buf.String()
}

func (h *Header) makeSectionLive(name section, showHeader bool, live reflect.Value, persist persistSet) string {
	var buf bytes.Buffer

	if h.Text != "" {
		buf.WriteString(h.Text)
	}

	space := ""

	if !h.NoHeader {
		space = " "
		left, right := "[", "]"

		if h.Kind == list {
			left, right = "[[", "]]"
		}

		comment := ""
		if h.Kind == list && !showHeader {
			comment = "#"
		}

		buf.WriteString(comment + left + string(name) + right + "\n")
	}

	live = derefValue(live)
	h.writeLiveParams(&buf, name, space, live, persist)

	return buf.String()
}

func (h *Header) writeLiveParams(
	buf *bytes.Buffer, name section, space string, live reflect.Value, persist persistSet,
) {
	for _, param := range h.Params {
		writeLiveParam(buf, name, space, live, persist, h.NoHeader, param)
	}

	buf.WriteString("\n")
	buf.WriteString(h.renderNestedLive(name, live))
}

func writeLiveParam(
	buf *bytes.Buffer,
	name section,
	space string,
	live reflect.Value,
	persist persistSet,
	noHeader bool,
	param *Param,
) {
	if param == nil {
		return
	}

	if noHeader && param.Desc != "" {
		buf.WriteString("\n")
	}

	if param.Desc != "" {
		buf.WriteString("## " + strings.ReplaceAll(strings.TrimSpace(param.Desc), "\n", "\n## ") + "\n")
	}

	if param.isNested() {
		return
	}

	field, ok := fieldByTOML(live, param.Name)
	text := param.defaultText()
	atDefault := true

	if ok && field.IsValid() && field.CanInterface() && !isNilish(field) {
		text = formatTOML(param.Name, field.Interface())
		atDefault = valuesEqual(field, param.Default)
	}

	comment := ""
	if atDefault && !persist.has(name, param.Name) {
		comment = "#"
	}

	fmt.Fprintf(buf, "%s%s%s = %s\n", comment, space, param.Name, text)
}

func (h *Header) renderNestedLive(section section, live reflect.Value) string {
	live = derefValue(live)
	if !live.IsValid() {
		return ""
	}

	var buf bytes.Buffer

	for _, param := range h.Params {
		if !param.isNested() {
			continue
		}

		field, ok := fieldByTOML(live, param.Name)
		if !ok {
			continue
		}

		buf.WriteString(renderNestedValue(section, param.Name, param.Kind, derefValue(field)))
	}

	return buf.String()
}

func renderNestedValue(section section, name, kind string, value reflect.Value) string {
	if !value.IsValid() || isNilish(value) {
		return ""
	}

	switch value.Kind() {
	case reflect.Slice, reflect.Map:
		if value.Len() == 0 {
			return ""
		}
	default:
		return ""
	}

	switch kind {
	case tables:
		return renderNestedTables(section, name, value)
	case "map":
		return renderNestedMap(section, name, value)
	default:
		return ""
	}
}

func renderNestedTables(section section, name string, value reflect.Value) string {
	if value.Kind() != reflect.Slice {
		return ""
	}

	var buf bytes.Buffer

	for idx := range value.Len() {
		fmt.Fprintf(&buf, "[[%s.%s]]\n", section, name)
		writeStructFields(&buf, derefValue(value.Index(idx)))
		buf.WriteByte('\n')
	}

	return buf.String()
}

func renderNestedMap(section section, name string, value reflect.Value) string {
	if value.Kind() != reflect.Map {
		return ""
	}

	keys := value.MapKeys()
	slices.SortFunc(keys, func(left, right reflect.Value) int {
		return strings.Compare(left.String(), right.String())
	})

	var buf bytes.Buffer

	for _, key := range keys {
		fmt.Fprintf(&buf, "[%s.%s.%s]\n", section, name, tomlKey(key.String()))
		writeStructFields(&buf, derefValue(value.MapIndex(key)))
		buf.WriteByte('\n')
	}

	return buf.String()
}

func writeStructFields(buf *bytes.Buffer, value reflect.Value) {
	value = derefValue(value)
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return
	}

	typ := value.Type()

	for idx := range typ.NumField() {
		field := typ.Field(idx)
		if !field.IsExported() {
			continue
		}

		tag, _, _ := strings.Cut(field.Tag.Get("toml"), ",")
		if tag == "" || tag == "-" {
			continue
		}

		fmt.Fprintf(buf, " %s = %s\n", tag, formatTOML(tag, value.Field(idx).Interface()))
	}
}

func tomlKey(name string) string {
	if name != "" && isBareTOMLKey(name) {
		return name
	}

	return quoteTOMLString(name)
}

func quoteTOMLString(name string) string {
	var buf strings.Builder

	buf.Grow(len(name) + len(`""`))
	buf.WriteByte('"')

	for _, char := range name {
		switch {
		case char == '"':
			buf.WriteString(`\"`)
		case char == '\\':
			buf.WriteString(`\\`)
		case char == '\n':
			buf.WriteString(`\n`)
		case char == '\t':
			buf.WriteString(`\t`)
		case char == '\r':
			buf.WriteString(`\r`)
		case char < ' ':
			fmt.Fprintf(&buf, `\u%04X`, char)
		default:
			buf.WriteRune(char)
		}
	}

	buf.WriteByte('"')

	return buf.String()
}

func isBareTOMLKey(name string) bool {
	for _, char := range name {
		switch {
		case char >= 'A' && char <= 'Z', char >= 'a' && char <= 'z', char >= '0' && char <= '9', char == '_', char == '-':
		default:
			return false
		}
	}

	return true
}

func (p *Param) defaultText() string {
	return formatTOML(p.Name, p.Default)
}

func formatTOML(name string, val any) string {
	value := reflect.ValueOf(val)
	if isNilish(value) {
		return emptyCollectionTOML(value)
	}

	value = derefValue(value)
	if !value.IsValid() {
		return "''"
	}

	if value.Kind() == reflect.Map && value.Len() == 0 {
		return "{}"
	}

	if (value.Kind() == reflect.Slice || value.Kind() == reflect.Array) && value.Len() == 0 {
		return "[]"
	}

	out := marshalTOML(value)

	return strings.TrimSpace(string(preferPathQuotes(name, out)))
}

func emptyCollectionTOML(value reflect.Value) string {
	if value.IsValid() {
		switch value.Kind() {
		case reflect.Slice, reflect.Array:
			return "[]"
		case reflect.Map:
			return "{}"
		}
	}

	return "''"
}

func marshalTOML(value reflect.Value) []byte {
	if out, ok := namedUint8TOML(value); ok {
		return out
	}

	if !canInlineTOML(value) {
		return []byte("''")
	}

	out, err := toml.Marshal(value.Interface())
	if err != nil {
		return []byte("''")
	}

	return out
}

func canInlineTOML(value reflect.Value) bool {
	value = derefValue(value)
	if !value.IsValid() {
		return true
	}

	if _, ok := reflect.TypeAssert[encoding.TextMarshaler](value); ok {
		return true
	}

	switch value.Kind() {
	case reflect.Struct:
		return false
	case reflect.Map:
		if value.Len() == 0 {
			return true
		}

		iter := value.MapRange()
		if iter.Next() {
			return canInlineTOML(iter.Value())
		}

		return true
	case reflect.Slice, reflect.Array:
		if value.Len() == 0 {
			return true
		}

		return canInlineTOML(value.Index(0))
	default:
		return true
	}
}

// namedUint8TOML converts a named []uint8 (ExtractStatus, etc) to ints so TOML
// keeps numeric event IDs instead of TextMarshaler strings the decoder rejects.
func namedUint8TOML(value reflect.Value) ([]byte, bool) {
	value = derefValue(value)
	if !value.IsValid() || value.Kind() != reflect.Slice {
		return nil, false
	}

	elem := value.Type().Elem()
	if elem.Kind() != reflect.Uint8 || elem.PkgPath() == "" {
		return nil, false
	}

	var buf strings.Builder

	buf.WriteByte('[')

	for idx := range value.Len() {
		if idx > 0 {
			buf.WriteString(", ")
		}

		buf.WriteString(strconv.FormatUint(value.Index(idx).Uint(), 10))
	}

	buf.WriteByte(']')

	return []byte(buf.String()), true
}

func preferPathQuotes(name string, out []byte) []byte {
	if !pathishName(name) {
		return out
	}

	if bytes.ContainsAny(out, `\'`) {
		return out
	}

	return bytes.ReplaceAll(out, []byte{'"'}, []byte{'\''})
}

func pathishName(name string) bool {
	return strings.Contains(name, "path") || strings.HasSuffix(name, "file") || name == "command"
}

func isNilish(val reflect.Value) bool {
	if !val.IsValid() {
		return true
	}

	switch val.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func:
		return val.IsNil()
	default:
		return false
	}
}

func valuesEqual(live reflect.Value, yamlDefault any) bool {
	if !live.IsValid() {
		return yamlZero(yamlDefault)
	}

	live = derefValue(live)
	if !live.IsValid() {
		return yamlZero(yamlDefault)
	}

	if left, ok := durationOf(live.Interface()); ok {
		if right, ok := durationOf(yamlDefault); ok {
			return left == right
		}
	}

	return strings.TrimSpace(formatTOML("", live.Interface())) ==
		strings.TrimSpace(formatTOML("", yamlDefault))
}

func yamlZero(val any) bool {
	if val == nil {
		return true
	}

	value := reflect.ValueOf(val)
	if !value.IsValid() {
		return true
	}

	switch value.Kind() {
	case reflect.Slice, reflect.Map, reflect.String:
		return value.Len() == 0
	case reflect.Bool:
		return !value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return value.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return value.Float() == 0
	default:
		return value.IsZero()
	}
}

func durationOf(val any) (time.Duration, bool) {
	switch typed := val.(type) {
	case time.Duration:
		return typed, true
	case string:
		parsed, err := time.ParseDuration(typed)
		return parsed, err == nil
	}

	if marshaler, ok := val.(encoding.TextMarshaler); ok {
		text, err := marshaler.MarshalText()
		if err != nil {
			return 0, false
		}

		parsed, err := time.ParseDuration(string(text))

		return parsed, err == nil
	}

	value := reflect.ValueOf(val)
	if value.Kind() == reflect.Pointer && !value.IsNil() {
		return durationOf(value.Elem().Interface())
	}

	return 0, false
}

func derefValue(val reflect.Value) reflect.Value {
	for val.IsValid() && (val.Kind() == reflect.Pointer || val.Kind() == reflect.Interface) {
		if val.IsNil() {
			return reflect.Value{}
		}

		val = val.Elem()
	}

	return val
}

func fieldByTOML(val reflect.Value, key string) (reflect.Value, bool) {
	val = derefValue(val)
	if !val.IsValid() || val.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}

	typ := val.Type()

	for idx := range typ.NumField() {
		field := typ.Field(idx)
		if !field.IsExported() && !field.Anonymous {
			continue
		}

		tag, _, _ := strings.Cut(field.Tag.Get("toml"), ",")
		if tag == "-" {
			continue
		}

		member := val.Field(idx)
		if tag == key {
			return member, true
		}

		if field.Anonymous || tag == "" {
			if inner, found := fieldByTOML(member, key); found {
				return inner, true
			}
		}
	}

	return reflect.Value{}, false
}
