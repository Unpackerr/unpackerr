package unpackerr

import (
	"reflect"
	"strings"
)

func envPrefixForSection(section ConfigSection) string {
	switch section {
	case SectionFolders:
		return "FOLDER_"
	case SectionWebhooks:
		return "WEBHOOK_"
	case SectionCmdhooks:
		return "CMDHOOK_"
	default:
		return strings.ToUpper(string(section)) + "_"
	}
}

func (u *Unpackerr) stripEnvFromStarr[T any, P starrApp[T]](
	section ConfigSection, items InstanceMap[T],
) InstanceMap[T] {
	if items == nil {
		return nil
	}

	u.zeroEnvOwnedFields(section, items)

	for key, item := range items {
		if item == nil {
			delete(items, key)
			continue
		}

		app := asStarr[T, P](item)
		if strings.TrimSpace(app.conf().URL) == "" {
			delete(items, key)
		}
	}

	if len(items) == 0 {
		return nil
	}

	return items
}

func (u *Unpackerr) stripEnvFromFolders(items InstanceMap[FolderConfig]) InstanceMap[FolderConfig] {
	if items == nil {
		return nil
	}

	u.zeroEnvOwnedFields(SectionFolders, items)

	for key, item := range items {
		if item == nil || strings.TrimSpace(item.Path) == "" {
			delete(items, key)
		}
	}

	if len(items) == 0 {
		return nil
	}

	return items
}

func (u *Unpackerr) stripEnvFromHooks(
	section ConfigSection, items InstanceMap[WebhookConfig], cmd bool,
) InstanceMap[WebhookConfig] {
	if items == nil {
		return nil
	}

	u.zeroEnvOwnedFields(section, items)

	for key, item := range items {
		if item == nil {
			delete(items, key)
			continue
		}

		if cmd && strings.TrimSpace(item.Command) == "" {
			delete(items, key)
			continue
		}

		if !cmd && strings.TrimSpace(item.URL) == "" {
			delete(items, key)
		}
	}

	if len(items) == 0 {
		return nil
	}

	return items
}

func (u *Unpackerr) zeroEnvOwnedFields[T any](section ConfigSection, items InstanceMap[T]) {
	if u == nil || len(u.envUsed) == 0 || len(items) == 0 {
		return
	}

	pfx := envPrefixForSection(section)
	fields := tomlEnvFieldNames(reflect.TypeFor[T]())

	for suffix := range u.envUsed {
		key, field, ok := peelInstanceEnv(suffix, pfx, fields)
		if !ok {
			continue
		}

		item, exists := items[key]
		if !exists || item == nil {
			continue
		}

		zeroTOMLField(item, field)
	}
}

// keepPutInstanceFields copies PUT fields back onto slugs that ParseENV replaced,
// then writes env-owned fields from the overlay. Env-only slugs stay as parsed.
func (u *Unpackerr) keepPutInstanceFields(before, after *Config) {
	if before == nil || after == nil {
		return
	}

	keepPutMap(u, SectionSonarr, before.Sonarr, after.Sonarr)
	keepPutMap(u, SectionRadarr, before.Radarr, after.Radarr)
	keepPutMap(u, SectionLidarr, before.Lidarr, after.Lidarr)
	keepPutMap(u, SectionReadarr, before.Readarr, after.Readarr)
	keepPutMap(u, SectionFolders, before.Folders, after.Folders)
	keepPutMap(u, SectionWebhooks, before.Webhook, after.Webhook)
	keepPutMap(u, SectionCmdhooks, before.Cmdhook, after.Cmdhook)
}

func keepPutMap[T any](unpackerr *Unpackerr, section ConfigSection, before, after InstanceMap[T]) {
	if len(before) == 0 || after == nil {
		return
	}

	for key, old := range before {
		if old == nil {
			continue
		}

		neu := after[key]
		if neu == nil {
			after[key] = old
			continue
		}

		merged := *old
		unpackerr.copyEnvOwnedFields(section, key, neu, &merged)
		after[key] = &merged
	}
}

func (u *Unpackerr) copyEnvOwnedFields[T any](section ConfigSection, key string, src, dst *T) {
	if u == nil || src == nil || dst == nil || len(u.envUsed) == 0 {
		return
	}

	pfx := envPrefixForSection(section)
	fields := tomlEnvFieldNames(reflect.TypeFor[T]())

	for suffix := range u.envUsed {
		envKey, field, ok := peelInstanceEnv(suffix, pfx, fields)
		if !ok || envKey != key {
			continue
		}

		copyTOMLField(src, dst, field)
	}
}

func copyTOMLField(src, dst any, envField string) {
	copyNamedTOMLField(reflect.ValueOf(src), reflect.ValueOf(dst), envField)
}

