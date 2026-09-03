package unpackerr

import "slices"

// Permission names are verb:area:resource. Built-in role admin grants all of them.
const (
	PermReadSystemStats    = "read:system:stats"
	PermReadSystemInfo     = "read:system:info"
	PermReadSystemQueue    = "read:system:queue"
	PermWriteSystemQueue   = "write:system:queue"
	PermReadSystemHistory  = "read:system:history"
	PermWriteSystemHistory = "write:system:history"
	PermReadSystemMetrics  = "read:system:metrics"
	PermAll                = "*"
	RoleAdmin              = "admin"
	systemPermCount        = 8
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
	return "read:config:" + string(section)
}

func PermWriteConfig(section ConfigSection) string {
	return "write:config:" + string(section)
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
