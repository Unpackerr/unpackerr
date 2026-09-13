package unpackerr

import (
	"reflect"
	"strconv"
	"strings"

	"golift.io/cnfg"
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
	return dropEmptyInstances(u.zeroEnvOwnedFields(section, items))
}

func (u *Unpackerr) stripEnvFromFolders(items InstanceMap[FolderConfig]) InstanceMap[FolderConfig] {
	return dropEmptyInstances(u.zeroEnvOwnedFields(SectionFolders, items))
}

func (u *Unpackerr) stripEnvFromHooks(
	section ConfigSection, items InstanceMap[WebhookConfig],
) InstanceMap[WebhookConfig] {
	return dropEmptyInstances(u.zeroEnvOwnedFields(section, items))
}

func dropEmptyInstances[T any](items InstanceMap[T]) InstanceMap[T] {
	if items == nil {
		return nil
	}

	for key, item := range items {
		if item == nil || !persistableConfig(item) {
			delete(items, key)
		}
	}

	if len(items) == 0 {
		return nil
	}

	return items
}

// persistableConfig is true when a stripped instance still has file-owned
// fields. Name-only stubs stay out of the file; env fills live identity.
func persistableConfig(item any) bool {
	return persistableValue(reflect.ValueOf(item))
}

func persistableValue(val reflect.Value) bool {
	val = derefValue(val)
	if !val.IsValid() {
		return false
	}

	if val.Kind() != reflect.Struct {
		return !val.IsZero()
	}

	typ := val.Type()

	for idx := range typ.NumField() {
		field := typ.Field(idx)
		if !field.IsExported() {
			continue
		}

		tag, _, _ := strings.Cut(field.Tag.Get(cnfg.ENVTag), ",")
		if tag == "-" {
			continue
		}

		member := val.Field(idx)
		if field.Anonymous && tag == "" {
			if persistableValue(member) {
				return true
			}

			continue
		}

		name := strings.ToUpper(strings.ReplaceAll(tag, "-", "_"))
		if name == "NAME" {
			continue
		}

		if persistableValue(member) {
			return true
		}
	}

	return false
}

func (u *Unpackerr) zeroEnvOwnedFields[T any](section ConfigSection, items InstanceMap[T]) InstanceMap[T] {
	if items == nil {
		return nil
	}

	if u == nil || len(u.envUsed) == 0 || len(items) == 0 {
		return items
	}

	pfx := envPrefixForSection(section)
	typ := reflect.TypeFor[T]()

	for suffix := range u.envUsed {
		key, field, indexes, ok := peelInstanceEnv(suffix, pfx, typ)
		if !ok {
			continue
		}

		item, exists := items[key]
		if !exists || item == nil {
			continue
		}

		zeroTOMLField(item, field, indexes)
	}

	return items
}

// keepPutInstanceFields copies file/PUT fields back onto slugs that ParseENV replaced,
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
	typ := reflect.TypeFor[T]()

	for suffix := range u.envUsed {
		envKey, field, indexes, ok := peelInstanceEnv(suffix, pfx, typ)
		if !ok || envKey != key {
			continue
		}

		copyTOMLField(src, dst, field, indexes)
	}
}

func copyTOMLField(src, dst any, envField string, indexes []int) {
	copyNamedTOMLField(reflect.ValueOf(src), reflect.ValueOf(dst), envField, indexes)
}

func copyNamedTOMLField(src, dst reflect.Value, envField string, indexes []int) bool {
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

		tag, _, _ := strings.Cut(field.Tag.Get(cnfg.ENVTag), ",")
		if tag == "-" {
			continue
		}

		member := dst.Field(idx)

		from := src.Field(idx)
		if field.Anonymous && tag == "" {
			if copyNamedTOMLField(from, member, envField, indexes) {
				return true
			}

			continue
		}

		name := strings.ToUpper(strings.ReplaceAll(tag, "-", "_"))
		if name != envField {
			continue
		}

		writeEnvMember(from, member, indexes, false)

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
	typ := reflect.TypeFor[T]()

	for suffix := range u.envUsed {
		if !envValueSecret(suffix) {
			continue
		}

		key, field, indexes, ok := peelInstanceEnv(suffix, pfx, typ)
		if !ok {
			continue
		}

		item, exists := items[key]
		if !exists || item == nil {
			continue
		}

		zeroTOMLField(item, field, indexes)
	}
}

