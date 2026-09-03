package unpackerr

func cloneConfig(src *Config) *Config {
	if src == nil {
		return nil
	}

	dst := *src
	dst.Passwords = append(StringSlice(nil), src.Passwords...)
	dst.Webserver = cloneWebserver(src.Webserver)
	dst.Lidarr = cloneLidarrList(src.Lidarr)
	dst.Radarr = cloneRadarrList(src.Radarr)
	dst.Whisparr = cloneRadarrList(src.Whisparr)
	dst.Readarr = cloneReadarrList(src.Readarr)
	dst.Sonarr = cloneSonarrList(src.Sonarr)
	dst.Folders = cloneFolderList(src.Folders)
	dst.Webhook = cloneHookList(src.Webhook)
	dst.Cmdhook = cloneHookList(src.Cmdhook)

	return &dst
}

func cloneWebserver(src *WebServer) *WebServer {
	if src == nil {
		return &WebServer{}
	}

	dst := *src
	dst.Upstreams = append(StringSlice(nil), src.Upstreams...)
	dst.APIKeys = cloneAPIKeys(src.APIKeys)
	dst.Roles = cloneRoles(src.Roles)
	dst.router = nil
	dst.server = nil
	dst.keyPerms = nil
	dst.cookies = nil

	return &dst
}

func cloneAPIKeys(src []APIKey) []APIKey {
	if src == nil {
		return nil
	}

	out := make([]APIKey, len(src))
	for idx, key := range src {
		out[idx] = APIKey{
			Name:  key.Name,
			Key:   key.Key,
			Roles: append([]string(nil), key.Roles...),
		}
	}

	return out
}

func cloneRoles(src map[string]Role) map[string]Role {
	if src == nil {
		return nil
	}

	out := make(map[string]Role, len(src))
	for name, role := range src {
		out[name] = Role{Permissions: append([]string(nil), role.Permissions...)}
	}

	return out
}

func cloneStarrConfig(src StarrConfig) StarrConfig {
	dst := src
	dst.Paths = append(StringSlice(nil), src.Paths...)

	return dst
}

func cloneLidarrList(src []*LidarrConfig) []*LidarrConfig {
	if src == nil {
		return nil
	}

	out := make([]*LidarrConfig, len(src))
	for idx, app := range src {
		if app == nil {
			continue
		}

		cloned := *app
		cloned.StarrConfig = cloneStarrConfig(app.StarrConfig)
		cloned.Queue = nil
		cloned.Lidarr = nil
		out[idx] = &cloned
	}

	return out
}

func cloneRadarrList(src []*RadarrConfig) []*RadarrConfig {
	if src == nil {
		return nil
	}

	out := make([]*RadarrConfig, len(src))
	for idx, app := range src {
		if app == nil {
			continue
		}

		cloned := *app
		cloned.StarrConfig = cloneStarrConfig(app.StarrConfig)
		cloned.Queue = nil
		cloned.Radarr = nil
		out[idx] = &cloned
	}

	return out
}

func cloneReadarrList(src []*ReadarrConfig) []*ReadarrConfig {
	if src == nil {
		return nil
	}

	out := make([]*ReadarrConfig, len(src))
	for idx, app := range src {
		if app == nil {
			continue
		}

		cloned := *app
		cloned.StarrConfig = cloneStarrConfig(app.StarrConfig)
		cloned.Queue = nil
		cloned.Readarr = nil
		out[idx] = &cloned
	}

	return out
}

func cloneSonarrList(src []*SonarrConfig) []*SonarrConfig {
	if src == nil {
		return nil
	}

	out := make([]*SonarrConfig, len(src))
	for idx, app := range src {
		if app == nil {
			continue
		}

		cloned := *app
		cloned.StarrConfig = cloneStarrConfig(app.StarrConfig)
		cloned.Queue = nil
		cloned.Sonarr = nil
		out[idx] = &cloned
	}

	return out
}

func cloneFolderList(src []*FolderConfig) []*FolderConfig {
	if src == nil {
		return nil
	}

	out := make([]*FolderConfig, len(src))
	for idx, folder := range src {
		if folder == nil {
			continue
		}

		cloned := *folder
		if folder.DeleteAfter != nil {
			dur := *folder.DeleteAfter
			cloned.DeleteAfter = &dur
		}

		cloned.ExcludePaths = append([]string(nil), folder.ExcludePaths...)
		out[idx] = &cloned
	}

	return out
}

func cloneHookList(src []*WebhookConfig) []*WebhookConfig {
	if src == nil {
		return nil
	}

	out := make([]*WebhookConfig, len(src))
	for idx, hook := range src {
		if hook == nil {
			continue
		}

		cloned := &WebhookConfig{
			Name:      hook.Name,
			URL:       hook.URL,
			Command:   hook.Command,
			CType:     hook.CType,
			TmplPath:  hook.TmplPath,
			TempName:  hook.TempName,
			Timeout:   hook.Timeout,
			Shell:     hook.Shell,
			IgnoreSSL: hook.IgnoreSSL,
			Silent:    hook.Silent,
			Events:    append(ExtractStatuses(nil), hook.Events...),
			Exclude:   append(StringSlice(nil), hook.Exclude...),
			Nickname:  hook.Nickname,
			Token:     hook.Token,
			Channel:   hook.Channel,
		}
		out[idx] = cloned
	}

	return out
}
