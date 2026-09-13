package unpackerr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"

	"github.com/BurntSushi/toml"
)

// InstanceMap is a slug-keyed instance list. JSON and TOML still accept the
// 0.x array form ([[sonarr]] / [{…}]) and convert rows to keys "0", "1", ….
type InstanceMap[T any] map[string]*T

var (
	errInvalidInstanceJSON = errors.New("instance list must be a JSON object or array")
	errInvalidInstanceTOML = errors.New("instance list must be a TOML table or array of tables")
	errInvalidInstanceSlug = errors.New("instance key must be letters, digits, underscore, or hyphen")
)

func (m *InstanceMap[T]) UnmarshalJSON(raw []byte) error {
	return unmarshalInstances(raw, m)
}

func (m *InstanceMap[T]) UnmarshalTOML(data any) error {
	if data == nil {
		*m = nil
		return nil
	}

	val := reflect.ValueOf(data)
	switch val.Kind() {
	case reflect.Slice:
		return m.unmarshalTOMLSlice(val)
	case reflect.Map:
		return m.unmarshalTOMLMap(val)
	default:
		return fmt.Errorf("%w: %T", errInvalidInstanceTOML, data)
	}
}

func (m *InstanceMap[T]) unmarshalTOMLSlice(val reflect.Value) error {
	out := make(InstanceMap[T], val.Len())

	for idx := range val.Len() {
		dest := new(T)
		if err := decodeTOMLValue(val.Index(idx).Interface(), dest); err != nil {
			return err
		}

		out[strconv.Itoa(idx)] = dest
	}

	*m = out

	return nil
}

func (m *InstanceMap[T]) unmarshalTOMLMap(val reflect.Value) error {
	if val.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("%w: %s", errInvalidInstanceTOML, val.Type())
	}

	out := make(InstanceMap[T], val.Len())

	for _, keyVal := range val.MapKeys() {
		key := keyVal.String()
		if err := validateInstanceSlug(key); err != nil {
			return err
		}

		dest := new(T)
		if err := decodeTOMLValue(val.MapIndex(keyVal).Interface(), dest); err != nil {
			return err
		}

		out[key] = dest
	}

	*m = out

	return nil
}

func unmarshalInstances[T any](raw json.RawMessage, dest *InstanceMap[T]) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*dest = nil
		return nil
	}

	switch trimmed[0] {
	case '[':
		var list []*T
		if err := unmarshalList(raw, &list); err != nil {
			return err
		}

		out := make(InstanceMap[T], len(list))
		for idx, item := range list {
			out[strconv.Itoa(idx)] = item
		}

		*dest = out

		return nil
	case '{':
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(raw, &probe); err != nil {
			return wrapJSONErr(err)
		}

		out := make(InstanceMap[T], len(probe))

		for key, val := range probe {
			if err := validateInstanceSlug(key); err != nil {
				return err
			}

			if bytes.Equal(bytes.TrimSpace(val), []byte("null")) {
				return errNilConfigEntry
			}

			var item T
			if err := unmarshalStrict(val, &item); err != nil {
				return err
			}

			out[key] = &item
		}

		*dest = out

		return nil
	default:
		return fmt.Errorf("%w", errInvalidInstanceJSON)
	}
}

func decodeTOMLValue(src, dest any) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(src); err != nil {
		return fmt.Errorf("encoding instance table: %w", err)
	}

	if err := toml.Unmarshal(buf.Bytes(), dest); err != nil {
		return fmt.Errorf("decoding instance table: %w", err)
	}

	return nil
}

func validateInstanceSlug(name string) error {
	if !validRoleName(name) {
		return fmt.Errorf("%w: %q", errInvalidInstanceSlug, name)
	}

	return nil
}

func emptyIfNilMap[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return map[K]V{}
	}

	return m
}

func instanceKeys[T any](m InstanceMap[T]) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
}

func instanceValues[T any](m InstanceMap[T]) []*T {
	keys := instanceKeys(m)
	out := make([]*T, 0, len(keys))

	for _, key := range keys {
		if item := m[key]; item != nil {
			out = append(out, item)
		}
	}

	return out
}