func peelInstanceEnv(suffix, prefix string, typ reflect.Type) (string, string, []int, bool) {
	rest, found := strings.CutPrefix(suffix, prefix)
	if !found || rest == "" {
		return "", "", nil, false
	}

	key, field, ok := cnfg.PeelMapKey(rest, typ, cnfg.ENVTag, false)
	if !ok || field == "" {
		return "", "", nil, false
	}

	return key, field, envFieldIndexes(envFieldExtra(rest, key, field)), true
}

func envFieldExtra(rest, key, field string) string {
	head := strings.Join([]string{key, field}, cnfg.LevelSeparator)

	extra, found := strings.CutPrefix(rest, head)
	if !found {
		return ""
	}

	return strings.TrimPrefix(extra, cnfg.LevelSeparator)
}

func envFieldIndexes(extra string) []int {
	if extra == "" {
		return nil
	}

	var out []int

	for tok := range strings.SplitSeq(extra, cnfg.LevelSeparator) {
		if tok == "" || !envIndexToken(tok) {
			return nil
		}

		n, err := strconv.Atoi(tok)
		if err != nil {
			return nil
		}

		out = append(out, n)
	}

	return out
}

func envIndexToken(tok string) bool {
	for _, r := range tok {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}

func zeroTOMLField(ptr any, envField string, indexes []int) {
	val := reflect.ValueOf(ptr)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return
		}

		val = val.Elem()
	}

	zeroNamedTOMLField(val, envField, indexes)
}

func zeroNamedTOMLField(val reflect.Value, envField string, indexes []int) bool {
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

		tag, _, _ := strings.Cut(field.Tag.Get(cnfg.ENVTag), ",")
		if tag == "-" {
			continue
		}

		member := val.Field(idx)
		if field.Anonymous && tag == "" {
			if zeroNamedTOMLField(member, envField, indexes) {
				return true
			}

			continue
		}

		name := strings.ToUpper(strings.ReplaceAll(tag, "-", "_"))
		if name != envField {
			continue
		}

		writeEnvMember(reflect.Value{}, member, indexes, true)

		return true
	}

	return false
}

func writeEnvMember(from, member reflect.Value, indexes []int, zero bool) {
	if len(indexes) == 0 {
		if !member.CanSet() {
			return
		}

		if zero {
			member.Set(reflect.Zero(member.Type()))
			return
		}

		member.Set(from)

		return
	}

	if zero {
		zeroIndexedValue(member, indexes)
		return
	}

	copyIndexedValue(from, member, indexes)
}

func copyIndexedValue(from, to reflect.Value, indexes []int) {
	dst, found := indexValue(to, indexes, true)
	if !found || !dst.CanSet() {
		return
	}

	src, found := indexValue(from, indexes, false)
	if !found {
		return
	}

	dst.Set(src)
}

func zeroIndexedValue(to reflect.Value, indexes []int) {
	dst, ok := indexValue(to, indexes, false)
	if !ok || !dst.CanSet() {
		return
	}

	dst.Set(reflect.Zero(dst.Type()))
}

func indexValue(val reflect.Value, indexes []int, grow bool) (reflect.Value, bool) {
	for _, idx := range indexes {
		val = derefValue(val)
		if !val.IsValid() || idx < 0 {
			return reflect.Value{}, false
		}

		switch val.Kind() { //nolint:exhaustive
		case reflect.Slice:
			if grow && idx >= val.Len() && val.CanSet() {
				next := reflect.MakeSlice(val.Type(), idx+1, idx+1)
				reflect.Copy(next, val)
				val.Set(next)
			}

			if idx >= val.Len() {
				return reflect.Value{}, false
			}

			val = val.Index(idx)
		case reflect.Array:
			if idx >= val.Len() {
				return reflect.Value{}, false
			}

			val = val.Index(idx)
		default:
			return reflect.Value{}, false
		}
	}

	return val, val.IsValid()
}