func copyNamedTOMLField(src, dst reflect.Value, envField string) bool {
	src = derefValue(src)

	dst = derefValue(dst)
	if src.Kind() != reflect.Struct || dst.Kind() != reflect.Struct {
		return false
	}

	typ := dst.Type()

	for idx := range typ.NumField() {
		field := typ.Field(idx)
		if !field.IsExported() {
			continue
		}

		tag, _, _ := strings.Cut(field.Tag.Get("toml"), ",")
		if tag == "-" {
			continue
		}

		member := dst.Field(idx)

		from := src.Field(idx)
		if field.Anonymous && tag == "" {
			if copyNamedTOMLField(from, member, envField) {
				return true
			}

			continue
		}

		name := strings.ToUpper(strings.ReplaceAll(tag, "-", "_"))
		if name != envField || !member.CanSet() {
			continue
		}

		member.Set(from)

		return true
	}

	return false
}

func (u *Unpackerr) redactEnvSecrets(cfg *Config) {
	if u == nil || cfg == nil || len(u.envUsed) == 0 {
		return
	}

	u.redactMapSecrets(SectionSonarr, cfg.Sonarr)
	u.redactMapSecrets(SectionRadarr, cfg.Radarr)
	u.redactMapSecrets(SectionLidarr, cfg.Lidarr)
	u.redactMapSecrets(SectionReadarr, cfg.Readarr)
	u.redactMapSecrets(SectionFolders, cfg.Folders)
	u.redactMapSecrets(SectionWebhooks, cfg.Webhook)
	u.redactMapSecrets(SectionCmdhooks, cfg.Cmdhook)
}

func (u *Unpackerr) redactMapSecrets[T any](section ConfigSection, items InstanceMap[T]) {
	if len(items) == 0 {
		return
	}

	pfx := envPrefixForSection(section)
	fields := tomlEnvFieldNames(reflect.TypeFor[T]())

	for suffix := range u.envUsed {
		if !envValueSecret(suffix) {
			continue
		}

		key, field, ok := peelInstanceEnv(suffix, pfx, fields)
		if !ok {
			continue
		}

		item, exists := items[key]
		if !exists || item == nil {
			continue
		}

		zeroTOMLField(item, field)
	}
}

func peelInstanceEnv(suffix, prefix string, fields map[string]struct{}) (string, string, bool) {
	rest, found := strings.CutPrefix(suffix, prefix)
	if !found || rest == "" {
		return "", "", false
	}

	parts := strings.Split(rest, "_")
	for take := len(parts) - 1; take >= 1; take-- {
		name := strings.Join(parts[take:], "_")
		if _, exists := fields[name]; exists {
			return strings.Join(parts[:take], "_"), name, true
		}

		if isEnvIndex(parts[len(parts)-1]) && take < len(parts)-1 {
			name = strings.Join(parts[take:len(parts)-1], "_")
			if _, exists := fields[name]; exists {
				return strings.Join(parts[:take], "_"), name, true
			}
		}
	}

	return "", "", false
}

func isEnvIndex(token string) bool {
	if token == "" {
		return false
	}

	for _, char := range token {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}

func tomlEnvFieldNames(typ reflect.Type) map[string]struct{} {
	out := make(map[string]struct{})
	collectTOMLEnvFields(typ, out)

	return out
}

func collectTOMLEnvFields(typ reflect.Type, out map[string]struct{}) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return
	}

	for field := range typ.Fields() {
		if !field.IsExported() {
			continue
		}

		tag, _, _ := strings.Cut(field.Tag.Get("toml"), ",")
		if tag == "-" {
			continue
		}

		if field.Anonymous && tag == "" {
			collectTOMLEnvFields(field.Type, out)
			continue
		}

		if tag == "" {
			continue
		}

		out[strings.ToUpper(strings.ReplaceAll(tag, "-", "_"))] = struct{}{}
	}
}

func zeroTOMLField(ptr any, envField string) {
	val := reflect.ValueOf(ptr)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return
		}

		val = val.Elem()
	}

	zeroNamedTOMLField(val, envField)
}

func zeroNamedTOMLField(val reflect.Value, envField string) bool {
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return false
		}

		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return false
	}

	typ := val.Type()

	for idx := range typ.NumField() {
		field := typ.Field(idx)
		if !field.IsExported() {
			continue
		}

		tag, _, _ := strings.Cut(field.Tag.Get("toml"), ",")
		if tag == "-" {
			continue
		}

		member := val.Field(idx)
		if field.Anonymous && tag == "" {
			if zeroNamedTOMLField(member, envField) {
				return true
			}

			continue
		}

		name := strings.ToUpper(strings.ReplaceAll(tag, "-", "_"))
		if name != envField || !member.CanSet() {
			continue
		}

		member.Set(reflect.Zero(member.Type()))

		return true
	}

	return false
}
