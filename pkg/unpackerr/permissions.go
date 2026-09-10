package unpackerr

import "slices"

// Permission names are area:resource:verb so later verbs (execute, …) stay
// on the same resource. Built-in role admin grants all of them.
const (
	PermReadSystemStats    = "system:stats:read"
	PermReadSystemInfo     = "system:info:read"
	PermReadSystemQueue    = "system:queue:read"
	PermWriteSystemQueue   = "system:queue:write"
	PermReadSystemHistory  = "system:history:read"
	PermWriteSystemHistory = "system:history:write"
	PermReadSystemMetrics  = "system:metrics:read"
	PermReadSystemHeaders  = "system:headers:read"
	PermReadSystemBrowse   = "system:browse:read"
	PermWriteSystemBrowse  = "system:browse:write"
	PermAll                = "*"
	RoleAdmin              = "admin"
	systemPermCount        = 11
)

// ConfigSection is a per-section config API resource name.
type ConfigSection string

const (
	SectionGeneral   ConfigSection = "general"
	SectionWebserver ConfigSection = "webserver"
	SectionSonarr    ConfigSection = "sonarr"
	SectionRadarr    ConfigSection = "radarr"
	SectionLidarr    ConfigSection = "lidarr"
	SectionReadarr   ConfigSection = "readarr"
	SectionWhisparr  ConfigSection = "whisparr"
	SectionFolders   ConfigSection = "folders"
	SectionWebhooks  ConfigSection = "webhooks"
	SectionCmdhooks  ConfigSection = "cmdhooks"
)

// ConfigSections is the GET/PUT /api/config/{section} list.
func ConfigSections() []ConfigSection {
	return []ConfigSection{
		SectionGeneral, SectionWebserver,
		SectionSonarr, SectionRadarr, SectionLidarr, SectionReadarr, SectionWhisparr,
		SectionFolders, SectionWebhooks, SectionCmdhooks,
	}
}

func PermReadConfig(section ConfigSection) string {
	return "config:" + string(section) + ":read"
}

func PermWriteConfig(section ConfigSection) string {
	return "config:" + string(section) + ":write"
}

func KnownSection(name ConfigSection) bool {
	return slices.Contains(ConfigSections(), name)
}

// AllPermissions is every known permission, including per-section config ones.
func AllPermissions() []string {
	sections := ConfigSections()
	perms := make([]string, 0, systemPermCount+len(sections)*2)
	perms = append(perms,
		PermReadSystemStats,
		PermReadSystemInfo,
		PermReadSystemQueue,
		PermWriteSystemQueue,
		PermReadSystemHistory,
		PermWriteSystemHistory,
		PermReadSystemMetrics,
		PermReadSystemHeaders,
		PermReadSystemBrowse,
		PermWriteSystemBrowse,
		PermAll,
	)

	for _, section := range sections {
		perms = append(perms, PermReadConfig(section), PermWriteConfig(section))
	}

	return perms
}

func KnownPermission(name string) bool {
	return slices.Contains(AllPermissions(), name)
}
